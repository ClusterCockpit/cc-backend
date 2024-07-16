// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"database/sql"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	sqrl "github.com/Masterminds/squirrel"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/mattn/go-sqlite3"
	"github.com/qustavo/sqlhooks/v2"
)

var (
	dbConnOnce     sync.Once
	dbConnInstance *DBConnection
)

type DBConnection struct {
	DB     *sqlx.DB
	SQ     sqrl.StatementBuilderType
	Driver string
}

type DatabaseOptions struct {
	URL                   string
	MaxOpenConnections    int
	MaxIdleConnections    int
	ConnectionMaxLifetime time.Duration
	ConnectionMaxIdleTime time.Duration
}

func Connect(driver string, db string) {
	var err error
	var dbHandle *sqlx.DB

	dbConnOnce.Do(func() {
		opts := DatabaseOptions{
			URL:                   db,
			MaxOpenConnections:    4,
			MaxIdleConnections:    4,
			ConnectionMaxLifetime: time.Hour,
			ConnectionMaxIdleTime: time.Hour,
		}

		sq := sqrl.StatementBuilderType{}

		switch driver {
		case "sqlite3":
			// - Set WAL mode (not strictly necessary each time because it's persisted in the database, but good for first run)
			// - Set busy timeout, so concurrent writers wait on each other instead of erroring immediately
			// - Enable foreign key checks
			opts.URL += "?_journal=WAL&_timeout=5000&_fk=true"

			if log.Loglevel() == "debug" {
				sql.Register("sqlite3WithHooks", sqlhooks.Wrap(&sqlite3.SQLiteDriver{}, &Hooks{}))
				dbHandle, err = sqlx.Open("sqlite3WithHooks", opts.URL)
			} else {
				dbHandle, err = sqlx.Open("sqlite3", opts.URL)
			}
			if err != nil {
				log.Fatal(err)
			}
		case "mysql":
			opts.URL += "?multiStatements=true"
			dbHandle, err = sqlx.Open("mysql", opts.URL)
			if err != nil {
				log.Fatalf("sqlx.Open() error: %v", err)
			}
		case "postgres":
			opts.URL += ""
			dbHandle, err = sqlx.Open("pgx", opts.URL)
			sq = sqrl.StatementBuilder.PlaceholderFormat(sqrl.Dollar)
			if err != nil {
				log.Fatalf("sqlx.Open() error: %v", err)
			}
		default:
			log.Fatalf("unsupported database driver: %s", driver)
		}

		dbHandle.SetMaxOpenConns(opts.MaxOpenConnections)
		dbHandle.SetMaxIdleConns(opts.MaxIdleConnections)
		dbHandle.SetConnMaxLifetime(opts.ConnectionMaxLifetime)
		dbHandle.SetConnMaxIdleTime(opts.ConnectionMaxIdleTime)

		dbConnInstance = &DBConnection{
			DB:     dbHandle,
			SQ:     sq,
			Driver: driver}
		err = checkDBVersion(driver, dbHandle.DB)
		if err != nil {
			log.Fatal(err)
		}
	})
}

func GetConnection() *DBConnection {
	if dbConnInstance == nil {
		log.Fatalf("Database connection not initialized!")
	}

	return dbConnInstance
}
