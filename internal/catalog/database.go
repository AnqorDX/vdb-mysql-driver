package catalog

import (
	"database/sql"
	"fmt"

	gmssql "github.com/AnqorDX/mysql-engine/sql"
	"github.com/virtual-db/vdb-mysql-driver/internal/rows"
	intschema "github.com/virtual-db/vdb-mysql-driver/internal/schema"
)

// Database implements gmssql.Database, serving a single logical schema backed
// by a schemaProvider and a rowProvider.
type Database struct {
	name     string
	rows     rows.Provider
	schema   intschema.Provider
	db       *sql.DB
	provider *DatabaseProvider // for shared auto-increment registry
}

var _ gmssql.Database = (*Database)(nil)

// Name returns the database name.
func (d *Database) Name() string { return d.name }

// GetTableInsensitive looks up the table schema via the schema provider. If
// found it constructs and returns a Table backed by the full column descriptors
// returned by GetSchema. Any schema error is treated as "table does not exist"
// from GMS's perspective (returns nil, false, nil).
func (d *Database) GetTableInsensitive(
	ctx *gmssql.Context,
	tblName string,
) (gmssql.Table, bool, error) {
	// Check the schema delta first. A dropped table returns not-found. A
	// created-only table (no source backing) is returned from the delta.
	if d.provider != nil {
		if delta := d.provider.peekDelta(tblName); delta != nil {
			if delta.Dropped {
				return nil, false, nil
			}
			if delta.Created {
				ai := d.provider.getOrCreateAI(tblName)
				return &Table{
					name:     tblName,
					dbName:   d.name,
					schema:   gmssql.Schema{},
					rows:     d.rows,
					db:       d.db,
					ai:       ai,
					delta:    delta,
					provider: d.provider,
				}, true, nil
			}
		}
	}

	if d.schema == nil {
		return nil, false, nil
	}
	cols, _, err := d.schema.GetSchema(tblName)
	if err != nil {
		return nil, false, nil
	}
	gmsSchema := ToGMSSchema(tblName, cols)

	var ai *autoIncrState
	var delta *SchemaDelta
	if d.provider != nil {
		ai = d.provider.getOrCreateAI(tblName)
		delta = d.provider.peekDelta(tblName)
	}
	return &Table{
		name:     tblName,
		dbName:   d.name,
		schema:   gmsSchema,
		rows:     d.rows,
		db:       d.db,
		ai:       ai,
		delta:    delta,
		provider: d.provider,
	}, true, nil
}

// GetTableNames returns nil. GMS uses this for SHOW TABLES; the source
// database's information_schema handles those queries directly.
func (d *Database) GetTableNames(_ *gmssql.Context) ([]string, error) {
	return nil, nil
}

var _ gmssql.TableCreator = (*Database)(nil)
var _ gmssql.TableDropper = (*Database)(nil)
var _ gmssql.TableRenamer = (*Database)(nil)

// CreateTable satisfies gmssql.TableCreator. Records the table in the schema
// delta with no source backing. All rows for this table live in the row delta.
func (d *Database) CreateTable(ctx *gmssql.Context, name string, schema gmssql.PrimaryKeySchema, collation gmssql.CollationID, comment string) error {
	if d.provider == nil {
		return fmt.Errorf("catalog: CreateTable: no provider")
	}
	delta := d.provider.deltaFor(name)
	delta.Created = true
	delta.Dropped = false
	for _, col := range schema.Schema {
		c := *col
		c.Source = name
		delta.AddedCols = append(delta.AddedCols, c)
	}
	return nil
}

// DropTable satisfies gmssql.TableDropper. Marks the table as dropped in the
// schema delta; all subsequent access returns not-found.
func (d *Database) DropTable(ctx *gmssql.Context, name string) error {
	if d.provider == nil {
		return fmt.Errorf("catalog: DropTable: no provider")
	}
	delta := d.provider.deltaFor(name)
	delta.Dropped = true
	return nil
}

// RenameTable satisfies gmssql.TableRenamer. Moves the delta and AI state from
// oldName to newName.
func (d *Database) RenameTable(ctx *gmssql.Context, oldName, newName string) error {
	if d.provider == nil {
		return fmt.Errorf("catalog: RenameTable: no provider")
	}
	delta := d.provider.deltaFor(oldName)
	if delta.SourceName == "" {
		delta.SourceName = oldName
	}
	d.provider.renameDelta(oldName, newName)
	return nil
}
