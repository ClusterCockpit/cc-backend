// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repository

import (
	"database/sql"
	"embed"
	"fmt"

	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

const Version uint = 10

//go:embed migrations/*
var migrationFiles embed.FS

func checkDBVersion(backend string, db *sql.DB) error {
	var m *migrate.Migrate

	switch backend {
	case "sqlite3":
		driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
		if err != nil {
			return err
		}
		d, err := iofs.New(migrationFiles, "migrations/sqlite3")
		if err != nil {
			return err
		}

		m, err = migrate.NewWithInstance("iofs", d, "sqlite3", driver)
		if err != nil {
			return err
		}
	case "mysql":
		driver, err := mysql.WithInstance(db, &mysql.Config{})
		if err != nil {
			return err
		}
		d, err := iofs.New(migrationFiles, "migrations/mysql")
		if err != nil {
			return err
		}

		m, err = migrate.NewWithInstance("iofs", d, "mysql", driver)
		if err != nil {
			return err
		}
	default:
		cclog.Abortf("Migration: Unsupported database backend '%s'.\n", backend)
	}

	v, dirty, err := m.Version()
	if err != nil {
		if err == migrate.ErrNilVersion {
			cclog.Warn("Legacy database without version or missing database file!")
		} else {
			return err
		}
	}

	if v < Version {
		return fmt.Errorf("unsupported database version %d, need %d.\nPlease backup your database file and run cc-backend -migrate-db", v, Version)
	} else if v > Version {
		return fmt.Errorf("unsupported database version %d, need %d.\nPlease refer to documentation how to downgrade db with external migrate tool", v, Version)
	}

	if dirty {
		return fmt.Errorf("last migration to version %d has failed, please fix the db manually and force version with -force-db flag", Version)
	}

	return nil
}

func getMigrateInstance(backend string, db string) (m *migrate.Migrate, err error) {
	switch backend {
	case "sqlite3":
		d, err := iofs.New(migrationFiles, "migrations/sqlite3")
		if err != nil {
			cclog.Fatal(err)
		}

		m, err = migrate.NewWithSourceInstance("iofs", d, fmt.Sprintf("sqlite3://%s?_foreign_keys=on", db))
		if err != nil {
			return m, err
		}
	case "mysql":
		d, err := iofs.New(migrationFiles, "migrations/mysql")
		if err != nil {
			return m, err
		}

		m, err = migrate.NewWithSourceInstance("iofs", d, fmt.Sprintf("mysql://%s?multiStatements=true", db))
		if err != nil {
			return m, err
		}
	default:
		cclog.Abortf("Migration: Unsupported database backend '%s'.\n", backend)
	}

	return m, nil
}

func MigrateDB(backend string, db string) error {
	m, err := getMigrateInstance(backend, db)
	if err != nil {
		return err
	}

	v, dirty, err := m.Version()
	if err != nil {
		if err == migrate.ErrNilVersion {
			cclog.Info("Legacy database without version or missing database file!")
		} else {
			return err
		}
	}

	if v < Version {
		cclog.Infof("unsupported database version %d, need %d.\nPlease backup your database file and run cc-backend -migrate-db", v, Version)
	}

	if dirty {
		return fmt.Errorf("last migration to version %d has failed, please fix the db manually and force version with -force-db flag", Version)
	}

	if err := m.Up(); err != nil {
		if err == migrate.ErrNoChange {
			cclog.Info("DB already up to date!")
		} else {
			return err
		}
	}

	m.Close()
	return nil
}

func RevertDB(backend string, db string) error {
	m, err := getMigrateInstance(backend, db)
	if err != nil {
		return err
	}

	if err := m.Migrate(Version - 1); err != nil {
		if err == migrate.ErrNoChange {
			cclog.Info("DB already up to date!")
		} else {
			return err
		}
	}

	m.Close()
	return nil
}

func ForceDB(backend string, db string) error {
	m, err := getMigrateInstance(backend, db)
	if err != nil {
		return err
	}

	if err := m.Force(int(Version)); err != nil {
		return err
	}

	m.Close()
	return nil
}
