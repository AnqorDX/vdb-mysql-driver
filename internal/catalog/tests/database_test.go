package catalog_test

import (
	"errors"
	"testing"

	gmssql "github.com/AnqorDX/mysql-engine/sql"

	. "github.com/virtual-db/vdb-mysql-driver/internal/catalog"
)

// ---------------------------------------------------------------------------
// Database
// ---------------------------------------------------------------------------

func TestDatabase_Name_MatchesConstructorArg(t *testing.T) {
	p := NewDatabaseProvider("inventory", &stubRowsProvider{}, &stubSchemaProvider{}, nil)
	ctx := gmssql.NewEmptyContext()

	db, err := p.Database(ctx, "inventory")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if db.Name() != "inventory" {
		t.Errorf("Name(): got %q, want %q", db.Name(), "inventory")
	}
}

func TestDatabase_GetTableInsensitive_KnownTable_ReturnsTable(t *testing.T) {
	schemaProv := &stubSchemaProvider{cols: makeDescriptors("id", "int", "name", "varchar", "price", "int"), pkCol: "id"}
	p := NewDatabaseProvider("shop", &stubRowsProvider{}, schemaProv, nil)
	ctx := gmssql.NewEmptyContext()

	db, err := p.Database(ctx, "shop")
	if err != nil {
		t.Fatalf("unexpected error getting database: %v", err)
	}

	tbl, ok, err := db.GetTableInsensitive(ctx, "products")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("GetTableInsensitive: got ok=false, want true")
	}
	if tbl == nil {
		t.Fatal("GetTableInsensitive: got nil table, want non-nil")
	}
}

func TestDatabase_GetTableInsensitive_UnknownTable_ReturnsNilNoError(t *testing.T) {
	schemaProv := &stubSchemaProvider{err: errors.New("table not found")}
	p := NewDatabaseProvider("shop", &stubRowsProvider{}, schemaProv, nil)
	ctx := gmssql.NewEmptyContext()

	db, err := p.Database(ctx, "shop")
	if err != nil {
		t.Fatalf("unexpected error getting database: %v", err)
	}

	tbl, ok, err := db.GetTableInsensitive(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("expected nil error for unknown table, got: %v", err)
	}
	if ok {
		t.Error("GetTableInsensitive: got ok=true, want false for unknown table")
	}
	if tbl != nil {
		t.Errorf("GetTableInsensitive: got non-nil table, want nil for unknown table")
	}
}
