package catalog_test

import (
	"strings"
	"testing"

	gmstypes "github.com/AnqorDX/mysql-engine/types"

	"github.com/virtual-db/vdb-mysql-driver/internal/catalog"
	intschema "github.com/virtual-db/vdb-mysql-driver/internal/schema"
)

func TestColumnTypeToGMS_Varchar_ReturnsStringType(t *testing.T) {
	col := intschema.ColumnDescriptor{
		Name:          "username",
		DataType:      "varchar",
		ColumnType:    "varchar(64)",
		IsNullable:    "NO",
		CharMaxLength: i64Ptr(64),
	}
	s := catalog.ToGMSSchema("users", []intschema.ColumnDescriptor{col})
	got := s[0].Type.String()
	if !strings.HasPrefix(got, "varchar(64)") {
		t.Errorf("VARCHAR(64) column type String(): got %q, want prefix %q", got, "varchar(64)")
	}
}

func TestColumnTypeToGMS_Varchar_DifferentLengths(t *testing.T) {
	cases := []struct {
		length     int64
		wantPrefix string
	}{
		{16, "varchar(16)"},
		{128, "varchar(128)"},
		{255, "varchar(255)"},
	}
	for _, tc := range cases {
		col := intschema.ColumnDescriptor{
			Name:          "field",
			DataType:      "varchar",
			CharMaxLength: i64Ptr(tc.length),
		}
		s := catalog.ToGMSSchema("t", []intschema.ColumnDescriptor{col})
		got := s[0].Type.String()
		if !strings.HasPrefix(got, tc.wantPrefix) {
			t.Errorf("VARCHAR(%d): got type %q, want prefix %q", tc.length, got, tc.wantPrefix)
		}
	}
}

func TestColumnTypeToGMS_Varchar_NotLongText(t *testing.T) {
	col := intschema.ColumnDescriptor{
		Name:          "email",
		DataType:      "varchar",
		CharMaxLength: i64Ptr(128),
	}
	s := catalog.ToGMSSchema("users", []intschema.ColumnDescriptor{col})
	if s[0].Type == gmstypes.LongText {
		t.Errorf("VARCHAR column must not map to gmstypes.LongText")
	}
	got := s[0].Type.String()
	if strings.HasPrefix(got, "longtext") {
		t.Errorf("VARCHAR(128) type String(): got %q, must not start with \"longtext\"", got)
	}
	if !strings.HasPrefix(got, "varchar") {
		t.Errorf("VARCHAR(128) type String(): got %q, want prefix \"varchar\"", got)
	}
}

func TestColumnTypeToGMS_Text_ReturnsText(t *testing.T) {
	col := makeCol("body", "text")
	s := catalog.ToGMSSchema("t", []intschema.ColumnDescriptor{col})
	if s[0].Type != gmstypes.Text {
		t.Errorf("TEXT column: expected gmstypes.Text, got %T / %v", s[0].Type, s[0].Type)
	}
}

func TestColumnTypeToGMS_LongText_ReturnsLongText(t *testing.T) {
	col := makeCol("content", "longtext")
	s := catalog.ToGMSSchema("t", []intschema.ColumnDescriptor{col})
	if s[0].Type != gmstypes.LongText {
		t.Errorf("LONGTEXT column: expected gmstypes.LongText, got %T / %v", s[0].Type, s[0].Type)
	}
}
