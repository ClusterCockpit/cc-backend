// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package main provides the entry point for the ClusterCockpit backend server.
// It orchestrates initialization of all subsystems including configuration,
// database, authentication, and the HTTP server.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/archiver"
	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/importer"
	"github.com/ClusterCockpit/cc-backend/internal/memorystore"
	"github.com/ClusterCockpit/cc-backend/internal/metricdata"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/internal/tagger"
	"github.com/ClusterCockpit/cc-backend/internal/taskManager"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/web"
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

// Environment variable names
const (
	envGOGC = "GOGC"
)

// Default configurations
const (
	defaultArchiveConfig = `{"kind":"file","path":"./var/job-archive"}`
)

var (
	date    string
	commit  string
	version string
)

func printVersion() {
	fmt.Print(logoString)
	fmt.Printf("Version:\t%s\n", version)
	fmt.Printf("Git hash:\t%s\n", commit)
	fmt.Printf("Build time:\t%s\n", date)
	fmt.Printf("SQL db version:\t%d\n", repository.Version)
	fmt.Printf("Job archive version:\t%d\n", archive.Version)
}

func initGops() error {
	if !flagGops {
		return nil
	}

	if err := agent.Listen(agent.Options{}); err != nil {
		return fmt.Errorf("starting gops agent: %w", err)
	}
	return nil
}

func loadEnvironment() error {
	if err := godotenv.Load(); err != nil {
		return fmt.Errorf("loading .env file: %w", err)
	}
	return nil
}

func initConfiguration() error {
	ccconf.Init(flagConfigFile)

	cfg := ccconf.GetPackageConfig("main")
	if cfg == nil {
		return fmt.Errorf("main configuration must be present")
	}

	clustercfg := ccconf.GetPackageConfig("clusters")
	if clustercfg == nil {
		return fmt.Errorf("cluster configuration must be present")
	}

	config.Init(cfg, clustercfg)
	return nil
}

func initDatabase() error {
	repository.Connect(config.Keys.DBDriver, config.Keys.DB)
	return nil
}

func handleDatabaseCommands() error {
	if flagMigrateDB {
		err := repository.MigrateDB(config.Keys.DBDriver, config.Keys.DB)
		if err != nil {
			return fmt.Errorf("migrating database to version %d: %w", repository.Version, err)
		}
		cclog.Exitf("MigrateDB Success: Migrated '%s' database at location '%s' to version %d.\n",
			config.Keys.DBDriver, config.Keys.DB, repository.Version)
	}

	if flagRevertDB {
		err := repository.RevertDB(config.Keys.DBDriver, config.Keys.DB)
		if err != nil {
			return fmt.Errorf("reverting database to version %d: %w", repository.Version-1, err)
		}
		cclog.Exitf("RevertDB Success: Reverted '%s' database at location '%s' to version %d.\n",
			config.Keys.DBDriver, config.Keys.DB, repository.Version-1)
	}

	if flagForceDB {
		err := repository.ForceDB(config.Keys.DBDriver, config.Keys.DB)
		if err != nil {
			return fmt.Errorf("forcing database to version %d: %w", repository.Version, err)
		}
		cclog.Exitf("ForceDB Success: Forced '%s' database at location '%s' to version %d.\n",
			config.Keys.DBDriver, config.Keys.DB, repository.Version)
	}

	return nil
}

func handleUserCommands() error {
	if config.Keys.DisableAuthentication && (flagNewUser != "" || flagDelUser != "") {
		return fmt.Errorf("--add-user and --del-user can only be used if authentication is enabled")
	}

	if !config.Keys.DisableAuthentication {
		if cfg := ccconf.GetPackageConfig("auth"); cfg != nil {
			auth.Init(&cfg)
		} else {
			cclog.Warn("Authentication disabled due to missing configuration")
			auth.Init(nil)
		}

		// Check for default security keys
		checkDefaultSecurityKeys()

		if flagNewUser != "" {
			if err := addUser(flagNewUser); err != nil {
				return err
			}
		}

		if flagDelUser != "" {
			if err := delUser(flagDelUser); err != nil {
				return err
			}
		}

		authHandle := auth.GetAuthInstance()

		if flagSyncLDAP {
			if err := syncLDAP(authHandle); err != nil {
				return err
			}
		}

		if flagGenJWT != "" {
			if err := generateJWT(authHandle, flagGenJWT); err != nil {
				return err
			}
		}
	}

	return nil
}

// checkDefaultSecurityKeys warns if default JWT keys are detected
func checkDefaultSecurityKeys() {
	// Default JWT public key from init.go
	defaultJWTPublic := "kzfYrYy+TzpanWZHJ5qSdMj5uKUWgq74BWhQG6copP0="

	if os.Getenv("JWT_PUBLIC_KEY") == defaultJWTPublic {
		cclog.Warn("Using default JWT keys - not recommended for production environments")
	}
}

func addUser(userSpec string) error {
	parts := strings.SplitN(userSpec, ":", 3)
	if len(parts) != 3 || len(parts[0]) == 0 {
		return fmt.Errorf("invalid user format, want: <username>:[admin,support,manager,api,user]:<password>, have: %s", userSpec)
	}

	ur := repository.GetUserRepository()
	if err := ur.AddUser(&schema.User{
		Username: parts[0],
		Projects: make([]string, 0),
		Password: parts[2],
		Roles:    strings.Split(parts[1], ","),
	}); err != nil {
		return fmt.Errorf("adding user '%s' with roles '%s': %w", parts[0], parts[1], err)
	}

	cclog.Printf("Add User: Added new user '%s' with roles '%s'.\n", parts[0], parts[1])
	return nil
}

func delUser(username string) error {
	ur := repository.GetUserRepository()
	if err := ur.DelUser(username); err != nil {
		return fmt.Errorf("deleting user '%s': %w", username, err)
	}
	cclog.Printf("Delete User: Deleted user '%s' from DB.\n", username)
	return nil
}

func syncLDAP(authHandle *auth.Authentication) error {
	if authHandle.LdapAuth == nil {
		return fmt.Errorf("LDAP authentication is not configured")
	}

	if err := authHandle.LdapAuth.Sync(); err != nil {
		return fmt.Errorf("synchronizing LDAP: %w", err)
	}

	cclog.Print("Sync LDAP: LDAP synchronization successfull.")
	return nil
}

func generateJWT(authHandle *auth.Authentication, username string) error {
	ur := repository.GetUserRepository()
	user, err := ur.GetUser(username)
	if err != nil {
		return fmt.Errorf("getting user '%s': %w", username, err)
	}

	if !user.HasRole(schema.RoleApi) {
		cclog.Warnf("JWT: User '%s' does not have the role 'api'. REST API endpoints will return error!\n", user.Username)
	}

	jwt, err := authHandle.JwtAuth.ProvideJWT(user)
	if err != nil {
		return fmt.Errorf("generating JWT for user '%s': %w", user.Username, err)
	}

	cclog.Printf("JWT: Successfully generated JWT for user '%s': %s\n", user.Username, jwt)
	return nil
}

func initSubsystems() error {
	// Initialize job archive
	archiveCfg := ccconf.GetPackageConfig("archive")
	if archiveCfg == nil {
		archiveCfg = json.RawMessage(defaultArchiveConfig)
	}
	if err := archive.Init(archiveCfg, config.Keys.DisableArchive); err != nil {
		return fmt.Errorf("initializing archive: %w", err)
	}

	// Initialize metricdata
	if err := metricdata.Init(); err != nil {
		return fmt.Errorf("initializing metricdata repository: %w", err)
	}

	// Handle database re-initialization
	if flagReinitDB {
		if err := importer.InitDB(); err != nil {
			return fmt.Errorf("re-initializing repository DB: %w", err)
		}
		cclog.Print("Init DB: Successfully re-initialized repository DB.")
	}

	// Handle job import
	if flagImportJob != "" {
		if err := importer.HandleImportFlag(flagImportJob); err != nil {
			return fmt.Errorf("importing job: %w", err)
		}
		cclog.Printf("Import Job: Imported Job '%s' into DB.\n", flagImportJob)
	}

	// Initialize taggers
	if config.Keys.EnableJobTaggers {
		tagger.Init()
	}

	// Apply tags if requested
	if flagApplyTags {
		if err := tagger.RunTaggers(); err != nil {
			return fmt.Errorf("running job taggers: %w", err)
		}
	}

	return nil
}

func runServer(ctx context.Context) error {
	var wg sync.WaitGroup

	// Start metric store if enabled
	if memorystore.InternalCCMSFlag {
		mscfg := ccconf.GetPackageConfig("metric-store")
		if mscfg == nil {
			return fmt.Errorf("metric store configuration must be present")
		}
		memorystore.Init(mscfg, &wg)
	}

	// Start archiver and task manager
	archiver.Start(repository.GetJobRepository(), ctx)
	taskManager.Start(ccconf.GetPackageConfig("cron"), ccconf.GetPackageConfig("archive"))

	// Initialize web UI
	cfg := ccconf.GetPackageConfig("ui")
	web.Init(cfg)

	// Initialize HTTP server
	srv, err := NewServer(version, commit, date)
	if err != nil {
		return fmt.Errorf("creating server: %w", err)
	}

	// Channel to collect errors from server
	errChan := make(chan error, 1)

	// Start HTTP server
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.Start(ctx); err != nil {
			errChan <- err
		}
	}()

	// Handle shutdown signals
	wg.Add(1)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		defer wg.Done()
		select {
		case <-sigs:
			cclog.Info("Shutdown signal received")
		case <-ctx.Done():
		}

		runtimeEnv.SystemdNotifiy(false, "Shutting down ...")
		srv.Shutdown(ctx)
		util.FsWatcherShutdown()
		taskManager.Shutdown()
	}()

	// Set GC percent if not configured
	if os.Getenv(envGOGC) == "" {
		debug.SetGCPercent(25)
	}
	runtimeEnv.SystemdNotifiy(true, "running")

	// Wait for completion or error
	go func() {
		wg.Wait()
		close(errChan)
	}()

	// Check for server startup errors
	select {
	case err := <-errChan:
		if err != nil {
			return err
		}
	case <-time.After(100 * time.Millisecond):
		// Server started successfully, wait for completion
		if err := <-errChan; err != nil {
			return err
		}
	}

	cclog.Print("Graceful shutdown completed!")
	return nil
}

func run() error {
	cliInit()

	if flagVersion {
		printVersion()
		return nil
	}

	// Initialize logger
	cclog.Init(flagLogLevel, flagLogDateTime)

	// Handle init flag
	if flagInit {
		initEnv()
		cclog.Exit("Successfully setup environment!\n" +
			"Please review config.json and .env and adjust it to your needs.\n" +
			"Add your job-archive at ./var/job-archive.")
	}

	// Initialize gops agent
	if err := initGops(); err != nil {
		return err
	}

	// Initialize subsystems in dependency order:
	// 1. Load environment variables from .env file (contains sensitive configuration)
	// 2. Load configuration from config.json (may reference environment variables)
	// 3. Handle database migration commands if requested
	// 4. Initialize database connection (requires config for connection string)
	// 5. Handle user commands if requested (requires database and authentication config)
	// 6. Initialize subsystems like archive and metrics (require config and database)

	// Load environment and configuration
	if err := loadEnvironment(); err != nil {
		return err
	}

	if err := initConfiguration(); err != nil {
		return err
	}

	// Handle database migration (migrate, revert, force)
	if err := handleDatabaseCommands(); err != nil {
		return err
	}

	// Initialize database
	if err := initDatabase(); err != nil {
		return err
	}

	// Handle user commands (add, delete, sync, JWT)
	if err := handleUserCommands(); err != nil {
		return err
	}

	// Initialize subsystems (archive, metrics, taggers)
	if err := initSubsystems(); err != nil {
		return err
	}

	// Exit if start server is not requested
	if !flagServer {
		cclog.Exit("No errors, server flag not set. Exiting cc-backend.")
	}

	// Run server with context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return runServer(ctx)
}

func main() {
	if err := run(); err != nil {
		cclog.Error(err.Error())
		os.Exit(1)
	}
}
