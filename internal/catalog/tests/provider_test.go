package catalog_test

import (
	"testing"

	gmssql "github.com/dolthub/go-mysql-server/sql"

	"github.com/virtual-db/vdb-mysql-driver/internal/rows"
	"github.com/virtual-db/vdb-mysql-driver/internal/schema"

	. "github.com/virtual-db/vdb-mysql-driver/internal/catalog"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

// stubSchemaProvider satisfies schema.Provider with configurable return values.
type stubSchemaProvider struct {
	cols  []schema.ColumnDescriptor
	pkCol string
	err   error
}

var _ schema.Provider = (*stubSchemaProvider)(nil)

func (s *stubSchemaProvider) GetSchema(_ string) ([]schema.ColumnDescriptor, string, error) {
	return s.cols, s.pkCol, s.err
}

// makeDescriptors builds a []ColumnDescriptor from (name, dataType) pairs.
func makeDescriptors(pairs ...string) []schema.ColumnDescriptor {
	if len(pairs)%2 != 0 {
		panic("makeDescriptors: pairs must be even")
	}
	out := make([]schema.ColumnDescriptor, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out[i/2] = schema.ColumnDescriptor{
			Name:       pairs[i],
			DataType:   pairs[i+1],
			ColumnType: pairs[i+1],
			IsNullable: "YES",
		}
	}
	return out
}

// stubRowsProvider satisfies rows.Provider with minimal no-op implementations.
// It documents exactly which methods GMSProvider requires; none are called
// by the DatabaseProvider or Database tests below.
type stubRowsProvider struct{}

var _ rows.Provider = (*stubRowsProvider)(nil)

func (stubRowsProvider) FetchRows(_ *gmssql.Context, _ string, _ gmssql.RowIter, _ gmssql.Schema) ([]map[string]any, error) {
	return nil, nil
}
func (stubRowsProvider) CommitRows(_ *gmssql.Context, _ string, _ []map[string]any) ([]map[string]any, error) {
	return nil, nil
}
func (stubRowsProvider) InsertRow(_ *gmssql.Context, _ string, _ gmssql.Row, _ gmssql.Schema) (map[string]any, error) {
	return nil, nil
}
func (stubRowsProvider) UpdateRow(_ *gmssql.Context, _ string, _, _ gmssql.Row, _ gmssql.Schema) (map[string]any, error) {
	return nil, nil
}
func (stubRowsProvider) DeleteRow(_ *gmssql.Context, _ string, _ gmssql.Row, _ gmssql.Schema) error {
	return nil
}
func (stubRowsProvider) TruncateRows(_ *gmssql.Context, _ string) (int, error) {
	return 0, nil
}

// ---------------------------------------------------------------------------
// DatabaseProvider
// ---------------------------------------------------------------------------

func TestNewDatabaseProvider_ReturnsNonNil(t *testing.T) {
	p := NewDatabaseProvider("mydb", &stubRowsProvider{}, &stubSchemaProvider{}, nil)
	if p == nil {
		t.Fatal("NewDatabaseProvider returned nil")
	}
}

func TestDatabaseProvider_Database_KnownName_ReturnsDatabase(t *testing.T) {
	p := NewDatabaseProvider("mydb", &stubRowsProvider{}, &stubSchemaProvider{}, nil)
	ctx := gmssql.NewEmptyContext()

	db, err := p.Database(ctx, "mydb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil Database, got nil")
	}
}

func TestDatabaseProvider_Database_KnownName_CaseInsensitive(t *testing.T) {
	p := NewDatabaseProvider("MyDB", &stubRowsProvider{}, &stubSchemaProvider{}, nil)
	ctx := gmssql.NewEmptyContext()

	db, err := p.Database(ctx, "mydb")
	if err != nil {
		t.Fatalf("unexpected error for case-insensitive match: %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil Database for case-insensitive name, got nil")
	}
}

func TestDatabaseProvider_Database_UnknownName_ReturnsError(t *testing.T) {
	p := NewDatabaseProvider("mydb", &stubRowsProvider{}, &stubSchemaProvider{}, nil)
	ctx := gmssql.NewEmptyContext()

	_, err := p.Database(ctx, "does_not_exist")
	if err == nil {
		t.Fatal("expected non-nil error for unknown database name, got nil")
	}
}

func TestDatabaseProvider_AllDatabases_ContainsRegisteredDB(t *testing.T) {
	p := NewDatabaseProvider("mydb", &stubRowsProvider{}, &stubSchemaProvider{}, nil)
	ctx := gmssql.NewEmptyContext()

	all := p.AllDatabases(ctx)
	if len(all) != 1 {
		t.Fatalf("AllDatabases: got %d entries, want 1", len(all))
	}
	if all[0].Name() != "mydb" {
		t.Errorf("AllDatabases[0].Name(): got %q, want %q", all[0].Name(), "mydb")
	}
}

func TestDatabaseProvider_HasDatabase_KnownName_ReturnsTrue(t *testing.T) {
	p := NewDatabaseProvider("mydb", &stubRowsProvider{}, &stubSchemaProvider{}, nil)
	ctx := gmssql.NewEmptyContext()

	if !p.HasDatabase(ctx, "mydb") {
		t.Error("HasDatabase returned false for a registered database name")
	}
	if p.HasDatabase(ctx, "other") {
		t.Error("HasDatabase returned true for an unknown database name")
	}
}
