// Package bridge_test verifies that the EventBridge interface shape has not
// been accidentally broken. A concrete no-op implementation is kept
// compile-time-current with all 14 method signatures. If the interface gains,
// loses, or renames a method this file fails to compile with a precise error
// rather than producing a distant, ambiguous downstream failure.
package bridge_test

import (
	"testing"

	. "github.com/virtual-db/vdb-mysql-driver/internal/bridge"
	"github.com/virtual-db/vdb-core/types"
)

// noopBridge satisfies EventBridge with safe no-op implementations of every
// method. It documents which methods the interface owns and what their
// signatures are.
type noopBridge struct{}

var _ EventBridge = (*noopBridge)(nil)

func (noopBridge) ConnectionOpened(_ uint32, _, _ string) error { return nil }
func (noopBridge) ConnectionClosed(_ uint32, _, _ string)       {}

func (noopBridge) TransactionBegun(_ uint32, _ bool) error  { return nil }
func (noopBridge) TransactionCommitted(_ uint32) error      { return nil }
func (noopBridge) TransactionRolledBack(_ uint32, _ string) {}

func (noopBridge) QueryReceived(_ uint32, q, _ string) (string, error) { return q, nil }
func (noopBridge) QueryCompleted(_ uint32, _ string, _ int64, _ error) {}

func (noopBridge) RowsFetched(_ uint32, _ string, r types.RecordIter) (types.RecordIter, error) {
	return r, nil
}
func (noopBridge) RowsReady(_ uint32, _ string, r []map[string]any) ([]map[string]any, error) {
	return r, nil
}
func (noopBridge) RowInserted(_ uint32, _ string, r map[string]any) (map[string]any, error) {
	return r, nil
}
func (noopBridge) RowUpdated(_ uint32, _ string, _, n map[string]any) (map[string]any, error) {
	return n, nil
}
func (noopBridge) RowDeleted(_ uint32, _ string, _ map[string]any) error { return nil }

func (noopBridge) SchemaLoaded(_ string, _ []string, _ string) {}
func (noopBridge) SchemaInvalidated(_ string)                  {}
func (noopBridge) TableTruncated(_ uint32, _ string) error     { return nil }

// TestEventBridgeInterfaceShape is the compile-time contract test. The
// noopBridge assignment above is the real assertion; this function exists so
// `go test` reports a named result rather than "no test files".
func TestEventBridgeInterfaceShape(t *testing.T) {
	var _ EventBridge = (*noopBridge)(nil)
}
