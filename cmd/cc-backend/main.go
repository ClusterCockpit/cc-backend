// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/ClusterCockpit/cc-backend/internal/api"
	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/graph"
	"github.com/ClusterCockpit/cc-backend/internal/graph/generated"
	"github.com/ClusterCockpit/cc-backend/internal/importer"
	"github.com/ClusterCockpit/cc-backend/internal/metricdata"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/internal/routerConfig"
	"github.com/ClusterCockpit/cc-backend/internal/runtimeEnv"
	"github.com/ClusterCockpit/cc-backend/internal/util"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/ClusterCockpit/cc-backend/web"
	"github.com/go-co-op/gocron"
	"github.com/google/gops/agent"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

const logoString = `
 ____ _           _             ____           _          _ _
/ ___| |_   _ ___| |_ ___ _ __ / ___|___   ___| | ___ __ (_) |_
| |   | | | | / __| __/ _ \ '__| |   / _ \ / __| |/ / '_ \| | __|
| |___| | |_| \__ \ ||  __/ |  | |__| (_) | (__|   <| |_) | | |_
\____|_|\__,_|___/\__\___|_|   \____\___/ \___|_|\_\ .__/|_|\__|
                                                    |_|
`

const envString = `
# Base64 encoded Ed25519 keys (DO NOT USE THESE TWO IN PRODUCTION!)
# You can generate your own keypair using the gen-keypair tool
JWT_PUBLIC_KEY="kzfYrYy+TzpanWZHJ5qSdMj5uKUWgq74BWhQG6copP0="
JWT_PRIVATE_KEY="dtPC/6dWJFKZK7KZ78CvWuynylOmjBFyMsUWArwmodOTN9itjL5POlqdZkcnmpJ0yPm4pRaCrvgFaFAbpyik/Q=="

# Some random bytes used as secret for cookie-based sessions (DO NOT USE THIS ONE IN PRODUCTION)
SESSION_KEY="67d829bf61dc5f87a73fd814e2c9f629"
`

const configString = `
{
    "addr": "127.0.0.1:8080",
    "archive": {
        "kind": "file",
        "path": "./var/job-archive"
    },
    "clusters": [
        {
            "name": "name",
            "metricDataRepository": {
                "kind": "cc-metric-store",
                "url": "http://localhost:8082",
                "token": ""
            },
            "filterRanges": {
                "numNodes": {
                    "from": 1,
                    "to": 64
                },
                "duration": {
                    "from": 0,
                    "to": 86400
                },
                "startTime": {
                    "from": "2023-01-01T00:00:00Z",
                    "to": null
                }
            }
        }
    ]
}
`

var (
	date    string
	commit  string
	version string
)

func initEnv() {
	if util.CheckFileExists("var") {
		fmt.Print("Directory ./var already exists. Exiting!\n")
		os.Exit(0)
	}

	if err := os.WriteFile("config.json", []byte(configString), 0666); err != nil {
		log.Fatalf("Writing config.json failed: %s", err.Error())
	}

	if err := os.WriteFile(".env", []byte(envString), 0666); err != nil {
		log.Fatalf("Writing .env failed: %s", err.Error())
	}

	if err := os.Mkdir("var", 0777); err != nil {
		log.Fatalf("Mkdir var failed: %s", err.Error())
	}

	err := repository.MigrateDB("sqlite3", "./var/job.db")
	if err != nil {
		log.Fatalf("Initialize job.db failed: %s", err.Error())
	}
}

func main() {
	var flagReinitDB, flagInit, flagServer, flagSyncLDAP, flagGops, flagMigrateDB, flagDev, flagVersion, flagLogDateTime bool
	var flagNewUser, flagDelUser, flagGenJWT, flagConfigFile, flagImportJob, flagLogLevel string
	flag.BoolVar(&flagInit, "init", false, "Setup var directory, initialize swlite database file, config.json and .env")
	flag.BoolVar(&flagReinitDB, "init-db", false, "Go through job-archive and re-initialize the 'job', 'tag', and 'jobtag' tables (all running jobs will be lost!)")
	flag.BoolVar(&flagSyncLDAP, "sync-ldap", false, "Sync the 'user' table with ldap")
	flag.BoolVar(&flagServer, "server", false, "Start a server, continues listening on port after initialization and argument handling")
	flag.BoolVar(&flagGops, "gops", false, "Listen via github.com/google/gops/agent (for debugging)")
	flag.BoolVar(&flagDev, "dev", false, "Enable development components: GraphQL Playground and Swagger UI")
	flag.BoolVar(&flagVersion, "version", false, "Show version information and exit")
	flag.BoolVar(&flagMigrateDB, "migrate-db", false, "Migrate database to supported version and exit")
	flag.BoolVar(&flagLogDateTime, "logdate", false, "Set this flag to add date and time to log messages")
	flag.StringVar(&flagConfigFile, "config", "./config.json", "Specify alternative path to `config.json`")
	flag.StringVar(&flagNewUser, "add-user", "", "Add a new user. Argument format: `<username>:[admin,support,manager,api,user]:<password>`")
	flag.StringVar(&flagDelUser, "del-user", "", "Remove user by `username`")
	flag.StringVar(&flagGenJWT, "jwt", "", "Generate and print a JWT for the user specified by its `username`")
	flag.StringVar(&flagImportJob, "import-job", "", "Import a job. Argument format: `<path-to-meta.json>:<path-to-data.json>,...`")
	flag.StringVar(&flagLogLevel, "loglevel", "warn", "Sets the logging level: `[debug,info,warn (default),err,fatal,crit]`")
	flag.Parse()

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

	if flagInit {
		initEnv()
		fmt.Print("Succesfully setup environment!\n")
		fmt.Print("Please review config.json and .env and adjust it to your needs.\n")
		fmt.Print("Add your job-archive at ./var/job-archive.\n")
		os.Exit(0)
	}

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

	repository.Connect(config.Keys.DBDriver, config.Keys.DB)
	db := repository.GetConnection()

	var authentication *auth.Authentication
	if !config.Keys.DisableAuthentication {
		var err error
		if authentication, err = auth.Init(); err != nil {
			log.Fatalf("auth initialization failed: %v", err)
		}

		if d, err := time.ParseDuration(config.Keys.SessionMaxAge); err != nil {
			authentication.SessionMaxAge = d
		}

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

		if flagSyncLDAP {
			if authentication.LdapAuth == nil {
				log.Fatal("cannot sync: LDAP authentication is not configured")
			}

			if err := authentication.LdapAuth.Sync(); err != nil {
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

			jwt, err := authentication.JwtAuth.ProvideJWT(user)
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

	if err := metricdata.Init(config.Keys.DisableArchive); err != nil {
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

	// Setup the http.Handler/Router used by the server
	jobRepo := repository.GetJobRepository()
	resolver := &graph.Resolver{DB: db.DB, Repo: jobRepo}
	graphQLEndpoint := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))
	if os.Getenv("DEBUG") != "1" {
		// Having this handler means that a error message is returned via GraphQL instead of the connection simply beeing closed.
		// The problem with this is that then, no more stacktrace is printed to stderr.
		graphQLEndpoint.SetRecoverFunc(func(ctx context.Context, err interface{}) error {
			switch e := err.(type) {
			case string:
				return fmt.Errorf("MAIN > Panic: %s", e)
			case error:
				return fmt.Errorf("MAIN > Panic caused by: %w", e)
			}

			return errors.New("MAIN > Internal server error (panic)")
		})
	}

	api := &api.RestApi{
		JobRepository:   jobRepo,
		Resolver:        resolver,
		MachineStateDir: config.Keys.MachineStateDir,
		Authentication:  authentication,
	}

	r := mux.NewRouter()
	buildInfo := web.Build{Version: version, Hash: commit, Buildtime: date}

	r.HandleFunc("/login", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "text/html; charset=utf-8")
		web.RenderTemplate(rw, "login.tmpl", &web.Page{Title: "Login", Build: buildInfo})
	}).Methods(http.MethodGet)
	r.HandleFunc("/imprint", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "text/html; charset=utf-8")
		web.RenderTemplate(rw, "imprint.tmpl", &web.Page{Title: "Imprint", Build: buildInfo})
	})
	r.HandleFunc("/privacy", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "text/html; charset=utf-8")
		web.RenderTemplate(rw, "privacy.tmpl", &web.Page{Title: "Privacy", Build: buildInfo})
	})

	secured := r.PathPrefix("/").Subrouter()

	if !config.Keys.DisableAuthentication {
		r.Handle("/login", authentication.Login(
			// On success:
			http.RedirectHandler("/", http.StatusTemporaryRedirect),

			// On failure:
			func(rw http.ResponseWriter, r *http.Request, err error) {
				rw.Header().Add("Content-Type", "text/html; charset=utf-8")
				rw.WriteHeader(http.StatusUnauthorized)
				web.RenderTemplate(rw, "login.tmpl", &web.Page{
					Title:   "Login failed - ClusterCockpit",
					MsgType: "alert-warning",
					Message: err.Error(),
					Build:   buildInfo,
				})
			})).Methods(http.MethodPost)

		r.Handle("/jwt-login", authentication.Login(
			// On success:
			http.RedirectHandler("/", http.StatusTemporaryRedirect),

			// On failure:
			func(rw http.ResponseWriter, r *http.Request, err error) {
				rw.Header().Add("Content-Type", "text/html; charset=utf-8")
				rw.WriteHeader(http.StatusUnauthorized)
				web.RenderTemplate(rw, "login.tmpl", &web.Page{
					Title:   "Login failed - ClusterCockpit",
					MsgType: "alert-warning",
					Message: err.Error(),
					Build:   buildInfo,
				})
			}))

		r.Handle("/logout", authentication.Logout(
			http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				rw.Header().Add("Content-Type", "text/html; charset=utf-8")
				rw.WriteHeader(http.StatusOK)
				web.RenderTemplate(rw, "login.tmpl", &web.Page{
					Title:   "Bye - ClusterCockpit",
					MsgType: "alert-info",
					Message: "Logout successful",
					Build:   buildInfo,
				})
			}))).Methods(http.MethodPost)

		secured.Use(func(next http.Handler) http.Handler {
			return authentication.Auth(
				// On success;
				next,

				// On failure:
				func(rw http.ResponseWriter, r *http.Request, err error) {
					rw.WriteHeader(http.StatusUnauthorized)
					web.RenderTemplate(rw, "login.tmpl", &web.Page{
						Title:   "Authentication failed - ClusterCockpit",
						MsgType: "alert-danger",
						Message: err.Error(),
						Build:   buildInfo,
					})
				})
		})
	}

	if flagDev {
		r.Handle("/playground", playground.Handler("GraphQL playground", "/query"))
		r.PathPrefix("/swagger/").Handler(httpSwagger.Handler(
			httpSwagger.URL("http://" + config.Keys.Addr + "/swagger/doc.json"))).Methods(http.MethodGet)
	}
	secured.Handle("/query", graphQLEndpoint)

	// Send a searchId and then reply with a redirect to a user, or directly send query to job table for jobid and project.
	secured.HandleFunc("/search", func(rw http.ResponseWriter, r *http.Request) {
		routerConfig.HandleSearchBar(rw, r, buildInfo)
	})

	// Mount all /monitoring/... and /api/... routes.
	routerConfig.SetupRoutes(secured, buildInfo)
	api.MountRoutes(secured)

	if config.Keys.EmbedStaticFiles {
		if i, err := os.Stat("./var/img"); err == nil {
			if i.IsDir() {
				log.Info("Use local directory for static images")
				r.PathPrefix("/img/").Handler(http.StripPrefix("/img/", http.FileServer(http.Dir("./var/img"))))
			}
		}
		r.PathPrefix("/").Handler(web.ServeFiles())
	} else {
		r.PathPrefix("/").Handler(http.FileServer(http.Dir(config.Keys.StaticFiles)))
	}

	r.Use(handlers.CompressHandler)
	r.Use(handlers.RecoveryHandler(handlers.PrintRecoveryStack(true)))
	r.Use(handlers.CORS(
		handlers.AllowCredentials(),
		handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization", "Origin"}),
		handlers.AllowedMethods([]string{"GET", "POST", "HEAD", "OPTIONS"}),
		handlers.AllowedOrigins([]string{"*"})))
	handler := handlers.CustomLoggingHandler(io.Discard, r, func(_ io.Writer, params handlers.LogFormatterParams) {
		if strings.HasPrefix(params.Request.RequestURI, "/api/") {
			log.Debugf("%s %s (%d, %.02fkb, %dms)",
				params.Request.Method, params.URL.RequestURI(),
				params.StatusCode, float32(params.Size)/1024,
				time.Since(params.TimeStamp).Milliseconds())
		} else {
			log.Debugf("%s %s (%d, %.02fkb, %dms)",
				params.Request.Method, params.URL.RequestURI(),
				params.StatusCode, float32(params.Size)/1024,
				time.Since(params.TimeStamp).Milliseconds())
		}
	})

	var wg sync.WaitGroup
	server := http.Server{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Handler:      handler,
		Addr:         config.Keys.Addr,
	}

	// Start http or https server
	listener, err := net.Listen("tcp", config.Keys.Addr)
	if err != nil {
		log.Fatalf("starting http listener failed: %v", err)
	}

	if !strings.HasSuffix(config.Keys.Addr, ":80") && config.Keys.RedirectHttpTo != "" {
		go func() {
			http.ListenAndServe(":80", http.RedirectHandler(config.Keys.RedirectHttpTo, http.StatusMovedPermanently))
		}()
	}

	if config.Keys.HttpsCertFile != "" && config.Keys.HttpsKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(config.Keys.HttpsCertFile, config.Keys.HttpsKeyFile)
		if err != nil {
			log.Fatalf("loading X509 keypair failed: %v", err)
		}
		listener = tls.NewListener(listener, &tls.Config{
			Certificates: []tls.Certificate{cert},
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			},
			MinVersion:               tls.VersionTLS12,
			PreferServerCipherSuites: true,
		})
		fmt.Printf("HTTPS server listening at %s...", config.Keys.Addr)
	} else {
		fmt.Printf("HTTP server listening at %s...", config.Keys.Addr)
	}

	// Because this program will want to bind to a privileged port (like 80), the listener must
	// be established first, then the user can be changed, and after that,
	// the actual http server can be started.
	if err = runtimeEnv.DropPrivileges(config.Keys.Group, config.Keys.User); err != nil {
		log.Fatalf("error while preparing server start: %s", err.Error())
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err = server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatalf("starting server failed: %v", err)
		}
	}()

	wg.Add(1)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		defer wg.Done()
		<-sigs
		runtimeEnv.SystemdNotifiy(false, "Shutting down ...")

		// First shut down the server gracefully (waiting for all ongoing requests)
		server.Shutdown(context.Background())

		// Then, wait for any async archivings still pending...
		api.JobRepository.WaitForArchiving()
	}()

	s := gocron.NewScheduler(time.Local)

	if config.Keys.StopJobsExceedingWalltime > 0 {
		log.Info("Register undead jobs service")

		s.Every(1).Day().At("3:00").Do(func() {
			err = jobRepo.StopJobsExceedingWalltimeBy(config.Keys.StopJobsExceedingWalltime)
			if err != nil {
				log.Warnf("Error while looking for jobs exceeding their walltime: %s", err.Error())
			}
			runtime.GC()
		})
	}

	var cfg struct {
		Compression int              `json:"compression"`
		Retention   schema.Retention `json:"retention"`
	}

	cfg.Retention.IncludeDB = true

	if err = json.Unmarshal(config.Keys.Archive, &cfg); err != nil {
		log.Warn("Error while unmarshaling raw config json")
	}

	switch cfg.Retention.Policy {
	case "delete":
		log.Info("Register retention delete service")

		s.Every(1).Day().At("4:00").Do(func() {
			startTime := time.Now().Unix() - int64(cfg.Retention.Age*24*3600)
			jobs, err := jobRepo.FindJobsBetween(0, startTime)
			if err != nil {
				log.Warnf("Error while looking for retention jobs: %s", err.Error())
			}
			archive.GetHandle().CleanUp(jobs)

			if cfg.Retention.IncludeDB {
				cnt, err := jobRepo.DeleteJobsBefore(startTime)
				if err != nil {
					log.Errorf("Error while deleting retention jobs from db: %s", err.Error())
				} else {
					log.Infof("Retention: Removed %d jobs from db", cnt)
				}
				if err = jobRepo.Optimize(); err != nil {
					log.Errorf("Error occured in db optimization: %s", err.Error())
				}
			}
		})
	case "move":
		log.Info("Register retention move service")

		s.Every(1).Day().At("4:00").Do(func() {
			startTime := time.Now().Unix() - int64(cfg.Retention.Age*24*3600)
			jobs, err := jobRepo.FindJobsBetween(0, startTime)
			if err != nil {
				log.Warnf("Error while looking for retention jobs: %s", err.Error())
			}
			archive.GetHandle().Move(jobs, cfg.Retention.Location)

			if cfg.Retention.IncludeDB {
				cnt, err := jobRepo.DeleteJobsBefore(startTime)
				if err != nil {
					log.Errorf("Error while deleting retention jobs from db: %v", err)
				} else {
					log.Infof("Retention: Removed %d jobs from db", cnt)
				}
				if err = jobRepo.Optimize(); err != nil {
					log.Errorf("Error occured in db optimization: %v", err)
				}
			}
		})
	}

	if cfg.Compression > 0 {
		log.Info("Register compression service")

		s.Every(1).Day().At("5:00").Do(func() {
			var jobs []*schema.Job

			ar := archive.GetHandle()
			startTime := time.Now().Unix() - int64(cfg.Compression*24*3600)
			lastTime := ar.CompressLast(startTime)
			if startTime == lastTime {
				log.Info("Compression Service - Complete archive run")
				jobs, err = jobRepo.FindJobsBetween(0, startTime)

			} else {
				jobs, err = jobRepo.FindJobsBetween(lastTime, startTime)
			}

			if err != nil {
				log.Warnf("Error while looking for compression jobs: %v", err)
			}
			ar.Compress(jobs)
		})
	}

	s.StartAsync()

	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(25)
	}
	runtimeEnv.SystemdNotifiy(true, "running")
	wg.Wait()
	log.Print("Gracefull shutdown completed!")
}
