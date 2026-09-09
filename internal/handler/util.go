package handler

import (
	"fmt"

	"github.com/dolthub/vitess/go/sqltypes"
	querypb "github.com/dolthub/vitess/go/vt/proto/query"
	"github.com/dolthub/vitess/go/vt/sqlparser"

	gmssql "github.com/AnqorDX/mysql-engine/sql"
)

// castError converts any GMS error into a *vitessmysql.SQLError so Vitess
// sends the correct MySQL error code to the client. Without this, GMS errors
// like ErrPrimaryKeyViolation arrive at the client as 1105 (HY000) "unknown
// error" instead of the correct code (e.g. 1062 for duplicate key).
// If err is already a *vitessmysql.SQLError it is returned unchanged.
func castError(err error) error {
	if err == nil {
		return nil
	}
	return gmssql.CastSQLError(err)
}

// bindingsToExprs converts Vitess bind variables (from a prepared statement
// execution) into the sqlparser.Expr map that engine.QueryWithBindings expects.
func bindingsToExprs(bindings map[string]*querypb.BindVariable) (map[string]sqlparser.Expr, error) {
	if len(bindings) == 0 {
		return nil, nil
	}
	res := make(map[string]sqlparser.Expr, len(bindings))
	for name, bv := range bindings {
		val, err := sqltypes.BindVariableToValue(bv)
		if err != nil {
			return nil, fmt.Errorf("handler: bind variable %q: %w", name, err)
		}
		expr, err := sqlparser.ExprFromValue(val)
		if err != nil {
			return nil, fmt.Errorf("handler: bind variable %q to expr: %w", name, err)
		}
		res[name] = expr
	}
	return res, nil
}
