package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/ClusterCockpit/cc-jobarchive/config"
	"github.com/ClusterCockpit/cc-jobarchive/graph"
	"github.com/ClusterCockpit/cc-jobarchive/graph/generated"
	"github.com/ClusterCockpit/cc-jobarchive/graph/model"
	"github.com/ClusterCockpit/cc-jobarchive/metricdata"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	var reinitDB bool
	var port, staticFiles, jobDBFile string

	flag.StringVar(&port, "port", "8080", "Port on which to listen")
	flag.StringVar(&staticFiles, "static-files", "./frontend/public", "Directory who's contents shall be served as static files")
	flag.StringVar(&jobDBFile, "job-db", "./var/job.db", "SQLite 3 Jobs Database File")
	flag.BoolVar(&reinitDB, "init-db", false, "Initialize new SQLite Database")
	flag.Parse()

	db, err := sqlx.Open("sqlite3", jobDBFile)
	if err != nil {
		log.Fatal(err)
	}

	// See https://github.com/mattn/go-sqlite3/issues/274
	db.SetMaxOpenConns(1)
	defer db.Close()

	if reinitDB {
		if err = initDB(db, metricdata.JobArchivePath); err != nil {
			log.Fatal(err)
		}
	}

	clusters, err := loadClusters()
	if err != nil {
		log.Fatal(err)
	}

	r := mux.NewRouter()
	loggedRouter := handlers.LoggingHandler(os.Stdout, r)

	srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{
		Resolvers: &graph.Resolver{DB: db, ClusterConfigs: clusters}}))
	r.HandleFunc("/graphql-playground", playground.Handler("GraphQL playground", "/query"))
	r.Handle("/query", srv)
	r.HandleFunc("/config.json", config.ServeConfig).Methods("GET")

	if len(staticFiles) != 0 {
		r.PathPrefix("/").Handler(http.FileServer(http.Dir(staticFiles)))
	}

	log.Printf("GraphQL playground: http://localhost:%s/graphql-playground", port)
	log.Printf("Home:               http://localhost:%s/index.html", port)
	log.Fatal(http.ListenAndServe("127.0.0.1:"+port,
		handlers.CORS(handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization"}),
			handlers.AllowedMethods([]string{"GET", "POST", "HEAD", "OPTIONS"}),
			handlers.AllowedOrigins([]string{"*"}))(loggedRouter)))
}

func loadClusters() ([]*model.Cluster, error) {
	entries, err := os.ReadDir(metricdata.JobArchivePath)
	if err != nil {
		return nil, err
	}

	clusters := []*model.Cluster{}
	for _, de := range entries {
		bytes, err := os.ReadFile(filepath.Join(metricdata.JobArchivePath, de.Name(), "cluster.json"))
		if err != nil {
			return nil, err
		}

		var cluster model.Cluster
		if err := json.Unmarshal(bytes, &cluster); err != nil {
			return nil, err
		}

		if cluster.FilterRanges.StartTime.To.IsZero() {
			cluster.FilterRanges.StartTime.To = time.Unix(0, 0)
		}

		clusters = append(clusters, &cluster)
	}

	return clusters, nil
}
