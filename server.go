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
	"github.com/ClusterCockpit/cc-backend/schema"
	"github.com/ClusterCockpit/cc-backend/templates"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

var db *sqlx.DB
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

	// If overwriten, at least all the options in the defaults below must
	// be provided! Most options here can be overwritten by the user.
	UiDefaults map[string]interface{} `json:"ui-defaults"`

	// Where to store MachineState files
	MachineStateDir string `json:"machine-state-dir"`
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
	HttpsCertFile:         "",
	HttpsKeyFile:          "",
	UiDefaults: map[string]interface{}{
		"analysis_view_histogramMetrics":     []string{"flops_any", "mem_bw", "mem_used"},
		"analysis_view_scatterPlotMetrics":   [][]string{{"flops_any", "mem_bw"}, {"flops_any", "cpu_load"}, {"cpu_load", "mem_bw"}},
		"job_view_nodestats_selectedMetrics": []string{"flops_any", "mem_bw", "mem_used"},
		"job_view_polarPlotMetrics":          []string{"flops_any", "mem_bw", "mem_used", "net_bw", "file_bw"},
		"job_view_selectedMetrics":           []string{"flops_any", "mem_bw", "mem_used"},
		"plot_general_colorBackground":       true,
		"plot_general_colorscheme":           []string{"#00bfff", "#0000ff", "#ff00ff", "#ff0000", "#ff8000", "#ffff00", "#80ff00"},
		"plot_general_lineWidth":             1,
		"plot_list_jobsPerPage":              10,
		"plot_list_selectedMetrics":          []string{"cpu_load", "mem_used", "flops_any", "mem_bw", "clock"},
		"plot_view_plotsPerRow":              2,
		"plot_view_showPolarplot":            true,
		"plot_view_showRoofline":             true,
		"plot_view_showStatTable":            true,
		"system_view_selectedMetric":         "cpu_load",
	},
}

func setupHomeRoute(i InfoType, r *http.Request) InfoType {
	type cluster struct {
		Name        string
		RunningJobs int
		TotalJobs   int
	}

	state := schema.JobStateRunning
	runningJobs, err := jobRepo.CountJobs(r.Context(), &state)
	if err != nil {
		log.Errorf("failed to count jobs: %s", err.Error())
		runningJobs = map[string]int{}
	}
	totalJobs, err := jobRepo.CountJobs(r.Context(), nil)
	if err != nil {
		log.Errorf("failed to count jobs: %s", err.Error())
		totalJobs = map[string]int{}
	}

	clusters := make([]cluster, 0)
	for _, c := range config.Clusters {
		clusters = append(clusters, cluster{
			Name:        c.Name,
			RunningJobs: runningJobs[c.Name],
			TotalJobs:   totalJobs[c.Name],
		})
	}

	i["clusters"] = clusters
	return i
}

func setupJobRoute(i InfoType, r *http.Request) InfoType {
	i["id"] = mux.Vars(r)["id"]
	return i
}

func setupUserRoute(i InfoType, r *http.Request) InfoType {
	i["id"] = mux.Vars(r)["id"]
	i["username"] = mux.Vars(r)["id"]
	return i
}

func setupClusterRoute(i InfoType, r *http.Request) InfoType {
	vars := mux.Vars(r)
	i["id"] = vars["cluster"]
	i["cluster"] = vars["cluster"]
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if from != "" || to != "" {
		i["from"] = from
		i["to"] = to
	}
	return i
}

func setupNodeRoute(i InfoType, r *http.Request) InfoType {
	vars := mux.Vars(r)
	i["cluster"] = vars["cluster"]
	i["hostname"] = vars["hostname"]
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if from != "" || to != "" {
		i["from"] = from
		i["to"] = to
	}
	return i
}

func setupAnalysisRoute(i InfoType, r *http.Request) InfoType {
	i["cluster"] = mux.Vars(r)["cluster"]
	return i
}

func setupTaglistRoute(i InfoType, r *http.Request) InfoType {
	var username *string = nil
	if user := auth.GetUser(r.Context()); user != nil && !user.HasRole(auth.RoleAdmin) {
		username = &user.Username
	}

	tags, counts, err := jobRepo.GetTags(username)
	tagMap := make(map[string][]map[string]interface{})
	if err != nil {
		log.Errorf("GetTags failed: %s", err.Error())
		i["tagmap"] = tagMap
		return i
	}

	for _, tag := range tags {
		tagItem := map[string]interface{}{
			"id":    tag.ID,
			"name":  tag.Name,
			"count": counts[tag.Name],
		}
		tagMap[tag.Type] = append(tagMap[tag.Type], tagItem)
	}
	log.Infof("TAGS %+v", tags)
	i["tagmap"] = tagMap
	return i
}

var routes []Route = []Route{
	{"/", "home.tmpl", "ClusterCockpit", false, setupHomeRoute},
	{"/monitoring/jobs/", "monitoring/jobs.tmpl", "Jobs - ClusterCockpit", true, func(i InfoType, r *http.Request) InfoType { return i }},
	{"/monitoring/job/{id:[0-9]+}", "monitoring/job.tmpl", "Job <ID> - ClusterCockpit", false, setupJobRoute},
	{"/monitoring/users/", "monitoring/list.tmpl", "Users - ClusterCockpit", true, func(i InfoType, r *http.Request) InfoType { i["listType"] = "USER"; return i }},
	{"/monitoring/projects/", "monitoring/list.tmpl", "Projects - ClusterCockpit", true, func(i InfoType, r *http.Request) InfoType { i["listType"] = "PROJECT"; return i }},
	{"/monitoring/tags/", "monitoring/taglist.tmpl", "Tags - ClusterCockpit", false, setupTaglistRoute},
	{"/monitoring/user/{id}", "monitoring/user.tmpl", "User <ID> - ClusterCockpit", true, setupUserRoute},
	{"/monitoring/systems/{cluster}", "monitoring/systems.tmpl", "Cluster <ID> - ClusterCockpit", false, setupClusterRoute},
	{"/monitoring/node/{cluster}/{hostname}", "monitoring/node.tmpl", "Node <ID> - ClusterCockpit", false, setupNodeRoute},
	{"/monitoring/analysis/{cluster}", "monitoring/analysis.tmpl", "Analaysis - ClusterCockpit", true, setupAnalysisRoute},
}

func main() {
	var flagReinitDB, flagStopImmediately, flagSyncLDAP bool
	var flagConfigFile string
	var flagNewUser, flagDelUser, flagGenJWT string
	flag.BoolVar(&flagReinitDB, "init-db", false, "Go through job-archive and re-initialize `job`, `tag`, and `jobtag` tables")
	flag.BoolVar(&flagSyncLDAP, "sync-ldap", false, "Sync the `user` table with ldap")
	flag.BoolVar(&flagStopImmediately, "no-server", false, "Do not start a server, stop right after initialization and argument handling")
	flag.StringVar(&flagConfigFile, "config", "", "Location of the config file for this server (overwrites the defaults)")
	flag.StringVar(&flagNewUser, "add-user", "", "Add a new user. Argument format: `<username>:[admin,api,user]:<password>`")
	flag.StringVar(&flagDelUser, "del-user", "", "Remove user by username")
	flag.StringVar(&flagGenJWT, "jwt", "", "Generate and print a JWT for the user specified by the username")
	flag.Parse()

	if err := loadEnv("./.env"); err != nil && !os.IsNotExist(err) {
		log.Fatalf("parsing './.env' file failed: %s", err.Error())
	}

	if flagConfigFile != "" {
		data, err := os.ReadFile(flagConfigFile)
		if err != nil {
			log.Fatal(err)
		}
		if err := json.Unmarshal(data, &programConfig); err != nil {
			log.Fatal(err)
		}
	}

	if strings.HasPrefix(programConfig.DB, "env:") {
		envvar := strings.TrimPrefix(programConfig.DB, "env:")
		programConfig.DB = os.Getenv(envvar)
	}

	var err error
	if programConfig.DBDriver == "sqlite3" {
		db, err = sqlx.Open("sqlite3", fmt.Sprintf("%s?_foreign_keys=on", programConfig.DB))
		if err != nil {
			log.Fatal(err)
		}

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

	// Initialize sub-modules...

	authentication := &auth.Authentication{}
	if !programConfig.DisableAuthentication {
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
		if err := initDB(db, programConfig.JobArchive); err != nil {
			log.Fatal(err)
		}
	}

	if flagStopImmediately {
		return
	}

	// Build routes...

	resolver := &graph.Resolver{DB: db}
	if err := resolver.Init(); err != nil {
		log.Fatal(err)
	}
	graphQLEndpoint := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))
	if os.Getenv("DEBUG") != "1" {
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

	jobRepo = &repository.JobRepository{DB: db}

	graphQLPlayground := playground.Handler("GraphQL playground", "/query")
	api := &api.RestApi{
		JobRepository:   jobRepo,
		Resolver:        resolver,
		MachineStateDir: programConfig.MachineStateDir,
	}

	handleGetLogin := func(rw http.ResponseWriter, r *http.Request) {
		templates.Render(rw, r, "login.tmpl", &templates.Page{
			Title: "Login",
		})
	}

	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		templates.Render(rw, r, "404.tmpl", &templates.Page{
			Title: "Not found",
		})
	})

	r.Handle("/playground", graphQLPlayground)

	r.HandleFunc("/login", handleGetLogin).Methods(http.MethodGet)
	r.HandleFunc("/imprint", func(rw http.ResponseWriter, r *http.Request) {
		templates.Render(rw, r, "imprint.tmpl", &templates.Page{
			Title: "Imprint",
		})
	})
	r.HandleFunc("/privacy", func(rw http.ResponseWriter, r *http.Request) {
		templates.Render(rw, r, "privacy.tmpl", &templates.Page{
			Title: "Privacy",
		})
	})

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
	secured.Handle("/query", graphQLEndpoint)

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

	setupRoutes(secured, routes)
	api.MountRoutes(secured)

	r.PathPrefix("/").Handler(http.FileServer(http.Dir(programConfig.StaticFiles)))
	r.Use(handlers.CompressHandler)
	r.Use(handlers.CORS(
		handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization"}),
		handlers.AllowedMethods([]string{"GET", "POST", "HEAD", "OPTIONS"}),
		handlers.AllowedOrigins([]string{"*"})))
	handler := handlers.CustomLoggingHandler(log.InfoWriter, r, func(w io.Writer, params handlers.LogFormatterParams) {
		log.Finfof(w, "%s %s (status: %d, size: %d, duration: %dms)",
			params.Request.Method, params.URL.RequestURI(),
			params.StatusCode, params.Size,
			time.Since(params.TimeStamp).Milliseconds())
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

	if programConfig.HttpsCertFile != "" && programConfig.HttpsKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(programConfig.HttpsCertFile, programConfig.HttpsKeyFile)
		if err != nil {
			log.Fatal(err)
		}
		listener = tls.NewListener(listener, &tls.Config{
			Certificates: []tls.Certificate{cert},
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

	systemdNotifiy(true, "running")
	wg.Wait()
	log.Print("Gracefull shutdown completed!")
}
