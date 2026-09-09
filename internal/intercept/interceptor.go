// Package intercept provides the GMS server.Interceptor implementation that hooks
// into the query lifecycle and forwards events to a bridge.EventBridge.
package intercept

import (
	"context"

	vitessmysql "github.com/dolthub/vitess/go/mysql"
	sqltypes "github.com/dolthub/vitess/go/sqltypes"
	querypb "github.com/dolthub/vitess/go/vt/proto/query"
	"github.com/virtual-db/vdb-mysql-driver/internal/bridge"

	"github.com/AnqorDX/mysql-engine/server"
)

// Interceptor implements server.Interceptor to hook into the query lifecycle.
// It calls EventBridge.QueryReceived before each query (allowing rewrites) and
// EventBridge.QueryCompleted after the chain returns.
type Interceptor struct {
	events bridge.EventBridge
}

// Compile-time interface assertion.
var _ server.Interceptor = (*Interceptor)(nil)

// NewInterceptor constructs an Interceptor backed by the given EventBridge.
// events must not be nil.
func NewInterceptor(events bridge.EventBridge) *Interceptor {
	return &Interceptor{events: events}
}

// Priority returns 0. There is only one interceptor in this chain.
func (qi *Interceptor) Priority() int { return 0 }

// Prepare delegates to the chain without invoking any callback.
func (qi *Interceptor) Prepare(
	ctx context.Context,
	chain server.Chain,
	conn *vitessmysql.Conn,
	query string,
	prepare *vitessmysql.PrepareData,
) ([]*querypb.Field, error) {
	return chain.ComPrepare(ctx, conn, query, prepare)
}

// StmtExecute delegates to the chain without invoking any callback.
func (qi *Interceptor) StmtExecute(
	ctx context.Context,
	chain server.Chain,
	conn *vitessmysql.Conn,
	prepare *vitessmysql.PrepareData,
	callback func(*sqltypes.Result) error,
) error {
	return chain.ComStmtExecute(ctx, conn, prepare, callback)
}
