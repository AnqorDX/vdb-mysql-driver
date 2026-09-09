package catalog

import (
	"strconv"
	"strings"

	gmssql "github.com/AnqorDX/mysql-engine/sql"
	"github.com/AnqorDX/mysql-engine/expression"
	gmstypes "github.com/AnqorDX/mysql-engine/types"
	sqltypes "github.com/dolthub/vitess/go/sqltypes"

	intschema "github.com/virtual-db/vdb-mysql-driver/internal/schema"
)

// ToGMSSchema converts a slice of ColumnDescriptors into a GMS sql.Schema.
// Each column receives the correct GMS type, nullability, primary-key flag,
// auto-increment flag, and default value derived from the source database
// metadata. The table argument is set as the Source field on every column,
// which GMS uses for column resolution in JOIN and subquery contexts.
func ToGMSSchema(table string, cols []intschema.ColumnDescriptor) gmssql.Schema {
	schema := make(gmssql.Schema, len(cols))
	for i, col := range cols {
		gmsType := columnTypeToGMS(col)
		isAutoIncrement := strings.Contains(col.Extra, "auto_increment")
		schema[i] = &gmssql.Column{
			Name:          col.Name,
			Type:          gmsType,
			Source:        table,
			Nullable:      col.IsNullable == "YES",
			PrimaryKey:    col.ColumnKey == "PRI",
			AutoIncrement: isAutoIncrement,
			// Extra is read directly by GMS for SHOW COLUMNS output.
			// AutoIncrement alone does not populate it automatically.
			Extra:   col.Extra,
			Default: columnDefaultValue(col.ColumnDefault, gmsType),
		}
	}
	return schema
}

// columnTypeToGMS maps a ColumnDescriptor to the appropriate GMS sql.Type.
// It switches on the bare DataType (e.g. "int", "varchar") returned by
// INFORMATION_SCHEMA.COLUMNS.DATA_TYPE. CharMaxLength is consulted for
// character-family types so that the GMS planner receives the correct declared
// length rather than an unbounded approximation.
//
// Unknown or unsupported types fall back to gmstypes.LongText so that the
// proxy degrades gracefully rather than panicking during schema load.
func columnTypeToGMS(col intschema.ColumnDescriptor) gmssql.Type {
	switch strings.ToLower(col.DataType) {

	// ── Integer family ────────────────────────────────────────────────────────

	case "tinyint":
		return gmstypes.Int8
	case "smallint":
		return gmstypes.Int16
	case "mediumint", "int", "integer":
		return gmstypes.Int32
	case "bigint":
		return gmstypes.Int64

	// ── Floating-point family ─────────────────────────────────────────────────

	case "float":
		return gmstypes.Float32
	case "double", "real":
		return gmstypes.Float64

	// ── Fixed-point ───────────────────────────────────────────────────────────

	case "decimal", "numeric":
		// Default precision=10, scale=0. Exact values require NUMERIC_PRECISION
		// and NUMERIC_SCALE from INFORMATION_SCHEMA, which are not yet fetched.
		return gmstypes.MustCreateDecimalType(10, 0)

	// ── Character / string family ─────────────────────────────────────────────

	case "char":
		// Use Collation_Default (utf8mb4_0900_bin) so that StringType.String()
		// does not append a " COLLATE …" suffix in SHOW COLUMNS output.
		// StringType.String() calls StringWithTableCollation(Collation_Default),
		// and only appends the suffix when the type's collation differs from the
		// table's default collation — which is also Collation_Default.
		return gmstypes.MustCreateStringWithDefaults(
			sqltypes.Char,
			charLength(col, 1),
		)
	case "varchar":
		return gmstypes.MustCreateStringWithDefaults(
			sqltypes.VarChar,
			charLength(col, 255),
		)

	// ── Text family ───────────────────────────────────────────────────────────

	case "tinytext":
		return gmstypes.TinyText
	case "text":
		return gmstypes.Text
	case "mediumtext":
		return gmstypes.MediumText
	case "longtext":
		return gmstypes.LongText

	// ── Binary / blob family ──────────────────────────────────────────────────

	case "tinyblob":
		return gmstypes.TinyBlob
	case "blob":
		return gmstypes.Blob
	case "mediumblob":
		return gmstypes.MediumBlob
	case "binary", "varbinary", "longblob":
		return gmstypes.LongBlob

	// ── Date / time family ────────────────────────────────────────────────────

	case "date":
		return gmstypes.Date
	case "datetime":
		return gmstypes.Datetime
	case "timestamp":
		return gmstypes.Timestamp
	case "time":
		return gmstypes.Time
	case "year":
		return gmstypes.Year

	// ── Miscellaneous ─────────────────────────────────────────────────────────

	case "json":
		return gmstypes.JSON
	case "bit":
		// bit(1) is the overwhelmingly common case; wider BIT types would need
		// gmstypes.MustCreateBitType but Boolean is an acceptable proxy here.
		return gmstypes.Boolean

	case "enum", "set":
		// A proper mapping would require fetching the value list from the full
		// COLUMN_TYPE string. LongText is a safe, readable fallback.
		return gmstypes.LongText

	default:
		return gmstypes.LongText
	}
}

// charLength returns the declared CHARACTER_MAXIMUM_LENGTH from col,
// or fallback when the field is nil. A nil CharMaxLength should not occur
// for VARCHAR or CHAR columns in a well-formed schema, but is guarded
// defensively.
func charLength(col intschema.ColumnDescriptor, fallback int64) int64 {
	if col.CharMaxLength != nil {
		return *col.CharMaxLength
	}
	return fallback
}

// columnDefaultValue builds a GMS *ColumnDefaultValue from the raw
// COLUMN_DEFAULT string returned by INFORMATION_SCHEMA.COLUMNS. Returns nil
// when defaultStr is nil, which means the column has no DEFAULT clause and
// GMS will report NULL / empty for the Default field in SHOW COLUMNS output.
//
// For integer and floating-point column types the default string is parsed to
// its native Go type so that GMS evaluates and formats it correctly (e.g. "1"
// for DEFAULT 1 on an INT column).
//
// For all other types the raw INFORMATION_SCHEMA string is wrapped in a
// rawStringDefault expression whose String() returns the bare value (e.g.
// "active") rather than the SQL-quoted form ("'active'") that
// expression.NewLiteral produces. This matches what MySQL's own SHOW COLUMNS
// emits for string default values.
func columnDefaultValue(defaultStr *string, colType gmssql.Type) *gmssql.ColumnDefaultValue {
	if defaultStr == nil {
		return nil
	}

	raw := *defaultStr

	var lit gmssql.Expression

	switch colType {
	case gmstypes.Int8, gmstypes.Int16, gmstypes.Int32, gmstypes.Int64,
		gmstypes.Uint8, gmstypes.Uint16, gmstypes.Uint32, gmstypes.Uint64:
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			lit = expression.NewLiteral(n, colType)
		} else {
			lit = expression.NewLiteral(raw, gmstypes.LongText)
		}

	case gmstypes.Float32, gmstypes.Float64:
		if f, err := strconv.ParseFloat(raw, 64); err == nil {
			lit = expression.NewLiteral(f, colType)
		} else {
			lit = expression.NewLiteral(raw, gmstypes.LongText)
		}

	default:
		// rawStringDefault.String() returns the bare value without SQL quoting,
		// matching MySQL's SHOW COLUMNS behaviour. Eval() returns the Go string
		// so GMS can fill it in for INSERT statements that omit the column.
		lit = rawStringDefault{val: raw, typ: colType}
	}

	cdv, err := gmssql.NewColumnDefaultValue(lit, colType, true, false, false)
	if err != nil {
		return nil
	}
	return cdv
}

// ---------------------------------------------------------------------------
// rawStringDefault — expression.Expression for non-numeric column defaults
// ---------------------------------------------------------------------------

// rawStringDefault is a minimal sql.Expression that holds a pre-evaluated
// string default value. It differs from expression.NewLiteral in one key way:
// String() returns the raw value without SQL quoting (e.g. "active" instead
// of "'active'"), which matches what MySQL's SHOW COLUMNS emits.
type rawStringDefault struct {
	val string
	typ gmssql.Type
}

var _ gmssql.Expression = rawStringDefault{}

func (r rawStringDefault) Resolved() bool                { return true }
func (r rawStringDefault) String() string                { return r.val }
func (r rawStringDefault) Type() gmssql.Type             { return r.typ }
func (r rawStringDefault) IsNullable() bool              { return false }
func (r rawStringDefault) Children() []gmssql.Expression { return nil }
func (r rawStringDefault) CollationCoercibility(*gmssql.Context) (gmssql.CollationID, byte) {
	return gmssql.Collation_Default, 4
}

func (r rawStringDefault) Eval(_ *gmssql.Context, _ gmssql.Row) (interface{}, error) {
	return r.val, nil
}

func (r rawStringDefault) WithChildren(children ...gmssql.Expression) (gmssql.Expression, error) {
	if len(children) != 0 {
		return nil, gmssql.ErrInvalidChildrenNumber.New(r, len(children), 0)
	}
	return r, nil
}

// DebugString satisfies the sql.DebugStringer interface that GMS checks for
// in some code paths.
func (r rawStringDefault) DebugString() string { return r.val }
