// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
// Package main provides the entry point for the ClusterCockpit backend server.
// This file contains HTTP server setup, routing configuration, and
// authentication middleware integration.
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
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/ClusterCockpit/cc-backend/internal/api"
	"github.com/ClusterCockpit/cc-backend/internal/archiver"
	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/graph"
	"github.com/ClusterCockpit/cc-backend/internal/graph/generated"
	"github.com/ClusterCockpit/cc-backend/internal/memorystore"
	"github.com/ClusterCockpit/cc-backend/internal/routerConfig"
	"github.com/ClusterCockpit/cc-backend/web"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/runtimeEnv"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"
)

var buildInfo web.Build

// Environment variable names
const (
	envDebug = "DEBUG"
)

// Server encapsulates the HTTP server state and dependencies
type Server struct {
	router    *mux.Router
	server    *http.Server
	apiHandle *api.RestApi
}

func onFailureResponse(rw http.ResponseWriter, r *http.Request, err error) {
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(rw).Encode(map[string]string{
		"status": http.StatusText(http.StatusUnauthorized),
		"error":  err.Error(),
	})
}

// NewServer creates and initializes a new Server instance
func NewServer(version, commit, buildDate string) (*Server, error) {
	buildInfo = web.Build{Version: version, Hash: commit, Buildtime: buildDate}

	s := &Server{
		router: mux.NewRouter(),
	}

	if err := s.init(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Server) init() error {
	// Setup the http.Handler/Router used by the server
	graph.Init()
	resolver := graph.GetResolverInstance()
	graphQLServer := handler.New(
		generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))

	graphQLServer.AddTransport(transport.POST{})

	if os.Getenv(envDebug) != "1" {
		// Having this handler means that a error message is returned via GraphQL instead of the connection simply beeing closed.
		// The problem with this is that then, no more stacktrace is printed to stderr.
		graphQLServer.SetRecoverFunc(func(ctx context.Context, err any) error {
			switch e := err.(type) {
			case string:
				return fmt.Errorf("MAIN > Panic: %s", e)
			case error:
				return fmt.Errorf("MAIN > Panic caused by: %s", e.Error())
			}

			return errors.New("MAIN > Internal server error (panic)")
		})
	}

	authHandle := auth.GetAuthInstance()

	s.apiHandle = api.New()

	info := map[string]any{}
	info["hasOpenIDConnect"] = false

	if auth.Keys.OpenIDConfig != nil {
		openIDConnect := auth.NewOIDC(authHandle)
		openIDConnect.RegisterEndpoints(s.router)
		info["hasOpenIDConnect"] = true
	}

	s.router.HandleFunc("/login", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "text/html; charset=utf-8")
		cclog.Debugf("##%v##", info)
		web.RenderTemplate(rw, "login.tmpl", &web.Page{Title: "Login", Build: buildInfo, Infos: info})
	}).Methods(http.MethodGet)
	s.router.HandleFunc("/imprint", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "text/html; charset=utf-8")
		web.RenderTemplate(rw, "imprint.tmpl", &web.Page{Title: "Imprint", Build: buildInfo})
	})
	s.router.HandleFunc("/privacy", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "text/html; charset=utf-8")
		web.RenderTemplate(rw, "privacy.tmpl", &web.Page{Title: "Privacy", Build: buildInfo})
	})

	secured := s.router.PathPrefix("/").Subrouter()
	securedapi := s.router.PathPrefix("/api").Subrouter()
	userapi := s.router.PathPrefix("/userapi").Subrouter()
	configapi := s.router.PathPrefix("/config").Subrouter()
	frontendapi := s.router.PathPrefix("/frontend").Subrouter()
	metricstoreapi := s.router.PathPrefix("/metricstore").Subrouter()

	if !config.Keys.DisableAuthentication {
		// Create login failure handler (used by both /login and /jwt-login)
		loginFailureHandler := func(rw http.ResponseWriter, r *http.Request, err error) {
			rw.Header().Add("Content-Type", "text/html; charset=utf-8")
			rw.WriteHeader(http.StatusUnauthorized)
			web.RenderTemplate(rw, "login.tmpl", &web.Page{
				Title:   "Login failed - ClusterCockpit",
				MsgType: "alert-warning",
				Message: err.Error(),
				Build:   buildInfo,
				Infos:   info,
			})
		}

		s.router.Handle("/login", authHandle.Login(loginFailureHandler)).Methods(http.MethodPost)
		s.router.Handle("/jwt-login", authHandle.Login(loginFailureHandler))

		s.router.Handle("/logout", authHandle.Logout(
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
						Title:    "Authentication failed - ClusterCockpit",
						MsgType:  "alert-danger",
						Message:  err.Error(),
						Build:    buildInfo,
						Infos:    info,
						Redirect: r.RequestURI,
					})
				})
		})

		securedapi.Use(func(next http.Handler) http.Handler {
			return authHandle.AuthAPI(
				// On success;
				next,
				// On failure: JSON Response
				onFailureResponse)
		})

		userapi.Use(func(next http.Handler) http.Handler {
			return authHandle.AuthUserAPI(
				// On success;
				next,
				// On failure: JSON Response
				onFailureResponse)
		})

		metricstoreapi.Use(func(next http.Handler) http.Handler {
			return authHandle.AuthMetricStoreAPI(
				// On success;
				next,
				// On failure: JSON Response
				onFailureResponse)
		})

		configapi.Use(func(next http.Handler) http.Handler {
			return authHandle.AuthConfigAPI(
				// On success;
				next,
				// On failure: JSON Response
				onFailureResponse)
		})

		frontendapi.Use(func(next http.Handler) http.Handler {
			return authHandle.AuthFrontendAPI(
				// On success;
				next,
				// On failure: JSON Response
				onFailureResponse)
		})
	}

	if flagDev {
		s.router.Handle("/playground", playground.Handler("GraphQL playground", "/query"))
		s.router.PathPrefix("/swagger/").Handler(httpSwagger.Handler(
			httpSwagger.URL("http://" + config.Keys.Addr + "/swagger/doc.json"))).Methods(http.MethodGet)
	}
	secured.Handle("/query", graphQLServer)

	// Send a searchId and then reply with a redirect to a user, or directly send query to job table for jobid and project.
	secured.HandleFunc("/search", func(rw http.ResponseWriter, r *http.Request) {
		routerConfig.HandleSearchBar(rw, r, buildInfo)
	})

	// Mount all /monitoring/... and /api/... routes.
	routerConfig.SetupRoutes(secured, buildInfo)
	s.apiHandle.MountApiRoutes(securedapi)
	s.apiHandle.MountUserApiRoutes(userapi)
	s.apiHandle.MountConfigApiRoutes(configapi)
	s.apiHandle.MountFrontendApiRoutes(frontendapi)

	if memorystore.InternalCCMSFlag {
		s.apiHandle.MountMetricStoreApiRoutes(metricstoreapi)
	}

	if config.Keys.EmbedStaticFiles {
		if i, err := os.Stat("./var/img"); err == nil {
			if i.IsDir() {
				cclog.Info("Use local directory for static images")
				s.router.PathPrefix("/img/").Handler(http.StripPrefix("/img/", http.FileServer(http.Dir("./var/img"))))
			}
		}
		s.router.PathPrefix("/").Handler(http.StripPrefix("/", web.ServeFiles()))
	} else {
		s.router.PathPrefix("/").Handler(http.FileServer(http.Dir(config.Keys.StaticFiles)))
	}

	s.router.Use(handlers.CompressHandler)
	s.router.Use(handlers.RecoveryHandler(handlers.PrintRecoveryStack(true)))
	s.router.Use(handlers.CORS(
		handlers.AllowCredentials(),
		handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization", "Origin"}),
		handlers.AllowedMethods([]string{"GET", "POST", "HEAD", "OPTIONS"}),
		handlers.AllowedOrigins([]string{"*"})))

	return nil
}

// Server timeout defaults (in seconds)
const (
	defaultReadTimeout  = 20
	defaultWriteTimeout = 20
)

func (s *Server) Start(ctx context.Context) error {
	handler := handlers.CustomLoggingHandler(io.Discard, s.router, func(_ io.Writer, params handlers.LogFormatterParams) {
		if strings.HasPrefix(params.Request.RequestURI, "/api/") {
			cclog.Debugf("%s %s (%d, %.02fkb, %dms)",
				params.Request.Method, params.URL.RequestURI(),
				params.StatusCode, float32(params.Size)/1024,
				time.Since(params.TimeStamp).Milliseconds())
		} else {
			cclog.Debugf("%s %s (%d, %.02fkb, %dms)",
				params.Request.Method, params.URL.RequestURI(),
				params.StatusCode, float32(params.Size)/1024,
				time.Since(params.TimeStamp).Milliseconds())
		}
	})

	// Use configurable timeouts with defaults
	readTimeout := time.Duration(defaultReadTimeout) * time.Second
	writeTimeout := time.Duration(defaultWriteTimeout) * time.Second

	s.server = &http.Server{
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		Handler:      handler,
		Addr:         config.Keys.Addr,
	}

	// Start http or https server
	listener, err := net.Listen("tcp", config.Keys.Addr)
	if err != nil {
		return fmt.Errorf("starting listener on '%s': %w", config.Keys.Addr, err)
	}

	if !strings.HasSuffix(config.Keys.Addr, ":80") && config.Keys.RedirectHTTPTo != "" {
		go func() {
			http.ListenAndServe(":80", http.RedirectHandler(config.Keys.RedirectHTTPTo, http.StatusMovedPermanently))
		}()
	}

	if config.Keys.HTTPSCertFile != "" && config.Keys.HTTPSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(
			config.Keys.HTTPSCertFile, config.Keys.HTTPSKeyFile)
		if err != nil {
			return fmt.Errorf("loading X509 keypair (check 'https-cert-file' and 'https-key-file' in config.json): %w", err)
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
		cclog.Infof("HTTPS server listening at %s...", config.Keys.Addr)
	} else {
		cclog.Infof("HTTP server listening at %s...", config.Keys.Addr)
	}
	//
	// Because this program will want to bind to a privileged port (like 80), the listener must
	// be established first, then the user can be changed, and after that,
	// the actual http server can be started.
	if err := runtimeEnv.DropPrivileges(config.Keys.Group, config.Keys.User); err != nil {
		return fmt.Errorf("dropping privileges: %w", err)
	}

	// Handle context cancellation for graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			cclog.Errorf("Server shutdown error: %v", err)
		}
	}()

	if err = s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) {
	// Create a shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// First shut down the server gracefully (waiting for all ongoing requests)
	if err := s.server.Shutdown(shutdownCtx); err != nil {
		cclog.Errorf("Server shutdown error: %v", err)
	}

	// Archive all the metric store data
	if memorystore.InternalCCMSFlag {
		memorystore.Shutdown()
	}

	// Shutdown archiver with 10 second timeout for fast shutdown
	if err := archiver.Shutdown(10 * time.Second); err != nil {
		cclog.Warnf("Archiver shutdown: %v", err)
	}
}
