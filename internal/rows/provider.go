// Package rows translates between GMS row representations and the map[string]any
// form used by the EventBridge interface.
package rows

import (
	"fmt"

	gmssql "github.com/dolthub/go-mysql-server/sql"
	"github.com/virtual-db/vdb-mysql-driver/internal/bridge"
)

// Provider translates GMS row representations into map[string]any form and
// invokes the appropriate EventBridge method at each row lifecycle moment.
//
// All implementations must be safe for concurrent use.
type Provider interface {
	// FetchRows streams rows from a RowIter source, converts them to
	// map[string]any records, and invokes the RowsFetched callback. The
	// iterator is consumed lazily — rows are converted one at a time rather
	// than materialising the full result set upfront.
	FetchRows(ctx *gmssql.Context, table string, iter gmssql.RowIter, schema gmssql.Schema) ([]map[string]any, error)

	// CommitRows is called after the delta overlay has been applied and the
	// final row set is ready to return to the client.
	CommitRows(ctx *gmssql.Context, table string, records []map[string]any) ([]map[string]any, error)

	// InsertRow is called when GMS executes an INSERT for a single row.
	InsertRow(ctx *gmssql.Context, table string, row gmssql.Row, schema gmssql.Schema) (map[string]any, error)

	// UpdateRow is called when GMS executes an UPDATE, providing both the old
	// and new row values.
	UpdateRow(ctx *gmssql.Context, table string, old, new gmssql.Row, schema gmssql.Schema) (map[string]any, error)

	// DeleteRow is called when GMS executes a DELETE for a single row.
	DeleteRow(ctx *gmssql.Context, table string, row gmssql.Row, schema gmssql.Schema) error

	// TruncateRows is called when TRUNCATE TABLE is executed. The implementation
	// must clear all delta state for the table (both inserts and tombstones) so
	// that subsequent reads return an empty result until new rows are inserted.
	// Returns the number of rows that were removed (may be 0 when the count is
	// not tracked).
	TruncateRows(ctx *gmssql.Context, table string) (int, error)
}

// GMSProvider is the concrete Provider implementation backed by an EventBridge.
// It holds no schema state — column names are derived from the GMS schema
// parameter passed to each method.
type GMSProvider struct {
	events bridge.EventBridge
}

// NewGMSProvider constructs a Provider backed by the given EventBridge.
// events must not be nil.
func NewGMSProvider(events bridge.EventBridge) *GMSProvider {
	return &GMSProvider{events: events}
}

// FetchRows streams rows from the source iterator through the delta overlay
// and returns the merged result as []map[string]any. The source iterator is
// never materialised — it is wrapped as a RecordIter and streamed through the
// overlay pipeline. The overlay result is consumed lazily and materialised
// only into the final []map[string]any required by the DriverAPI contract.
// The source iterator is always closed when FetchRows returns.
func (p *GMSProvider) FetchRows(
	ctx *gmssql.Context, table string, iter gmssql.RowIter, schema gmssql.Schema,
) ([]map[string]any, error) {
	defer iter.Close(ctx)

	// Wrap the GMS RowIter as a payloads.RecordIter for streaming.
	srcIter := &gmsRowIterAdapter{iter: iter, schema: schema}

	// Stream through the overlay pipeline.
	resultIter, err := p.events.RowsFetched(connIDFromCtx(ctx), table, srcIter)
	if err != nil {
		return nil, err
	}
	defer resultIter.Close()

	// Consume the result iterator and materialise into []map[string]any.
	var records []map[string]any
	for {
		rec, err := resultIter.Next()
		if err != nil {
			break // EOF or error — both stop iteration
		}
		if rec == nil {
			break
		}
		records = append(records, rec)
	}
	return p.events.RowsReady(connIDFromCtx(ctx), table, records)
}

// CommitRows delegates to events.RowsReady with the final record set.
func (p *GMSProvider) CommitRows(
	ctx *gmssql.Context, table string, records []map[string]any,
) ([]map[string]any, error) {
	return p.events.RowsReady(connIDFromCtx(ctx), table, records)
}

// InsertRow converts the row to a record and delegates to events.RowInserted.
func (p *GMSProvider) InsertRow(
	ctx *gmssql.Context, table string, row gmssql.Row, schema gmssql.Schema,
) (map[string]any, error) {
	record := RowToMap(row, SchemaColumns(schema))
	return p.events.RowInserted(connIDFromCtx(ctx), table, record)
}

// UpdateRow converts old and new rows to records and delegates to events.RowUpdated.
func (p *GMSProvider) UpdateRow(
	ctx *gmssql.Context, table string, old, new gmssql.Row, schema gmssql.Schema,
) (map[string]any, error) {
	cols := SchemaColumns(schema)
	oldRec := RowToMap(old, cols)
	newRec := RowToMap(new, cols)
	return p.events.RowUpdated(connIDFromCtx(ctx), table, oldRec, newRec)
}

// DeleteRow converts the row to a record and delegates to events.RowDeleted.
func (p *GMSProvider) DeleteRow(
	ctx *gmssql.Context, table string, row gmssql.Row, schema gmssql.Schema,
) error {
	record := RowToMap(row, SchemaColumns(schema))
	return p.events.RowDeleted(connIDFromCtx(ctx), table, record)
}

// TruncateRows delegates to events.TableTruncated to clear delta state.
func (p *GMSProvider) TruncateRows(ctx *gmssql.Context, table string) (int, error) {
	if err := p.events.TableTruncated(connIDFromCtx(ctx), table); err != nil {
		return 0, err
	}
	return 0, nil
}

// ---------------------------------------------------------------------------
// Unexported helpers
// ---------------------------------------------------------------------------

// connIDFromCtx extracts the connection ID from a GMS sql.Context.
// Returns 0 if the context or session is nil.
func connIDFromCtx(ctx *gmssql.Context) uint32 {
	if ctx == nil || ctx.Session == nil {
		return 0
	}
	return ctx.Session.ID()
}

// gmsRowIterAdapter wraps a GMS gmssql.RowIter as a payloads.RecordIter,
// converting rows to map[string]any on the fly. This bridges the GMS
// streaming world with the vdb-core RecordIter interface.
type gmsRowIterAdapter struct {
	iter   gmssql.RowIter
	schema gmssql.Schema
	cols   []string
	closed bool
}

func (a *gmsRowIterAdapter) Next() (map[string]any, error) {
	if a.closed {
		return nil, fmt.Errorf("EOF")
	}
	// Lazy-init column names from the schema.
	if a.cols == nil {
		a.cols = SchemaColumns(a.schema)
	}
	row, err := a.iter.Next(nil) // context not used by sourceRowIter
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("EOF")
	}
	return RowToMap(row, a.cols), nil
}

func (a *gmsRowIterAdapter) Close() error {
	if a.closed {
		return nil
	}
	a.closed = true
	return a.iter.Close(nil)
}
