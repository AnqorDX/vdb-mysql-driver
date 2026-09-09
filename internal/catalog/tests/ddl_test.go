package catalog_test

import (
	"testing"

	gmssql "github.com/AnqorDX/mysql-engine/sql"
	gmstypes "github.com/AnqorDX/mysql-engine/types"

	. "github.com/virtual-db/vdb-mysql-driver/internal/catalog"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildTable constructs a Table with the given source schema via the
// DatabaseProvider path so it has a valid delta pointer.
func buildTable(t *testing.T, cols ...string) (*DatabaseProvider, gmssql.Table) {
	t.Helper()
	schemaProv := &stubSchemaProvider{cols: makeDescriptors(cols...), pkCol: cols[0]}
	p := NewDatabaseProvider("shop", &stubRowsProvider{}, schemaProv, nil)
	ctx := gmssql.NewEmptyContext()
	db, err := p.Database(ctx, "shop")
	if err != nil {
		t.Fatalf("Database: %v", err)
	}
	tbl, ok, err := db.GetTableInsensitive(ctx, "products")
	if err != nil || !ok {
		t.Fatalf("GetTableInsensitive: ok=%v err=%v", ok, err)
	}
	return p, tbl
}

// col builds a minimal *gmssql.Column for DDL tests.
func col(name string, typ gmssql.Type) *gmssql.Column {
	return &gmssql.Column{Name: name, Type: typ}
}

// schemaNames returns the column names from a GMS schema in order.
func schemaNames(s gmssql.Schema) []string {
	out := make([]string, len(s))
	for i, c := range s {
		out[i] = c.Name
	}
	return out
}

// ---------------------------------------------------------------------------
// applyDeltaToSchema — unit tests for the schema merge helper
// ---------------------------------------------------------------------------

func TestApplyDeltaToSchema_AddColumn_AppendsToEnd(t *testing.T) {
	_, tbl := buildTable(t, "id", "int", "name", "varchar")
	ctx := gmssql.NewEmptyContext()

	at := tbl.(gmssql.AlterableTable)
	if err := at.AddColumn(ctx, col("score", gmstypes.Int32), nil); err != nil {
		t.Fatalf("AddColumn: %v", err)
	}

	names := schemaNames(tbl.Schema())
	if len(names) != 3 {
		t.Fatalf("expected 3 columns, got %d: %v", len(names), names)
	}
	if names[2] != "score" {
		t.Errorf("expected added column at index 2 to be 'score', got %q", names[2])
	}
}

func TestApplyDeltaToSchema_DropColumn_RemovesFromSchema(t *testing.T) {
	_, tbl := buildTable(t, "id", "int", "name", "varchar", "price", "int")
	ctx := gmssql.NewEmptyContext()

	at := tbl.(gmssql.AlterableTable)
	if err := at.DropColumn(ctx, "price"); err != nil {
		t.Fatalf("DropColumn: %v", err)
	}

	names := schemaNames(tbl.Schema())
	for _, n := range names {
		if n == "price" {
			t.Error("expected 'price' to be absent from schema after DropColumn")
		}
	}
	if len(names) != 2 {
		t.Errorf("expected 2 columns after drop, got %d: %v", len(names), names)
	}
}

func TestApplyDeltaToSchema_ModifyColumn_UpdatesDefinition(t *testing.T) {
	_, tbl := buildTable(t, "id", "int", "status", "varchar")
	ctx := gmssql.NewEmptyContext()

	at := tbl.(gmssql.AlterableTable)
	newCol := col("status", gmstypes.LongText)
	if err := at.ModifyColumn(ctx, "status", newCol, nil); err != nil {
		t.Fatalf("ModifyColumn: %v", err)
	}

	schema := tbl.Schema()
	var found *gmssql.Column
	for _, c := range schema {
		if c.Name == "status" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("expected 'status' column to still be present after ModifyColumn")
	}
	if found.Type != gmstypes.LongText {
		t.Errorf("expected modified type LongText, got %v", found.Type)
	}
}

func TestApplyDeltaToSchema_RenameColumn_SourceColumn(t *testing.T) {
	_, tbl := buildTable(t, "id", "int", "user_name", "varchar")
	ctx := gmssql.NewEmptyContext()

	// RenameColumn is a helper on Table — call via type assertion.
	type renamer interface {
		RenameColumn(*gmssql.Context, string, string) error
	}
	if err := tbl.(renamer).RenameColumn(ctx, "user_name", "username"); err != nil {
		t.Fatalf("RenameColumn: %v", err)
	}

	names := schemaNames(tbl.Schema())
	for _, n := range names {
		if n == "user_name" {
			t.Error("old name 'user_name' must not appear after rename")
		}
	}
	found := false
	for _, n := range names {
		if n == "username" {
			found = true
		}
	}
	if !found {
		t.Errorf("new name 'username' not found in schema: %v", names)
	}
}

func TestApplyDeltaToSchema_RenameColumn_VirtualColumn(t *testing.T) {
	// A column added via AddColumn and then renamed must update AddedCols,
	// not create a RenamedCols entry pointing to the source DB.
	_, tbl := buildTable(t, "id", "int")
	ctx := gmssql.NewEmptyContext()

	at := tbl.(gmssql.AlterableTable)
	if err := at.AddColumn(ctx, col("score", gmstypes.Int32), nil); err != nil {
		t.Fatalf("AddColumn: %v", err)
	}

	type renamer interface {
		RenameColumn(*gmssql.Context, string, string) error
	}
	if err := tbl.(renamer).RenameColumn(ctx, "score", "points"); err != nil {
		t.Fatalf("RenameColumn on virtual col: %v", err)
	}

	names := schemaNames(tbl.Schema())
	found := false
	for _, n := range names {
		if n == "points" {
			found = true
		}
		if n == "score" {
			t.Error("old virtual name 'score' must not appear after rename")
		}
	}
	if !found {
		t.Errorf("new name 'points' not found in schema: %v", names)
	}
}

// ---------------------------------------------------------------------------
// synthesiseAddedCols — source rows expanded to virtual schema width
// ---------------------------------------------------------------------------

func TestAddColumn_Schema_HasCorrectLength(t *testing.T) {
	_, tbl := buildTable(t, "id", "int", "name", "varchar")
	ctx := gmssql.NewEmptyContext()

	at := tbl.(gmssql.AlterableTable)
	_ = at.AddColumn(ctx, col("score", gmstypes.Int32), nil)
	_ = at.AddColumn(ctx, col("grade", gmstypes.Text), nil)

	if got := len(tbl.Schema()); got != 4 {
		t.Errorf("expected 4 columns after 2 AddColumn calls, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// CreateTable — virtual table with no source backing
// ---------------------------------------------------------------------------

func TestCreateTable_GetTableInsensitive_ReturnsTable(t *testing.T) {
	p := NewDatabaseProvider("shop", &stubRowsProvider{}, &stubSchemaProvider{}, nil)
	ctx := gmssql.NewEmptyContext()
	db, _ := p.Database(ctx, "shop")
	creator := db.(gmssql.TableCreator)

	schema := gmssql.PrimaryKeySchema{
		Schema: gmssql.Schema{
			{Name: "id", Type: gmstypes.Int32, PrimaryKey: true},
			{Name: "label", Type: gmstypes.LongText},
		},
	}
	if err := creator.CreateTable(ctx, "events", schema, gmssql.Collation_Default, ""); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	tbl, ok, err := db.GetTableInsensitive(ctx, "events")
	if err != nil {
		t.Fatalf("GetTableInsensitive after CreateTable: %v", err)
	}
	if !ok {
		t.Fatal("expected GetTableInsensitive to find the created table")
	}
	if tbl == nil {
		t.Fatal("expected non-nil Table after CreateTable")
	}
}

func TestCreateTable_Schema_MatchesDeclaredColumns(t *testing.T) {
	p := NewDatabaseProvider("shop", &stubRowsProvider{}, &stubSchemaProvider{}, nil)
	ctx := gmssql.NewEmptyContext()
	db, _ := p.Database(ctx, "shop")
	creator := db.(gmssql.TableCreator)

	declared := gmssql.PrimaryKeySchema{
		Schema: gmssql.Schema{
			{Name: "id", Type: gmstypes.Int32, PrimaryKey: true},
			{Name: "label", Type: gmstypes.LongText},
		},
	}
	_ = creator.CreateTable(ctx, "events", declared, gmssql.Collation_Default, "")

	tbl, _, _ := db.GetTableInsensitive(ctx, "events")
	names := schemaNames(tbl.Schema())
	if len(names) != 2 {
		t.Fatalf("expected 2 columns, got %d: %v", len(names), names)
	}
	if names[0] != "id" || names[1] != "label" {
		t.Errorf("unexpected column names: %v", names)
	}
}

// ---------------------------------------------------------------------------
// DropTable — table becomes invisible
// ---------------------------------------------------------------------------

func TestDropTable_GetTableInsensitive_ReturnsNotFound(t *testing.T) {
	schemaProv := &stubSchemaProvider{cols: makeDescriptors("id", "int"), pkCol: "id"}
	p := NewDatabaseProvider("shop", &stubRowsProvider{}, schemaProv, nil)
	ctx := gmssql.NewEmptyContext()
	db, _ := p.Database(ctx, "shop")

	// Confirm the table is visible before drop.
	_, ok, _ := db.GetTableInsensitive(ctx, "products")
	if !ok {
		t.Fatal("expected products to be visible before DropTable")
	}

	dropper := db.(gmssql.TableDropper)
	if err := dropper.DropTable(ctx, "products"); err != nil {
		t.Fatalf("DropTable: %v", err)
	}

	_, ok, err := db.GetTableInsensitive(ctx, "products")
	if err != nil {
		t.Fatalf("GetTableInsensitive after DropTable: unexpected error: %v", err)
	}
	if ok {
		t.Error("expected GetTableInsensitive to return ok=false after DropTable")
	}
}

// ---------------------------------------------------------------------------
// Index DDL
// ---------------------------------------------------------------------------

func TestCreateIndex_DoesNotError(t *testing.T) {
	_, tbl := buildTable(t, "id", "int", "email", "varchar")
	ctx := gmssql.NewEmptyContext()

	ia := tbl.(gmssql.IndexAlterableTable)
	idx := gmssql.IndexDef{Name: "idx_email"}
	if err := ia.CreateIndex(ctx, idx); err != nil {
		t.Fatalf("CreateIndex returned unexpected error: %v", err)
	}
}

func TestDropIndex_OnNonExistentIndex_DoesNotError(t *testing.T) {
	_, tbl := buildTable(t, "id", "int")
	ctx := gmssql.NewEmptyContext()

	ia := tbl.(gmssql.IndexAlterableTable)
	if err := ia.DropIndex(ctx, "idx_nonexistent"); err != nil {
		t.Fatalf("DropIndex on nonexistent index returned unexpected error: %v", err)
	}
}

func TestDropIndex_AfterCreate_DoesNotError(t *testing.T) {
	_, tbl := buildTable(t, "id", "int")
	ctx := gmssql.NewEmptyContext()

	ia := tbl.(gmssql.IndexAlterableTable)
	_ = ia.CreateIndex(ctx, gmssql.IndexDef{Name: "idx_x"})

	if err := ia.DropIndex(ctx, "idx_x"); err != nil {
		t.Fatalf("DropIndex after CreateIndex returned unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Multiple DDL operations in sequence
// ---------------------------------------------------------------------------

func TestDDL_AddThenDropColumn_NotInSchema(t *testing.T) {
	_, tbl := buildTable(t, "id", "int", "name", "varchar")
	ctx := gmssql.NewEmptyContext()

	at := tbl.(gmssql.AlterableTable)
	_ = at.AddColumn(ctx, col("temp", gmstypes.LongText), nil)
	_ = at.DropColumn(ctx, "temp")

	names := schemaNames(tbl.Schema())
	for _, n := range names {
		if n == "temp" {
			t.Error("'temp' must not appear after add+drop sequence")
		}
	}
}

func TestDDL_RenameColumn_ChainedTwice_PreservesOriginalSourceName(t *testing.T) {
	// user_name → username → uname — the source DB name must remain "user_name".
	_, tbl := buildTable(t, "id", "int", "user_name", "varchar")
	ctx := gmssql.NewEmptyContext()

	type renamer interface {
		RenameColumn(*gmssql.Context, string, string) error
	}
	r := tbl.(renamer)
	_ = r.RenameColumn(ctx, "user_name", "username")
	_ = r.RenameColumn(ctx, "username", "uname")

	names := schemaNames(tbl.Schema())
	found := false
	for _, n := range names {
		if n == "uname" {
			found = true
		}
		if n == "username" || n == "user_name" {
			t.Errorf("intermediate/old name %q must not appear in schema after chained rename", n)
		}
	}
	if !found {
		t.Errorf("final name 'uname' not found in schema: %v", names)
	}
}
