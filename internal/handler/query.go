package handler

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"runtime/debug"

	vitessmysql "github.com/dolthub/vitess/go/mysql"
	"github.com/dolthub/vitess/go/sqltypes"
	querypb "github.com/dolthub/vitess/go/vt/proto/query"

	gmssql "github.com/AnqorDX/mysql-engine/sql"

	"github.com/virtual-db/vdb-mysql-driver/internal/session"
)

// ComQuery satisfies vitessmysql.Handler. Builds a *sql.Context from the
// session, fires the vdb.query.received pipeline (which may rewrite the
// query), calls engine.Query, spools rows to the callback, and fires the
// vdb.query.completed event when done.
func (h *Handler) ComQuery(
	ctx context.Context,
	c *vitessmysql.Conn,
	query string,
	callback vitessmysql.ResultSpoolFn,
) error {
	// FreeOSMemory must be the first defer so it fires last (LIFO),
	// after rowIter.Close() has released all query data.
	defer debug.FreeOSMemory()

	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	defer func() {
		runtime.GC()
		debug.FreeOSMemory()
		var memAfter runtime.MemStats
		runtime.ReadMemStats(&memAfter)
		log.Printf("[mem] query=%q alloc_before=%dMB alloc_after=%dMB heap_inuse=%dMB heap_idle=%dMB",
			query[:min(len(query), 64)],
			memBefore.Alloc/1024/1024, memAfter.Alloc/1024/1024,
			memAfter.HeapInuse/1024/1024, memAfter.HeapIdle/1024/1024)
	}()

	sess, ok := c.ClientData.(*session.Session)
	if !ok || sess == nil {
		return fmt.Errorf("no session for connection %d", c.ConnectionID)
	}

	connID := c.ConnectionID
	database := sess.GetCurrentDatabase()

	// Fire the query.received pipeline. The handler may rewrite the query;
	// a non-nil error rejects the query before execution.
	rewritten, err := h.events.QueryReceived(connID, query, database)
	if err != nil {
		return err
	}
	execQuery := query
	if rewritten != "" {
		execQuery = rewritten
	}

	sqlCtx := gmssql.NewContext(ctx, gmssql.WithSession(sess))

	schema, rowIter, _, execErr := h.engine.Query(sqlCtx, execQuery)

	// Always close rowIter. GMS may return a non-nil iterator even on error,
	// and leaving it open leaks the internal partition iterator, any data
	// buffered inside GMS, and prevents GC from reclaiming that memory.
	if rowIter != nil {
		defer rowIter.Close(sqlCtx)
	}

	var rowsAffected int64
	var spoolErr error

	if execErr == nil {
		// Wrap the callback to accumulate the affected-row count for
		// the QueryCompleted event payload.
		wrappedCallback := func(res *sqltypes.Result, more bool) error {
			if res != nil {
				if res.RowsAffected > 0 {
					rowsAffected = int64(res.RowsAffected)
				} else {
					rowsAffected += int64(len(res.Rows))
				}
			}
			return callback(res, more)
		}

		spoolErr = spoolResult(sqlCtx, schema, rowIter, wrappedCallback)
	}

	// Fire the query.completed event (fire-and-forget).
	finalErr := execErr
	if finalErr == nil {
		finalErr = spoolErr
	}
	h.events.QueryCompleted(connID, query, rowsAffected, finalErr)

	if execErr != nil {
		return castError(execErr)
	}
	return castError(spoolErr)
}

// ComMultiQuery satisfies vitessmysql.Handler. Executes the first statement
// and returns the remainder. Multi-statement support mirrors ComQuery.
func (h *Handler) ComMultiQuery(
	ctx context.Context,
	c *vitessmysql.Conn,
	query string,
	callback vitessmysql.ResultSpoolFn,
) (string, error) {
	// Execute only the first statement; return the remainder unparsed.
	// The Vitess listener will call us again for each subsequent statement.
	return "", h.ComQuery(ctx, c, query, callback)
}

// ComPrepare satisfies vitessmysql.Handler. Returns no fields — prepared
// statement parameter metadata is not yet implemented.
func (h *Handler) ComPrepare(_ context.Context, _ *vitessmysql.Conn, _ string, _ *vitessmysql.PrepareData) ([]*querypb.Field, error) {
	return nil, nil
}

// ComStmtExecute satisfies vitessmysql.Handler. Converts the Vitess bind
// variables into sqlparser expressions and calls engine.QueryWithBindings so
// that positional parameters are substituted before execution.
func (h *Handler) ComStmtExecute(
	ctx context.Context,
	c *vitessmysql.Conn,
	prepare *vitessmysql.PrepareData,
	callback func(*sqltypes.Result) error,
) error {
	defer debug.FreeOSMemory()

	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	defer func() {
		runtime.GC()
		debug.FreeOSMemory()
		var memAfter runtime.MemStats
		runtime.ReadMemStats(&memAfter)
		log.Printf("[mem] stmt=%q alloc_before=%dMB alloc_after=%dMB heap_inuse=%dMB heap_idle=%dMB",
			prepare.PrepareStmt[:min(len(prepare.PrepareStmt), 64)],
			memBefore.Alloc/1024/1024, memAfter.Alloc/1024/1024,
			memAfter.HeapInuse/1024/1024, memAfter.HeapIdle/1024/1024)
	}()

	sess, ok := c.ClientData.(*session.Session)
	if !ok || sess == nil {
		return fmt.Errorf("no session for connection %d", c.ConnectionID)
	}

	sqlCtx := gmssql.NewContext(ctx, gmssql.WithSession(sess))

	exprs, err := bindingsToExprs(prepare.BindVars)
	if err != nil {
		return err
	}

	schema, rowIter, _, err := h.engine.QueryWithBindings(sqlCtx, prepare.PrepareStmt, nil, exprs, nil)
	if err != nil {
		return castError(err)
	}
	defer rowIter.Close(sqlCtx)

	err = castError(spoolResult(sqlCtx, schema, rowIter, func(res *sqltypes.Result, more bool) error {
		return callback(res)
	}))
	return err
}
