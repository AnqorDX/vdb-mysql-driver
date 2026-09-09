package catalog

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"

	gmssql "github.com/AnqorDX/mysql-engine/sql"
	rowspkg "github.com/virtual-db/vdb-mysql-driver/internal/rows"
	schemapkg "github.com/virtual-db/vdb-mysql-driver/internal/schema"
)

// DatabaseProvider implements gmssql.DatabaseProvider and serves as the root
// of the GMS engine's object graph. It holds the single logical database that
// the driver exposes to connecting clients.
type DatabaseProvider struct {
	dbName string
	rows   rowspkg.Provider
	schema schemapkg.Provider
	db     *sql.DB

	schemaMu     sync.RWMutex
	schemaDeltas map[string]*SchemaDelta // virtual table name → delta

	// aiMu guards aiTables. The auto-increment registry lives here so that the
	// counter for each table is shared across all Table instances (which are
	// created fresh on every GetTableInsensitive call). Without this, a second
	// INSERT into the same table would re-seed the counter from the source and
	// re-assign an id already consumed by an earlier delta insert.
	aiMu     sync.Mutex
	aiTables map[string]*autoIncrState
}

// getOrCreateAI returns the shared autoIncrState for the given table, creating
// it if this is the first time the table has been seen.
func (p *DatabaseProvider) getOrCreateAI(table string) *autoIncrState {
	p.aiMu.Lock()
	defer p.aiMu.Unlock()
	if p.aiTables == nil {
		p.aiTables = make(map[string]*autoIncrState)
	}
	ai, ok := p.aiTables[table]
	if !ok {
		ai = &autoIncrState{}
		p.aiTables[table] = ai
	}
	return ai
}

// deltaFor returns the SchemaDelta for table, creating it if absent.
func (p *DatabaseProvider) deltaFor(table string) *SchemaDelta {
	p.schemaMu.Lock()
	defer p.schemaMu.Unlock()
	if p.schemaDeltas == nil {
		p.schemaDeltas = make(map[string]*SchemaDelta)
	}
	d, ok := p.schemaDeltas[table]
	if !ok {
		d = newSchemaDelta()
		p.schemaDeltas[table] = d
	}
	return d
}

// peekDelta returns the SchemaDelta for table if one exists, or nil.
// Does not create a new delta.
func (p *DatabaseProvider) peekDelta(table string) *SchemaDelta {
	p.schemaMu.RLock()
	defer p.schemaMu.RUnlock()
	if p.schemaDeltas == nil {
		return nil
	}
	return p.schemaDeltas[table]
}

// renameDelta moves the delta (and AI state) from oldName to newName.
func (p *DatabaseProvider) renameDelta(oldName, newName string) {
	p.schemaMu.Lock()
	if p.schemaDeltas != nil {
		if d, ok := p.schemaDeltas[oldName]; ok {
			delete(p.schemaDeltas, oldName)
			p.schemaDeltas[newName] = d
		}
	}
	p.schemaMu.Unlock()

	p.aiMu.Lock()
	if p.aiTables != nil {
		if ai, ok := p.aiTables[oldName]; ok {
			delete(p.aiTables, oldName)
			p.aiTables[newName] = ai
		}
	}
	p.aiMu.Unlock()
}

var _ gmssql.DatabaseProvider = (*DatabaseProvider)(nil)

// NewDatabaseProvider constructs a DatabaseProvider for the given logical
// database name, backed by rowProv and schemaProv, with db as the source
// connection pool.
func NewDatabaseProvider(
	dbName string,
	rowProv rowspkg.Provider,
	schemaProv schemapkg.Provider,
	db *sql.DB,
) *DatabaseProvider {
	return &DatabaseProvider{
		dbName: dbName,
		rows:   rowProv,
		schema: schemaProv,
		db:     db,
	}
}

// Database satisfies gmssql.DatabaseProvider. Returns the single logical
// database when name matches (case-insensitive), otherwise an error.
func (p *DatabaseProvider) Database(_ *gmssql.Context, name string) (gmssql.Database, error) {
	if !strings.EqualFold(name, p.dbName) {
		return nil, fmt.Errorf("database not found: %s", name)
	}
	return &Database{
		name:     p.dbName,
		rows:     p.rows,
		schema:   p.schema,
		db:       p.db,
		provider: p,
	}, nil
}

// HasDatabase satisfies gmssql.DatabaseProvider.
func (p *DatabaseProvider) HasDatabase(_ *gmssql.Context, name string) bool {
	return strings.EqualFold(name, p.dbName)
}

// AllDatabases satisfies gmssql.DatabaseProvider.
func (p *DatabaseProvider) AllDatabases(_ *gmssql.Context) []gmssql.Database {
	return []gmssql.Database{
		&Database{
			name:     p.dbName,
			rows:     p.rows,
			schema:   p.schema,
			db:       p.db,
			provider: p,
		},
	}
}
