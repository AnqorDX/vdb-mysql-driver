package catalog

import (
	gmssql "github.com/AnqorDX/mysql-engine/sql"
)

// SchemaDelta holds all structural mutations applied to one table within a
// virtual session. Zero value is NOT valid — use newSchemaDelta().
type SchemaDelta struct {
	// AddedCols are columns appended via ADD COLUMN, in the order they were added.
	// Source queries must not include these; their values are synthesised on read.
	AddedCols []gmssql.Column

	// DroppedCols is the set of column names removed via DROP COLUMN.
	// These must be excluded from source SELECT lists and stripped from results.
	DroppedCols map[string]struct{}

	// ModifiedCols maps virtual column name → updated GMS Column descriptor.
	ModifiedCols map[string]gmssql.Column

	// RenamedCols maps virtual name → original source name.
	// Source queries use the source name; results are aliased to the virtual name.
	RenamedCols map[string]string

	// AddedIndexes is the set of index names added via ADD INDEX / CREATE INDEX.
	AddedIndexes map[string]gmssql.IndexDef

	// DroppedIndexes is the set of index names removed via DROP INDEX.
	DroppedIndexes map[string]struct{}

	// SourceName is set when the table was renamed. Source queries use this name.
	// Empty means no rename — use the Table's current virtual name.
	SourceName string

	// Created is true when this table was created entirely through VDB (CREATE TABLE).
	// PartitionRows must return rows from the row delta only; no source query is made.
	Created bool

	// Dropped is true when the table was dropped (DROP TABLE).
	// Any access through GetTableInsensitive must return (nil, false, nil).
	Dropped bool
}

func newSchemaDelta() *SchemaDelta {
	return &SchemaDelta{
		DroppedCols:    make(map[string]struct{}),
		ModifiedCols:   make(map[string]gmssql.Column),
		RenamedCols:    make(map[string]string),
		AddedIndexes:   make(map[string]gmssql.IndexDef),
		DroppedIndexes: make(map[string]struct{}),
	}
}

// applyDeltaToSchema applies all schema mutations in d to base, returning the
// virtual schema that GMS should see. base is never mutated.
func applyDeltaToSchema(base gmssql.Schema, d *SchemaDelta) gmssql.Schema {
	if d == nil {
		return base
	}
	result := make(gmssql.Schema, 0, len(base)+len(d.AddedCols))
	for _, col := range base {
		if _, dropped := d.DroppedCols[col.Name]; dropped {
			continue
		}
		c := *col // copy to avoid mutating the base
		if modified, ok := d.ModifiedCols[col.Name]; ok {
			c = modified
		}
		// Apply rename: find if any renamed entry maps to this source column.
		for vn, sn := range d.RenamedCols {
			if sn == col.Name {
				c.Name = vn
				break
			}
		}
		result = append(result, &c)
	}
	// AddedCols are appended after source columns.
	for i := range d.AddedCols {
		c := d.AddedCols[i]
		result = append(result, &c)
	}
	return result
}

// synthesiseAddedCols expands each source row to the virtual schema length,
// filling slots for added columns with nil or their declared default value.
// Returns the original slice if there are no added columns.
func synthesiseAddedCols(rows []gmssql.Row, d *SchemaDelta, virtualSchema gmssql.Schema) []gmssql.Row {
	if d == nil || len(d.AddedCols) == 0 {
		return rows
	}
	for i, row := range rows {
		if len(row) >= len(virtualSchema) {
			continue
		}
		expanded := make(gmssql.Row, len(virtualSchema))
		copy(expanded, row)
		for j := len(row); j < len(virtualSchema); j++ {
			col := virtualSchema[j]
			if col.Default != nil {
				v, _ := col.Default.Eval(nil, nil)
				expanded[j] = v
			} else {
				expanded[j] = nil
			}
		}
		rows[i] = expanded
	}
	return rows
}
