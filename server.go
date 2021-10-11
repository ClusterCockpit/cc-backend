package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/ClusterCockpit/cc-jobarchive/graph"
	"github.com/ClusterCockpit/cc-jobarchive/graph/generated"
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
	flag.StringVar(&jobDBFile, "job-db", "./job.db", "SQLite 3 Jobs Database File")
	flag.BoolVar(&reinitDB, "init-db", false, "Initialize new SQLite Database")
	flag.Parse()

	db, err := sqlx.Open("sqlite3", jobDBFile)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if reinitDB {
		if err = initDB(db, "./job-data"); err != nil {
			log.Fatal(err)
		}

		if err = db.Close(); err != nil {
			log.Fatal(err)
		}

		return
	}

	r := mux.NewRouter()
	loggedRouter := handlers.LoggingHandler(os.Stdout, r)

	srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: &graph.Resolver{DB: db}}))
	r.HandleFunc("/graphql-playground", playground.Handler("GraphQL playground", "/query"))
	r.Handle("/query", srv)

	if len(staticFiles) != 0 {
		r.PathPrefix("/").Handler(http.FileServer(http.Dir(staticFiles)))
	}

	log.Printf("connect to http://localhost:%s/graphql-playground for GraphQL playground", port)
	log.Fatal(http.ListenAndServe("127.0.0.1:"+port,
		handlers.CORS(handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization"}),
			handlers.AllowedMethods([]string{"GET", "POST", "HEAD", "OPTIONS"}),
			handlers.AllowedOrigins([]string{"*"}))(loggedRouter)))
}
