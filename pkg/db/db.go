package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mattes/migrate"
	"github.com/mattes/migrate/database/postgres"
	bindata "github.com/mattes/migrate/source/go-bindata"
	"github.com/prometheus/client_golang/prometheus"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/db/migrations"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/dbutil2"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/env"
)

var (
	globalDB          *sql.DB
	globalMigrate     *migrate.Migrate
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

	var err error
	globalDB, err = openDBWithStartupWait(dataSource)
	if err != nil {
		log.Fatal("DB not available: " + err.Error())
	}
	registerPrometheusCollector(globalDB, "_app")
	configureConnectionPool(globalDB)

	globalMigrate = newMigrate(globalDB)

	if err := globalMigrate.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatal("error migrating DB: " + err.Error())
	}
}

func openDBWithStartupWait(dataSource string) (db *sql.DB, err error) {
	// Allow the DB to take up to 10s while it reports "pq: the database system is starting up".
	const startupTimeout = 10 * time.Second
	startupDeadline := time.Now().Add(startupTimeout)
	for {
		if time.Now().After(startupDeadline) {
			return nil, fmt.Errorf("database did not start up within %s (%v)", startupTimeout, err)
		}
		db, err = dbutil2.Open(dataSource)
		if err != nil && strings.Contains(err.Error(), "pq: the database system is starting up") {
			time.Sleep(startupTimeout / 10)
			continue
		}
		return db, err
	}
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
