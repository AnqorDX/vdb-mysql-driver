package driver

import (
	"crypto/tls"
	"database/sql"
	"fmt"
	"net"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"

	sqle "github.com/AnqorDX/mysql-engine"
	vitessmysql "github.com/dolthub/vitess/go/mysql"
	core "github.com/virtual-db/vdb-core"

	"github.com/virtual-db/vdb-mysql-driver/internal/auth"
	"github.com/virtual-db/vdb-mysql-driver/internal/catalog"
	"github.com/virtual-db/vdb-mysql-driver/internal/handler"
	"github.com/virtual-db/vdb-mysql-driver/internal/rows"
	"github.com/virtual-db/vdb-mysql-driver/internal/schema"
)

// Config holds the parameters for constructing a Driver.
type Config struct {
	Addr             string
	DBName           string
	SourceDSN        string
	AuthSourceAddr   string
	AuthProbeTimeout time.Duration
	ConnReadTimeout  time.Duration
	ConnWriteTimeout time.Duration
	// TLSConfig is the server-side TLS configuration.
	// When non-nil, caching_sha2_password is advertised and TLS is required.
	// When nil, mysql_clear_password is advertised (local dev / no-cert mode)
	// and connections are accepted without TLS.
	TLSConfig *tls.Config
}

// Driver implements core.Server. It owns the Vitess mysql.Listener (Layer 1),
// the GMS sqle.Engine (Layer 2), and the Delta storage backend.
type Driver struct {
	cfg      Config
	bridge   *apiAdapter
	schema   schema.Provider
	rows     rows.Provider
	db       *sql.DB
	gms      *sqle.Engine
	listener *vitessmysql.Listener
	mu       sync.Mutex
}

var _ core.Server = (*Driver)(nil)

// NewDriver constructs a Driver. The listener is not started until Run is called.
func NewDriver(cfg Config, api core.DriverAPI) *Driver {
	if cfg.AuthSourceAddr == "" {
		panic("vdb-mysql-driver: Config.AuthSourceAddr must not be empty")
	}
	if cfg.AuthProbeTimeout == 0 {
		cfg.AuthProbeTimeout = 5 * time.Second
	}
	if cfg.ConnReadTimeout == 0 {
		cfg.ConnReadTimeout = time.Minute
	}
	if cfg.ConnWriteTimeout == 0 {
		cfg.ConnWriteTimeout = time.Minute
	}

	db := mustOpenDB(cfg.SourceDSN)
	adapt := &apiAdapter{api: api}
	rawSchema := schema.NewSQLProvider(db, cfg.DBName)
	wrappedSchema := schema.NewNotifyingProvider(rawSchema, adapt)
	rowProv := rows.NewGMSProvider(adapt)
	gmsEngine := sqle.NewDefault(catalog.NewDatabaseProvider(
		cfg.DBName, rowProv, wrappedSchema, db,
	))

	return &Driver{
		cfg:    cfg,
		bridge: adapt,
		schema: wrappedSchema,
		rows:   rowProv,
		db:     db,
		gms:    gmsEngine,
	}
}

// Run binds the TCP listener, wires the Vitess mysql.Listener with
// VDBAuthServer and the VDB handler, and begins accepting connections.
// Layer 1 (Vitess go/mysql) handles the wire protocol and auth.
// Layer 2 (sqle.Engine) is called directly from the handler's ComQuery.
func (d *Driver) Run() error {
	ln, err := net.Listen("tcp", d.cfg.Addr)
	if err != nil {
		return fmt.Errorf("driver: listen on %s: %w", d.cfg.Addr, err)
	}

	authServer := auth.New(auth.Config{
		SourceAddr:   d.cfg.AuthSourceAddr,
		ProbeTimeout: d.cfg.AuthProbeTimeout,
		TLSConfig:    d.cfg.TLSConfig,
	})

	h := handler.New(d.gms, d.cfg.DBName, d.bridge)

	vtListener, err := vitessmysql.NewFromListener(
		ln,
		authServer,
		h,
		d.cfg.ConnReadTimeout,
		d.cfg.ConnWriteTimeout,
	)
	if err != nil {
		return fmt.Errorf("driver: create listener: %w", err)
	}

	if d.cfg.TLSConfig != nil {
		vtListener.TLSConfig = d.cfg.TLSConfig
	} else {
		vtListener.AllowClearTextWithoutTLS.Set(true)
	}

	d.mu.Lock()
	d.listener = vtListener
	d.mu.Unlock()

	vtListener.Accept()
	return nil
}

// Stop closes the Vitess listener and the source DB pool.
func (d *Driver) Stop() error {
	d.mu.Lock()
	l := d.listener
	d.mu.Unlock()
	if l != nil {
		l.Close()
	}
	if d.db != nil {
		d.db.Close()
	}
	return nil
}

func mustOpenDB(dsn string) *sql.DB {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		panic(fmt.Sprintf("driver: failed to open source DB: %v", err))
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(30 * time.Second)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db
}
