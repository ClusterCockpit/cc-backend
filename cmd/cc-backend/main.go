// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"

	"github.com/ClusterCockpit/cc-backend/internal/archiver"
	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/importer"
	"github.com/ClusterCockpit/cc-backend/internal/metricdata"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/internal/tagger"
	"github.com/ClusterCockpit/cc-backend/internal/taskManager"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	ccconf "github.com/ClusterCockpit/cc-lib/ccConfig"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/runtimeEnv"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/ClusterCockpit/cc-lib/util"
	"github.com/google/gops/agent"
	"github.com/joho/godotenv"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

const logoString = `
 _____ _           _             ____           _          _ _
/  ___| |_   _ ___| |_ ___ _ __ / ___|___   ___| | ___ __ (_) |_
| |   | | | | / __| __/ _ \ '__| |   / _ \ / __| |/ / '_ \| | __|
| |___| | |_| \__ \ ||  __/ |  | |__| (_) | (__|   <| |_) | | |_
\_____|_|\__,_|___/\__\___|_|   \____\___/ \___|_|\_\ .__/|_|\__|
                                                    |_|
`

var (
	date    string
	commit  string
	version string
)

func main() {
	cliInit()

	if flagVersion {
		fmt.Print(logoString)
		fmt.Printf("Version:\t%s\n", version)
		fmt.Printf("Git hash:\t%s\n", commit)
		fmt.Printf("Build time:\t%s\n", date)
		fmt.Printf("SQL db version:\t%d\n", repository.Version)
		fmt.Printf("Job archive version:\t%d\n", archive.Version)
		os.Exit(0)
	}

	cclog.Init(flagLogLevel, flagLogDateTime)

	// If init flag set, run tasks here before any file dependencies cause errors
	if flagInit {
		initEnv()
		cclog.Exit("Successfully setup environment!\n" +
			"Please review config.json and .env and adjust it to your needs.\n" +
			"Add your job-archive at ./var/job-archive.")
	}

	// See https://github.com/google/gops (Runtime overhead is almost zero)
	if flagGops {
		if err := agent.Listen(agent.Options{}); err != nil {
			cclog.Abortf("Could not start gops agent with 'gops/agent.Listen(agent.Options{})'. Application startup failed, exited.\nError: %s\n", err.Error())
		}
	}

	err := godotenv.Load()
	if err != nil {
		cclog.Abortf("Could not parse existing .env file at location './.env'. Application startup failed, exited.\nError: %s\n", err.Error())
	}

	// Initialize sub-modules and handle command line flags.
	// The order here is important!
	ccconf.Init(flagConfigFile)

	// Load and check main configuration
	if cfg := ccconf.GetPackageConfig("main"); cfg != nil {
		if clustercfg := ccconf.GetPackageConfig("clusters"); clustercfg != nil {
			config.Init(cfg, clustercfg)
		} else {
			cclog.Abort("Cluster configuration must be present")
		}
	} else {
		cclog.Abort("Main configuration must be present")
	}

	if flagMigrateDB {
		err := repository.MigrateDB(config.Keys.DBDriver, config.Keys.DB)
		if err != nil {
			cclog.Abortf("MigrateDB Failed: Could not migrate '%s' database at location '%s' to version %d.\nError: %s\n", config.Keys.DBDriver, config.Keys.DB, repository.Version, err.Error())
		}
		cclog.Exitf("MigrateDB Success: Migrated '%s' database at location '%s' to version %d.\n", config.Keys.DBDriver, config.Keys.DB, repository.Version)
	}

	if flagRevertDB {
		err := repository.RevertDB(config.Keys.DBDriver, config.Keys.DB)
		if err != nil {
			cclog.Abortf("RevertDB Failed: Could not revert '%s' database at location '%s' to version %d.\nError: %s\n", config.Keys.DBDriver, config.Keys.DB, (repository.Version - 1), err.Error())
		}
		cclog.Exitf("RevertDB Success: Reverted '%s' database at location '%s' to version %d.\n", config.Keys.DBDriver, config.Keys.DB, (repository.Version - 1))
	}

	if flagForceDB {
		err := repository.ForceDB(config.Keys.DBDriver, config.Keys.DB)
		if err != nil {
			cclog.Abortf("ForceDB Failed: Could not force '%s' database at location '%s' to version %d.\nError: %s\n", config.Keys.DBDriver, config.Keys.DB, repository.Version, err.Error())
		}
		cclog.Exitf("ForceDB Success: Forced '%s' database at location '%s' to version %d.\n", config.Keys.DBDriver, config.Keys.DB, repository.Version)
	}

	repository.Connect(config.Keys.DBDriver, config.Keys.DB)

	if !config.Keys.DisableAuthentication {

		if cfg := ccconf.GetPackageConfig("auth"); cfg != nil {
			auth.Init(&cfg)
		} else {
			cclog.Warn("Authentication disabled due to missing configuration")
			auth.Init(nil)
		}

		if flagNewUser != "" {
			parts := strings.SplitN(flagNewUser, ":", 3)
			if len(parts) != 3 || len(parts[0]) == 0 {
				cclog.Abortf("Add User: Could not parse supplied argument format: No changes.\n"+
					"Want: <username>:[admin,support,manager,api,user]:<password>\n"+
					"Have: %s\n", flagNewUser)
			}

			ur := repository.GetUserRepository()
			if err := ur.AddUser(&schema.User{
				Username: parts[0], Projects: make([]string, 0), Password: parts[2], Roles: strings.Split(parts[1], ","),
			}); err != nil {
				cclog.Abortf("Add User: Could not add new user authentication for '%s' and roles '%s'.\nError: %s\n", parts[0], parts[1], err.Error())
			} else {
				cclog.Printf("Add User: Added new user '%s' with roles '%s'.\n", parts[0], parts[1])
			}
		}

		if flagDelUser != "" {
			ur := repository.GetUserRepository()
			if err := ur.DelUser(flagDelUser); err != nil {
				cclog.Abortf("Delete User: Could not delete user '%s' from DB.\nError: %s\n", flagDelUser, err.Error())
			} else {
				cclog.Printf("Delete User: Deleted user '%s' from DB.\n", flagDelUser)
			}
		}

		authHandle := auth.GetAuthInstance()

		if flagSyncLDAP {
			if authHandle.LdapAuth == nil {
				cclog.Abort("Sync LDAP: LDAP authentication is not configured, could not synchronize. No changes, exited.")
			}

			if err := authHandle.LdapAuth.Sync(); err != nil {
				cclog.Abortf("Sync LDAP: Could not synchronize, failed with error.\nError: %s\n", err.Error())
			}
			cclog.Print("Sync LDAP: LDAP synchronization successfull.")
		}

		if flagGenJWT != "" {
			ur := repository.GetUserRepository()
			user, err := ur.GetUser(flagGenJWT)
			if err != nil {
				cclog.Abortf("JWT: Could not get supplied user '%s' from DB. No changes, exited.\nError: %s\n", flagGenJWT, err.Error())
			}

			if !user.HasRole(schema.RoleApi) {
				cclog.Warnf("JWT: User '%s' does not have the role 'api'. REST API endpoints will return error!\n", user.Username)
			}

			jwt, err := authHandle.JwtAuth.ProvideJWT(user)
			if err != nil {
				cclog.Abortf("JWT: User '%s' found in DB, but failed to provide JWT.\nError: %s\n", user.Username, err.Error())
			}

			cclog.Printf("JWT: Successfully generated JWT for user '%s': %s\n", user.Username, jwt)
		}

	} else if flagNewUser != "" || flagDelUser != "" {
		cclog.Abort("Error: Arguments '--add-user' and '--del-user' can only be used if authentication is enabled. No changes, exited.")
	}

	if archiveCfg := ccconf.GetPackageConfig("archive"); archiveCfg != nil {
		err = archive.Init(archiveCfg, config.Keys.DisableArchive)
	} else {
		err = archive.Init(json.RawMessage(`{\"kind\":\"file\",\"path\":\"./var/job-archive\"}`), config.Keys.DisableArchive)
	}
	if err != nil {
		cclog.Abortf("Init: Failed to initialize archive.\nError: %s\n", err.Error())
	}

	if err := metricdata.Init(); err != nil {
		cclog.Abortf("Init: Failed to initialize metricdata repository.\nError %s\n", err.Error())
	}

	if flagReinitDB {
		if err := importer.InitDB(); err != nil {
			cclog.Abortf("Init DB: Failed to re-initialize repository DB.\nError: %s\n", err.Error())
		} else {
			cclog.Print("Init DB: Sucessfully re-initialized repository DB.")
		}
	}

	if flagImportJob != "" {
		if err := importer.HandleImportFlag(flagImportJob); err != nil {
			cclog.Abortf("Import Job: Job import failed.\nError: %s\n", err.Error())
		} else {
			cclog.Printf("Import Job: Imported Job '%s' into DB.\n", flagImportJob)
		}
	}

	if config.Keys.EnableJobTaggers {
		tagger.Init()
	}

	if flagApplyTags {
		if err := tagger.RunTaggers(); err != nil {
			cclog.Abortf("Running job taggers.\nError: %s\n", err.Error())
		}
	}

	if !flagServer {
		cclog.Exit("No errors, server flag not set. Exiting cc-backend.")
	}

	archiver.Start(repository.GetJobRepository())

	taskManager.Start(ccconf.GetPackageConfig("cron"),
		ccconf.GetPackageConfig("archive"))
	serverInit()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		serverStart()
	}()

	wg.Add(1)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		defer wg.Done()
		<-sigs
		runtimeEnv.SystemdNotifiy(false, "Shutting down ...")

		serverShutdown()

		util.FsWatcherShutdown()

		taskManager.Shutdown()
	}()

	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(25)
	}
	runtimeEnv.SystemdNotifiy(true, "running")
	wg.Wait()
	cclog.Print("Graceful shutdown completed!")
}
