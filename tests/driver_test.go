package driver_test

import (
	"testing"

	driver "github.com/virtual-db/vdb-mysql-driver"
	"github.com/virtual-db/vdb-core/types"
)

type stubCoreAPI struct{}

func (s *stubCoreAPI) ConnectionOpened(_ uint32, _, _ string) error            { return nil }
func (s *stubCoreAPI) ConnectionClosed(_ uint32, _, _ string)                  {}
func (s *stubCoreAPI) TransactionBegun(_ uint32, _ bool) error                 { return nil }
func (s *stubCoreAPI) TransactionCommitted(_ uint32) error                     { return nil }
func (s *stubCoreAPI) TransactionRolledBack(_ uint32, _ string)                {}
func (s *stubCoreAPI) QueryReceived(_ uint32, query, _ string) (string, error) { return query, nil }
func (s *stubCoreAPI) QueryCompleted(_ uint32, _ string, _ int64, _ error)     {}
func (s *stubCoreAPI) RecordsSource(_ uint32, _ string, records types.RecordIter) (types.RecordIter, error) {
	return records, nil
}
func (s *stubCoreAPI) RecordsMerged(_ uint32, _ string, records []map[string]any) ([]map[string]any, error) {
	return records, nil
}
func (s *stubCoreAPI) RecordInserted(_ uint32, _ string, record map[string]any) (map[string]any, error) {
	return record, nil
}
func (s *stubCoreAPI) RecordUpdated(_ uint32, _ string, _, new map[string]any) (map[string]any, error) {
	return new, nil
}
func (s *stubCoreAPI) RecordDeleted(_ uint32, _ string, _ map[string]any) error { return nil }
func (s *stubCoreAPI) SchemaLoaded(_ string, _ []string, _ string)              {}
func (s *stubCoreAPI) SchemaInvalidated(_ string)                               {}
func (s *stubCoreAPI) TableTruncated(_ uint32, _ string) error                  { return nil }

func newTestDriver() *driver.Driver {
	return driver.NewDriver(driver.Config{
		Addr:           ":0",
		DBName:         "testdb",
		SourceDSN:      "user:pass@tcp(127.0.0.1:3306)/testdb",
		AuthSourceAddr: "127.0.0.1:3306",
	}, &stubCoreAPI{})
}

func TestStop_BeforeRun_ReturnsNil(t *testing.T) {
	d := newTestDriver()
	if err := d.Stop(); err != nil {
		t.Fatalf("Stop() before Run returned a non-nil error: %v", err)
	}
}

func TestStop_BeforeRun_DoesNotPanic(t *testing.T) {
	d := newTestDriver()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Stop() before Run panicked: %v", r)
		}
	}()
	_ = d.Stop()
}
