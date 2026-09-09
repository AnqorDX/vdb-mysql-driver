package catalog_test

import (
	"testing"

	gmstypes "github.com/AnqorDX/mysql-engine/types"

	"github.com/virtual-db/vdb-mysql-driver/internal/catalog"
	intschema "github.com/virtual-db/vdb-mysql-driver/internal/schema"
)

func TestColumnTypeToGMS_Int_ReturnsInt32(t *testing.T) {
	col := intschema.ColumnDescriptor{Name: "total_cents", DataType: "int", ColumnType: "int", IsNullable: "YES"}
	s := catalog.ToGMSSchema("orders", []intschema.ColumnDescriptor{col})
	if s[0].Type != gmstypes.Int32 {
		t.Errorf("INT column: expected gmstypes.Int32, got %T / %v", s[0].Type, s[0].Type)
	}
}

func TestColumnTypeToGMS_Integer_ReturnsInt32(t *testing.T) {
	col := intschema.ColumnDescriptor{Name: "count", DataType: "integer", ColumnType: "integer", IsNullable: "YES"}
	s := catalog.ToGMSSchema("stats", []intschema.ColumnDescriptor{col})
	if s[0].Type != gmstypes.Int32 {
		t.Errorf("INTEGER column: expected gmstypes.Int32, got %T / %v", s[0].Type, s[0].Type)
	}
}

func TestColumnTypeToGMS_TinyInt_ReturnsInt8(t *testing.T) {
	col := intschema.ColumnDescriptor{Name: "active", DataType: "tinyint", ColumnType: "tinyint", IsNullable: "YES"}
	s := catalog.ToGMSSchema("t", []intschema.ColumnDescriptor{col})
	if s[0].Type != gmstypes.Int8 {
		t.Errorf("TINYINT column: expected gmstypes.Int8, got %T / %v", s[0].Type, s[0].Type)
	}
}

func TestColumnTypeToGMS_SmallInt_ReturnsInt16(t *testing.T) {
	col := intschema.ColumnDescriptor{Name: "rank", DataType: "smallint", ColumnType: "smallint", IsNullable: "YES"}
	s := catalog.ToGMSSchema("t", []intschema.ColumnDescriptor{col})
	if s[0].Type != gmstypes.Int16 {
		t.Errorf("SMALLINT column: expected gmstypes.Int16, got %T / %v", s[0].Type, s[0].Type)
	}
}

func TestColumnTypeToGMS_MediumInt_ReturnsInt32(t *testing.T) {
	col := intschema.ColumnDescriptor{Name: "score", DataType: "mediumint", ColumnType: "mediumint", IsNullable: "YES"}
	s := catalog.ToGMSSchema("t", []intschema.ColumnDescriptor{col})
	if s[0].Type != gmstypes.Int32 {
		t.Errorf("MEDIUMINT column: expected gmstypes.Int32, got %T / %v", s[0].Type, s[0].Type)
	}
}

func TestColumnTypeToGMS_BigInt_ReturnsInt64(t *testing.T) {
	col := intschema.ColumnDescriptor{Name: "population", DataType: "bigint", ColumnType: "bigint", IsNullable: "YES"}
	s := catalog.ToGMSSchema("t", []intschema.ColumnDescriptor{col})
	if s[0].Type != gmstypes.Int64 {
		t.Errorf("BIGINT column: expected gmstypes.Int64, got %T / %v", s[0].Type, s[0].Type)
	}
}

func TestColumnTypeToGMS_Float_ReturnsFloat32(t *testing.T) {
	col := intschema.ColumnDescriptor{Name: "ratio", DataType: "float", ColumnType: "float", IsNullable: "YES"}
	s := catalog.ToGMSSchema("t", []intschema.ColumnDescriptor{col})
	if s[0].Type != gmstypes.Float32 {
		t.Errorf("FLOAT column: expected gmstypes.Float32, got %T / %v", s[0].Type, s[0].Type)
	}
}

func TestColumnTypeToGMS_Double_ReturnsFloat64(t *testing.T) {
	col := intschema.ColumnDescriptor{Name: "weight", DataType: "double", ColumnType: "double", IsNullable: "YES"}
	s := catalog.ToGMSSchema("t", []intschema.ColumnDescriptor{col})
	if s[0].Type != gmstypes.Float64 {
		t.Errorf("DOUBLE column: expected gmstypes.Float64, got %T / %v", s[0].Type, s[0].Type)
	}
}
