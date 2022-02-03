package main

import (
	"bufio"
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
	"os/exec"
	"os/signal"
	"os/user"
	"strconv"
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
	"github.com/ClusterCockpit/cc-backend/schema"
	"github.com/ClusterCockpit/cc-backend/templates"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

var db *sqlx.DB

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

	// Make the /api/jobs/stop_job endpoint do the heavy work in the background.
	AsyncArchiving bool `json:"async-archive"`

	// Keep all metric data in the metric data repositories,
	// do not write to the job-archive.
	DisableArchive bool `json:"disable-archive"`

	// For LDAP Authentication and user syncronisation.
	LdapConfig *auth.LdapConfig `json:"ldap"`

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
	AsyncArchiving:        true,
	DisableArchive:        false,
	LdapConfig: &auth.LdapConfig{
		Url:        "ldap://localhost",
		UserBase:   "ou=hpc,dc=rrze,dc=uni-erlangen,dc=de",
		SearchDN:   "cn=admin,dc=rrze,dc=uni-erlangen,dc=de",
		UserBind:   "uid={username},ou=hpc,dc=rrze,dc=uni-erlangen,dc=de",
		UserFilter: "(&(objectclass=posixAccount)(uid=*))",
	},
	HttpsCertFile: "",
	HttpsKeyFile:  "",
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

func setupJobRoute(i InfoType, r *http.Request) InfoType {
	i["id"] = mux.Vars(r)["id"]
	return i
}

func setupUserRoute(i InfoType, r *http.Request) InfoType {
	i["id"] = mux.Vars(r)["id"]
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

var routes []Route = []Route{
	{"/monitoring/jobs/", "monitoring/jobs.tmpl", "Jobs - ClusterCockpit", true, func(i InfoType, r *http.Request) InfoType { return i }},
	{"/monitoring/job/{id:[0-9]+}", "monitoring/job.tmpl", "Job <ID> - ClusterCockpit", false, setupJobRoute},
	{"/monitoring/users/", "monitoring/list.tmpl", "Users - ClusterCockpit", true, func(i InfoType, r *http.Request) InfoType { i["listType"] = "USER"; return i }},
	{"/monitoring/projects/", "monitoring/list.tmpl", "Projects - ClusterCockpit", true, func(i InfoType, r *http.Request) InfoType { i["listType"] = "PROJECT"; return i }},
	{"/monitoring/user/{id}", "monitoring/user.tmpl", "User <ID> - ClusterCockpit", true, setupUserRoute},
	{"/monitoring/systems/{cluster}", "monitoring/systems.tmpl", "Cluster <ID> - ClusterCockpit", false, setupClusterRoute},
	{"/monitoring/node/{cluster}/{hostname}", "monitoring/node.tmpl", "Node <ID> - ClusterCockpit", false, setupNodeRoute},
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

	if !programConfig.DisableAuthentication {
		if err := auth.Init(db, programConfig.LdapConfig); err != nil {
			log.Fatal(err)
		}

		if flagNewUser != "" {
			if err := auth.AddUserToDB(db, flagNewUser); err != nil {
				log.Fatal(err)
			}
		}
		if flagDelUser != "" {
			if err := auth.DelUserFromDB(db, flagDelUser); err != nil {
				log.Fatal(err)
			}
		}

		if flagSyncLDAP {
			auth.SyncWithLDAP(db)
		}

		if flagGenJWT != "" {
			user, err := auth.FetchUserFromDB(db, flagGenJWT)
			if err != nil {
				log.Fatal(err)
			}

			if !user.HasRole(auth.RoleApi) {
				log.Warn("that user does not have the API role")
			}

			jwt, err := auth.ProvideJWT(user)
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

	graphQLPlayground := playground.Handler("GraphQL playground", "/query")
	api := &api.RestApi{
		DB:              db,
		AsyncArchiving:  programConfig.AsyncArchiving,
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
	r.Handle("/login", auth.Login(db)).Methods(http.MethodPost)

	r.HandleFunc("/login", handleGetLogin).Methods(http.MethodGet)
	r.HandleFunc("/logout", auth.Logout).Methods(http.MethodPost)
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
		secured.Use(auth.Auth)
	}
	secured.Handle("/query", graphQLEndpoint)

	secured.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		conf, err := config.GetUIConfig(r)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		infos := map[string]interface{}{
			"clusters": config.Clusters,
		}

		if user := auth.GetUser(r.Context()); user != nil {
			infos["username"] = user.Username
			infos["admin"] = user.HasRole(auth.RoleAdmin)
		} else {
			infos["username"] = false
			infos["admin"] = false
		}

		templates.Render(rw, r, "home.tmpl", &templates.Page{
			Title:  "ClusterCockpit",
			Config: conf,
			Infos:  infos,
		})
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
		log.Finfof(w, "%s %s (Response: %d, Size: %d)", params.Request.Method, params.URL.RequestURI(), params.StatusCode, params.Size)
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

func buildFilterPresets(query url.Values) map[string]interface{} {
	filterPresets := map[string]interface{}{}

	if query.Get("cluster") != "" {
		filterPresets["cluster"] = query.Get("cluster")
	}
	if query.Get("partition") != "" {
		filterPresets["partition"] = query.Get("partition")
	}
	if query.Get("project") != "" {
		filterPresets["project"] = query.Get("project")
		filterPresets["projectMatch"] = "eq"
	}
	if query.Get("state") != "" && schema.JobState(query.Get("state")).Valid() {
		filterPresets["state"] = query.Get("state")
	}
	if rawtags, ok := query["tag"]; ok {
		tags := make([]int, len(rawtags))
		for i, tid := range rawtags {
			var err error
			tags[i], err = strconv.Atoi(tid)
			if err != nil {
				tags[i] = -1
			}
		}
		filterPresets["tags"] = tags
	}
	if query.Get("numNodes") != "" {
		parts := strings.Split(query.Get("numNodes"), "-")
		if len(parts) == 2 {
			a, e1 := strconv.Atoi(parts[0])
			b, e2 := strconv.Atoi(parts[1])
			if e1 == nil && e2 == nil {
				filterPresets["numNodes"] = map[string]int{"from": a, "to": b}
			}
		}
	}
	if query.Get("jobId") != "" {
		filterPresets["jobId"] = query.Get("jobId")
	}
	if query.Get("arrayJobId") != "" {
		if num, err := strconv.Atoi(query.Get("arrayJobId")); err == nil {
			filterPresets["arrayJobId"] = num
		}
	}

	return filterPresets
}

// func monitoringRoutes(router *mux.Router, resolver *graph.Resolver) {

// 	router.HandleFunc("/monitoring/jobs/", func(rw http.ResponseWriter, r *http.Request) {
// 		conf, infos, err := prepareRoute(r)
// 		if err != nil {
// 			http.Error(rw, err.Error(), http.StatusInternalServerError)
// 			return
// 		}

// 		templates.Render(rw, r, "monitoring/jobs.tmpl", &templates.Page{
// 			Title:         "Jobs - ClusterCockpit",
// 			Config:        conf,
// 			Infos:         infos,
// 			FilterPresets: buildFilterPresets(r.URL.Query()),
// 		})
// 	})

// 	router.HandleFunc("/monitoring/job/{id:[0-9]+}", func(rw http.ResponseWriter, r *http.Request) {
// 		conf, infos, err := prepareRoute(r)
// 		if err != nil {
// 			http.Error(rw, err.Error(), http.StatusInternalServerError)
// 			return
// 		}

// 		id := mux.Vars(r)["id"]
// 		job, err := resolver.Query().Job(r.Context(), id)
// 		if err != nil {
// 			http.Error(rw, err.Error(), http.StatusNotFound)
// 			return
// 		}

// 		infos["id"] = id
// 		infos["jobId"] = job.JobID
// 		infos["clusterId"] = job.Cluster

// 		templates.Render(rw, r, "monitoring/job.tmpl", &templates.Page{
// 			Title:  fmt.Sprintf("Job %d - ClusterCockpit", job.JobID),
// 			Config: conf,
// 			Infos:  infos,
// 		})
// 	})

// 	router.HandleFunc("/monitoring/users/", func(rw http.ResponseWriter, r *http.Request) {
// 		conf, infos, err := prepareRoute(r)
// 		if err != nil {
// 			http.Error(rw, err.Error(), http.StatusInternalServerError)
// 			return
// 		}

// 		infos["listType"] = "USER"

// 		templates.Render(rw, r, "monitoring/list.tmpl", &templates.Page{
// 			Title:         "Users - ClusterCockpit",
// 			Config:        conf,
// 			FilterPresets: buildFilterPresets(r.URL.Query()),
// 			Infos:         infos,
// 		})
// 	})

// 	router.HandleFunc("/monitoring/projects/", func(rw http.ResponseWriter, r *http.Request) {
// 		conf, infos, err := prepareRoute(r)
// 		if err != nil {
// 			http.Error(rw, err.Error(), http.StatusInternalServerError)
// 			return
// 		}

// 		infos["listType"] = "PROJECT"

// 		templates.Render(rw, r, "monitoring/list.tmpl", &templates.Page{
// 			Title:         "Projects - ClusterCockpit",
// 			Config:        conf,
// 			FilterPresets: buildFilterPresets(r.URL.Query()),
// 			Infos:         infos,
// 		})
// 	})

// 	router.HandleFunc("/monitoring/user/{id}", func(rw http.ResponseWriter, r *http.Request) {
// 		conf, infos, err := prepareRoute(r)
// 		if err != nil {
// 			http.Error(rw, err.Error(), http.StatusInternalServerError)
// 			return
// 		}

// 		id := mux.Vars(r)["id"]
// 		// TODO: One could check if the user exists, but that would be unhelpfull if authentication
// 		// is disabled or the user does not exist but has started jobs.
// 		infos["username"] = id

// 		templates.Render(rw, r, "monitoring/user.tmpl", &templates.Page{
// 			Title:         fmt.Sprintf("User %s - ClusterCockpit", id),
// 			Config:        conf,
// 			Infos:         infos,
// 			FilterPresets: buildFilterPresets(r.URL.Query()),
// 		})
// 	})

// 	router.HandleFunc("/monitoring/systems/", func(rw http.ResponseWriter, r *http.Request) {
// 		// TODO: List all clusters?
// 		http.Redirect(rw, r, "/", http.StatusTemporaryRedirect)
// 	})

// 	router.HandleFunc("/monitoring/systems/{cluster}", func(rw http.ResponseWriter, r *http.Request) {
// 		conf, infos, err := prepareRoute(r)
// 		if err != nil {
// 			http.Error(rw, err.Error(), http.StatusInternalServerError)
// 			return
// 		}

// 		vars := mux.Vars(r)
// 		infos["cluster"] = vars["cluster"]
// 		from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
// 		if from != "" || to != "" {
// 			infos["from"] = from
// 			infos["to"] = to
// 		}

// 		templates.Render(rw, r, "monitoring/systems.tmpl", &templates.Page{
// 			Title:  fmt.Sprintf("Cluster %s - ClusterCockpit", vars["cluster"]),
// 			Config: conf,
// 			Infos:  infos,
// 		})
// 	})

// 	router.HandleFunc("/monitoring/node/{cluster}/{hostname}", func(rw http.ResponseWriter, r *http.Request) {
// 		conf, infos, err := prepareRoute(r)
// 		if err != nil {
// 			http.Error(rw, err.Error(), http.StatusInternalServerError)
// 			return
// 		}

// 		vars := mux.Vars(r)
// 		infos["cluster"] = vars["cluster"]
// 		infos["hostname"] = vars["hostname"]
// 		from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
// 		if from != "" || to != "" {
// 			infos["from"] = from
// 			infos["to"] = to
// 		}

// 		templates.Render(rw, r, "monitoring/node.tmpl", &templates.Page{
// 			Title:  fmt.Sprintf("Host %s - ClusterCockpit", vars["hostname"]),
// 			Config: conf,
// 			Infos:  infos,
// 		})
// 	})
// }

func loadEnv(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}

	defer f.Close()
	s := bufio.NewScanner(bufio.NewReader(f))
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") || len(line) == 0 {
			continue
		}

		if strings.Contains(line, "#") {
			return errors.New("'#' are only supported at the start of a line")
		}

		line = strings.TrimPrefix(line, "export ")
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("unsupported line: %#v", line)
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if strings.HasPrefix(val, "\"") {
			if !strings.HasSuffix(val, "\"") {
				return fmt.Errorf("unsupported line: %#v", line)
			}

			runes := []rune(val[1 : len(val)-1])
			sb := strings.Builder{}
			for i := 0; i < len(runes); i++ {
				if runes[i] == '\\' {
					i++
					switch runes[i] {
					case 'n':
						sb.WriteRune('\n')
					case 'r':
						sb.WriteRune('\r')
					case 't':
						sb.WriteRune('\t')
					case '"':
						sb.WriteRune('"')
					default:
						return fmt.Errorf("unsupprorted escape sequence in quoted string: backslash %#v", runes[i])
					}
					continue
				}
				sb.WriteRune(runes[i])
			}

			val = sb.String()
		}

		os.Setenv(key, val)
	}

	return s.Err()
}

func dropPrivileges() error {
	if programConfig.Group != "" {
		g, err := user.LookupGroup(programConfig.Group)
		if err != nil {
			return err
		}

		gid, _ := strconv.Atoi(g.Gid)
		if err := syscall.Setgid(gid); err != nil {
			return err
		}
	}

	if programConfig.User != "" {
		u, err := user.Lookup(programConfig.User)
		if err != nil {
			return err
		}

		uid, _ := strconv.Atoi(u.Uid)
		if err := syscall.Setuid(uid); err != nil {
			return err
		}
	}

	return nil
}

// If started via systemd, inform systemd that we are running:
// https://www.freedesktop.org/software/systemd/man/sd_notify.html
func systemdNotifiy(ready bool, status string) {
	if os.Getenv("NOTIFY_SOCKET") == "" {
		// Not started using systemd
		return
	}

	args := []string{fmt.Sprintf("--pid=%d", os.Getpid())}
	if ready {
		args = append(args, "--ready")
	}

	if status != "" {
		args = append(args, fmt.Sprintf("--status=%s", status))
	}

	cmd := exec.Command("systemd-notify", args...)
	cmd.Run() // errors ignored on purpose, there is not much to do anyways.
}
