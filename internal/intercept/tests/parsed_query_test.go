package intercept_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/virtual-db/vdb-mysql-driver/internal/intercept"

	vitessmysql "github.com/dolthub/vitess/go/mysql"
	sqltypes "github.com/dolthub/vitess/go/sqltypes"
	querypb "github.com/dolthub/vitess/go/vt/proto/query"

	"github.com/virtual-db/vdb-mysql-driver/internal/bridge"
	"github.com/virtual-db/vdb-core/types"

	"github.com/AnqorDX/mysql-engine/server"
)

// ---------------------------------------------------------------------------
// stubEventBridge — satisfies bridge.EventBridge for parsed-query tests.
// ---------------------------------------------------------------------------

type stubEventBridgePQ struct {
	queryReceived  func(connID uint32, query, database string) (string, error)
	queryCompleted func(connID uint32, query string, rowsAffected int64, err error)
}

var _ bridge.EventBridge = (*stubEventBridgePQ)(nil)

func (s *stubEventBridgePQ) ConnectionOpened(_ uint32, _, _ string) error { return nil }
func (s *stubEventBridgePQ) ConnectionClosed(_ uint32, _, _ string)       {}
func (s *stubEventBridgePQ) TransactionBegun(_ uint32, _ bool) error      { return nil }
func (s *stubEventBridgePQ) TransactionCommitted(_ uint32) error          { return nil }
func (s *stubEventBridgePQ) TransactionRolledBack(_ uint32, _ string)     {}

func (s *stubEventBridgePQ) QueryReceived(connID uint32, query, database string) (string, error) {
	if s.queryReceived != nil {
		return s.queryReceived(connID, query, database)
	}
	return "", nil
}

func (s *stubEventBridgePQ) QueryCompleted(connID uint32, query string, rowsAffected int64, err error) {
	if s.queryCompleted != nil {
		s.queryCompleted(connID, query, rowsAffected, err)
	}
}

func (s *stubEventBridgePQ) RowsFetched(_ uint32, _ string, r types.RecordIter) (types.RecordIter, error) {
	return r, nil
}
func (s *stubEventBridgePQ) RowsReady(_ uint32, _ string, r []map[string]any) ([]map[string]any, error) {
	return r, nil
}
func (s *stubEventBridgePQ) RowInserted(_ uint32, _ string, r map[string]any) (map[string]any, error) {
	return r, nil
}
func (s *stubEventBridgePQ) RowUpdated(_ uint32, _ string, _, n map[string]any) (map[string]any, error) {
	return n, nil
}
func (s *stubEventBridgePQ) RowDeleted(_ uint32, _ string, _ map[string]any) error { return nil }
func (s *stubEventBridgePQ) SchemaLoaded(_ string, _ []string, _ string)           {}
func (s *stubEventBridgePQ) SchemaInvalidated(_ string)                            {}
func (s *stubEventBridgePQ) TableTruncated(_ uint32, _ string) error               { return nil }

// ---------------------------------------------------------------------------
// stubChainPQ — satisfies server.Chain for parsed-query tests.
// ---------------------------------------------------------------------------

type stubChainPQ struct {
	comQuery func(ctx context.Context, conn *vitessmysql.Conn, query string, callback vitessmysql.ResultSpoolFn) error
}

var _ server.Chain = (*stubChainPQ)(nil)

func (s *stubChainPQ) ComQuery(
	ctx context.Context,
	conn *vitessmysql.Conn,
	query string,
	callback vitessmysql.ResultSpoolFn,
) error {
	if s.comQuery != nil {
		return s.comQuery(ctx, conn, query, callback)
	}
	return nil
}

func (s *stubChainPQ) ComMultiQuery(
	_ context.Context,
	_ *vitessmysql.Conn,
	_ string,
	_ vitessmysql.ResultSpoolFn,
) (string, error) {
	return "", nil
}

func (s *stubChainPQ) ComPrepare(
	_ context.Context,
	_ *vitessmysql.Conn,
	_ string,
	_ *vitessmysql.PrepareData,
) ([]*querypb.Field, error) {
	return nil, nil
}

func (s *stubChainPQ) ComStmtExecute(
	_ context.Context,
	_ *vitessmysql.Conn,
	_ *vitessmysql.PrepareData,
	_ func(*sqltypes.Result) error,
) error {
	return nil
}

// noopQueryCallbackPQ is a no-op result callback for parsed-query tests.
var noopQueryCallbackPQ vitessmysql.ResultSpoolFn = func(*sqltypes.Result, bool) error { return nil }

// ---------------------------------------------------------------------------
// TestParsedQuery_*
// ---------------------------------------------------------------------------

func TestParsedQuery_CallsQueryReceived(t *testing.T) {
	var called bool

	b := &stubEventBridgePQ{
		queryReceived: func(_ uint32, _ string, _ string) (string, error) {
			called = true
			return "", nil
		},
	}
	qi := NewInterceptor(b)

	// parsed is nil — the Interceptor does not inspect the parsed statement.
	if err := qi.ParsedQuery(&stubChainPQ{}, &vitessmysql.Conn{}, "SELECT 1", nil, noopQueryCallbackPQ); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("QueryReceived was not called from ParsedQuery")
	}
}

func TestParsedQuery_CallsQueryCompleted(t *testing.T) {
	var completed bool

	b := &stubEventBridgePQ{
		queryCompleted: func(_ uint32, _ string, _ int64, _ error) {
			completed = true
		},
	}
	qi := NewInterceptor(b)

	if err := qi.ParsedQuery(&stubChainPQ{}, &vitessmysql.Conn{}, "SELECT 1", nil, noopQueryCallbackPQ); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completed {
		t.Fatal("QueryCompleted was not called from ParsedQuery")
	}
}

func TestParsedQuery_QueryReceived_Error_ShortCircuits(t *testing.T) {
	want := errors.New("rejected")

	b := &stubEventBridgePQ{
		queryReceived: func(_ uint32, _, _ string) (string, error) {
			return "", want
		},
	}
	qi := NewInterceptor(b)

	var chainCalled bool
	chain := &stubChainPQ{
		comQuery: func(_ context.Context, _ *vitessmysql.Conn, _ string, _ vitessmysql.ResultSpoolFn) error {
			chainCalled = true
			return nil
		},
	}

	err := qi.ParsedQuery(chain, &vitessmysql.Conn{}, "SELECT 1", nil, noopQueryCallbackPQ)
	if !errors.Is(err, want) {
		t.Errorf("expected %v, got %v", want, err)
	}
	if chainCalled {
		t.Fatal("chain was called despite QueryReceived returning an error")
	}
}
