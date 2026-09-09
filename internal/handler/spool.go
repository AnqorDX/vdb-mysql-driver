package handler

import (
	"io"

	vitessmysql "github.com/dolthub/vitess/go/mysql"
	"github.com/dolthub/vitess/go/sqltypes"
	querypb "github.com/dolthub/vitess/go/vt/proto/query"

	"github.com/AnqorDX/mysql-engine/server"
	gmssql "github.com/AnqorDX/mysql-engine/sql"
	gmstypes "github.com/AnqorDX/mysql-engine/types"
)

// spoolResult dispatches to spoolOkResult or spoolRows depending on whether
// the schema represents a DML OkResult or a normal row-returning query.
func spoolResult(
	ctx *gmssql.Context,
	schema gmssql.Schema,
	iter gmssql.RowIter,
	callback vitessmysql.ResultSpoolFn,
) error {
	if gmstypes.IsOkResultSchema(schema) {
		return spoolOkResult(ctx, iter, callback)
	}
	return spoolRows(ctx, schema, iter, callback)
}

// spoolOkResult handles DML queries (INSERT, UPDATE, DELETE, DDL) that return
// an OkResult rather than a row set. It reads the single OkResult from the
// iterator and returns an sqltypes.Result carrying RowsAffected and InsertID.
func spoolOkResult(
	ctx *gmssql.Context,
	iter gmssql.RowIter,
	callback vitessmysql.ResultSpoolFn,
) error {
	row, err := iter.Next(ctx)
	if err != nil && err != io.EOF {
		return err
	}

	res := &sqltypes.Result{}
	if len(row) > 0 {
		ok := gmstypes.GetOkResult(row)
		res.RowsAffected = ok.RowsAffected
		res.InsertID = ok.InsertID
	}

	return callback(res, false)
}

// spoolRows converts a GMS (schema, rowIter) pair into a single Vitess
// sqltypes.Result and delivers it to callback in one call. The MySQL wire
// protocol represents a SELECT response as exactly one result set; calling
// callback more than once per query sends multiple result sets over the wire,
// which desyncs the protocol state on pinned connections (e.g. transactions).
func spoolRows(
	ctx *gmssql.Context,
	schema gmssql.Schema,
	iter gmssql.RowIter,
	callback vitessmysql.ResultSpoolFn,
) error {
	fields := schemaToFields(ctx, schema)
	buf := gmssql.NewByteBuffer(4096)

	result := &sqltypes.Result{Fields: fields}

	for {
		row, err := iter.Next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		sqlVals, err := server.RowToSQL(ctx, schema, row, nil, buf)
		if err != nil {
			return err
		}

		result.Rows = append(result.Rows, sqlVals)
		result.RowsAffected++
	}

	// Single callback call — one result set over the wire.
	return callback(result, false)
}

// schemaToFields converts a GMS sql.Schema into the Vitess []*querypb.Field
// representation used in Result packets. Covers the common type set; charset
// and flag metadata matches what GMS's own handler produces.
func schemaToFields(ctx *gmssql.Context, s gmssql.Schema) []*querypb.Field {
	charSetResults := ctx.GetCharacterSetResults()
	fields := make([]*querypb.Field, len(s))
	for i, col := range s {
		charset := uint32(gmssql.Collation_Default.CharacterSet())
		if ct, ok := col.Type.(gmssql.TypeWithCollation); ok {
			charset = uint32(ct.Collation().CharacterSet())
		}
		if gmstypes.IsBinaryType(col.Type) {
			charset = uint32(gmssql.Collation_binary)
		} else if charSetResults != gmssql.CharacterSet_Unspecified {
			charset = uint32(charSetResults)
		}

		var flags querypb.MySqlFlag
		if !col.Nullable {
			flags |= querypb.MySqlFlag_NOT_NULL_FLAG
		}
		if col.AutoIncrement {
			flags |= querypb.MySqlFlag_AUTO_INCREMENT_FLAG
		}
		if col.PrimaryKey {
			flags |= querypb.MySqlFlag_PRI_KEY_FLAG
		}
		if gmstypes.IsUnsigned(col.Type) {
			flags |= querypb.MySqlFlag_UNSIGNED_FLAG
		}

		fields[i] = &querypb.Field{
			Name:     col.Name,
			OrgName:  col.Name,
			Table:    col.Source,
			OrgTable: col.Source,
			Database: col.DatabaseSource,
			Type:     col.Type.Type(),
			Charset:  charset,
			Flags:    uint32(flags),
		}
	}
	return fields
}
