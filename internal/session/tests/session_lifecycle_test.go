package session_test

import (
	"errors"
	"testing"

	gmssql "github.com/dolthub/go-mysql-server/sql"
	vitessmysql "github.com/dolthub/vitess/go/mysql"
	"github.com/virtual-db/vdb-mysql-driver/internal/bridge"
	"github.com/virtual-db/vdb-core/types"
	. "github.com/virtual-db/vdb-mysql-driver/internal/session"
)

// ---------------------------------------------------------------------------
// stubEventBridge — satisfies bridge.EventBridge for session tests.
//
// Only ConnectionOpened, ConnectionClosed, TransactionBegun,
// TransactionCommitted, and TransactionRolledBack are exercised by Session;
// the remaining nine methods are no-ops that document which methods the
// session does NOT touch.
// ---------------------------------------------------------------------------

type stubEventBridge struct {
	connectionOpened      func(id uint32, user, addr string) error
	connectionClosed      func(id uint32, user, addr string)
	transactionBegun      func(connID uint32, readOnly bool) error
	transactionCommitted  func(connID uint32) error
	transactionRolledBack func(connID uint32, savepoint string)
}

var _ bridge.EventBridge = (*stubEventBridge)(nil)

func (s *stubEventBridge) ConnectionOpened(id uint32, user, addr string) error {
	if s.connectionOpened != nil {
		return s.connectionOpened(id, user, addr)
	}
	return nil
}

func (s *stubEventBridge) ConnectionClosed(id uint32, user, addr string) {
	if s.connectionClosed != nil {
		s.connectionClosed(id, user, addr)
	}
}

func (s *stubEventBridge) TransactionBegun(connID uint32, readOnly bool) error {
	if s.transactionBegun != nil {
		return s.transactionBegun(connID, readOnly)
	}
	return nil
}

func (s *stubEventBridge) TransactionCommitted(connID uint32) error {
	if s.transactionCommitted != nil {
		return s.transactionCommitted(connID)
	}
	return nil
}

func (s *stubEventBridge) TransactionRolledBack(connID uint32, savepoint string) {
	if s.transactionRolledBack != nil {
		s.transactionRolledBack(connID, savepoint)
	}
}

func (s *stubEventBridge) QueryReceived(_ uint32, q, _ string) (string, error) { return q, nil }
func (s *stubEventBridge) QueryCompleted(_ uint32, _ string, _ int64, _ error) {}

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

func (s *stubEventBridge) SchemaLoaded(_ string, _ []string, _ string) {}
func (s *stubEventBridge) SchemaInvalidated(_ string)                  {}
func (s *stubEventBridge) TableTruncated(_ uint32, _ string) error     { return nil }

// ---------------------------------------------------------------------------
// buildSession — helper that constructs a *Session via New().
// ---------------------------------------------------------------------------

func buildSession(t *testing.T, dbName string, events bridge.EventBridge, conn *vitessmysql.Conn, addr string) *Session {
	t.Helper()
	connID := conn.ConnectionID
	user := conn.User
	if err := events.ConnectionOpened(connID, user, addr); err != nil {
		t.Fatalf("ConnectionOpened returned unexpected error: %v", err)
	}
	base := gmssql.NewBaseSessionWithClientServer(
		addr,
		gmssql.Client{User: user, Address: addr},
		connID,
	)
	base.SetCurrentDatabase(dbName)
	return New(base, connID, events, nil)
}

// ---------------------------------------------------------------------------
// Session lifecycle
// ---------------------------------------------------------------------------

func TestSession_StartTransaction_CallsTransactionBegun(t *testing.T) {
	var gotConnID uint32
	var gotReadOnly bool
	stub := &stubEventBridge{
		transactionBegun: func(connID uint32, readOnly bool) error {
			gotConnID = connID
			gotReadOnly = readOnly
			return nil
		},
	}
	conn := &vitessmysql.Conn{ConnectionID: 7, User: "bob"}
	s := buildSession(t, "testdb", stub, conn, "127.0.0.1:5555")

	ctx := gmssql.NewEmptyContext()
	tx, err := s.StartTransaction(ctx, gmssql.ReadOnly)
	if err != nil {
		t.Fatalf("StartTransaction returned unexpected error: %v", err)
	}
	if tx == nil {
		t.Fatal("StartTransaction returned nil transaction")
	}
	if gotConnID != 7 {
		t.Errorf("TransactionBegun connID: got %d, want 7", gotConnID)
	}
	if !gotReadOnly {
		t.Error("TransactionBegun readOnly: got false, want true")
	}
}

func TestSession_StartTransaction_Error_IsReturned(t *testing.T) {
	wantErr := errors.New("transaction refused by policy")
	stub := &stubEventBridge{
		transactionBegun: func(_ uint32, _ bool) error { return wantErr },
	}
	conn := &vitessmysql.Conn{ConnectionID: 1, User: "alice"}
	s := buildSession(t, "testdb", stub, conn, "127.0.0.1:1234")

	ctx := gmssql.NewEmptyContext()
	_, err := s.StartTransaction(ctx, gmssql.ReadWrite)
	if !errors.Is(err, wantErr) {
		t.Errorf("StartTransaction error: got %v, want %v", err, wantErr)
	}
}

func TestSession_CommitTransaction_CallsTransactionCommitted(t *testing.T) {
	var called bool
	stub := &stubEventBridge{
		transactionCommitted: func(_ uint32) error {
			called = true
			return nil
		},
	}
	conn := &vitessmysql.Conn{ConnectionID: 3, User: "carol"}
	s := buildSession(t, "testdb", stub, conn, "127.0.0.1:2222")

	ctx := gmssql.NewEmptyContext()
	tx := &Transaction{ReadOnly: false}
	if err := s.CommitTransaction(ctx, tx); err != nil {
		t.Fatalf("CommitTransaction returned unexpected error: %v", err)
	}
	if !called {
		t.Fatal("TransactionCommitted was not called")
	}
}

func TestSession_Rollback_CallsTransactionRolledBack(t *testing.T) {
	var gotSavepoint = "SENTINEL"
	stub := &stubEventBridge{
		transactionRolledBack: func(_ uint32, sp string) {
			gotSavepoint = sp
		},
	}
	conn := &vitessmysql.Conn{ConnectionID: 5, User: "dave"}
	s := buildSession(t, "testdb", stub, conn, "127.0.0.1:3333")

	ctx := gmssql.NewEmptyContext()
	tx := &Transaction{ReadOnly: false}
	if err := s.Rollback(ctx, tx); err != nil {
		t.Fatalf("Rollback returned unexpected error: %v", err)
	}
	if gotSavepoint != "" {
		t.Errorf("TransactionRolledBack savepoint: got %q, want empty string (full rollback)", gotSavepoint)
	}
}

func TestSession_CreateSavepoint_DoesNotError(t *testing.T) {
	conn := &vitessmysql.Conn{ConnectionID: 1, User: "alice"}
	s := buildSession(t, "testdb", &stubEventBridge{}, conn, "127.0.0.1:1234")

	ctx := gmssql.NewEmptyContext()
	tx := &Transaction{ReadOnly: false}
	if err := s.CreateSavepoint(ctx, tx, "sp1"); err != nil {
		t.Fatalf("CreateSavepoint returned unexpected error: %v", err)
	}
}

func TestSession_SessionEnd_CallsConnectionClosed(t *testing.T) {
	var (
		gotID   uint32
		gotUser string
		gotAddr string
	)
	stub := &stubEventBridge{
		connectionClosed: func(id uint32, user, addr string) {
			gotID = id
			gotUser = user
			gotAddr = addr
		},
	}
	conn := &vitessmysql.Conn{ConnectionID: 99, User: "charlie"}
	s := buildSession(t, "testdb", stub, conn, "10.0.0.1:9999")

	s.SessionEnd()

	if gotID != 99 {
		t.Errorf("ConnectionClosed connID: got %d, want 99", gotID)
	}
	if gotUser != "charlie" {
		t.Errorf("ConnectionClosed user: got %q, want %q", gotUser, "charlie")
	}
	if gotAddr == "" {
		t.Error("ConnectionClosed addr: got empty string, want non-empty")
	}
}
