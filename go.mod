module github.com/virtual-db/vdb-mysql-driver

go 1.25.0

require (
	github.com/AnqorDX/mysql-engine v0.0.0
	// go-sqlmock provides a database/sql mock used in mysqlengine/schema_test.go
	// to unit-test the SchemaProvider without a real MySQL instance. DRV-001
	// mandates exactly three direct production dependencies; go-sqlmock is
	// test-only and does not appear in the production import graph. Extracting
	// it into a test sub-module would be the strict alternative, but the test
	// surface is small enough that a direct dependency is acceptable here.
	github.com/DATA-DOG/go-sqlmock v1.5.2

	// dolthub/vitess is a direct dependency of mysqlengine because GMS v0.20.x
	// exposes Vitess types (vitessmysql.Conn, sqltypes.Result,
	// sqlparser.Statement, querypb.Field) in the server.Interceptor interface
	// that mysqlengine.queryInterceptor must implement. DRV-001 and DRV-002
	// allow only GMS and stdlib in mysqlengine; this entry is permitted because
	// Dolthub publishes vitess as a first-class sibling of go-mysql-server —
	// it is an unavoidable part of the GMS v0.20.x public API surface.
	github.com/dolthub/vitess v0.0.0-20250512224608-8fb9c6ea092c
	github.com/go-sql-driver/mysql v1.9.3
	github.com/virtual-db/vdb-core v0.0.1-alpha.2
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/AnqorDX/dispatch v0.0.1-alpha-1 // indirect
	github.com/AnqorDX/pipeline v0.0.1-alpha-1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dolthub/flatbuffers/v23 v23.3.3-dh.2 // indirect
	github.com/dolthub/go-icu-regex v0.0.0-20250327004329-6799764f2dad // indirect
	github.com/dolthub/jsonpath v0.0.2-0.20240227200619-19675ab05c71 // indirect
	github.com/go-kit/kit v0.13.0 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/lestrrat-go/strftime v1.2.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/shopspring/decimal v1.3.1 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/tetratelabs/wazero v1.8.2 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	golang.org/x/sync v0.3.0 // indirect
	golang.org/x/sys v0.12.0 // indirect
	golang.org/x/text v0.6.0 // indirect
	golang.org/x/tools v0.13.0 // indirect
	google.golang.org/genproto v0.0.0-20230110181048-76db0878b65f // indirect
	google.golang.org/grpc v1.53.0 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/src-d/go-errors.v1 v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/virtual-db/vdb-core => ../vdb-core
