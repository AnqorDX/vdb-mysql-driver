package catalog_test

import (
	"testing"

	gmstypes "github.com/AnqorDX/mysql-engine/types"

	"github.com/virtual-db/vdb-mysql-driver/internal/catalog"
	intschema "github.com/virtual-db/vdb-mysql-driver/internal/schema"
)

func TestColumnTypeToGMS_Date_ReturnsDate(t *testing.T) {
	col := intschema.ColumnDescriptor{
		Name:       "dob",
		DataType:   "date",
		ColumnType: "date",
		IsNullable: "YES",
	}
	s := catalog.ToGMSSchema("t", []intschema.ColumnDescriptor{col})
	if s[0].Type != gmstypes.Date {
		t.Errorf("DATE column: expected gmstypes.Date, got %T / %v", s[0].Type, s[0].Type)
	}
}

func TestColumnTypeToGMS_Datetime_ReturnDatetime(t *testing.T) {
	col := intschema.ColumnDescriptor{
		Name:       "created_at",
		DataType:   "datetime",
		ColumnType: "datetime",
		IsNullable: "YES",
	}
	s := catalog.ToGMSSchema("t", []intschema.ColumnDescriptor{col})
	if s[0].Type != gmstypes.Datetime {
		t.Errorf("DATETIME column: expected gmstypes.Datetime, got %T / %v", s[0].Type, s[0].Type)
	}
}

func TestColumnTypeToGMS_UnknownType_FallsBackToLongText(t *testing.T) {
	col := intschema.ColumnDescriptor{
		Name:       "x",
		DataType:   "geometry",
		ColumnType: "geometry",
		IsNullable: "YES",
	}
	s := catalog.ToGMSSchema("t", []intschema.ColumnDescriptor{col})
	if s[0].Type == nil {
		t.Fatal("expected non-nil type for unknown DataType, got nil")
	}
	if s[0].Type != gmstypes.LongText {
		t.Errorf("unknown type: expected LongText fallback, got %T / %v", s[0].Type, s[0].Type)
	}
}
