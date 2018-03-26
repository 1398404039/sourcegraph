package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gchaincl/sqlhooks"
	"github.com/lib/pq"
	"github.com/mattes/migrate"
	"github.com/mattes/migrate/database/postgres"
	bindata "github.com/mattes/migrate/source/go-bindata"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	log15 "gopkg.in/inconshreveable/log15.v2"
	"sourcegraph.com/sourcegraph/sourcegraph/cmd/frontend/internal/db/migrations"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/env"
)

var (
	globalDB          *sql.DB
	defaultDataSource = env.Get("PGDATASOURCE", "", "Default dataSource to pass to Postgres. See https://godoc.org/github.com/lib/pq for more information.")
)

// ConnectToDB connects to the given DB and stores the handle globally.
//
// Note: github.com/lib/pq parses the environment as well. This function will
// also use the value of PGDATASOURCE if supplied and dataSource is the empty
// string.
func ConnectToDB(dataSource string) {
	if dataSource == "" {
		dataSource = defaultDataSource
	}

	// Force PostgreSQL session timezone to UTC.
	if v, ok := os.LookupEnv("PGTZ"); ok && v != "UTC" && v != "utc" {
		log15.Warn("Ignoring PGTZ environment variable; using PGTZ=UTC.", "ignoredPGTZ", v)
	}
	if err := os.Setenv("PGTZ", "UTC"); err != nil {
		log.Fatalln("Error setting PGTZ=UTC:", err)
	}

	var err error
	globalDB, err = openDBWithStartupWait(dataSource)
	if err != nil {
		log.Fatal("DB not available: " + err.Error())
	}
	registerPrometheusCollector(globalDB, "_app")
	configureConnectionPool(globalDB)

	if err := doMigrate(newMigrate(globalDB)); err != nil {
		log.Fatal("failed to migrate the DB. Please contact hi@sourcegraph.com for further assistance:", err.Error())
	}
}

var (
	startupTimeout = func() time.Duration {
		str := env.Get("DB_STARTUP_TIMEOUT", "10s", "keep trying for this long to connect to PostgreSQL database before failing")
		d, err := time.ParseDuration(str)
		if err != nil {
			log.Fatalln("DB_STARTUP_TIMEOUT:", err)
		}
		return d
	}()
)

func openDBWithStartupWait(dataSource string) (db *sql.DB, err error) {
	// Allow the DB to take up to 10s while it reports "pq: the database system is starting up".
	startupDeadline := time.Now().Add(startupTimeout)
	for {
		if time.Now().After(startupDeadline) {
			return nil, fmt.Errorf("database did not start up within %s (%v)", startupTimeout, err)
		}
		db, err = Open(dataSource)
		if err == nil {
			err = db.Ping()
		}
		if err != nil && isDatabaseLikelyStartingUp(err) {
			time.Sleep(startupTimeout / 10)
			continue
		}
		return db, err
	}
}

// isDatabaseLikelyStartingUp returns whether the err likely just means the PostgreSQL database is
// starting up, and it should not be treated as a fatal error during program initialization.
func isDatabaseLikelyStartingUp(err error) bool {
	if strings.Contains(err.Error(), "pq: the database system is starting up") {
		// Wait for DB to start up.
		return true
	}
	if e, ok := errors.Cause(err).(net.Error); ok && strings.Contains(e.Error(), "connection refused") {
		// Wait for DB to start listening.
		return true
	}
	return false
}

var registerOnce sync.Once

// Open creates a new DB handle with the given schema by connecting to
// the database identified by dataSource (e.g., "dbname=mypgdb" or
// blank to use the PG* env vars).
//
// Open assumes that the database already exists.
func Open(dataSource string) (*sql.DB, error) {
	registerOnce.Do(func() {
		sql.Register("postgres-proxy", sqlhooks.Wrap(&pq.Driver{}, &hook{}))
	})
	db, err := sql.Open("postgres-proxy", dataSource)
	if err != nil {
		return nil, errors.Wrap(err, "postgresql open")
	}
	return db, nil
}

// Ping attempts to contact the database and returns a non-nil error upon failure. It is intended to
// be used by health checks.
func Ping(ctx context.Context) error { return globalDB.PingContext(ctx) }

type hook struct{}

// Before implements sqlhooks.Hooks
func (h *hook) Before(ctx context.Context, query string, args ...interface{}) (context.Context, error) {
	parent := opentracing.SpanFromContext(ctx)
	if parent == nil {
		return ctx, nil
	}
	span := opentracing.StartSpan("sql",
		opentracing.ChildOf(parent.Context()),
		ext.SpanKindRPCClient)
	ext.DBStatement.Set(span, query)
	ext.DBType.Set(span, "sql")
	span.LogFields(
		otlog.Object("args", args),
	)

	return opentracing.ContextWithSpan(ctx, span), nil
}

// After implements sqlhooks.Hooks
func (h *hook) After(ctx context.Context, query string, args ...interface{}) (context.Context, error) {
	span := opentracing.SpanFromContext(ctx)
	if span != nil {
		span.Finish()
	}
	return ctx, nil
}

// After implements sqlhooks.OnErroer
func (h *hook) OnError(ctx context.Context, err error, query string, args ...interface{}) error {
	span := opentracing.SpanFromContext(ctx)
	if span != nil {
		ext.Error.Set(span, true)
		span.LogFields(otlog.Error(err))
		span.Finish()
	}
	return err
}

func registerPrometheusCollector(db *sql.DB, dbNameSuffix string) {
	c := prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: "src",
			Subsystem: "pgsql" + dbNameSuffix,
			Name:      "open_connections",
			Help:      "Number of open connections to pgsql DB, as reported by pgsql.DB.Stats()",
		},
		func() float64 {
			s := db.Stats()
			return float64(s.OpenConnections)
		},
	)
	prometheus.MustRegister(c)
}

// configureConnectionPool sets reasonable sizes on the built in DB queue. By
// default the connection pool is unbounded, which leads to the error `pq:
// sorry too many clients already`.
func configureConnectionPool(db *sql.DB) {
	var err error
	maxOpen := 30
	if e := os.Getenv("SRC_PGSQL_MAX_OPEN"); e != "" {
		maxOpen, err = strconv.Atoi(e)
		if err != nil {
			log.Fatalf("SRC_PGSQL_MAX_OPEN is not an int: %s", e)
		}
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxOpen)
}

func newMigrate(db *sql.DB) *migrate.Migrate {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Fatal(err)
	}

	s := bindata.Resource(migrations.AssetNames(), migrations.Asset)
	d, err := bindata.WithInstance(s)
	if err != nil {
		log.Fatal(err)
	}

	m, err := migrate.NewWithInstance("go-bindata", d, "postgres", driver)
	if err != nil {
		log.Fatal(err)
	}

	return m
}

func doMigrate(m *migrate.Migrate) error {
	err := m.Up()
	if err == nil || err == migrate.ErrNoChange {
		return nil
	}
	if os.IsNotExist(err) {
		// This should only happen if the DB is ahead of the migrations available
		version, dirty, verr := m.Version()
		if verr != nil {
			return verr
		}
		if dirty { // this shouldn't happen, but checking anyways
			return err
		}
		return errors.Errorf("Detected an old version of Sourcegraph. The database has migrated to version %v, which is ahead of this version.", version)
	}
	return err
}
