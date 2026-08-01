package rows

import (
	"io"

	gmssql "github.com/dolthub/go-mysql-server/sql"
)

// Iter wraps a []gmssql.Row slice to implement gmssql.RowIter.
type Iter struct {
	rows []gmssql.Row
	pos  int
}

// NewIter creates a new Iter from a slice of rows.
func NewIter(rows []gmssql.Row) *Iter {
	return &Iter{rows: rows}
}

func (i *Iter) Next(_ *gmssql.Context) (gmssql.Row, error) {
	if i.pos >= len(i.rows) {
		return nil, io.EOF
	}
	row := i.rows[i.pos]
	i.pos++
	return row, nil
}

func (i *Iter) Close(_ *gmssql.Context) error { return nil }

// MapIter lazily converts []map[string]any to gmssql.Row on demand.
// Unlike NewIter, it never materialises a full []gmssql.Row slice — each
// row is converted when Next is called, and the previous row becomes
// eligible for GC. This eliminates the second full-size copy that
// PartitionRows used to build.
type MapIter struct {
	records []map[string]any
	cols    []string
	pos     int
}

// NewMapIter creates a MapIter that lazily converts records to gmssql.Row.
func NewMapIter(records []map[string]any, cols []string) *MapIter {
	return &MapIter{records: records, cols: cols}
}

func (it *MapIter) Next(_ *gmssql.Context) (gmssql.Row, error) {
	if it.pos >= len(it.records) {
		return nil, io.EOF
	}
	rec := it.records[it.pos]
	it.pos++
	return MapToRow(rec, it.cols), nil
}

func (it *MapIter) Close(_ *gmssql.Context) error {
	it.records = nil
	return nil
}
