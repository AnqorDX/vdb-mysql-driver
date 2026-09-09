package intercept_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/virtual-db/vdb-mysql-driver/internal/intercept"

	"github.com/AnqorDX/mysql-engine/server"
	vitessmysql "github.com/dolthub/vitess/go/mysql"
	sqltypes "github.com/dolthub/vitess/go/sqltypes"
	querypb "github.com/dolthub/vitess/go/vt/proto/query"
	"github.com/virtual-db/vdb-mysql-driver/internal/bridge"
	"github.com/virtual-db/vdb-core/types"
)

// ---------------------------------------------------------------------------
// stubEventBridge — satisfies bridge.EventBridge for query-interceptor tests.
// Only QueryReceived and QueryCompleted are configurable; all other methods
// are safe no-ops that document which EventBridge methods the Interceptor
// does NOT call.
// ---------------------------------------------------------------------------

type stubEventBridge struct {
	queryReceived  func(connID uint32, query, database string) (string, error)
	queryCompleted func(connID uint32, query string, rowsAffected int64, err error)
}

var _ bridge.EventBridge = (*stubEventBridge)(nil)

func (s *stubEventBridge) ConnectionOpened(_ uint32, _, _ string) error { return nil }
func (s *stubEventBridge) ConnectionClosed(_ uint32, _, _ string)       {}
func (s *stubEventBridge) TransactionBegun(_ uint32, _ bool) error      { return nil }
func (s *stubEventBridge) TransactionCommitted(_ uint32) error          { return nil }
func (s *stubEventBridge) TransactionRolledBack(_ uint32, _ string)     {}

func (s *stubEventBridge) QueryReceived(connID uint32, query, database string) (string, error) {
	if s.queryReceived != nil {
		return s.queryReceived(connID, query, database)
	}
	return "", nil
}

func (s *stubEventBridge) QueryCompleted(connID uint32, query string, rowsAffected int64, err error) {
	if s.queryCompleted != nil {
		s.queryCompleted(connID, query, rowsAffected, err)
	}
}

func (s *stubEventBridge) RowsFetched(_ uint32, _ string, r types.RecordIter) (types.RecordIter, error) {
	return r, nil
}
func (s *stubEventBridge) RowsReady(_ uint32, _ string, r []map[string]any) ([]map[string]any, error) {
	return r, nil
}
func (s *stubEventBridge) RowInserted(_ uint32, _ string, r map[string]any) (map[string]any, error) {
	return r, nil
}
func (s *stubEventBridge) RowUpdated(_ uint32, _ string, _, n map[string]any) (map[string]any, error) {
	return n, nil
}
func (s *stubEventBridge) RowDeleted(_ uint32, _ string, _ map[string]any) error { return nil }
func (s *stubEventBridge) SchemaLoaded(_ string, _ []string, _ string)           {}
func (s *stubEventBridge) SchemaInvalidated(_ string)                            {}
func (s *stubEventBridge) TableTruncated(_ uint32, _ string) error               { return nil }

// ---------------------------------------------------------------------------
// stubChain — satisfies server.Chain for query-interceptor tests.
// Each of the four methods is backed by an optional function field; when nil
// the method is a safe no-op.
// ---------------------------------------------------------------------------

type stubChain struct {
	comQuery       func(ctx context.Context, conn *vitessmysql.Conn, query string, callback vitessmysql.ResultSpoolFn) error
	comMultiQuery  func(ctx context.Context, conn *vitessmysql.Conn, query string, callback vitessmysql.ResultSpoolFn) (string, error)
	comPrepare     func(ctx context.Context, conn *vitessmysql.Conn, query string, prepare *vitessmysql.PrepareData) ([]*querypb.Field, error)
	comStmtExecute func(ctx context.Context, conn *vitessmysql.Conn, prepare *vitessmysql.PrepareData, callback func(*sqltypes.Result) error) error
}

var _ server.Chain = (*stubChain)(nil)

func (s *stubChain) ComQuery(
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

func (s *stubChain) ComMultiQuery(
	ctx context.Context,
	conn *vitessmysql.Conn,
	query string,
	callback vitessmysql.ResultSpoolFn,
) (string, error) {
	if s.comMultiQuery != nil {
		return s.comMultiQuery(ctx, conn, query, callback)
	}
	return "", nil
}

func (s *stubChain) ComPrepare(
	ctx context.Context,
	conn *vitessmysql.Conn,
	query string,
	prepare *vitessmysql.PrepareData,
) ([]*querypb.Field, error) {
	if s.comPrepare != nil {
		return s.comPrepare(ctx, conn, query, prepare)
	}
	return nil, nil
}

func (s *stubChain) ComStmtExecute(
	ctx context.Context,
	conn *vitessmysql.Conn,
	prepare *vitessmysql.PrepareData,
	callback func(*sqltypes.Result) error,
) error {
	if s.comStmtExecute != nil {
		return s.comStmtExecute(ctx, conn, prepare, callback)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// noopQueryCallback is a no-op result callback accepted by Query, ParsedQuery,
// and MultiQuery. Its type matches vitessmysql.ResultSpoolFn.
var noopQueryCallback vitessmysql.ResultSpoolFn = func(*sqltypes.Result, bool) error { return nil }

// ---------------------------------------------------------------------------
// Query
// ---------------------------------------------------------------------------

func TestQuery_CallsQueryReceived(t *testing.T) {
	var receivedConnID uint32
	var receivedQuery string

	b := &stubEventBridge{
		queryReceived: func(connID uint32, query, _ string) (string, error) {
			receivedConnID = connID
			receivedQuery = query
			return "", nil
		},
	}
	qi := NewInterceptor(b)
	conn := &vitessmysql.Conn{}

	if err := qi.Query(context.Background(), &stubChain{}, conn, "SELECT 1", noopQueryCallback); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedConnID != conn.ConnectionID {
		t.Errorf("connID: got %d, want %d", receivedConnID, conn.ConnectionID)
	}
	if receivedQuery != "SELECT 1" {
		t.Errorf("query: got %q, want %q", receivedQuery, "SELECT 1")
	}
}

func TestQuery_CallsQueryCompleted(t *testing.T) {
	var completed bool

	b := &stubEventBridge{
		queryCompleted: func(_ uint32, _ string, _ int64, _ error) {
			completed = true
		},
	}
	qi := NewInterceptor(b)

	if err := qi.Query(context.Background(), &stubChain{}, &vitessmysql.Conn{}, "SELECT 1", noopQueryCallback); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completed {
		t.Fatal("QueryCompleted was not called after chain returned")
	}
}

func TestQuery_QueryReceived_ReturnsRewrite_RewriteUsed(t *testing.T) {
	b := &stubEventBridge{
		queryReceived: func(_ uint32, _, _ string) (string, error) {
			return "SELECT 2", nil
		},
	}
	qi := NewInterceptor(b)

	var chainQuery string
	chain := &stubChain{
		comQuery: func(_ context.Context, _ *vitessmysql.Conn, q string, _ vitessmysql.ResultSpoolFn) error {
			chainQuery = q
			return nil
		},
	}

	if err := qi.Query(context.Background(), chain, &vitessmysql.Conn{}, "SELECT 1", noopQueryCallback); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chainQuery != "SELECT 2" {
		t.Errorf("chain received query: got %q, want %q (rewritten)", chainQuery, "SELECT 2")
	}
}

func TestQuery_QueryReceived_Error_ShortCircuits(t *testing.T) {
	want := errors.New("query rejected")

	b := &stubEventBridge{
		queryReceived: func(_ uint32, _, _ string) (string, error) {
			return "", want
		},
	}
	qi := NewInterceptor(b)

	var chainCalled bool
	chain := &stubChain{
		comQuery: func(_ context.Context, _ *vitessmysql.Conn, _ string, _ vitessmysql.ResultSpoolFn) error {
			chainCalled = true
			return nil
		},
	}

	err := qi.Query(context.Background(), chain, &vitessmysql.Conn{}, "SELECT 1", noopQueryCallback)
	if !errors.Is(err, want) {
		t.Errorf("expected %v, got %v", want, err)
	}
	if chainCalled {
		t.Fatal("chain was called despite QueryReceived returning an error")
	}
}
