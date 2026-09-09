package session

import (
	gmssql "github.com/AnqorDX/mysql-engine/sql"

	"github.com/virtual-db/vdb-mysql-driver/internal/auth"
	"github.com/virtual-db/vdb-mysql-driver/internal/bridge"
)

// Session embeds *gmssql.BaseSession and implements:
//   - sql.Session                (via embedding)
//   - sql.LifecycleAwareSession  (CommandBegin, CommandEnd, SessionEnd)
//   - sql.TransactionSession     (StartTransaction, CommitTransaction, Rollback,
//     CreateSavepoint, RollbackToSavepoint, ReleaseSavepoint)
//
// It also carries the authorization grants derived from the probe connection
// at NewConnection time. The Delta reads these via GetGrants() on every storage
// backend call to enforce authorization scope. Grants are NOT used to acquire
// a source DB connection — the read-only service-account pool is a
// construction-time field on the Delta, completely independent of this session.
type Session struct {
	*gmssql.BaseSession
	connID uint32
	events bridge.EventBridge
	grants *auth.Grants
}

// New constructs a Session. Called by internal/handler during NewConnection.
func New(base *gmssql.BaseSession, connID uint32, events bridge.EventBridge, grants *auth.Grants) *Session {
	return &Session{
		BaseSession: base,
		connID:      connID,
		events:      events,
		grants:      grants,
	}
}

// GetGrants returns the authorization grants derived from the probe connection
// at connection time. Returns nil if the session was constructed without grants
// (e.g. in tests that bypass auth).
func (s *Session) GetGrants() *auth.Grants {
	return s.grants
}

// CommandBegin satisfies sql.LifecycleAwareSession. Called before every SQL command.
func (s *Session) CommandBegin() error { return nil }

// CommandEnd satisfies sql.LifecycleAwareSession. Called after every SQL command.
func (s *Session) CommandEnd() {}

// SessionEnd satisfies sql.LifecycleAwareSession. Called when the connection
// tears down. Invokes events.ConnectionClosed.
func (s *Session) SessionEnd() {
	client := s.Client()
	s.events.ConnectionClosed(s.connID, client.User, client.Address)
}

// StartTransaction satisfies sql.TransactionSession. Invokes
// events.TransactionBegun. A non-nil error refuses the transaction.
func (s *Session) StartTransaction(
	ctx *gmssql.Context,
	tChar gmssql.TransactionCharacteristic,
) (gmssql.Transaction, error) {
	readOnly := tChar == gmssql.ReadOnly
	if err := s.events.TransactionBegun(s.connID, readOnly); err != nil {
		return nil, err
	}
	return &Transaction{ReadOnly: readOnly}, nil
}

// CommitTransaction satisfies sql.TransactionSession. Invokes
// events.TransactionCommitted.
func (s *Session) CommitTransaction(
	ctx *gmssql.Context,
	tx gmssql.Transaction,
) error {
	return s.events.TransactionCommitted(s.connID)
}

// Rollback satisfies sql.TransactionSession. Invokes
// events.TransactionRolledBack with an empty savepoint (full rollback).
func (s *Session) Rollback(ctx *gmssql.Context, tx gmssql.Transaction) error {
	s.events.TransactionRolledBack(s.connID, "")
	return nil
}

// CreateSavepoint satisfies sql.TransactionSession. No-op at the engine level.
func (s *Session) CreateSavepoint(
	ctx *gmssql.Context,
	tx gmssql.Transaction,
	name string,
) error {
	return nil
}

// RollbackToSavepoint satisfies sql.TransactionSession. Invokes
// events.TransactionRolledBack with the savepoint name.
func (s *Session) RollbackToSavepoint(
	ctx *gmssql.Context,
	tx gmssql.Transaction,
	name string,
) error {
	s.events.TransactionRolledBack(s.connID, name)
	return nil
}

// ReleaseSavepoint satisfies sql.TransactionSession. No-op at the engine level.
func (s *Session) ReleaseSavepoint(
	ctx *gmssql.Context,
	tx gmssql.Transaction,
	name string,
) error {
	return nil
}
