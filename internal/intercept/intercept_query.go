package intercept

import (
	"context"

	vitessmysql "github.com/dolthub/vitess/go/mysql"
	sqltypes "github.com/dolthub/vitess/go/sqltypes"
	sqlparser "github.com/dolthub/vitess/go/vt/sqlparser"

	"github.com/AnqorDX/mysql-engine/server"
)

// Query is called for each incoming text query, before it reaches the engine.
//  1. Calls QueryReceived — the handler may rewrite the query or return an error.
//  2. Wraps the result callback to accumulate affected-row counts.
//  3. Delegates to the inner chain with the (possibly rewritten) query.
//  4. Calls QueryCompleted after the chain returns.
func (qi *Interceptor) Query(
	ctx context.Context,
	chain server.Chain,
	conn *vitessmysql.Conn,
	query string,
	callback func(*sqltypes.Result, bool) error,
) error {
	connID := conn.ConnectionID
	database := databaseFromContext(ctx)

	rewritten, err := qi.events.QueryReceived(connID, query, database)
	if err != nil {
		return err
	}
	execQuery := query
	if rewritten != "" {
		execQuery = rewritten
	}

	var rowsAffected int64
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

	execErr := chain.ComQuery(ctx, conn, execQuery, wrappedCallback)
	qi.events.QueryCompleted(connID, query, rowsAffected, execErr)
	return execErr
}

// ParsedQuery intercepts a pre-parsed query before it reaches the engine.
// Calls QueryReceived (which may rewrite the query) and QueryCompleted,
// mirroring the behaviour of Query. Database is not available from the
// ParsedQuery signature; an empty string is passed to QueryReceived.
func (qi *Interceptor) ParsedQuery(
	chain server.Chain,
	conn *vitessmysql.Conn,
	query string,
	parsed sqlparser.Statement,
	callback func(*sqltypes.Result, bool) error,
) error {
	connID := conn.ConnectionID

	rewritten, err := qi.events.QueryReceived(connID, query, "")
	if err != nil {
		return err
	}
	execQuery := query
	if rewritten != "" {
		execQuery = rewritten
	}

	var rowsAffected int64
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

	execErr := chain.ComQuery(context.Background(), conn, execQuery, wrappedCallback)
	qi.events.QueryCompleted(connID, query, rowsAffected, execErr)
	return execErr
}

// MultiQuery intercepts a multi-statement query before it reaches the engine.
// Calls QueryReceived for the combined query string and QueryCompleted after it runs.
func (qi *Interceptor) MultiQuery(
	ctx context.Context,
	chain server.Chain,
	conn *vitessmysql.Conn,
	query string,
	callback func(*sqltypes.Result, bool) error,
) (string, error) {
	connID := conn.ConnectionID

	rewritten, err := qi.events.QueryReceived(connID, query, "")
	if err != nil {
		return "", err
	}
	execQuery := query
	if rewritten != "" {
		execQuery = rewritten
	}

	var rowsAffected int64
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

	remainder, execErr := chain.ComMultiQuery(ctx, conn, execQuery, wrappedCallback)
	qi.events.QueryCompleted(connID, query, rowsAffected, execErr)
	return remainder, execErr
}
