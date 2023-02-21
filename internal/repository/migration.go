// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"database/sql"
	"embed"
	"fmt"
	"os"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

const supportedVersion uint = 3

//go:embed migrations/*
var migrationFiles embed.FS

func checkDBVersion(backend string, db *sql.DB) {
	var m *migrate.Migrate

	if backend == "sqlite3" {

		driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
		if err != nil {
			log.Fatal(err)
		}
		d, err := iofs.New(migrationFiles, "migrations/sqlite3")
		if err != nil {
			log.Fatal(err)
		}

		m, err = migrate.NewWithInstance("iofs", d, "sqlite3", driver)
		if err != nil {
			log.Fatal(err)
		}
	} else if backend == "mysql" {
		driver, err := mysql.WithInstance(db, &mysql.Config{})
		if err != nil {
			log.Fatal(err)
		}
		d, err := iofs.New(migrationFiles, "migrations/mysql")
		if err != nil {
			log.Fatal(err)
		}

		m, err = migrate.NewWithInstance("iofs", d, "mysql", driver)
		if err != nil {
			log.Fatal(err)
		}
	}

	v, _, err := m.Version()
	if err != nil {
		if err == migrate.ErrNilVersion {
			log.Warn("Legacy database without version or missing database file!")
		} else {
			log.Fatal(err)
		}
	}

	if v < supportedVersion {
		log.Warnf("Unsupported database version %d, need %d.\nPlease backup your database file and run cc-backend --migrate-db", v, supportedVersion)
		os.Exit(0)
	}

	if v > supportedVersion {
		log.Warnf("Unsupported database version %d, need %d.\nPlease refer to documentation how to downgrade db with external migrate tool!", v, supportedVersion)
		os.Exit(0)
	}
}

func MigrateDB(backend string, db string) {
	var m *migrate.Migrate

	if backend == "sqlite3" {
		d, err := iofs.New(migrationFiles, "migrations/sqlite3")
		if err != nil {
			log.Fatal(err)
		}

		m, err = migrate.NewWithSourceInstance("iofs", d, fmt.Sprintf("sqlite3://%s?_foreign_keys=on", db))
		if err != nil {
			log.Fatal(err)
		}
	} else if backend == "mysql" {
		d, err := iofs.New(migrationFiles, "migrations/mysql")
		if err != nil {
			log.Fatal(err)
		}

		m, err = migrate.NewWithSourceInstance("iofs", d, fmt.Sprintf("mysql://%s?multiStatements=true", db))
		if err != nil {
			log.Fatal(err)
		}
	}

	if err := m.Up(); err != nil {
		log.Fatal(err)
	}

	m.Close()
}
