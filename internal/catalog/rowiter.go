package catalog

import (
	"database/sql"
	"fmt"
	"io"

	gmssql "github.com/dolthub/go-mysql-server/sql"
)

// sourceRowIter wraps a database/sql Rows result set and streams rows
// one at a time, avoiding materialisation of the full result into a slice.
type sourceRowIter struct {
	rows   *sql.Rows
	closed bool
}

// Next returns the next row from the source cursor. Returns io.EOF when
// the result set is exhausted.
func (it *sourceRowIter) Next(ctx *gmssql.Context) (gmssql.Row, error) {
	if it.closed {
		return nil, io.EOF
	}
	if !it.rows.Next() {
		if err := it.rows.Err(); err != nil {
			return nil, fmt.Errorf("catalog: source cursor: %w", err)
		}
		return nil, io.EOF
	}

	// Determine column count from the result set metadata.
	cols, err := it.rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("catalog: source cursor columns: %w", err)
	}
	n := len(cols)

	vals := make([]any, n)
	ptrs := make([]any, n)
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := it.rows.Scan(ptrs...); err != nil {
		return nil, fmt.Errorf("catalog: scan source row: %w", err)
	}
	for i, v := range vals {
		if b, ok := v.([]byte); ok {
			vals[i] = string(b)
		}
	}

	row := make(gmssql.Row, n)
	copy(row, vals)
	return row, nil
}

// Close releases the underlying database/sql Rows cursor.
func (it *sourceRowIter) Close(_ *gmssql.Context) error {
	if it.closed {
		return nil
	}
	it.closed = true
	return it.rows.Close()
}

// synthesiseIter wraps a RowIter and expands each row to the virtual schema
// length, filling slots for added columns with nil or their declared default.
type synthesiseIter struct {
	inner         gmssql.RowIter
	delta         *SchemaDelta
	virtualSchema gmssql.Schema
	closed        bool
}

// Next returns the next row, expanding it to the virtual schema length if needed.
func (it *synthesiseIter) Next(ctx *gmssql.Context) (gmssql.Row, error) {
	row, err := it.inner.Next(ctx)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	if len(row) >= len(it.virtualSchema) {
		return row, nil
	}

	expanded := make(gmssql.Row, len(it.virtualSchema))
	copy(expanded, row)
	for j := len(row); j < len(it.virtualSchema); j++ {
		col := it.virtualSchema[j]
		if col.Default != nil {
			v, _ := col.Default.Eval(nil, nil)
			expanded[j] = v
		} else {
			expanded[j] = nil
		}
	}
	return expanded, nil
}

// Close closes the inner iterator.
func (it *synthesiseIter) Close(ctx *gmssql.Context) error {
	if it.closed {
		return nil
	}
	it.closed = true
	return it.inner.Close(ctx)
}
