package handler_test

import (
	"context"
	"testing"

	gmssql "github.com/AnqorDX/mysql-engine/sql"
	gmstypes "github.com/AnqorDX/mysql-engine/types"
	querypb "github.com/dolthub/vitess/go/vt/proto/query"

	handler "github.com/virtual-db/vdb-mysql-driver/internal/handler"
)

func newCtxForSchema() *gmssql.Context {
	base := gmssql.NewBaseSession()
	return gmssql.NewContext(context.Background(), gmssql.WithSession(base))
}

func TestSchemaToFields_LengthMatchesSchema(t *testing.T) {
	ctx := newCtxForSchema()
	schema := gmssql.Schema{
		{Name: "a", Type: gmstypes.Int64, Nullable: true},
		{Name: "b", Type: gmstypes.LongText, Nullable: true},
		{Name: "c", Type: gmstypes.Float64, Nullable: true},
	}
	fields := handler.SchemaToFields(ctx, schema)
	if len(fields) != len(schema) {
		t.Fatalf("expected %d fields, got %d", len(schema), len(fields))
	}
}

func TestSchemaToFields_ColumnNamePreserved(t *testing.T) {
	ctx := newCtxForSchema()
	schema := gmssql.Schema{
		{Name: "my_col", Type: gmstypes.Int64, Nullable: true},
	}
	fields := handler.SchemaToFields(ctx, schema)
	if fields[0].Name != "my_col" {
		t.Fatalf("expected Name=%q, got %q", "my_col", fields[0].Name)
	}
	if fields[0].OrgName != "my_col" {
		t.Fatalf("expected OrgName=%q, got %q", "my_col", fields[0].OrgName)
	}
}

func TestSchemaToFields_TableSourcePreserved(t *testing.T) {
	ctx := newCtxForSchema()
	schema := gmssql.Schema{
		{Name: "id", Type: gmstypes.Int64, Source: "users", Nullable: true},
	}
	fields := handler.SchemaToFields(ctx, schema)
	if fields[0].Table != "users" {
		t.Fatalf("expected Table=%q, got %q", "users", fields[0].Table)
	}
	if fields[0].OrgTable != "users" {
		t.Fatalf("expected OrgTable=%q, got %q", "users", fields[0].OrgTable)
	}
}

func TestSchemaToFields_NotNullFlag_Set(t *testing.T) {
	ctx := newCtxForSchema()
	schema := gmssql.Schema{
		{Name: "id", Type: gmstypes.Int64, Nullable: false},
	}
	fields := handler.SchemaToFields(ctx, schema)
	flags := querypb.MySqlFlag(fields[0].Flags)
	if flags&querypb.MySqlFlag_NOT_NULL_FLAG == 0 {
		t.Fatal("expected NOT_NULL_FLAG to be set for non-nullable column")
	}
}

func TestSchemaToFields_NullableColumn_NotNullFlagAbsent(t *testing.T) {
	ctx := newCtxForSchema()
	schema := gmssql.Schema{
		{Name: "name", Type: gmstypes.LongText, Nullable: true},
	}
	fields := handler.SchemaToFields(ctx, schema)
	flags := querypb.MySqlFlag(fields[0].Flags)
	if flags&querypb.MySqlFlag_NOT_NULL_FLAG != 0 {
		t.Fatal("expected NOT_NULL_FLAG to be absent for nullable column")
	}
}

func TestSchemaToFields_PrimaryKeyFlag_Set(t *testing.T) {
	ctx := newCtxForSchema()
	schema := gmssql.Schema{
		{Name: "id", Type: gmstypes.Int64, Nullable: false, PrimaryKey: true},
	}
	fields := handler.SchemaToFields(ctx, schema)
	flags := querypb.MySqlFlag(fields[0].Flags)
	if flags&querypb.MySqlFlag_PRI_KEY_FLAG == 0 {
		t.Fatal("expected PRI_KEY_FLAG to be set for primary key column")
	}
}

func TestSchemaToFields_AutoIncrementFlag_Set(t *testing.T) {
	ctx := newCtxForSchema()
	schema := gmssql.Schema{
		{Name: "id", Type: gmstypes.Int64, Nullable: false, AutoIncrement: true},
	}
	fields := handler.SchemaToFields(ctx, schema)
	flags := querypb.MySqlFlag(fields[0].Flags)
	if flags&querypb.MySqlFlag_AUTO_INCREMENT_FLAG == 0 {
		t.Fatal("expected AUTO_INCREMENT_FLAG to be set for auto-increment column")
	}
}
