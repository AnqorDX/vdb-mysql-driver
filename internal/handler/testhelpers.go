package handler

// This file exposes package-internal types and functions for use by the
// internal/handler/tests package. Because the handler package is under
// internal/, these exports are invisible to any code outside this module,
// so they carry no public-API cost.
//
// Go's export_test.go mechanism only works for tests that live in the same
// directory; tests in a sub-directory (internal/handler/tests/) require real
// (non-_test.go) exported symbols. Keeping them here – in an internal package
// – is the standard work-around.

import (
	vitessmysql "github.com/dolthub/vitess/go/mysql"
	querypb "github.com/dolthub/vitess/go/vt/proto/query"

	gmssql "github.com/AnqorDX/mysql-engine/sql"
)

// CastError wraps the unexported castError function.
var CastError = castError

// BindingsToExprs wraps the unexported bindingsToExprs function.
var BindingsToExprs = bindingsToExprs

// SpoolResult wraps the unexported spoolResult function.
func SpoolResult(ctx *gmssql.Context, schema gmssql.Schema, iter gmssql.RowIter, cb vitessmysql.ResultSpoolFn) error {
	return spoolResult(ctx, schema, iter, cb)
}

// SpoolOkResult wraps the unexported spoolOkResult function.
func SpoolOkResult(ctx *gmssql.Context, iter gmssql.RowIter, cb vitessmysql.ResultSpoolFn) error {
	return spoolOkResult(ctx, iter, cb)
}

// SpoolRows wraps the unexported spoolRows function.
func SpoolRows(ctx *gmssql.Context, schema gmssql.Schema, iter gmssql.RowIter, cb vitessmysql.ResultSpoolFn) error {
	return spoolRows(ctx, schema, iter, cb)
}

// SchemaToFields wraps the unexported schemaToFields function.
func SchemaToFields(ctx *gmssql.Context, schema gmssql.Schema) []*querypb.Field {
	return schemaToFields(ctx, schema)
}
