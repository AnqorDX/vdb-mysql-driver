package catalog

import (
	"fmt"
	"io"

	gmssql "github.com/AnqorDX/mysql-engine/sql"
)

// RowWriter dispatches INSERT, UPDATE, and DELETE operations to the row
// Provider held by the owning Table. It satisfies the three GMS write
// interfaces so that GMS can use a single concrete type for all DML.
type RowWriter struct {
	table *Table
}

var _ gmssql.RowInserter = (*RowWriter)(nil)
var _ gmssql.RowUpdater = (*RowWriter)(nil)
var _ gmssql.RowDeleter = (*RowWriter)(nil)

// StatementBegin is called at the start of each DML statement. No-op here;
// the event bridge handles transaction-level signalling.
func (w *RowWriter) StatementBegin(_ *gmssql.Context) {}

// DiscardChanges is called when a statement is rolled back mid-execution.
// No-op: we have no in-memory buffer to discard.
func (w *RowWriter) DiscardChanges(_ *gmssql.Context, _ error) error { return nil }

// StatementComplete is called when a DML statement finishes successfully.
// No-op here.
func (w *RowWriter) StatementComplete(_ *gmssql.Context) error { return nil }

// Close is called when GMS is done with this writer instance. No resources
// are owned directly; the underlying DB connection belongs to the Table.
func (w *RowWriter) Close(_ *gmssql.Context) error { return nil }

// Insert satisfies gmssql.RowInserter. Before delegating to the row Provider
// it checks whether a row with the same primary key already exists in the
// merged view (delta + source). If one does, it returns a UniqueKeyError so
// that GMS can:
//   - dispatch to updater.Update for INSERT … ON DUPLICATE KEY UPDATE, or
//   - propagate ERROR 1062 to the client for a plain duplicate INSERT.
func (w *RowWriter) Insert(ctx *gmssql.Context, row gmssql.Row) error {
	if w.table.rows == nil {
		return nil
	}

	// Locate the primary-key column. If the schema has no PK we skip the
	// collision check and fall through to a plain insert (no 1062 semantics).
	pkIdx := pkColIndex(w.table.Schema())
	if pkIdx >= 0 && pkIdx < len(row) && row[pkIdx] != nil {
		existing, err := w.table.findExistingByPK(ctx, pkIdx, row[pkIdx])
		if err != nil {
			return err
		}
		if existing != nil {
			// Return UniqueKeyError so GMS handles ODKU or surfaces 1062.
			return gmssql.NewUniqueKeyErr(
				fmt.Sprintf("%v", row[pkIdx]),
				true,
				existing,
			)
		}
	}

	_, err := w.table.rows.InsertRow(ctx, w.table.name, row, w.table.Schema())
	return err
}

// Update satisfies gmssql.RowUpdater. It delegates to the row Provider,
// which converts old and new rows to maps and fires the RowUpdated event.
func (w *RowWriter) Update(ctx *gmssql.Context, old, new gmssql.Row) error {
	if w.table.rows == nil {
		return nil
	}
	_, err := w.table.rows.UpdateRow(ctx, w.table.name, old, new, w.table.Schema())
	return err
}

// Delete satisfies gmssql.RowDeleter. It delegates to the row Provider,
// which converts the row to a map and fires the RowDeleted event.
func (w *RowWriter) Delete(ctx *gmssql.Context, row gmssql.Row) error {
	if w.table.rows == nil {
		return nil
	}
	return w.table.rows.DeleteRow(ctx, w.table.name, row, w.table.Schema())
}

// ---------------------------------------------------------------------------
// PK collision helpers
// ---------------------------------------------------------------------------

// pkColIndex returns the index of the first column marked PrimaryKey in
// schema, or -1 if no such column exists.
func pkColIndex(schema gmssql.Schema) int {
	for i, col := range schema {
		if col.PrimaryKey {
			return i
		}
	}
	return -1
}

// findExistingByPK scans the fully merged table view (live delta + tx delta +
// source) for a row whose value at pkIdx equals pkVal. Returns the matching
// GMS row, or nil when no existing row is found.
//
// Using PartitionRows here is intentional: it produces exactly the same merged
// view that a SELECT would return, including uncommitted writes from the
// current transaction. This means ODKU against a row that was inserted earlier
// in the same transaction will be detected and handled correctly.
func (t *Table) findExistingByPK(ctx *gmssql.Context, pkIdx int, pkVal any) (gmssql.Row, error) {
	partIter, err := t.Partitions(ctx)
	if err != nil {
		return nil, fmt.Errorf("findExistingByPK: Partitions: %w", err)
	}
	defer partIter.Close(ctx)

	part, err := partIter.Next(ctx)
	if err != nil {
		return nil, fmt.Errorf("findExistingByPK: partition Next: %w", err)
	}

	rowIter, err := t.PartitionRows(ctx, part)
	if err != nil {
		return nil, fmt.Errorf("findExistingByPK: PartitionRows: %w", err)
	}
	defer rowIter.Close(ctx)

	pkStr := fmt.Sprintf("%v", pkVal)
	for {
		r, err := rowIter.Next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("findExistingByPK: row iteration: %w", err)
		}
		if pkIdx < len(r) && fmt.Sprintf("%v", r[pkIdx]) == pkStr {
			return r, nil
		}
	}
	return nil, nil
}
