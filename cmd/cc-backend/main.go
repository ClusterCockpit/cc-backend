// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
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
	"github.com/ClusterCockpit/cc-backend/internal/taskManager"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/runtimeEnv"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/google/gops/agent"

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

	// Apply config flags for pkg/log
	log.Init(flagLogLevel, flagLogDateTime)

	// See https://github.com/google/gops (Runtime overhead is almost zero)
	if flagGops {
		if err := agent.Listen(agent.Options{}); err != nil {
			log.Fatalf("gops/agent.Listen failed: %s", err.Error())
		}
	}

	if err := runtimeEnv.LoadEnv("./.env"); err != nil && !os.IsNotExist(err) {
		log.Fatalf("parsing './.env' file failed: %s", err.Error())
	}

	// Initialize sub-modules and handle command line flags.
	// The order here is important!
	config.Init(flagConfigFile)

	// As a special case for `db`, allow using an environment variable instead of the value
	// stored in the config. This can be done for people having security concerns about storing
	// the password for their mysql database in config.json.
	if strings.HasPrefix(config.Keys.DB, "env:") {
		envvar := strings.TrimPrefix(config.Keys.DB, "env:")
		config.Keys.DB = os.Getenv(envvar)
	}

	if flagMigrateDB {
		err := repository.MigrateDB(config.Keys.DBDriver, config.Keys.DB)
		if err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}

	if flagRevertDB {
		err := repository.RevertDB(config.Keys.DBDriver, config.Keys.DB)
		if err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}

	if flagForceDB {
		err := repository.ForceDB(config.Keys.DBDriver, config.Keys.DB)
		if err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}

	repository.Connect(config.Keys.DBDriver, config.Keys.DB)

	if flagInit {
		initEnv()
		fmt.Print("Successfully setup environment!\n")
		fmt.Print("Please review config.json and .env and adjust it to your needs.\n")
		fmt.Print("Add your job-archive at ./var/job-archive.\n")
		os.Exit(0)
	}

	if !config.Keys.DisableAuthentication {

		auth.Init()

		if flagNewUser != "" {
			parts := strings.SplitN(flagNewUser, ":", 3)
			if len(parts) != 3 || len(parts[0]) == 0 {
				log.Fatal("invalid argument format for user creation")
			}

			ur := repository.GetUserRepository()
			if err := ur.AddUser(&schema.User{
				Username: parts[0], Projects: make([]string, 0), Password: parts[2], Roles: strings.Split(parts[1], ","),
			}); err != nil {
				log.Fatalf("adding '%s' user authentication failed: %v", parts[0], err)
			}
		}
		if flagDelUser != "" {
			ur := repository.GetUserRepository()
			if err := ur.DelUser(flagDelUser); err != nil {
				log.Fatalf("deleting user failed: %v", err)
			}
		}

		authHandle := auth.GetAuthInstance()

		if flagSyncLDAP {
			if authHandle.LdapAuth == nil {
				log.Fatal("cannot sync: LDAP authentication is not configured")
			}

			if err := authHandle.LdapAuth.Sync(); err != nil {
				log.Fatalf("LDAP sync failed: %v", err)
			}
			log.Info("LDAP sync successfull")
		}

		if flagGenJWT != "" {
			ur := repository.GetUserRepository()
			user, err := ur.GetUser(flagGenJWT)
			if err != nil {
				log.Fatalf("could not get user from JWT: %v", err)
			}

			if !user.HasRole(schema.RoleApi) {
				log.Warnf("user '%s' does not have the API role", user.Username)
			}

			jwt, err := authHandle.JwtAuth.ProvideJWT(user)
			if err != nil {
				log.Fatalf("failed to provide JWT to user '%s': %v", user.Username, err)
			}

			fmt.Printf("MAIN > JWT for '%s': %s\n", user.Username, jwt)
		}

	} else if flagNewUser != "" || flagDelUser != "" {
		log.Fatal("arguments --add-user and --del-user can only be used if authentication is enabled")
	}

	if err := archive.Init(config.Keys.Archive, config.Keys.DisableArchive); err != nil {
		log.Fatalf("failed to initialize archive: %s", err.Error())
	}

	if err := metricdata.Init(); err != nil {
		log.Fatalf("failed to initialize metricdata repository: %s", err.Error())
	}

	if flagReinitDB {
		if err := importer.InitDB(); err != nil {
			log.Fatalf("failed to re-initialize repository DB: %s", err.Error())
		}
	}

	if flagImportJob != "" {
		if err := importer.HandleImportFlag(flagImportJob); err != nil {
			log.Fatalf("job import failed: %s", err.Error())
		}
	}

	if !flagServer {
		return
	}

	archiver.Start(repository.GetJobRepository())
	taskManager.Start()
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

		taskManager.Shutdown()
	}()

	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(25)
	}
	runtimeEnv.SystemdNotifiy(true, "running")
	wg.Wait()
	log.Print("Graceful shutdown completed!")
}
