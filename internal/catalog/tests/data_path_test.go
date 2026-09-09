package catalog_test

// data_path_test.go verifies that every method in Table and RowWriter that
// calls into the rows.Provider passes the *virtual* schema (t.Schema()) rather
// than the raw *source* schema (t.schema).
//
// The distinction matters whenever the delta modifies the schema:
//
//   ADD COLUMN  — virtual schema is wider than the source schema; passing the
//                 source schema to RowToMap causes a length-mismatch panic that
//                 kills the server connection.
//
//   RENAME via  — virtual schema has the new column name; passing the source
//   ModifyColumn  schema keys records by the old name so MapToRow can never
//                 find the value and the cell returns nil/empty.
//
//   CREATE TABLE — source schema is empty ({}); passing it to InsertRow calls
//                  RowToMap([row...], []) which panics, and pkColIndex returns
//                  -1 so duplicate-key detection is silently skipped.
//
// Each test owns a captureRowsProvider that records what schema argument each
// rows.Provider method received.  The tests assert on those captured values
// and on the observable output of PartitionRows / Insert so that both the
// "correct schema is forwarded" contract AND the "correct data is returned"
// contract are verified.

import (
	"io"
	"testing"

	gmssql "github.com/AnqorDX/mysql-engine/sql"
	gmstypes "github.com/AnqorDX/mysql-engine/types"

	. "github.com/virtual-db/vdb-mysql-driver/internal/catalog"
)

// ---------------------------------------------------------------------------
// captureRowsProvider — records every schema argument; optionally seeds data
// ---------------------------------------------------------------------------

// captureRowsProvider satisfies rows.Provider.  It records the gmssql.Schema
// argument passed to each method so tests can assert the virtual schema was
// forwarded correctly.
//
// If seedValues is non-nil, FetchRows synthesises one record keyed by the
// column names it receives.  This lets tests verify that data flows through
// the full PartitionRows pipeline under the correct names.
type captureRowsProvider struct {
	fetchSchema  gmssql.Schema
	insertSchema gmssql.Schema
	updateSchema gmssql.Schema
	deleteSchema gmssql.Schema

	// seedValues, if non-nil, produces one record from FetchRows.
	// The i-th value is assigned to schema.col[i].Name.
	seedValues []any
}

func (c *captureRowsProvider) FetchRows(_ *gmssql.Context, _ string, _ gmssql.RowIter, schema gmssql.Schema) ([]map[string]any, error) {
	c.fetchSchema = schema
	if c.seedValues == nil || len(schema) == 0 {
		return nil, nil
	}
	rec := make(map[string]any, len(schema))
	for i, col := range schema {
		if i < len(c.seedValues) {
			rec[col.Name] = c.seedValues[i]
		}
	}
	return []map[string]any{rec}, nil
}

func (c *captureRowsProvider) CommitRows(_ *gmssql.Context, _ string, records []map[string]any) ([]map[string]any, error) {
	return records, nil // pass through so PartitionRows can surface the data
}

func (c *captureRowsProvider) InsertRow(_ *gmssql.Context, _ string, _ gmssql.Row, schema gmssql.Schema) (map[string]any, error) {
	c.insertSchema = schema
	return nil, nil
}

func (c *captureRowsProvider) UpdateRow(_ *gmssql.Context, _ string, _, _ gmssql.Row, schema gmssql.Schema) (map[string]any, error) {
	c.updateSchema = schema
	return nil, nil
}

func (c *captureRowsProvider) DeleteRow(_ *gmssql.Context, _ string, _ gmssql.Row, schema gmssql.Schema) error {
	c.deleteSchema = schema
	return nil
}

func (c *captureRowsProvider) TruncateRows(_ *gmssql.Context, _ string) (int, error) {
	return 0, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// buildTableCapture builds a Table backed by cap so tests can inspect which
// schema each rows.Provider method received.
func buildTableCapture(t *testing.T, cap *captureRowsProvider, cols ...string) (*DatabaseProvider, gmssql.Table) {
	t.Helper()
	schemaProv := &stubSchemaProvider{cols: makeDescriptors(cols...), pkCol: cols[0]}
	p := NewDatabaseProvider("shop", cap, schemaProv, nil)
	ctx := gmssql.NewEmptyContext()
	db, err := p.Database(ctx, "shop")
	if err != nil {
		t.Fatalf("Database: %v", err)
	}
	tbl, ok, err := db.GetTableInsensitive(ctx, "products")
	if err != nil || !ok {
		t.Fatalf("GetTableInsensitive: ok=%v err=%v", ok, err)
	}
	return p, tbl
}

// partitionRows is a convenience wrapper that calls Partitions + PartitionRows
// and returns the RowIter (caller must Close it).
func partitionRows(t *testing.T, tbl gmssql.Table) gmssql.RowIter {
	t.Helper()
	ctx := gmssql.NewEmptyContext()
	partIter, err := tbl.Partitions(ctx)
	if err != nil {
		t.Fatalf("Partitions: %v", err)
	}
	defer partIter.Close(ctx)
	part, err := partIter.Next(ctx)
	if err != nil {
		t.Fatalf("partition Next: %v", err)
	}
	iter, err := tbl.PartitionRows(ctx, part)
	if err != nil {
		t.Fatalf("PartitionRows: %v", err)
	}
	return iter
}

// drainRows reads all rows from iter, closes it, and returns them.
func drainRows(t *testing.T, iter gmssql.RowIter) []gmssql.Row {
	t.Helper()
	ctx := gmssql.NewEmptyContext()
	defer iter.Close(ctx)
	var out []gmssql.Row
	for {
		r, err := iter.Next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("row iter Next: %v", err)
		}
		out = append(out, r)
	}
	return out
}

// ---------------------------------------------------------------------------
// ADD COLUMN: FetchRows must receive the virtual (wider) schema
// ---------------------------------------------------------------------------

// TestPartitionRows_AfterAddColumn_FetchReceivesVirtualSchema asserts that
// after ADD COLUMN the schema forwarded to FetchRows contains the new column.
// Before the fix, FetchRows received the narrow source schema (2 cols); after
// the fix it receives the virtual schema (3 cols).
func TestPartitionRows_AfterAddColumn_FetchReceivesVirtualSchema(t *testing.T) {
	cap := &captureRowsProvider{}
	_, tbl := buildTableCapture(t, cap, "id", "int", "name", "varchar")
	ctx := gmssql.NewEmptyContext()

	at := tbl.(gmssql.AlterableTable)
	if err := at.AddColumn(ctx, col("score", gmstypes.Int32), nil); err != nil {
		t.Fatalf("AddColumn: %v", err)
	}

	iter := partitionRows(t, tbl)
	iter.Close(ctx)

	if len(cap.fetchSchema) != 3 {
		t.Errorf("FetchRows received schema with %d columns, want 3 (source schema was forwarded instead of virtual)",
			len(cap.fetchSchema))
	}
	if len(cap.fetchSchema) == 3 && cap.fetchSchema[2].Name != "score" {
		t.Errorf("FetchRows schema[2].Name = %q, want \"score\"", cap.fetchSchema[2].Name)
	}
}

// ---------------------------------------------------------------------------
// RENAME COLUMN (via ModifyColumn): FetchRows must use the new column name
// ---------------------------------------------------------------------------

// TestPartitionRows_AfterModifyColumn_FetchReceivesNewName asserts that after
// a RENAME COLUMN (dispatched by GMS as ModifyColumn) the schema forwarded to
// FetchRows carries the new name, not the old source name.
//
// Before the fix, FetchRows received the source schema with "user_name";
// after the fix it receives the virtual schema with "username".
func TestPartitionRows_AfterModifyColumn_FetchReceivesNewName(t *testing.T) {
	cap := &captureRowsProvider{}
	_, tbl := buildTableCapture(t, cap, "id", "int", "user_name", "varchar")
	ctx := gmssql.NewEmptyContext()

	// GMS dispatches RENAME COLUMN as ModifyColumn(oldName, newCol, order).
	at := tbl.(gmssql.AlterableTable)
	if err := at.ModifyColumn(ctx, "user_name", col("username", gmstypes.LongText), nil); err != nil {
		t.Fatalf("ModifyColumn: %v", err)
	}

	iter := partitionRows(t, tbl)
	iter.Close(ctx)

	for _, c := range cap.fetchSchema {
		if c.Name == "user_name" {
			t.Error("FetchRows received old name \"user_name\" — virtual schema must be forwarded")
		}
	}
	found := false
	for _, c := range cap.fetchSchema {
		if c.Name == "username" {
			found = true
		}
	}
	if !found {
		t.Errorf("FetchRows schema does not contain \"username\"; got: %v", schemaNames(cap.fetchSchema))
	}
}

// TestPartitionRows_AfterModifyColumn_DataReturnedUnderNewName asserts that
// data seeded by FetchRows under the virtual schema name reaches the caller
// intact.  This is the observable symptom of the bug: before the fix the cell
// value is nil/empty; after the fix it is "alice".
func TestPartitionRows_AfterModifyColumn_DataReturnedUnderNewName(t *testing.T) {
	// seedValues are keyed by whatever column names FetchRows receives.
	// If FetchRows gets the virtual schema ["id","username"], the record will be
	// {"id": 1, "username": "alice"} and MapToRow will produce the correct row.
	// If FetchRows gets the source schema ["id","user_name"], the record will be
	// {"id": 1, "user_name": "alice"} and MapToRow("username") returns nil.
	cap := &captureRowsProvider{seedValues: []any{1, "alice"}}
	_, tbl := buildTableCapture(t, cap, "id", "int", "user_name", "varchar")
	ctx := gmssql.NewEmptyContext()

	at := tbl.(gmssql.AlterableTable)
	if err := at.ModifyColumn(ctx, "user_name", col("username", gmstypes.LongText), nil); err != nil {
		t.Fatalf("ModifyColumn: %v", err)
	}

	rows := drainRows(t, partitionRows(t, tbl))
	if len(rows) != 1 {
		t.Fatalf("expected 1 row from PartitionRows, got %d", len(rows))
	}
	// Virtual schema: [id(0), username(1)] — row[1] must be "alice".
	if rows[0][1] != "alice" {
		t.Errorf("row[1] (username) = %v, want \"alice\"; value was lost due to schema name mismatch", rows[0][1])
	}
}

// ---------------------------------------------------------------------------
// CREATE TABLE: InsertRow must receive the virtual schema, not empty {}
// ---------------------------------------------------------------------------

// TestInsert_AfterCreateTable_UsesVirtualSchema asserts that after a virtual
// CREATE TABLE the schema forwarded to InsertRow is the declared table schema,
// not the empty source schema.
//
// Before the fix, InsertRow received gmssql.Schema{} (empty); in the real
// GMSProvider this causes RowToMap to panic on the column-count mismatch,
// killing the connection.  After the fix InsertRow receives [id, label].
func TestInsert_AfterCreateTable_UsesVirtualSchema(t *testing.T) {
	cap := &captureRowsProvider{}
	p := NewDatabaseProvider("shop", cap, &stubSchemaProvider{}, nil)
	ctx := gmssql.NewEmptyContext()
	db, _ := p.Database(ctx, "shop")

	declared := gmssql.PrimaryKeySchema{
		Schema: gmssql.Schema{
			{Name: "id", Type: gmstypes.Int32, PrimaryKey: true, Source: "events"},
			{Name: "label", Type: gmstypes.LongText, Source: "events"},
		},
	}
	if err := db.(gmssql.TableCreator).CreateTable(ctx, "events", declared, gmssql.Collation_Default, ""); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	tbl, ok, err := db.GetTableInsensitive(ctx, "events")
	if err != nil || !ok {
		t.Fatalf("GetTableInsensitive after CreateTable: ok=%v err=%v", ok, err)
	}

	row := gmssql.Row{int32(1), "signup"}
	if err := tbl.(gmssql.InsertableTable).Inserter(ctx).Insert(ctx, row); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if len(cap.insertSchema) == 0 {
		t.Fatal("InsertRow received an empty schema — virtual schema from CREATE TABLE must be forwarded")
	}
	names := schemaNames(cap.insertSchema)
	if len(names) != 2 || names[0] != "id" || names[1] != "label" {
		t.Errorf("InsertRow schema column names = %v, want [id label]", names)
	}
}

// TestInsert_AfterCreateTable_PKDetectionUsesVirtualSchema asserts that
// duplicate-key detection is active for virtually-created tables.  Before the
// fix pkColIndex received the empty source schema and returned -1, silently
// skipping PK collision checks so a second INSERT with the same key would
// succeed and create a phantom duplicate row.
func TestInsert_AfterCreateTable_PKDetectionUsesVirtualSchema(t *testing.T) {
	// Use a rows provider that tracks inserts so we can count them.
	tracker := &insertCountProvider{}
	p := NewDatabaseProvider("shop", tracker, &stubSchemaProvider{}, nil)
	ctx := gmssql.NewEmptyContext()
	db, _ := p.Database(ctx, "shop")

	declared := gmssql.PrimaryKeySchema{
		Schema: gmssql.Schema{
			{Name: "id", Type: gmstypes.Int32, PrimaryKey: true, Source: "items"},
			{Name: "name", Type: gmstypes.LongText, Source: "items"},
		},
	}
	if err := db.(gmssql.TableCreator).CreateTable(ctx, "items", declared, gmssql.Collation_Default, ""); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	tbl, _, _ := db.GetTableInsensitive(ctx, "items")
	ins := tbl.(gmssql.InsertableTable).Inserter(ctx)

	// First insert — must succeed.
	if err := ins.Insert(ctx, gmssql.Row{int32(1), "bolt"}); err != nil {
		t.Fatalf("first Insert: %v", err)
	}

	// Second insert with same PK — must return a UniqueKeyError, not succeed.
	err := ins.Insert(ctx, gmssql.Row{int32(1), "duplicate"})
	if err == nil {
		t.Error("second Insert with duplicate PK returned nil — PK collision was not detected (pkColIndex used empty schema)")
	}
}

// insertCountProvider tracks inserts and makes them visible via PartitionRows.
type insertCountProvider struct {
	rows []map[string]any
}

func (p *insertCountProvider) FetchRows(_ *gmssql.Context, _ string, _ gmssql.RowIter, _ gmssql.Schema) ([]map[string]any, error) {
	return p.rows, nil
}
func (p *insertCountProvider) CommitRows(_ *gmssql.Context, _ string, records []map[string]any) ([]map[string]any, error) {
	return records, nil
}
func (p *insertCountProvider) InsertRow(_ *gmssql.Context, _ string, _ gmssql.Row, _ gmssql.Schema) (map[string]any, error) {
	// The real overlay would store the row; we do a minimal version here.
	rec := map[string]any{"id": int32(1), "name": "bolt"}
	p.rows = append(p.rows, rec)
	return rec, nil
}
func (p *insertCountProvider) UpdateRow(_ *gmssql.Context, _ string, _, _ gmssql.Row, _ gmssql.Schema) (map[string]any, error) {
	return nil, nil
}
func (p *insertCountProvider) DeleteRow(_ *gmssql.Context, _ string, _ gmssql.Row, _ gmssql.Schema) error {
	return nil
}
func (p *insertCountProvider) TruncateRows(_ *gmssql.Context, _ string) (int, error) {
	return 0, nil
}

// ---------------------------------------------------------------------------
// UPDATE and DELETE: must also receive the virtual schema
// ---------------------------------------------------------------------------

// TestUpdate_UsesVirtualSchema asserts that after ADD COLUMN the schema
// forwarded to UpdateRow is the virtual (wider) schema.
func TestUpdate_UsesVirtualSchema(t *testing.T) {
	cap := &captureRowsProvider{}
	_, tbl := buildTableCapture(t, cap, "id", "int", "name", "varchar")
	ctx := gmssql.NewEmptyContext()

	at := tbl.(gmssql.AlterableTable)
	if err := at.AddColumn(ctx, col("score", gmstypes.Int32), nil); err != nil {
		t.Fatalf("AddColumn: %v", err)
	}

	oldRow := gmssql.Row{int32(1), "alice", nil}
	newRow := gmssql.Row{int32(1), "alice", int32(99)}
	if err := tbl.(gmssql.UpdatableTable).Updater(ctx).Update(ctx, oldRow, newRow); err != nil {
		t.Fatalf("Update: %v", err)
	}

	if len(cap.updateSchema) != 3 {
		t.Errorf("UpdateRow received schema with %d columns, want 3 (virtual)", len(cap.updateSchema))
	}
}

// TestDelete_UsesVirtualSchema asserts that after ADD COLUMN the schema
// forwarded to DeleteRow is the virtual (wider) schema.
func TestDelete_UsesVirtualSchema(t *testing.T) {
	cap := &captureRowsProvider{}
	_, tbl := buildTableCapture(t, cap, "id", "int", "name", "varchar")
	ctx := gmssql.NewEmptyContext()

	at := tbl.(gmssql.AlterableTable)
	if err := at.AddColumn(ctx, col("score", gmstypes.Int32), nil); err != nil {
		t.Fatalf("AddColumn: %v", err)
	}

	row := gmssql.Row{int32(1), "alice", nil}
	if err := tbl.(gmssql.DeletableTable).Deleter(ctx).Delete(ctx, row); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if len(cap.deleteSchema) != 3 {
		t.Errorf("DeleteRow received schema with %d columns, want 3 (virtual)", len(cap.deleteSchema))
	}
}
