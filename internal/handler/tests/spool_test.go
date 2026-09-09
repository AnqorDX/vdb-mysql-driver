package handler_test

import (
	"context"
	"io"
	"testing"

	gmssql "github.com/AnqorDX/mysql-engine/sql"
	gmstypes "github.com/AnqorDX/mysql-engine/types"
	"github.com/dolthub/vitess/go/sqltypes"

	handler "github.com/virtual-db/vdb-mysql-driver/internal/handler"
)

// mockRowIter implements gmssql.RowIter over a static slice of rows.
type mockRowIter struct {
	rows []gmssql.Row
	idx  int
}

func (m *mockRowIter) Next(_ *gmssql.Context) (gmssql.Row, error) {
	if m.idx >= len(m.rows) {
		return nil, io.EOF
	}
	row := m.rows[m.idx]
	m.idx++
	return row, nil
}

func (m *mockRowIter) Close(_ *gmssql.Context) error { return nil }

// newSQLCtx creates a minimal *gmssql.Context sufficient for spool functions.
func newSQLCtx() *gmssql.Context {
	base := gmssql.NewBaseSession()
	return gmssql.NewContext(context.Background(), gmssql.WithSession(base))
}

// collectResults runs the callback-based spool functions and returns every
// *sqltypes.Result delivered to the callback.
func collectResults(cb func(fn func(*sqltypes.Result, bool) error) error) ([]*sqltypes.Result, error) {
	var results []*sqltypes.Result
	err := cb(func(r *sqltypes.Result, _ bool) error {
		results = append(results, r)
		return nil
	})
	return results, err
}

// ---------------------------------------------------------------------------
// SpoolOkResult
// ---------------------------------------------------------------------------

func TestSpoolOkResult_ZeroRowsAffected(t *testing.T) {
	ctx := newSQLCtx()
	okRow := gmstypes.OkResult{RowsAffected: 0, InsertID: 0}
	iter := &mockRowIter{rows: []gmssql.Row{{okRow}}}

	results, err := collectResults(func(fn func(*sqltypes.Result, bool) error) error {
		return handler.SpoolOkResult(ctx, iter, fn)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].RowsAffected != 0 {
		t.Fatalf("expected RowsAffected=0, got %d", results[0].RowsAffected)
	}
	if results[0].InsertID != 0 {
		t.Fatalf("expected InsertID=0, got %d", results[0].InsertID)
	}
}

func TestSpoolOkResult_WithRowsAffected(t *testing.T) {
	ctx := newSQLCtx()
	okRow := gmstypes.OkResult{RowsAffected: 3, InsertID: 0}
	iter := &mockRowIter{rows: []gmssql.Row{{okRow}}}

	results, err := collectResults(func(fn func(*sqltypes.Result, bool) error) error {
		return handler.SpoolOkResult(ctx, iter, fn)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].RowsAffected != 3 {
		t.Fatalf("expected RowsAffected=3, got %d", results[0].RowsAffected)
	}
}

func TestSpoolOkResult_WithInsertID(t *testing.T) {
	ctx := newSQLCtx()
	okRow := gmstypes.OkResult{RowsAffected: 1, InsertID: 42}
	iter := &mockRowIter{rows: []gmssql.Row{{okRow}}}

	results, err := collectResults(func(fn func(*sqltypes.Result, bool) error) error {
		return handler.SpoolOkResult(ctx, iter, fn)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].InsertID != 42 {
		t.Fatalf("expected InsertID=42, got %d", results[0].InsertID)
	}
}

// ---------------------------------------------------------------------------
// SpoolRows
// ---------------------------------------------------------------------------

// simpleSchema builds a one-column VARCHAR schema for row tests.
func simpleSchema(colName string) gmssql.Schema {
	return gmssql.Schema{
		{Name: colName, Type: gmstypes.LongText, Source: "t", Nullable: true},
	}
}

func TestSpoolRows_EmptyResult_CallsCallbackOnce(t *testing.T) {
	ctx := newSQLCtx()
	schema := simpleSchema("col")
	iter := &mockRowIter{}

	callCount := 0
	err := handler.SpoolRows(ctx, schema, iter, func(r *sqltypes.Result, _ bool) error {
		callCount++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected callback called once, got %d", callCount)
	}
}

func TestSpoolRows_EmptyResult_RowsIsEmpty(t *testing.T) {
	ctx := newSQLCtx()
	schema := simpleSchema("col")
	iter := &mockRowIter{}

	results, err := collectResults(func(fn func(*sqltypes.Result, bool) error) error {
		return handler.SpoolRows(ctx, schema, iter, fn)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results[0].Rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(results[0].Rows))
	}
}

func TestSpoolRows_SingleRow_AppearsInResult(t *testing.T) {
	ctx := newSQLCtx()
	schema := simpleSchema("col")
	iter := &mockRowIter{rows: []gmssql.Row{{"hello"}}}

	results, err := collectResults(func(fn func(*sqltypes.Result, bool) error) error {
		return handler.SpoolRows(ctx, schema, iter, fn)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results[0].Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(results[0].Rows))
	}
}

func TestSpoolRows_MultipleRows_AllAppearInResult(t *testing.T) {
	ctx := newSQLCtx()
	schema := simpleSchema("col")
	iter := &mockRowIter{rows: []gmssql.Row{{"a"}, {"b"}, {"c"}}}

	results, err := collectResults(func(fn func(*sqltypes.Result, bool) error) error {
		return handler.SpoolRows(ctx, schema, iter, fn)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results[0].Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(results[0].Rows))
	}
}

// ---------------------------------------------------------------------------
// SpoolResult dispatch
// ---------------------------------------------------------------------------

func TestSpoolResult_OkSchema_DispatchesToSpoolOkResult(t *testing.T) {
	ctx := newSQLCtx()
	okRow := gmstypes.OkResult{RowsAffected: 7, InsertID: 0}
	iter := &mockRowIter{rows: []gmssql.Row{{okRow}}}

	results, err := collectResults(func(fn func(*sqltypes.Result, bool) error) error {
		return handler.SpoolResult(ctx, gmstypes.OkResultSchema, iter, fn)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].RowsAffected != 7 {
		t.Fatalf("expected RowsAffected=7, got %d", results[0].RowsAffected)
	}
}

func TestSpoolResult_RowSchema_DispatchesToSpoolRows(t *testing.T) {
	ctx := newSQLCtx()
	schema := simpleSchema("val")
	iter := &mockRowIter{rows: []gmssql.Row{{"x"}, {"y"}}}

	results, err := collectResults(func(fn func(*sqltypes.Result, bool) error) error {
		return handler.SpoolResult(ctx, schema, iter, fn)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results[0].Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(results[0].Rows))
	}
}
