// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repository

import (
	"database/sql"
	"embed"
	"fmt"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Version is the current database schema version required by this version of cc-backend.
// When the database schema changes, this version is incremented and a new migration file
// is added to internal/repository/migrations/sqlite3/.
//
// Version history:
//   - Version 10: Current version
//
// Migration files are embedded at build time from the migrations directory.
const Version uint = 10

//go:embed migrations/*
var migrationFiles embed.FS

// checkDBVersion verifies that the database schema version matches the expected version.
// This is called automatically during Connect() to ensure schema compatibility.
//
// Returns an error if:
//   - Database version is older than expected (needs migration)
//   - Database version is newer than expected (needs app upgrade)
//   - Database is in a dirty state (failed migration)
//
// A "dirty" database indicates a migration was started but not completed successfully.
// This requires manual intervention to fix the database and force the version.
func checkDBVersion(db *sql.DB) error {
	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		return err
	}
	d, err := iofs.New(migrationFiles, "migrations/sqlite3")
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance("iofs", d, "sqlite3", driver)
	if err != nil {
		return err
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

// getMigrateInstance creates a new migration instance for the given database file.
// This is used internally by MigrateDB, RevertDB, and ForceDB.
func getMigrateInstance(db string) (m *migrate.Migrate, err error) {
	d, err := iofs.New(migrationFiles, "migrations/sqlite3")
	if err != nil {
		return nil, err
	}

	m, err = migrate.NewWithSourceInstance("iofs", d, fmt.Sprintf("sqlite3://%s?_foreign_keys=on", db))
	if err != nil {
		return nil, err
	}

	return m, nil
}

// MigrateDB applies all pending database migrations to bring the schema up to date.
// This should be run with the -migrate-db flag before starting the application
// after upgrading to a new version that requires schema changes.
//
// Process:
//  1. Checks current database version
//  2. Applies all migrations from current version to target Version
//  3. Updates schema_migrations table to track applied migrations
//
// Important:
//   - Always backup your database before running migrations
//   - Migrations are irreversible without manual intervention
//   - If a migration fails, the database is marked "dirty" and requires manual fix
//
// Usage:
//
//	cc-backend -migrate-db
func MigrateDB(db string) error {
	m, err := getMigrateInstance(db)
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

// RevertDB rolls back the database schema to the previous version (Version - 1).
// This is primarily used for testing or emergency rollback scenarios.
//
// Warning:
//   - This may cause data loss if newer schema added columns/tables
//   - Always backup before reverting
//   - Not all migrations are safely reversible
//
// Usage:
//
//	cc-backend -revert-db
func RevertDB(db string) error {
	m, err := getMigrateInstance(db)
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

// ForceDB forces the database schema version to the current Version without running migrations.
// This is only used to recover from failed migrations that left the database in a "dirty" state.
//
// When to use:
//   - After manually fixing a failed migration
//   - When you've manually applied schema changes and need to update the version marker
//
// Warning:
//   - This does NOT apply any schema changes
//   - Only use after manually verifying the schema is correct
//   - Improper use can cause schema/version mismatch
//
// Usage:
//
//	cc-backend -force-db
func ForceDB(db string) error {
	m, err := getMigrateInstance(db)
	if err != nil {
		return err
	}

	if err := m.Force(int(Version)); err != nil {
		return err
	}

	m.Close()
	return nil
}
