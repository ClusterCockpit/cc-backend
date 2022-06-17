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
	"github.com/ClusterCockpit/cc-backend/api"
	"github.com/ClusterCockpit/cc-backend/auth"
	"github.com/ClusterCockpit/cc-backend/config"
	"github.com/ClusterCockpit/cc-backend/graph"
	"github.com/ClusterCockpit/cc-backend/graph/generated"
	"github.com/ClusterCockpit/cc-backend/log"
	"github.com/ClusterCockpit/cc-backend/metricdata"
	"github.com/ClusterCockpit/cc-backend/repository"
	"github.com/ClusterCockpit/cc-backend/templates"
	"github.com/google/gops/agent"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

var jobRepo *repository.JobRepository

// Format of the configurartion (file). See below for the defaults.
type ProgramConfig struct {
	// Address where the http (or https) server will listen on (for example: 'localhost:80').
	Addr string `json:"addr"`

	// Drop root permissions once .env was read and the port was taken.
	User  string `json:"user"`
	Group string `json:"group"`

	// Disable authentication (for everything: API, Web-UI, ...)
	DisableAuthentication bool `json:"disable-authentication"`

	// Folder where static assets can be found, will be served directly
	StaticFiles string `json:"static-files"`

	// 'sqlite3' or 'mysql' (mysql will work for mariadb as well)
	DBDriver string `json:"db-driver"`

	// For sqlite3 a filename, for mysql a DSN in this format: https://github.com/go-sql-driver/mysql#dsn-data-source-name (Without query parameters!).
	DB string `json:"db"`

	// Path to the job-archive
	JobArchive string `json:"job-archive"`

	// Keep all metric data in the metric data repositories,
	// do not write to the job-archive.
	DisableArchive bool `json:"disable-archive"`

	// For LDAP Authentication and user syncronisation.
	LdapConfig *auth.LdapConfig `json:"ldap"`

	// Specifies for how long a session or JWT shall be valid
	// as a string parsable by time.ParseDuration().
	// If 0 or empty, the session/token does not expire!
	SessionMaxAge string `json:"session-max-age"`
	JwtMaxAge     string `json:"jwt-max-age"`

	// If both those options are not empty, use HTTPS using those certificates.
	HttpsCertFile string `json:"https-cert-file"`
	HttpsKeyFile  string `json:"https-key-file"`

	// If not the empty string and `addr` does not end in ":80",
	// redirect every request incoming at port 80 to that url.
	RedirectHttpTo string `json:"redirect-http-to"`

	// If overwriten, at least all the options in the defaults below must
	// be provided! Most options here can be overwritten by the user.
	UiDefaults map[string]interface{} `json:"ui-defaults"`

	// Where to store MachineState files
	MachineStateDir string `json:"machine-state-dir"`

	// If not zero, automatically mark jobs as stopped running X seconds longer than theire walltime.
	StopJobsExceedingWalltime int `json:"stop-jobs-exceeding-walltime"`
}

var programConfig ProgramConfig = ProgramConfig{
	Addr:                  ":8080",
	DisableAuthentication: false,
	StaticFiles:           "./frontend/public",
	DBDriver:              "sqlite3",
	DB:                    "./var/job.db",
	JobArchive:            "./var/job-archive",
	DisableArchive:        false,
	LdapConfig:            nil,
	SessionMaxAge:         "168h",
	JwtMaxAge:             "0",
	UiDefaults: map[string]interface{}{
		"analysis_view_histogramMetrics":     []string{"flops_any", "mem_bw", "mem_used"},
		"analysis_view_scatterPlotMetrics":   [][]string{{"flops_any", "mem_bw"}, {"flops_any", "cpu_load"}, {"cpu_load", "mem_bw"}},
		"job_view_nodestats_selectedMetrics": []string{"flops_any", "mem_bw", "mem_used"},
		"job_view_polarPlotMetrics":          []string{"flops_any", "mem_bw", "mem_used", "net_bw", "file_bw"},
		"job_view_selectedMetrics":           []string{"flops_any", "mem_bw", "mem_used"},
		"plot_general_colorBackground":       true,
		"plot_general_colorscheme":           []string{"#00bfff", "#0000ff", "#ff00ff", "#ff0000", "#ff8000", "#ffff00", "#80ff00"},
		"plot_general_lineWidth":             3,
		"plot_list_hideShortRunningJobs":     5 * 60,
		"plot_list_jobsPerPage":              10,
		"plot_list_selectedMetrics":          []string{"cpu_load", "ipc", "mem_used", "flops_any", "mem_bw"},
		"plot_view_plotsPerRow":              3,
		"plot_view_showPolarplot":            true,
		"plot_view_showRoofline":             true,
		"plot_view_showStatTable":            true,
		"system_view_selectedMetric":         "cpu_load",
	},
	StopJobsExceedingWalltime: -1,
}

func main() {
	var flagReinitDB, flagStopImmediately, flagSyncLDAP, flagGops bool
	var flagConfigFile, flagImportJob string
	var flagNewUser, flagDelUser, flagGenJWT string
	flag.BoolVar(&flagReinitDB, "init-db", false, "Go through job-archive and re-initialize the 'job', 'tag', and 'jobtag' tables (all running jobs will be lost!)")
	flag.BoolVar(&flagSyncLDAP, "sync-ldap", false, "Sync the 'user' table with ldap")
	flag.BoolVar(&flagStopImmediately, "no-server", false, "Do not start a server, stop right after initialization and argument handling")
	flag.BoolVar(&flagGops, "gops", false, "Listen via github.com/google/gops/agent (for debugging)")
	flag.StringVar(&flagConfigFile, "config", "", "Overwrite the global config options by those specified in `config.json`")
	flag.StringVar(&flagNewUser, "add-user", "", "Add a new user. Argument format: `<username>:[admin,api,user]:<password>`")
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

	if err := loadEnv("./.env"); err != nil && !os.IsNotExist(err) {
		log.Fatalf("parsing './.env' file failed: %s", err.Error())
	}

	if flagConfigFile != "" {
		f, err := os.Open(flagConfigFile)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		dec := json.NewDecoder(f)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&programConfig); err != nil {
			log.Fatal(err)
		}
	}

	// As a special case for `db`, allow using an environment variable instead of the value
	// stored in the config. This can be done for people having security concerns about storing
	// the password for their mysql database in the config.json.
	if strings.HasPrefix(programConfig.DB, "env:") {
		envvar := strings.TrimPrefix(programConfig.DB, "env:")
		programConfig.DB = os.Getenv(envvar)
	}

	var err error
	var db *sqlx.DB
	if programConfig.DBDriver == "sqlite3" {
		db, err = sqlx.Open("sqlite3", fmt.Sprintf("%s?_foreign_keys=on", programConfig.DB))
		if err != nil {
			log.Fatal(err)
		}

		// sqlite does not multithread. Having more than one connection open would just mean
		// waiting for locks.
		db.SetMaxOpenConns(1)
	} else if programConfig.DBDriver == "mysql" {
		db, err = sqlx.Open("mysql", fmt.Sprintf("%s?multiStatements=true", programConfig.DB))
		if err != nil {
			log.Fatal(err)
		}

		db.SetConnMaxLifetime(time.Minute * 3)
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(10)
	} else {
		log.Fatalf("unsupported database driver: %s", programConfig.DBDriver)
	}

	// Initialize sub-modules and handle all command line flags.
	// The order here is important! For example, the metricdata package
	// depends on the config package.

	var authentication *auth.Authentication
	if !programConfig.DisableAuthentication {
		authentication = &auth.Authentication{}
		if d, err := time.ParseDuration(programConfig.SessionMaxAge); err != nil {
			authentication.SessionMaxAge = d
		}
		if d, err := time.ParseDuration(programConfig.JwtMaxAge); err != nil {
			authentication.JwtMaxAge = d
		}

		if err := authentication.Init(db, programConfig.LdapConfig); err != nil {
			log.Fatal(err)
		}

		if flagNewUser != "" {
			if err := authentication.AddUser(flagNewUser); err != nil {
				log.Fatal(err)
			}
		}
		if flagDelUser != "" {
			if err := authentication.DelUser(flagDelUser); err != nil {
				log.Fatal(err)
			}
		}

		if flagSyncLDAP {
			if err := authentication.SyncWithLDAP(true); err != nil {
				log.Fatal(err)
			}
		}

		if flagGenJWT != "" {
			user, err := authentication.FetchUser(flagGenJWT)
			if err != nil {
				log.Fatal(err)
			}

			if !user.HasRole(auth.RoleApi) {
				log.Warn("that user does not have the API role")
			}

			jwt, err := authentication.ProvideJWT(user)
			if err != nil {
				log.Fatal(err)
			}

			fmt.Printf("JWT for '%s': %s\n", user.Username, jwt)
		}
	} else if flagNewUser != "" || flagDelUser != "" {
		log.Fatal("arguments --add-user and --del-user can only be used if authentication is enabled")
	}

	if err := config.Init(db, !programConfig.DisableAuthentication, programConfig.UiDefaults, programConfig.JobArchive); err != nil {
		log.Fatal(err)
	}

	if err := metricdata.Init(programConfig.JobArchive, programConfig.DisableArchive); err != nil {
		log.Fatal(err)
	}

	if flagReinitDB {
		if err := repository.InitDB(db, programConfig.JobArchive); err != nil {
			log.Fatal(err)
		}
	}

	jobRepo = &repository.JobRepository{DB: db}
	if err := jobRepo.Init(); err != nil {
		log.Fatal(err)
	}

	if flagImportJob != "" {
		if err := jobRepo.HandleImportFlag(flagImportJob); err != nil {
			log.Fatalf("import failed: %s", err.Error())
		}
	}

	if flagStopImmediately {
		return
	}

	// Setup the http.Handler/Router used by the server

	resolver := &graph.Resolver{DB: db, Repo: jobRepo}
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
		MachineStateDir: programConfig.MachineStateDir,
		Authentication:  authentication,
	}

	r := mux.NewRouter()

	r.HandleFunc("/login", func(rw http.ResponseWriter, r *http.Request) {
		templates.Render(rw, r, "login.tmpl", &templates.Page{Title: "Login"})
	}).Methods(http.MethodGet)
	r.HandleFunc("/imprint", func(rw http.ResponseWriter, r *http.Request) {
		templates.Render(rw, r, "imprint.tmpl", &templates.Page{Title: "Imprint"})
	})
	r.HandleFunc("/privacy", func(rw http.ResponseWriter, r *http.Request) {
		templates.Render(rw, r, "privacy.tmpl", &templates.Page{Title: "Privacy"})
	})

	// Some routes, such as /login or /query, should only be accessible to a user that is logged in.
	// Those should be mounted to this subrouter. If authentication is enabled, a middleware will prevent
	// any unauthenticated accesses.
	secured := r.PathPrefix("/").Subrouter()
	if !programConfig.DisableAuthentication {
		r.Handle("/login", authentication.Login(
			// On success:
			http.RedirectHandler("/", http.StatusTemporaryRedirect),

			// On failure:
			func(rw http.ResponseWriter, r *http.Request, err error) {
				rw.WriteHeader(http.StatusUnauthorized)
				templates.Render(rw, r, "login.tmpl", &templates.Page{
					Title: "Login failed - ClusterCockpit",
					Error: err.Error(),
				})
			})).Methods(http.MethodPost)

		r.Handle("/logout", authentication.Logout(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
			templates.Render(rw, r, "login.tmpl", &templates.Page{
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
					templates.Render(rw, r, "login.tmpl", &templates.Page{
						Title: "Authentication failed - ClusterCockpit",
						Error: err.Error(),
					})
				})
		})
	}

	r.Handle("/playground", playground.Handler("GraphQL playground", "/query"))
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
	setupRoutes(secured, routes)
	api.MountRoutes(secured)

	r.PathPrefix("/").Handler(http.FileServer(http.Dir(programConfig.StaticFiles)))
	r.Use(handlers.CompressHandler)
	r.Use(handlers.RecoveryHandler(handlers.PrintRecoveryStack(true)))
	r.Use(handlers.CORS(
		handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization"}),
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
		Addr:         programConfig.Addr,
	}

	// Start http or https server

	listener, err := net.Listen("tcp", programConfig.Addr)
	if err != nil {
		log.Fatal(err)
	}

	if !strings.HasSuffix(programConfig.Addr, ":80") && programConfig.RedirectHttpTo != "" {
		go func() {
			http.ListenAndServe(":80", http.RedirectHandler(programConfig.RedirectHttpTo, http.StatusMovedPermanently))
		}()
	}

	if programConfig.HttpsCertFile != "" && programConfig.HttpsKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(programConfig.HttpsCertFile, programConfig.HttpsKeyFile)
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
		log.Printf("HTTPS server listening at %s...", programConfig.Addr)
	} else {
		log.Printf("HTTP server listening at %s...", programConfig.Addr)
	}

	// Because this program will want to bind to a privileged port (like 80), the listener must
	// be established first, then the user can be changed, and after that,
	// the actuall http server can be started.
	if err := dropPrivileges(); err != nil {
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
		systemdNotifiy(false, "shutting down")

		// First shut down the server gracefully (waiting for all ongoing requests)
		server.Shutdown(context.Background())

		// Then, wait for any async archivings still pending...
		api.OngoingArchivings.Wait()
	}()

	if programConfig.StopJobsExceedingWalltime > 0 {
		go func() {
			for range time.Tick(30 * time.Minute) {
				err := jobRepo.StopJobsExceedingWalltimeBy(programConfig.StopJobsExceedingWalltime)
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
	systemdNotifiy(true, "running")
	wg.Wait()
	log.Print("Gracefull shutdown completed!")
}
