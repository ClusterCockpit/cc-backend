// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/ClusterCockpit/cc-backend/internal/api"
	"github.com/ClusterCockpit/cc-backend/internal/archiver"
	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/graph"
	"github.com/ClusterCockpit/cc-backend/internal/graph/generated"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/internal/routerConfig"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/runtimeEnv"
	"github.com/ClusterCockpit/cc-backend/web"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"
)

var (
	router    *mux.Router
	server    *http.Server
	apiHandle *api.RestApi
)

func onFailureResponse(rw http.ResponseWriter, r *http.Request, err error) {
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(rw).Encode(map[string]string{
		"status": http.StatusText(http.StatusUnauthorized),
		"error":  err.Error(),
	})
}

func serverInit() {
	// Setup the http.Handler/Router used by the server
	graph.Init()
	resolver := graph.GetResolverInstance()
	graphQLEndpoint := handler.NewDefaultServer(
		generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))

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

	authHandle := auth.GetAuthInstance()

	apiHandle = api.New()

	router = mux.NewRouter()
	buildInfo := web.Build{Version: version, Hash: commit, Buildtime: date}

	info := map[string]interface{}{}
	info["hasOpenIDConnect"] = false

	if config.Keys.OpenIDConfig != nil {
		openIDConnect := auth.NewOIDC(authHandle)
		openIDConnect.RegisterEndpoints(router)
		info["hasOpenIDConnect"] = true
	}

	router.HandleFunc("/login", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "text/html; charset=utf-8")
		log.Debugf("##%v##", info)
		web.RenderTemplate(rw, "login.tmpl", &web.Page{Title: "Login", Build: buildInfo, Infos: info})
	}).Methods(http.MethodGet)
	router.HandleFunc("/imprint", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "text/html; charset=utf-8")
		web.RenderTemplate(rw, "imprint.tmpl", &web.Page{Title: "Imprint", Build: buildInfo})
	})
	router.HandleFunc("/privacy", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "text/html; charset=utf-8")
		web.RenderTemplate(rw, "privacy.tmpl", &web.Page{Title: "Privacy", Build: buildInfo})
	})

	secured := router.PathPrefix("/").Subrouter()
	securedapi := router.PathPrefix("/api").Subrouter()
	userapi := router.PathPrefix("/userapi").Subrouter()
	configapi := router.PathPrefix("/config").Subrouter()
	frontendapi := router.PathPrefix("/frontend").Subrouter()

	if !config.Keys.DisableAuthentication {
		router.Handle("/login", authHandle.Login(
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
					Infos:   info,
				})
			})).Methods(http.MethodPost)

		router.Handle("/jwt-login", authHandle.Login(
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
					Infos:   info,
				})
			}))

		router.Handle("/logout", authHandle.Logout(
			http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				rw.Header().Add("Content-Type", "text/html; charset=utf-8")
				rw.WriteHeader(http.StatusOK)
				web.RenderTemplate(rw, "login.tmpl", &web.Page{
					Title:   "Bye - ClusterCockpit",
					MsgType: "alert-info",
					Message: "Logout successful",
					Build:   buildInfo,
					Infos:   info,
				})
			}))).Methods(http.MethodPost)

		secured.Use(func(next http.Handler) http.Handler {
			return authHandle.Auth(
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
						Infos:   info,
					})
				})
		})

		securedapi.Use(func(next http.Handler) http.Handler {
			return authHandle.AuthApi(
				// On success;
				next,
				// On failure: JSON Response
				onFailureResponse)
		})

		userapi.Use(func(next http.Handler) http.Handler {
			return authHandle.AuthUserApi(
				// On success;
				next,
				// On failure: JSON Response
				onFailureResponse)
		})

		configapi.Use(func(next http.Handler) http.Handler {
			return authHandle.AuthConfigApi(
				// On success;
				next,
				// On failure: JSON Response
				onFailureResponse)
		})

		frontendapi.Use(func(next http.Handler) http.Handler {
			return authHandle.AuthFrontendApi(
				// On success;
				next,
				// On failure: JSON Response
				onFailureResponse)
		})
	}

	if flagDev {
		router.Handle("/playground", playground.Handler("GraphQL playground", "/query"))
		router.PathPrefix("/swagger/").Handler(httpSwagger.Handler(
			httpSwagger.URL("http://" + config.Keys.Addr + "/swagger/doc.json"))).Methods(http.MethodGet)
	}
	secured.Handle("/query", graphQLEndpoint)

	// Send a searchId and then reply with a redirect to a user, or directly send query to job table for jobid and project.
	secured.HandleFunc("/search", func(rw http.ResponseWriter, r *http.Request) {
		routerConfig.HandleSearchBar(rw, r, buildInfo)
	})

	// Mount all /monitoring/... and /api/... routes.
	routerConfig.SetupRoutes(secured, buildInfo)
	apiHandle.MountApiRoutes(securedapi)
	apiHandle.MountUserApiRoutes(userapi)
	apiHandle.MountConfigApiRoutes(configapi)
	apiHandle.MountFrontendApiRoutes(frontendapi)

	if config.Keys.EmbedStaticFiles {
		if i, err := os.Stat("./var/img"); err == nil {
			if i.IsDir() {
				log.Info("Use local directory for static images")
				router.PathPrefix("/img/").Handler(http.StripPrefix("/img/", http.FileServer(http.Dir("./var/img"))))
			}
		}
		router.PathPrefix("/").Handler(web.ServeFiles())
	} else {
		router.PathPrefix("/").Handler(http.FileServer(http.Dir(config.Keys.StaticFiles)))
	}

	router.Use(handlers.CompressHandler)
	router.Use(handlers.RecoveryHandler(handlers.PrintRecoveryStack(true)))
	router.Use(handlers.CORS(
		handlers.AllowCredentials(),
		handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization", "Origin"}),
		handlers.AllowedMethods([]string{"GET", "POST", "HEAD", "OPTIONS"}),
		handlers.AllowedOrigins([]string{"*"})))
}

func serverStart() {
	handler := handlers.CustomLoggingHandler(io.Discard, router, func(_ io.Writer, params handlers.LogFormatterParams) {
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

	server = &http.Server{
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 20 * time.Second,
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
		cert, err := tls.LoadX509KeyPair(
			config.Keys.HttpsCertFile, config.Keys.HttpsKeyFile)
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
	//
	// Because this program will want to bind to a privileged port (like 80), the listener must
	// be established first, then the user can be changed, and after that,
	// the actual http server can be started.
	if err := runtimeEnv.DropPrivileges(config.Keys.Group, config.Keys.User); err != nil {
		log.Fatalf("error while preparing server start: %s", err.Error())
	}

	if err = server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatalf("starting server failed: %v", err)
	}
}

func serverShutdown() {
	// First shut down the server gracefully (waiting for all ongoing requests)
	server.Shutdown(context.Background())

	// Then, wait for any async jobStarts still pending...
	repository.WaitForJobStart()

	// Then, wait for any async archivings still pending...
	archiver.WaitForArchiving()
}
