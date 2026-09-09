package rows_test

import (
	"testing"

	gmssql "github.com/AnqorDX/mysql-engine/sql"
	. "github.com/virtual-db/vdb-mysql-driver/internal/rows"
)

func TestInsertRow_InvokesCallback(t *testing.T) {
	var called bool
	b := &stubEventBridge{
		rowInserted: func(_ uint32, table string, r map[string]any) (map[string]any, error) {
			called = true
			if table != "items" {
				t.Errorf("table: got %q, want items", table)
			}
			if r["id"] != 99 {
				t.Errorf("id: got %v, want 99", r["id"])
			}
			return r, nil
		},
	}
	p := NewGMSProvider(b)
	_, err := p.InsertRow(makeCtx(), "items", gmssql.Row{99, "thing"}, twoColSchema())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("RowInserted callback was not called")
	}
}

func TestInsertRow_NilCallback_ReturnsNilNoError(t *testing.T) {
	p := NewGMSProvider(&stubEventBridge{})
	rec, err := p.InsertRow(makeCtx(), "t", gmssql.Row{1, "x"}, twoColSchema())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec != nil {
		t.Errorf("expected nil record, got %v", rec)
	}
}

func TestUpdateRow_NilCallback_ReturnsNilNoError(t *testing.T) {
	p := NewGMSProvider(&stubEventBridge{})
	rec, err := p.UpdateRow(makeCtx(), "t", gmssql.Row{1, "old"}, gmssql.Row{1, "new"}, twoColSchema())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec != nil {
		t.Errorf("expected nil record, got %v", rec)
	}
}

func TestDeleteRow_NilCallback_ReturnsNil(t *testing.T) {
	p := NewGMSProvider(&stubEventBridge{})
	if err := p.DeleteRow(makeCtx(), "t", gmssql.Row{1, "x"}, twoColSchema()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
