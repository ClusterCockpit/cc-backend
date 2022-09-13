// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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
	"github.com/ClusterCockpit/cc-backend/internal/metricdata"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/internal/routerConfig"
	"github.com/ClusterCockpit/cc-backend/internal/runtimeEnv"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/web"
	"github.com/google/gops/agent"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
	_ "github.com/santhosh-tekuri/jsonschema/v5/httploader"
)

func main() {
	var flagReinitDB, flagServer, flagSyncLDAP, flagGops, flagDev bool
	var flagNewUser, flagDelUser, flagGenJWT, flagConfigFile, flagImportJob string
	flag.BoolVar(&flagReinitDB, "init-db", false, "Go through job-archive and re-initialize the 'job', 'tag', and 'jobtag' tables (all running jobs will be lost!)")
	flag.BoolVar(&flagSyncLDAP, "sync-ldap", false, "Sync the 'user' table with ldap")
	flag.BoolVar(&flagServer, "server", false, "Do not start a server, stop right after initialization and argument handling")
	flag.BoolVar(&flagGops, "gops", false, "Listen via github.com/google/gops/agent (for debugging)")
	flag.BoolVar(&flagDev, "dev", false, "Enable development components: GraphQL Playground and Swagger UI")
	flag.StringVar(&flagConfigFile, "config", "./config.json", "Overwrite the global config options by those specified in `config.json`")
	flag.StringVar(&flagNewUser, "add-user", "", "Add a new user. Argument format: `<username>:[admin,support,api,user]:<password>`")
	flag.StringVar(&flagDelUser, "del-user", "", "Remove user by `username`")
	flag.StringVar(&flagGenJWT, "jwt", "", "Generate and print a JWT for the user specified by its `username`")
	flag.StringVar(&flagImportJob, "import-job", "", "Import a job. Argument format: `<path-to-meta.json>:<path-to-data.json>,...`")
	flag.Parse()

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

	repository.Connect(config.Keys.DBDriver, config.Keys.DB)
	db := repository.GetConnection()

	var authentication *auth.Authentication
	if !config.Keys.DisableAuthentication {
		var err error
		if authentication, err = auth.Init(db.DB, map[string]interface{}{
			"ldap": config.Keys.LdapConfig,
			"jwt":  config.Keys.JwtConfig,
		}); err != nil {
			log.Fatal(err)
		}

		if d, err := time.ParseDuration(config.Keys.SessionMaxAge); err != nil {
			authentication.SessionMaxAge = d
		}

		if flagNewUser != "" {
			parts := strings.SplitN(flagNewUser, ":", 3)
			if len(parts) != 3 || len(parts[0]) == 0 {
				log.Fatal("invalid argument format for user creation")
			}

			if err := authentication.AddUser(&auth.User{
				Username: parts[0], Password: parts[2], Roles: strings.Split(parts[1], ","),
			}); err != nil {
				log.Fatal(err)
			}
		}
		if flagDelUser != "" {
			if err := authentication.DelUser(flagDelUser); err != nil {
				log.Fatal(err)
			}
		}

		if flagSyncLDAP {
			if authentication.LdapAuth == nil {
				log.Fatal("cannot sync: LDAP authentication is not configured")
			}

			if err := authentication.LdapAuth.Sync(); err != nil {
				log.Fatal(err)
			}
			log.Info("LDAP sync successfull")
		}

		if flagGenJWT != "" {
			user, err := authentication.GetUser(flagGenJWT)
			if err != nil {
				log.Fatal(err)
			}

			if !user.HasRole(auth.RoleApi) {
				log.Warn("that user does not have the API role")
			}

			jwt, err := authentication.JwtAuth.ProvideJWT(user)
			if err != nil {
				log.Fatal(err)
			}

			fmt.Printf("JWT for '%s': %s\n", user.Username, jwt)
		}
	} else if flagNewUser != "" || flagDelUser != "" {
		log.Fatal("arguments --add-user and --del-user can only be used if authentication is enabled")
	}

	if err := archive.Init(config.Keys.Archive); err != nil {
		log.Fatal(err)
	}

	if err := metricdata.Init(config.Keys.DisableArchive); err != nil {
		log.Fatal(err)
	}

	if flagReinitDB {
		if err := repository.InitDB(); err != nil {
			log.Fatal(err)
		}
	}

	if flagImportJob != "" {
		if err := repository.HandleImportFlag(flagImportJob); err != nil {
			log.Fatalf("import failed: %s", err.Error())
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
				return fmt.Errorf("panic: %s", e)
			case error:
				return fmt.Errorf("panic caused by: %w", e)
			}

			return errors.New("internal server error (panic)")
		})
	}

	api := &api.RestApi{
		JobRepository:   jobRepo,
		Resolver:        resolver,
		MachineStateDir: config.Keys.MachineStateDir,
		Authentication:  authentication,
	}

	r := mux.NewRouter()

	r.HandleFunc("/login", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "text/html; charset=utf-8")
		web.RenderTemplate(rw, r, "login.tmpl", &web.Page{Title: "Login"})
	}).Methods(http.MethodGet)
	r.HandleFunc("/imprint", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "text/html; charset=utf-8")
		web.RenderTemplate(rw, r, "imprint.tmpl", &web.Page{Title: "Imprint"})
	})
	r.HandleFunc("/privacy", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "text/html; charset=utf-8")
		web.RenderTemplate(rw, r, "privacy.tmpl", &web.Page{Title: "Privacy"})
	})

	// Some routes, such as /login or /query, should only be accessible to a user that is logged in.
	// Those should be mounted to this subrouter. If authentication is enabled, a middleware will prevent
	// any unauthenticated accesses.
	secured := r.PathPrefix("/").Subrouter()
	if !config.Keys.DisableAuthentication {
		r.Handle("/login", authentication.Login(
			// On success:
			http.RedirectHandler("/", http.StatusTemporaryRedirect),

			// On failure:
			func(rw http.ResponseWriter, r *http.Request, err error) {
				rw.Header().Add("Content-Type", "text/html; charset=utf-8")
				rw.WriteHeader(http.StatusUnauthorized)
				web.RenderTemplate(rw, r, "login.tmpl", &web.Page{
					Title: "Login failed - ClusterCockpit",
					Error: err.Error(),
				})
			})).Methods(http.MethodPost)

		r.Handle("/logout", authentication.Logout(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.Header().Add("Content-Type", "text/html; charset=utf-8")
			rw.WriteHeader(http.StatusOK)
			web.RenderTemplate(rw, r, "login.tmpl", &web.Page{
				Title: "Bye - ClusterCockpit",
				Info:  "Logout sucessful",
			})
		}))).Methods(http.MethodPost)

		secured.Use(func(next http.Handler) http.Handler {
			return authentication.Auth(
				// On success;
				next,

				// On failure:
				func(rw http.ResponseWriter, r *http.Request, err error) {
					rw.WriteHeader(http.StatusUnauthorized)
					web.RenderTemplate(rw, r, "login.tmpl", &web.Page{
						Title: "Authentication failed - ClusterCockpit",
						Error: err.Error(),
					})
				})
		})
	}

	if flagDev {
		r.Handle("/playground", playground.Handler("GraphQL playground", "/query"))
	}
	secured.Handle("/query", graphQLEndpoint)

	// Send a searchId and then reply with a redirect to a user or job.
	secured.HandleFunc("/search", func(rw http.ResponseWriter, r *http.Request) {
		if search := r.URL.Query().Get("searchId"); search != "" {
			job, username, err := api.JobRepository.FindJobOrUser(r.Context(), search)
			if err == repository.ErrNotFound {
				http.Redirect(rw, r, "/monitoring/jobs/?jobId="+url.QueryEscape(search), http.StatusTemporaryRedirect)
				return
			} else if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			if username != "" {
				http.Redirect(rw, r, "/monitoring/user/"+username, http.StatusTemporaryRedirect)
				return
			} else {
				http.Redirect(rw, r, fmt.Sprintf("/monitoring/job/%d", job), http.StatusTemporaryRedirect)
				return
			}
		} else {
			http.Error(rw, "'searchId' query parameter missing", http.StatusBadRequest)
		}
	})

	// Mount all /monitoring/... and /api/... routes.
	routerConfig.SetupRoutes(secured)
	api.MountRoutes(secured)

	if config.Keys.EmbedStaticFiles {
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
			log.Infof("%s %s (%d, %.02fkb, %dms)",
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
		log.Fatal(err)
	}

	if !strings.HasSuffix(config.Keys.Addr, ":80") && config.Keys.RedirectHttpTo != "" {
		go func() {
			http.ListenAndServe(":80", http.RedirectHandler(config.Keys.RedirectHttpTo, http.StatusMovedPermanently))
		}()
	}

	if config.Keys.HttpsCertFile != "" && config.Keys.HttpsKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(config.Keys.HttpsCertFile, config.Keys.HttpsKeyFile)
		if err != nil {
			log.Fatal(err)
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
		log.Printf("HTTPS server listening at %s...", config.Keys.Addr)
	} else {
		log.Printf("HTTP server listening at %s...", config.Keys.Addr)
	}

	// Because this program will want to bind to a privileged port (like 80), the listener must
	// be established first, then the user can be changed, and after that,
	// the actuall http server can be started.
	if err := runtimeEnv.DropPrivileges(config.Keys.Group, config.Keys.User); err != nil {
		log.Fatalf("error while changing user: %s", err.Error())
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	wg.Add(1)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		defer wg.Done()
		<-sigs
		runtimeEnv.SystemdNotifiy(false, "shutting down")

		// First shut down the server gracefully (waiting for all ongoing requests)
		server.Shutdown(context.Background())

		// Then, wait for any async archivings still pending...
		api.OngoingArchivings.Wait()
	}()

	if config.Keys.StopJobsExceedingWalltime > 0 {
		go func() {
			for range time.Tick(30 * time.Minute) {
				err := jobRepo.StopJobsExceedingWalltimeBy(config.Keys.StopJobsExceedingWalltime)
				if err != nil {
					log.Errorf("error while looking for jobs exceeding theire walltime: %s", err.Error())
				}
				runtime.GC()
			}
		}()
	}

	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(25)
	}
	runtimeEnv.SystemdNotifiy(true, "running")
	wg.Wait()
	log.Print("Gracefull shutdown completed!")
}
