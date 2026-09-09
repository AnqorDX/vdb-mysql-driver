package rows

import (
	"fmt"

	gmssql "github.com/AnqorDX/mysql-engine/sql"
)

// RowToMap converts a GMS sql.Row ([]interface{}) into a map[string]any using
// the ordered column name slice.
//
// Precondition: len(cols) == len(row). A mismatch is a programming error in
// schema resolution and panics rather than silently producing corrupt data.
func RowToMap(row gmssql.Row, cols []string) map[string]any {
	if len(row) != len(cols) {
		panic(fmt.Sprintf(
			"rows: RowToMap: column count mismatch: got %d columns, want %d",
			len(row), len(cols),
		))
	}
	m := make(map[string]any, len(cols))
	for i, col := range cols {
		m[col] = row[i]
	}
	return m
}

// SchemaColumns extracts the ordered column name slice from a GMS sql.Schema.
func SchemaColumns(schema gmssql.Schema) []string {
	cols := make([]string, len(schema))
	for i, col := range schema {
		cols[i] = col.Name
	}
	return cols
}

// MapToRow converts a map[string]any back to a GMS sql.Row using the ordered
// column name slice. It is the exact inverse of RowToMap.
//
// Precondition: cols ordering must match the table schema's column ordering.
// Any transposition produces silently corrupt data.
func MapToRow(record map[string]any, cols []string) gmssql.Row {
	row := make(gmssql.Row, len(cols))
	for i, col := range cols {
		row[i] = record[col]
	}
	return row
}
