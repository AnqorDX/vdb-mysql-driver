package intercept

import (
	"context"

	gmssql "github.com/AnqorDX/mysql-engine/sql"
)

// connIDFromCtx extracts the connection ID from a GMS sql.Context.
// Returns 0 if the context or session is nil.
func connIDFromCtx(ctx *gmssql.Context) uint32 {
	if ctx == nil || ctx.Session == nil {
		return 0
	}
	return ctx.Session.ID()
}

// databaseFromContext extracts the current database name from the sql.Session
// stored in the context. Returns an empty string if unavailable.
func databaseFromContext(ctx context.Context) string {
	sqlCtx, ok := ctx.(*gmssql.Context)
	if !ok || sqlCtx == nil {
		return ""
	}
	if sqlCtx.Session == nil {
		return ""
	}
	return sqlCtx.Session.GetCurrentDatabase()
}
