// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"log"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

var (
	dbConnOnce     sync.Once
	dbConnInstance *DBConnection
)

type DBConnection struct {
	DB     *sqlx.DB
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
			MaxOpenConnections:    5,
			MaxIdleConnections:    5,
			ConnectionMaxLifetime: time.Hour,
			ConnectionMaxIdleTime: time.Hour,
		}

		switch driver {
		case "sqlite3":
			// sql.Register("sqlite3WithHooks", sqlhooks.Wrap(&sqlite3.SQLiteDriver{}, &Hooks{}))

			// - Set WAL mode (not strictly necessary each time because it's persisted in the database, but good for first run)
			// - Set busy timeout, so concurrent writers wait on each other instead of erroring immediately
			// - Enable foreign key checks
			opts.URL += "?_journal=WAL&_timeout=5000&_fk=true"

			// dbHandle, err = sqlx.Open("sqlite3WithHooks", fmt.Sprintf("%s?_foreign_keys=on", db))
			dbHandle, err = sqlx.Open("sqlite3", opts.URL)
			if err != nil {
				log.Fatal(err)
			}
		case "mysql":
			opts.URL += "?multiStatements=true"
			dbHandle, err = sqlx.Open("mysql", opts.URL)
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

		dbConnInstance = &DBConnection{DB: dbHandle, Driver: driver}
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
