// Package bridge defines the EventBridge contract used by all internal GMS
// sub-packages to signal database lifecycle events back to the driver layer.
//
// No package inside internal/ imports vdb-core or any other framework package.
// The driver package adapts core.DriverAPI to EventBridge via apiAdapter.
package bridge

import "github.com/virtual-db/vdb-core/types"

// EventBridge is the contract the GMS session, query interceptor, and table
// implementations use to signal lifecycle events.
//
// All methods must be safe for concurrent use from multiple goroutines.
type EventBridge interface {
	// ConnectionOpened is called once per authenticated connection, after the
	// MySQL handshake succeeds. Return a non-nil error to refuse the connection.
	ConnectionOpened(id uint32, user, addr string) error

	// ConnectionClosed is called when a connection is torn down. The return
	// value is ignored; a close cannot be vetoed.
	ConnectionClosed(id uint32, user, addr string)

	// TransactionBegun is called at BEGIN / START TRANSACTION. Return a non-nil
	// error to refuse the transaction.
	TransactionBegun(connID uint32, readOnly bool) error

	// TransactionCommitted is called at COMMIT. Return a non-nil error to leave
	// the transaction open.
	TransactionCommitted(connID uint32) error

	// TransactionRolledBack is called at ROLLBACK or ROLLBACK TO SAVEPOINT.
	// savepoint is the empty string for a full rollback.
	TransactionRolledBack(connID uint32, savepoint string)

	// QueryReceived is called for each incoming text query before parsing. The
	// handler may rewrite the query by returning a different string. Return a
	// non-nil error to reject the query.
	QueryReceived(connID uint32, query, database string) (string, error)

	// QueryCompleted is called after each SQL command finishes executing and all
	// rows have been sent to the client.
	QueryCompleted(connID uint32, query string, rowsAffected int64, err error)

	// RowsFetched is called with a streaming iterator over rows just read from
	// the source database, before the delta overlay is applied. The handler
	// must return a streaming iterator over the merged result. Both the input
	// and output iterators must be closed by the caller.
	RowsFetched(connID uint32, table string, records types.RecordIter) (types.RecordIter, error)

	// RowsReady is called with the final row slice after the delta overlay has
	// been applied and the rows are ready to return to the client.
	RowsReady(connID uint32, table string, records []map[string]any) ([]map[string]any, error)

	// RowInserted is called for each row GMS has determined should be inserted.
	// The handler may transform the row. Return a non-nil error to abort.
	RowInserted(connID uint32, table string, record map[string]any) (map[string]any, error)

	// RowUpdated is called for each row GMS has determined should be updated.
	// old is the row before the update; new is the intended replacement. Return
	// a non-nil error to abort.
	RowUpdated(connID uint32, table string, old, new map[string]any) (map[string]any, error)

	// RowDeleted is called for each row GMS has determined should be deleted.
	// Return a non-nil error to abort.
	RowDeleted(connID uint32, table string, record map[string]any) error

	// SchemaLoaded is called after a table schema has been successfully fetched
	// from the source database.
	SchemaLoaded(table string, cols []string, pkCol string)

	// SchemaInvalidated is called when a table's cached schema should be
	// discarded and re-fetched on next access.
	SchemaInvalidated(table string)

	// TableTruncated is called when a TRUNCATE TABLE statement is executed.
	// The implementation must clear all delta state for the table so that
	// subsequent reads return an empty result until new rows are inserted.
	TableTruncated(connID uint32, table string) error
}
