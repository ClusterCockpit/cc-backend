// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import "flag"

var (
	flagReinitDB, flagInit, flagServer, flagSyncLDAP, flagGops, flagMigrateDB, flagRevertDB,
	flagForceDB, flagDev, flagVersion, flagLogDateTime, flagApplyTags bool
	flagNewUser, flagDelUser, flagGenJWT, flagConfigFile, flagImportJob, flagLogLevel string
)

func cliInit() {
	flag.BoolVar(&flagInit, "init", false, "Setup var directory, initialize sqlite database file, config.json and .env")
	flag.BoolVar(&flagReinitDB, "init-db", false, "Go through job-archive and re-initialize the 'job', 'tag', and 'jobtag' tables (all running jobs will be lost!)")
	flag.BoolVar(&flagSyncLDAP, "sync-ldap", false, "Sync the 'hpc_user' table with ldap")
	flag.BoolVar(&flagServer, "server", false, "Start a server, continues listening on port after initialization and argument handling")
	flag.BoolVar(&flagGops, "gops", false, "Listen via github.com/google/gops/agent (for debugging)")
	flag.BoolVar(&flagDev, "dev", false, "Enable development components: GraphQL Playground and Swagger UI")
	flag.BoolVar(&flagVersion, "version", false, "Show version information and exit")
	flag.BoolVar(&flagMigrateDB, "migrate-db", false, "Migrate database to supported version and exit")
	flag.BoolVar(&flagRevertDB, "revert-db", false, "Migrate database to previous version and exit")
	flag.BoolVar(&flagApplyTags, "apply-tags", false, "Run taggers on all completed jobs and exit")
	flag.BoolVar(&flagForceDB, "force-db", false, "Force database version, clear dirty flag and exit")
	flag.BoolVar(&flagLogDateTime, "logdate", false, "Set this flag to add date and time to log messages")
	flag.StringVar(&flagConfigFile, "config", "./config.json", "Specify alternative path to `config.json`")
	flag.StringVar(&flagNewUser, "add-user", "", "Add a new user. Argument format: <username>:[admin,support,manager,api,user]:<password>")
	flag.StringVar(&flagDelUser, "del-user", "", "Remove a existing user. Argument format: <username>")
	flag.StringVar(&flagGenJWT, "jwt", "", "Generate and print a JWT for the user specified by its `username`")
	flag.StringVar(&flagImportJob, "import-job", "", "Import a job. Argument format: `<path-to-meta.json>:<path-to-data.json>,...`")
	flag.StringVar(&flagLogLevel, "loglevel", "warn", "Sets the logging level: `[debug, info (default), warn, err, crit]`")
	flag.Parse()
}
