// Package driver — adapter.go
//
// This file owns the adapter/wrapper type that supports the Driver but is
// not part of the driver lifecycle:
//
//   - apiAdapter: translates every bridge.EventBridge call to the corresponding
//     core.DriverAPI call. It is the single boundary between the driver's
//     internal packages and the vdb-core framework.
package driver

import (
	core "github.com/virtual-db/vdb-core"
	"github.com/virtual-db/vdb-core/types"

	"github.com/virtual-db/vdb-mysql-driver/internal/bridge"
)

// ---------------------------------------------------------------------------
// apiAdapter — adapts core.DriverAPI to bridge.EventBridge
// ---------------------------------------------------------------------------

// apiAdapter translates every bridge.EventBridge call to the corresponding
// core.DriverAPI call. It is the single boundary between the driver's internal
// packages and the vdb-core framework.
type apiAdapter struct {
	api core.DriverAPI
}

// Compile-time assertion: apiAdapter satisfies bridge.EventBridge.
var _ bridge.EventBridge = (*apiAdapter)(nil)

func (a *apiAdapter) ConnectionOpened(id uint32, user, addr string) error {
	return a.api.ConnectionOpened(id, user, addr)
}

func (a *apiAdapter) ConnectionClosed(id uint32, user, addr string) {
	a.api.ConnectionClosed(id, user, addr)
}

func (a *apiAdapter) TransactionBegun(connID uint32, readOnly bool) error {
	return a.api.TransactionBegun(connID, readOnly)
}

func (a *apiAdapter) TransactionCommitted(connID uint32) error {
	return a.api.TransactionCommitted(connID)
}

func (a *apiAdapter) TransactionRolledBack(connID uint32, savepoint string) {
	a.api.TransactionRolledBack(connID, savepoint)
}

func (a *apiAdapter) QueryReceived(connID uint32, query, database string) (string, error) {
	return a.api.QueryReceived(connID, query, database)
}

func (a *apiAdapter) QueryCompleted(connID uint32, query string, rowsAffected int64, err error) {
	a.api.QueryCompleted(connID, query, rowsAffected, err)
}

// RowsFetched maps to core.DriverAPI.RecordsSource (rows just read from the
// source DB before the delta overlay is applied).
func (a *apiAdapter) RowsFetched(connID uint32, table string, records types.RecordIter) (types.RecordIter, error) {
	return a.api.RecordsSource(connID, table, records)
}

// RowsReady maps to core.DriverAPI.RecordsMerged (final rows after the delta
// overlay has been applied, ready to return to the client).
func (a *apiAdapter) RowsReady(connID uint32, table string, records []map[string]any) ([]map[string]any, error) {
	return a.api.RecordsMerged(connID, table, records)
}

func (a *apiAdapter) RowInserted(connID uint32, table string, record map[string]any) (map[string]any, error) {
	return a.api.RecordInserted(connID, table, record)
}

func (a *apiAdapter) RowUpdated(connID uint32, table string, old, new map[string]any) (map[string]any, error) {
	return a.api.RecordUpdated(connID, table, old, new)
}

func (a *apiAdapter) RowDeleted(connID uint32, table string, record map[string]any) error {
	return a.api.RecordDeleted(connID, table, record)
}

// SchemaLoaded satisfies both bridge.EventBridge and schema.LoadListener.
// The apiAdapter is passed directly as the NotifyingProvider's listener.
func (a *apiAdapter) SchemaLoaded(table string, cols []string, pkCol string) {
	a.api.SchemaLoaded(table, cols, pkCol)
}

func (a *apiAdapter) SchemaInvalidated(table string) {
	a.api.SchemaInvalidated(table)
}

func (a *apiAdapter) TableTruncated(connID uint32, table string) error {
	return a.api.TableTruncated(connID, table)
}
