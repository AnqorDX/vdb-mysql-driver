package catalog_test

import (
	"testing"

	gmssql "github.com/AnqorDX/mysql-engine/sql"
	gmstypes "github.com/AnqorDX/mysql-engine/types"

	"github.com/virtual-db/vdb-mysql-driver/internal/catalog"
	intschema "github.com/virtual-db/vdb-mysql-driver/internal/schema"
)

// ---------------------------------------------------------------------------
// INT sort comparator — the root cause of BUG-001
//
// This group directly validates that the GMS type produced for an INT column
// orders 1000 above 800 when comparing in descending order. Before the fix,
// all columns were typed as LongText, whose string comparator treats "1000"
// as less than "800" (lexicographic: "1" < "8"), causing ORDER BY total_cents
// DESC to place 1000 last instead of first in JOIN results.
// ---------------------------------------------------------------------------

func TestInt32Type_ComparesNumerically_1000_GreaterThan_800(t *testing.T) {
	col := intschema.ColumnDescriptor{
		Name:       "total_cents",
		DataType:   "int",
		ColumnType: "int",
		IsNullable: "YES",
	}
	s := catalog.ToGMSSchema("orders", []intschema.ColumnDescriptor{col})
	gmsType := s[0].Type

	ctx := gmssql.NewEmptyContext()
	cmp, err := gmsType.Compare(ctx, int32(1000), int32(800))
	if err != nil {
		t.Fatalf("Compare(1000, 800) returned error: %v", err)
	}
	if cmp <= 0 {
		t.Errorf("INT Compare(1000, 800): got %d, want > 0 (1000 must be greater than 800)", cmp)
	}
}

func TestInt32Type_ComparesNumerically_800_LessThan_1000(t *testing.T) {
	col := intschema.ColumnDescriptor{
		Name:       "total_cents",
		DataType:   "int",
		ColumnType: "int",
		IsNullable: "YES",
	}
	s := catalog.ToGMSSchema("orders", []intschema.ColumnDescriptor{col})
	gmsType := s[0].Type

	ctx := gmssql.NewEmptyContext()
	cmp, err := gmsType.Compare(ctx, int32(800), int32(1000))
	if err != nil {
		t.Fatalf("Compare(800, 1000) returned error: %v", err)
	}
	if cmp >= 0 {
		t.Errorf("INT Compare(800, 1000): got %d, want < 0 (800 must be less than 1000)", cmp)
	}
}

func TestLongText_ComparesTreats1000LessThan800_ConfirmsOldBug(t *testing.T) {
	// Negative control: this test documents the old (broken) behaviour.
	// LongText compares values as strings: "1000" < "800" lexicographically.
	// If this test starts failing (LongText no longer sorts this way), update
	// or remove it — it is purely documentary.
	ctx := gmssql.NewEmptyContext()
	cmp, err := gmstypes.LongText.Compare(ctx, "1000", "800")
	if err != nil {
		t.Fatalf("LongText.Compare error: %v", err)
	}
	if cmp >= 0 {
		t.Logf("NOTE: LongText.Compare(\"1000\",\"800\") = %d (>= 0). "+
			"If GMS changed its LongText comparator this test should be removed.", cmp)
	}
	// We just document; no t.Errorf here. The real protection is the Int32
	// test above.
}
