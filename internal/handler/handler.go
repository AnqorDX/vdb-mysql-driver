package handler

import (
	"fmt"

	vitessmysql "github.com/dolthub/vitess/go/mysql"
	"github.com/dolthub/vitess/go/vt/sqlparser"

	sqle "github.com/AnqorDX/mysql-engine"
	gmssql "github.com/AnqorDX/mysql-engine/sql"

	"github.com/virtual-db/vdb-mysql-driver/internal/auth"
	"github.com/virtual-db/vdb-mysql-driver/internal/bridge"
	"github.com/virtual-db/vdb-mysql-driver/internal/session"
)

// Handler implements vitessmysql.Handler. It is the boundary between the
// Vitess MySQL wire protocol layer (Layer 1) and the GMS sqle.Engine (Layer 2).
//
// NewConnection creates a VDBSession from the auth grants populated by
// VDBAuthServer during the handshake and stores it in conn.ClientData.
//
// ComQuery builds a *sql.Context from the session, calls engine.Query directly,
// converts the resulting rows to Vitess sqltypes.Result values, and spools them
// to the callback. No server.Server is involved at any point.
type Handler struct {
	engine *sqle.Engine
	dbName string
	events bridge.EventBridge
}

// New constructs a Handler.
func New(engine *sqle.Engine, dbName string, events bridge.EventBridge) *Handler {
	return &Handler{engine: engine, dbName: dbName, events: events}
}

// NewConnection satisfies vitessmysql.Handler. Called after the Vitess
// handshake and auth cycle complete. Extracts the grants populated by
// VDBAuthServer from conn.UserData, fires ConnectionOpened, and stores
// the resulting VDBSession in conn.ClientData for use by ComQuery.
func (h *Handler) NewConnection(c *vitessmysql.Conn) {
	grants := auth.GetGrantsFromConn(c)
	connID := c.ConnectionID
	user := c.User
	addr := c.RemoteAddr().String()

	if err := h.events.ConnectionOpened(connID, user, addr); err != nil {
		// NewConnection has no error return. Flag the connection as refused
		// by leaving ClientData nil; ComQuery will detect this and return an
		// error for every subsequent query.
		return
	}

	base := gmssql.NewBaseSessionWithClientServer(
		addr,
		gmssql.Client{User: user, Address: addr},
		connID,
	)
	base.SetCurrentDatabase(h.dbName)

	c.ClientData = session.New(base, connID, h.events, grants)
}

// ConnectionClosed satisfies vitessmysql.Handler. Fires SessionEnd on the
// stored VDBSession, which in turn fires the ConnectionClosed pipeline event.
func (h *Handler) ConnectionClosed(c *vitessmysql.Conn) {
	sess, ok := c.ClientData.(*session.Session)
	if !ok || sess == nil {
		return
	}
	sess.SessionEnd()
}

// ConnectionAborted satisfies vitessmysql.Handler. Called when a new
// connection cannot be fully established (e.g. auth failure). No session was
// created, so there is nothing to clean up.
func (h *Handler) ConnectionAborted(_ *vitessmysql.Conn, _ string) error {
	return nil
}

// ComInitDB satisfies vitessmysql.Handler. Updates the current database on the
// session stored in conn.ClientData.
func (h *Handler) ComInitDB(c *vitessmysql.Conn, schemaName string) error {
	sess, ok := c.ClientData.(*session.Session)
	if !ok || sess == nil {
		return fmt.Errorf("no session for connection %d", c.ConnectionID)
	}
	sess.SetCurrentDatabase(schemaName)
	return nil
}

// ComResetConnection satisfies vitessmysql.Handler. Resets the current
// database on the session to the driver's configured database name.
func (h *Handler) ComResetConnection(c *vitessmysql.Conn) error {
	sess, ok := c.ClientData.(*session.Session)
	if !ok || sess == nil {
		return nil
	}
	sess.SetCurrentDatabase(h.dbName)
	return nil
}

// WarningCount satisfies vitessmysql.Handler.
func (h *Handler) WarningCount(_ *vitessmysql.Conn) uint16 { return 0 }

// ParserOptionsForConnection satisfies vitessmysql.Handler.
func (h *Handler) ParserOptionsForConnection(_ *vitessmysql.Conn) (sqlparser.ParserOptions, error) {
	return sqlparser.ParserOptions{}, nil
}
