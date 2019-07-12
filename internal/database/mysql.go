// Copyright 2019 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package database

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/moov-io/base/docker"

	"github.com/go-kit/kit/log"
	gomysql "github.com/go-sql-driver/mysql"
	"github.com/lopezator/migrator"
	"github.com/ory/dockertest"
)

var (
	mysqlMigrator = migrator.New(
		execsql(
			"create_customer_name_watches",
			`create table if not exists customer_name_watches(id varchar(40) primary key, name varchar(40), webhook varchar(512), auth_token varchar(128), created_at datetime, deleted_at datetime);`,
		),
		execsql(
			"create_customer_status",
			`create table if not exists customer_status(customer_id varchar(40), user_id varchar(40), note varchar(1024), status varchar(10), created_at datetime, deleted_at datetime);`,
		),
		execsql(
			"create_customer_watches",
			`create table if not exists customer_watches(id varchar(40) primary key, customer_id varchar(40), webhook varchar(512), auth_token varchar(128), created_at datetime, deleted_at datetime);`,
		),
		execsql(
			"create_company_name_watches",
			`create table if not exists company_name_watches(id varchar(40) primary key, name varchar(256), webhook varchar(512), auth_token varchar(128), created_at datetime, deleted_at datetime);`,
		),
		execsql(
			"create_company_status",
			`create table if not exists company_status(company_id varchar(40), user_id varchar(40), note varchar(1024), status varchar(10), created_at datetime, deleted_at datetime);`,
		),
		execsql(
			"create_company_watches",
			`create table if not exists company_watches(id varchar(40) primary key, company_id varchar(40), webhook varchar(512), auth_token varchar(128), created_at datetime, deleted_at datetime);`,
		),
		execsql(
			"create_ofac_download_stats",
			`create table if not exists ofac_download_stats(downloaded_at datetime, sdns integer, alt_names integer, addresses integer);`,
		),
		execsql(
			"create_webhook_stats",
			`create table if not exists webhook_stats(watch_id varchar(40), attempted_at datetime, status varchar(10));`,
		),
		execsql("add__denied_persons__to__ofac_download_stats", "alter table ofac_download_stats add column denied_persons integer not null default 0;"),
	)
)

type discardLogger struct{}

func (l discardLogger) Print(v ...interface{}) {}

func init() {
	gomysql.SetLogger(discardLogger{})
}

type mysql struct {
	dsn    string
	logger log.Logger
}

func (my *mysql) Connect() (*sql.DB, error) {
	db, err := sql.Open("mysql", my.dsn)
	if err != nil {
		return nil, err
	}

	// Check out DB is up and working
	if err := db.Ping(); err != nil {
		return nil, err
	}

	// Migrate our database
	if err := mysqlMigrator.Migrate(db); err != nil {
		return nil, err
	}

	return db, nil
}

func mysqlConnection(logger log.Logger, user, pass string, address string, database string) *mysql {
	dsn := fmt.Sprintf("%s:%s@%s/%s?%s", user, pass, address, database, "timeout=30s&tls=false&charset=utf8mb4&parseTime=true&sql_mode=ALLOW_INVALID_DATES")
	return &mysql{
		dsn:    dsn,
		logger: logger,
	}
}

// TestMySQLDB is a wrapper around sql.DB for MySQL connections designed for tests to provide
// a clean database for each testcase.  Callers should cleanup with Close() when finished.
type TestMySQLDB struct {
	DB *sql.DB

	container *dockertest.Resource
}

func (r *TestMySQLDB) Close() error {
	r.container.Close()
	return r.DB.Close()
}

// CreateTestMySQLDB returns a TestMySQLDB which can be used in tests
// as a clean mysql database. All migrations are ran on the db before.
//
// Callers should call close on the returned *TestMySQLDB.
func CreateTestMySQLDB(t *testing.T) *TestMySQLDB {
	if testing.Short() {
		t.Skip("-short flag enabled")
	}
	if !docker.Enabled() {
		t.Skip("Docker not enabled")
	}

	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Fatal(err)
	}
	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "mysql",
		Tag:        "8",
		Env: []string{
			"MYSQL_USER=moov",
			"MYSQL_PASSWORD=secret",
			"MYSQL_ROOT_PASSWORD=secret",
			"MYSQL_DATABASE=ofac",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = pool.Retry(func() error {
		db, err := sql.Open("mysql", fmt.Sprintf("moov:secret@tcp(localhost:%s)/ofac", resource.GetPort("3306/tcp")))
		if err != nil {
			return err
		}
		defer db.Close()
		return db.Ping()
	})
	if err != nil {
		resource.Close()
		t.Fatal(err)
	}

	logger := log.NewNopLogger()
	address := fmt.Sprintf("tcp(localhost:%s)", resource.GetPort("3306/tcp"))

	db, err := mysqlConnection(logger, "moov", "secret", address, "ofac").Connect()
	if err != nil {
		t.Fatal(err)
	}
	return &TestMySQLDB{db, resource}
}
