package rows_test

import (
	"errors"
	"testing"

	gmssql "github.com/AnqorDX/mysql-engine/sql"
	"github.com/virtual-db/vdb-mysql-driver/internal/bridge"
	. "github.com/virtual-db/vdb-mysql-driver/internal/rows"
	"github.com/virtual-db/vdb-core/types"
)

// sliceIter wraps a []gmssql.Row as a gmssql.RowIter for test convenience.
type sliceIter struct {
	rows  []gmssql.Row
	index int
}

func (s *sliceIter) Next(_ *gmssql.Context) (gmssql.Row, error) {
	if s.index >= len(s.rows) {
		return nil, nil
	}
	row := s.rows[s.index]
	s.index++
	return row, nil
}

func (s *sliceIter) Close(_ *gmssql.Context) error { return nil }

func newSliceIter(rows []gmssql.Row) gmssql.RowIter {
	return &sliceIter{rows: rows}
}

// stubEventBridge implements bridge.EventBridge for GMSProvider tests.
// Only the five methods GMSProvider actually calls have active function fields.
// The remaining nine methods are no-op stubs that document the unused surface.
type stubEventBridge struct {
	rowsFetched func(connID uint32, table string, records types.RecordIter) (types.RecordIter, error)
	rowsReady   func(connID uint32, table string, records []map[string]any) ([]map[string]any, error)
	rowInserted func(connID uint32, table string, record map[string]any) (map[string]any, error)
	rowUpdated  func(connID uint32, table string, old, new map[string]any) (map[string]any, error)
	rowDeleted  func(connID uint32, table string, record map[string]any) error
}

var _ bridge.EventBridge = (*stubEventBridge)(nil)

func (s *stubEventBridge) RowsFetched(connID uint32, table string, records types.RecordIter) (types.RecordIter, error) {
	if s.rowsFetched != nil {
		return s.rowsFetched(connID, table, records)
	}
	return records, nil
}

func (s *stubEventBridge) RowsReady(connID uint32, table string, records []map[string]any) ([]map[string]any, error) {
	if s.rowsReady != nil {
		return s.rowsReady(connID, table, records)
	}
	return records, nil
}

func (s *stubEventBridge) RowInserted(connID uint32, table string, record map[string]any) (map[string]any, error) {
	if s.rowInserted != nil {
		return s.rowInserted(connID, table, record)
	}
	return nil, nil
}

func (s *stubEventBridge) RowUpdated(connID uint32, table string, old, new map[string]any) (map[string]any, error) {
	if s.rowUpdated != nil {
		return s.rowUpdated(connID, table, old, new)
	}
	return nil, nil
}

func (s *stubEventBridge) RowDeleted(connID uint32, table string, record map[string]any) error {
	if s.rowDeleted != nil {
		return s.rowDeleted(connID, table, record)
	}
	return nil
}

// Unused EventBridge methods — no-op stubs.
func (s *stubEventBridge) ConnectionOpened(_ uint32, _, _ string) error { return nil }
func (s *stubEventBridge) ConnectionClosed(_ uint32, _, _ string)       {}

func (s *stubEventBridge) TransactionBegun(_ uint32, _ bool) error  { return nil }
func (s *stubEventBridge) TransactionCommitted(_ uint32) error      { return nil }
func (s *stubEventBridge) TransactionRolledBack(_ uint32, _ string) {}

func (s *stubEventBridge) QueryReceived(_ uint32, q, _ string) (string, error) { return q, nil }
func (s *stubEventBridge) QueryCompleted(_ uint32, _ string, _ int64, _ error) {}

func (s *stubEventBridge) SchemaLoaded(_ string, _ []string, _ string) {}
func (s *stubEventBridge) SchemaInvalidated(_ string)                  {}
func (s *stubEventBridge) TableTruncated(_ uint32, _ string) error     { return nil }

// Helpers
// ---------------------------------------------------------------------------

func makeCtx() *gmssql.Context {
	return gmssql.NewEmptyContext()
}

func twoColSchema() gmssql.Schema {
	return gmssql.Schema{{Name: "id"}, {Name: "val"}}
}

// recordSliceIter wraps a []map[string]any as a types.RecordIter for tests.
type recordSliceIter struct {
	records []map[string]any
	index   int
}

func (s *recordSliceIter) Next() (map[string]any, error) {
	if s.index >= len(s.records) {
		return nil, nil
	}
	r := s.records[s.index]
	s.index++
	return r, nil
}

func (s *recordSliceIter) Close() error { return nil }

// ---------------------------------------------------------------------------
// FetchRows
// ---------------------------------------------------------------------------

func TestFetchRows_InvokesRowsFetchedCallback(t *testing.T) {
	var called bool
	b := &stubEventBridge{
		rowsFetched: func(_ uint32, table string, recs types.RecordIter) (types.RecordIter, error) {
			called = true
			if table != "orders" {
				t.Errorf("table: got %q, want %q", table, "orders")
			}
			var collected []map[string]any
			for {
				r, err := recs.Next()
				if err != nil || r == nil {
					break
				}
				collected = append(collected, r)
			}
			if len(collected) != 2 {
				t.Errorf("len(collected): got %d, want 2", len(collected))
			}
			return &recordSliceIter{records: collected}, nil
		},
	}

	p := NewGMSProvider(b)
	rawRows := []gmssql.Row{{1, "x"}, {2, "y"}}
	schema := gmssql.Schema{{Name: "id"}, {Name: "val"}}
	_, err := p.FetchRows(makeCtx(), "orders", newSliceIter(rawRows), schema)
	if err != nil {
		t.Fatalf("FetchRows error: %v", err)
	}
	if !called {
		t.Fatal("RowsFetched callback was not called")
	}
}

func TestFetchRows_NilCallback_ReturnsUnmodified(t *testing.T) {
	// rowsFetched field is nil — stub returns records unchanged.
	p := NewGMSProvider(&stubEventBridge{})
	rawRows := []gmssql.Row{{1, "a"}}
	schema := gmssql.Schema{{Name: "id"}, {Name: "v"}}
	got, err := p.FetchRows(makeCtx(), "t", newSliceIter(rawRows), schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len(got): got %d, want 1", len(got))
	}
}

func TestFetchRows_CallbackError_IsReturned(t *testing.T) {
	want := errors.New("fetch failed")
	b := &stubEventBridge{
		rowsFetched: func(_ uint32, _ string, _ types.RecordIter) (types.RecordIter, error) {
			return nil, want
		},
	}
	p := NewGMSProvider(b)
	_, err := p.FetchRows(makeCtx(), "t", newSliceIter([]gmssql.Row{{1}}), gmssql.Schema{{Name: "id"}})
	if !errors.Is(err, want) {
		t.Errorf("expected %v, got %v", want, err)
	}
}
