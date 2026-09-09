package rows_test

import (
	"testing"

	gmssql "github.com/AnqorDX/mysql-engine/sql"
	. "github.com/virtual-db/vdb-mysql-driver/internal/rows"
)

// ---------------------------------------------------------------------------
// RowToMap
// ---------------------------------------------------------------------------

func TestRowToMap_RoundTrip(t *testing.T) {
	cols := []string{"id", "name", "balance"}
	row := gmssql.Row{42, "alice", 99.5}
	got := RowToMap(row, cols)

	if got["id"] != 42 {
		t.Errorf("id: got %v, want 42", got["id"])
	}
	if got["name"] != "alice" {
		t.Errorf("name: got %v, want alice", got["name"])
	}
	if got["balance"] != 99.5 {
		t.Errorf("balance: got %v, want 99.5", got["balance"])
	}
}

func TestRowToMap_PanicsOnMismatch(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("RowToMap did not panic on column count mismatch")
		}
	}()
	RowToMap(gmssql.Row{1, 2}, []string{"only_one_col"})
}

// ---------------------------------------------------------------------------
// MapToRow
// ---------------------------------------------------------------------------

func TestMapToRow_IsInverseOfRowToMap(t *testing.T) {
	record := map[string]any{"id": 1, "name": "bob"}
	cols := []string{"id", "name"}
	row := MapToRow(record, cols)

	if row[0] != 1 {
		t.Errorf("row[0]: got %v, want 1", row[0])
	}
	if row[1] != "bob" {
		t.Errorf("row[1]: got %v, want bob", row[1])
	}
}

func TestMapToRow_PreservesColumnOrdering(t *testing.T) {
	cols := []string{"c", "b", "a"}
	record := map[string]any{"a": 1, "b": 2, "c": 3}
	row := MapToRow(record, cols)

	if row[0] != 3 { // c
		t.Errorf("row[0] (c): got %v, want 3", row[0])
	}
	if row[1] != 2 { // b
		t.Errorf("row[1] (b): got %v, want 2", row[1])
	}
	if row[2] != 1 { // a
		t.Errorf("row[2] (a): got %v, want 1", row[2])
	}
}

// ---------------------------------------------------------------------------
// SchemaColumns
// ---------------------------------------------------------------------------

func TestSchemaColumns_ExtractsNames(t *testing.T) {
	schema := gmssql.Schema{
		{Name: "id"},
		{Name: "title"},
		{Name: "created_at"},
	}
	cols := SchemaColumns(schema)
	want := []string{"id", "title", "created_at"}
	if len(cols) != len(want) {
		t.Fatalf("length: got %d, want %d", len(cols), len(want))
	}
	for i, c := range cols {
		if c != want[i] {
			t.Errorf("cols[%d]: got %q, want %q", i, c, want[i])
		}
	}
}
