// Package catalog provides the GMS DatabaseProvider, Database, Table, and
// RowWriter implementations that wire the VirtualDB row and schema providers
// into the go-mysql-server execution layer.
package catalog

import (
	"database/sql"
	"fmt"
	"io"
	"strings"
	"sync"

	gmssql "github.com/dolthub/go-mysql-server/sql"
	"github.com/virtual-db/vdb-mysql-driver/internal/rows"
)

// Table implements gmssql.Table and the write interfaces InsertableTable,
// UpdatableTable, and DeletableTable. It delegates row reads to fetchFromSource
// (against the source MySQL database) followed by the rows.Provider lifecycle
// hooks, and delegates writes directly to the rows.Provider.
type Table struct {
	name   string
	dbName string // needed for the fetchFromSource SQL query
	schema gmssql.Schema
	rows   rows.Provider
	db     *sql.DB // source database connection; may be nil in tests

	// ai is the shared auto-increment state for this table. It is owned by the
	// DatabaseProvider registry so that the counter survives across Table
	// instances (which are created fresh on every GetTableInsensitive call).
	// nil only in unit tests that construct Table directly.
	ai *autoIncrState

	delta    *SchemaDelta      // nil when no DDL has been issued for this table
	provider *DatabaseProvider // nil only in unit tests
}

// autoIncrState holds the per-table auto-increment counter. It is allocated
// once per table name in DatabaseProvider and shared across all Table
// instances for that table.
type autoIncrState struct {
	mu     sync.Mutex
	next   uint64
	loaded bool
}

var _ gmssql.Table = (*Table)(nil)
var _ gmssql.InsertableTable = (*Table)(nil)
var _ gmssql.UpdatableTable = (*Table)(nil)
var _ gmssql.DeletableTable = (*Table)(nil)
var _ gmssql.AutoIncrementTable = (*Table)(nil)
var _ gmssql.AlterableTable = (*Table)(nil)
var _ gmssql.IndexAlterableTable = (*Table)(nil)
var _ gmssql.TruncateableTable = (*Table)(nil)

// aiState returns the table's autoIncrState, creating a throwaway one if ai
// is nil (unit-test path where no DatabaseProvider is wired up).
func (t *Table) aiState() *autoIncrState {
	if t.ai != nil {
		return t.ai
	}
	// Allocate a fresh state so tests that don't wire a provider still work.
	t.ai = &autoIncrState{}
	return t.ai
}

func (t *Table) Name() string   { return t.name }
func (t *Table) String() string { return t.name }
func (t *Table) Schema() gmssql.Schema {
	if t.delta == nil {
		return t.schema
	}
	return applyDeltaToSchema(t.schema, t.delta)
}
func (t *Table) Collation() gmssql.CollationID { return gmssql.Collation_Default }

// Partitions satisfies gmssql.Table. This implementation always has exactly one
// logical partition.
func (t *Table) Partitions(_ *gmssql.Context) (gmssql.PartitionIter, error) {
	return &singlePartitionIter{}, nil
}

// PartitionRows streams rows from the source database through the delta
// overlay, and returns a RowIter over the final record set. Source rows are
// consumed one at a time — the full result set is never materialised in
// memory. The materialisation into []map[string]any happens inside FetchRows
// (required by the DriverAPI contract) but the source []gmssql.Row slice is
// eliminated.
func (t *Table) PartitionRows(ctx *gmssql.Context, _ gmssql.Partition) (gmssql.RowIter, error) {
	rawIter, err := t.fetchFromSource(ctx)
	if err != nil {
		return nil, err
	}

	// Wrap with synthesiseIter to expand added columns lazily.
	if t.delta != nil {
		rawIter = &synthesiseIter{
			inner:         rawIter,
			delta:         t.delta,
			virtualSchema: t.Schema(),
		}
	}

	if t.rows == nil {
		// No rows provider (unit tests) — drain the iterator into a slice.
		if rawIter != nil {
			defer rawIter.Close(ctx)
		}
		var all []gmssql.Row
		if rawIter != nil {
			for {
				row, err := rawIter.Next(ctx)
				if err != nil {
					return nil, err
				}
				if row == nil {
					break
				}
				all = append(all, row)
			}
		}
		return rows.NewIter(all), nil
	}

	merged, err := t.rows.FetchRows(ctx, t.name, rawIter, t.Schema())
	if err != nil {
		return nil, err
	}

	final, err := t.rows.CommitRows(ctx, t.name, merged)
	if err != nil {
		return nil, err
	}

	// Stream the final records as gmssql.Row without materialising a full
	// []gmssql.Row slice. MapIter converts lazily on each Next() call.
	cols := rows.SchemaColumns(t.Schema())
	return rows.NewMapIter(final, cols), nil
}

// fetchFromSource opens a streaming cursor over the source MySQL database.
// Returns a RowIter that yields rows one at a time, avoiding materialisation
// of the full result set. The caller must close the returned iterator.
// If db is nil (e.g. in unit tests), it returns nil without error.
func (t *Table) fetchFromSource(ctx *gmssql.Context) (gmssql.RowIter, error) {
	if t.delta != nil && t.delta.Created {
		return nil, nil // no source backing; all rows come from row delta
	}

	// Resolve the name the source DB knows.
	sourceName := t.name
	if t.delta != nil && t.delta.SourceName != "" {
		sourceName = t.delta.SourceName
	}

	if t.db == nil {
		return nil, nil
	}

	cols := t.sourceColumns()
	if len(cols) == 0 {
		return nil, nil
	}

	query := "SELECT " + strings.Join(cols, ", ") +
		" FROM `" + t.dbName + "`.`" + sourceName + "`"

	dbRows, err := t.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("catalog: fetch from %q.%q: %w", t.dbName, sourceName, err)
	}

	return &sourceRowIter{rows: dbRows}, nil
}

// sourceColumns returns the SELECT column fragments for fetchFromSource.
// Added columns are excluded. Renamed columns use `src AS virtual` syntax.
// Dropped columns are excluded.
func (t *Table) sourceColumns() []string {
	var frags []string
	for _, col := range t.schema {
		name := col.Name
		if t.delta == nil {
			frags = append(frags, "`"+name+"`")
			continue
		}
		if _, dropped := t.delta.DroppedCols[name]; dropped {
			continue
		}
		// Find if this source column has been renamed to a virtual name.
		virtualName := name
		for vn, sn := range t.delta.RenamedCols {
			if sn == name {
				virtualName = vn
				break
			}
		}
		if virtualName != name {
			frags = append(frags, "`"+name+"` AS `"+virtualName+"`")
		} else {
			frags = append(frags, "`"+name+"`")
		}
	}
	return frags
}

// Inserter satisfies gmssql.InsertableTable.
func (t *Table) Inserter(_ *gmssql.Context) gmssql.RowInserter { return &RowWriter{table: t} }

// Updater satisfies gmssql.UpdatableTable.
func (t *Table) Updater(_ *gmssql.Context) gmssql.RowUpdater { return &RowWriter{table: t} }

// Deleter satisfies gmssql.DeletableTable.
func (t *Table) Deleter(_ *gmssql.Context) gmssql.RowDeleter { return &RowWriter{table: t} }

// ---------------------------------------------------------------------------
// AutoIncrementTable — gmssql.AutoIncrementTable implementation
// ---------------------------------------------------------------------------

// loadAutoIncrCounter seeds ai.next from information_schema.TABLES the first
// time it is needed. Must be called with ai.mu held.
func (t *Table) loadAutoIncrCounter(ai *autoIncrState) {
	if ai.loaded {
		return
	}
	ai.loaded = true
	ai.next = 1 // safe fallback when db is nil or query fails

	if t.db == nil {
		return
	}

	var next uint64
	err := t.db.QueryRow(
		"SELECT COALESCE(AUTO_INCREMENT, 1) FROM information_schema.TABLES"+
			" WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?",
		t.dbName, t.name,
	).Scan(&next)
	if err == nil && next > 0 {
		ai.next = next
	}
}

// PeekNextAutoIncrementValue satisfies gmssql.AutoIncrementGetter.
// It returns the next value that would be assigned without advancing the
// counter.
func (t *Table) PeekNextAutoIncrementValue(_ *gmssql.Context) (uint64, error) {
	ai := t.aiState()
	ai.mu.Lock()
	defer ai.mu.Unlock()
	t.loadAutoIncrCounter(ai)
	return ai.next, nil
}

// GetNextAutoIncrementValue satisfies gmssql.AutoIncrementTable. GMS calls
// this once per inserted row after evaluating the AUTO_INCREMENT expression.
//
//   - insertVal == nil means the user omitted the column (or wrote DEFAULT /
//     NULL / 0): we consume and return the next counter value, then advance.
//   - insertVal != nil means the user supplied an explicit id: we advance the
//     counter past it (if necessary) so subsequent unspecified inserts don't
//     collide, then return the explicit value so GMS uses it unchanged.
func (t *Table) GetNextAutoIncrementValue(_ *gmssql.Context, insertVal interface{}) (uint64, error) {
	ai := t.aiState()
	ai.mu.Lock()
	defer ai.mu.Unlock()
	t.loadAutoIncrCounter(ai)

	if insertVal == nil {
		seq := ai.next
		ai.next++
		return seq, nil
	}

	// Convert the caller-supplied value to uint64. GMS may deliver it as any
	// of the integer types depending on the column's declared type.
	var given uint64
	switch v := insertVal.(type) {
	case int64:
		if v > 0 {
			given = uint64(v)
		}
	case uint64:
		given = v
	case int32:
		if v > 0 {
			given = uint64(v)
		}
	case uint32:
		given = uint64(v)
	case int:
		if v > 0 {
			given = uint64(v)
		}
	}

	// Keep the counter ahead of any explicitly inserted id so the next
	// auto-assigned id does not collide with a row the caller already wrote.
	if given >= ai.next {
		ai.next = given + 1
	}
	return given, nil
}

// AutoIncrementSetter satisfies gmssql.AutoIncrementTable. The returned setter
// allows GMS (and ALTER TABLE … AUTO_INCREMENT = n) to update the in-memory
// counter.
func (t *Table) AutoIncrementSetter(_ *gmssql.Context) gmssql.AutoIncrementSetter {
	return &autoIncrSetter{ai: t.aiState()}
}

// ---------------------------------------------------------------------------
// singlePartition — trivial single-partition implementation
// ---------------------------------------------------------------------------

type singlePartition struct{}

func (singlePartition) Key() []byte { return []byte("single") }

type singlePartitionIter struct {
	done bool
}

func (i *singlePartitionIter) Next(_ *gmssql.Context) (gmssql.Partition, error) {
	if i.done {
		return nil, io.EOF
	}
	i.done = true
	return singlePartition{}, nil
}

func (i *singlePartitionIter) Close(_ *gmssql.Context) error { return nil }

// ---------------------------------------------------------------------------
// autoIncrSetter — gmssql.AutoIncrementSetter implementation
// ---------------------------------------------------------------------------

// autoIncrSetter implements gmssql.AutoIncrementSetter for Table.
type autoIncrSetter struct {
	ai *autoIncrState
}

// SetAutoIncrementValue updates the shared auto-increment counter.
// Called by GMS for ALTER TABLE … AUTO_INCREMENT = n statements.
func (s *autoIncrSetter) SetAutoIncrementValue(_ *gmssql.Context, val uint64) error {
	s.ai.mu.Lock()
	defer s.ai.mu.Unlock()
	s.ai.next = val
	s.ai.loaded = true // suppress the lazy load; val is authoritative
	return nil
}

// AcquireAutoIncrementLock is a no-op for the proxy. The proxy is
// single-writer per connection so no cross-connection locking is required.
func (s *autoIncrSetter) AcquireAutoIncrementLock(_ *gmssql.Context) (func(), error) {
	return func() {}, nil
}

// Close satisfies gmssql.Closer (embedded in gmssql.AutoIncrementSetter).
// No persistent state to flush.
func (s *autoIncrSetter) Close(_ *gmssql.Context) error { return nil }

// ---------------------------------------------------------------------------
// DDL interfaces — gmssql.AlterableTable
// ---------------------------------------------------------------------------

func (t *Table) AddColumn(_ *gmssql.Context, col *gmssql.Column, order *gmssql.ColumnOrder) error {
	if t.delta == nil {
		t.delta = t.provider.deltaFor(t.name)
	}
	col.Source = t.name
	t.delta.AddedCols = append(t.delta.AddedCols, *col)
	return nil
}

func (t *Table) DropColumn(_ *gmssql.Context, colName string) error {
	if t.delta == nil {
		t.delta = t.provider.deltaFor(t.name)
	}
	// If the column was added virtually (not backed by the source DB), simply
	// remove it from AddedCols — there is nothing to record in DroppedCols.
	for i, c := range t.delta.AddedCols {
		if c.Name == colName {
			t.delta.AddedCols = append(t.delta.AddedCols[:i], t.delta.AddedCols[i+1:]...)
			return nil
		}
	}
	t.delta.DroppedCols[colName] = struct{}{}
	return nil
}

func (t *Table) ModifyColumn(_ *gmssql.Context, colName string, col *gmssql.Column, order *gmssql.ColumnOrder) error {
	if t.delta == nil {
		t.delta = t.provider.deltaFor(t.name)
	}
	col.Source = t.name
	t.delta.ModifiedCols[colName] = *col
	return nil
}

// RenameColumn is a helper that tracks a column rename in the delta.
// GMS routes RENAME COLUMN through ModifyColumn (via AlterableTable), but
// this method is available for direct callers.
func (t *Table) RenameColumn(_ *gmssql.Context, oldName, newName string) error {
	if t.delta == nil {
		t.delta = t.provider.deltaFor(t.name)
	}
	// If this column was added virtually (not in source), rename it in AddedCols.
	for i, c := range t.delta.AddedCols {
		if c.Name == oldName {
			t.delta.AddedCols[i].Name = newName
			return nil
		}
	}
	// It is a source column. Follow the rename chain: if oldName was already
	// renamed, the stored source name is the canonical original.
	srcName := oldName
	if existing, ok := t.delta.RenamedCols[oldName]; ok {
		srcName = existing
		delete(t.delta.RenamedCols, oldName)
	}
	t.delta.RenamedCols[newName] = srcName
	return nil
}

// ---------------------------------------------------------------------------
// DDL interfaces — gmssql.IndexAlterableTable
// ---------------------------------------------------------------------------

func (t *Table) CreateIndex(_ *gmssql.Context, idx gmssql.IndexDef) error {
	if t.delta == nil {
		t.delta = t.provider.deltaFor(t.name)
	}
	delete(t.delta.DroppedIndexes, idx.Name)
	t.delta.AddedIndexes[idx.Name] = idx
	return nil
}

func (t *Table) DropIndex(_ *gmssql.Context, idxName string) error {
	if t.delta == nil {
		t.delta = t.provider.deltaFor(t.name)
	}
	delete(t.delta.AddedIndexes, idxName)
	t.delta.DroppedIndexes[idxName] = struct{}{}
	return nil
}

func (t *Table) RenameIndex(_ *gmssql.Context, fromIndexName, toIndexName string) error {
	if t.delta == nil {
		t.delta = t.provider.deltaFor(t.name)
	}
	if idx, ok := t.delta.AddedIndexes[fromIndexName]; ok {
		delete(t.delta.AddedIndexes, fromIndexName)
		t.delta.AddedIndexes[toIndexName] = idx
	}
	if _, ok := t.delta.DroppedIndexes[fromIndexName]; ok {
		delete(t.delta.DroppedIndexes, fromIndexName)
		t.delta.DroppedIndexes[toIndexName] = struct{}{}
	}
	return nil
}

func (t *Table) GetIndexes(_ *gmssql.Context) ([]gmssql.Index, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// DDL interfaces — gmssql.TruncateableTable
// ---------------------------------------------------------------------------

func (t *Table) Truncate(ctx *gmssql.Context) (int, error) {
	if t.rows == nil {
		return 0, nil
	}
	return t.rows.TruncateRows(ctx, t.name)
}
