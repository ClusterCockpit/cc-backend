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
	"github.com/ClusterCockpit/cc-backend/internal/routerConfig"
	"github.com/ClusterCockpit/cc-backend/pkg/metricstore"
	"github.com/ClusterCockpit/cc-backend/web"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/nats"
	"github.com/ClusterCockpit/cc-lib/v2/runtime"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	httpSwagger "github.com/swaggo/http-swagger"
)

var buildInfo web.Build

// Environment variable names
const (
	envDebug = "DEBUG"
)

// Server encapsulates the HTTP server state and dependencies
type Server struct {
	router        chi.Router
	server        *http.Server
	restAPIHandle *api.RestAPI
	natsAPIHandle *api.NatsAPI
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
		router: chi.NewRouter(),
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

	// Middleware must be defined before routes in chi
	s.router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(rw, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			cclog.Debugf("%s %s (%d, %.02fkb, %dms)",
				r.Method, r.URL.RequestURI(),
				ww.Status(), float32(ww.BytesWritten())/1024,
				time.Since(start).Milliseconds())
		})
	})
	s.router.Use(middleware.Compress(5))
	s.router.Use(middleware.Recoverer)
	s.router.Use(cors.Handler(cors.Options{
		AllowCredentials: true,
		AllowedHeaders:   []string{"X-Requested-With", "Content-Type", "Authorization", "Origin"},
		AllowedMethods:   []string{"GET", "POST", "HEAD", "OPTIONS"},
		AllowedOrigins:   []string{"*"},
	}))

	s.restAPIHandle = api.New()

	info := map[string]any{}
	info["hasOpenIDConnect"] = false

	if auth.Keys.OpenIDConfig != nil {
		openIDConnect := auth.NewOIDC(authHandle)
		openIDConnect.RegisterEndpoints(s.router)
		info["hasOpenIDConnect"] = true
	}

	s.router.Get("/login", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "text/html; charset=utf-8")
		cclog.Debugf("##%v##", info)
		web.RenderTemplate(rw, "login.tmpl", &web.Page{Title: "Login", Build: buildInfo, Infos: info})
	})
	s.router.HandleFunc("/imprint", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "text/html; charset=utf-8")
		web.RenderTemplate(rw, "imprint.tmpl", &web.Page{Title: "Imprint", Build: buildInfo})
	})
	s.router.HandleFunc("/privacy", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "text/html; charset=utf-8")
		web.RenderTemplate(rw, "privacy.tmpl", &web.Page{Title: "Privacy", Build: buildInfo})
	})

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

		s.router.Post("/login", authHandle.Login(loginFailureHandler).ServeHTTP)
		s.router.HandleFunc("/jwt-login", authHandle.Login(loginFailureHandler).ServeHTTP)

		s.router.Post("/logout", authHandle.Logout(
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
			})).ServeHTTP)
	}

	if flagDev {
		s.router.Handle("/playground", playground.Handler("GraphQL playground", "/query"))
		s.router.Get("/swagger/*", httpSwagger.Handler(
			httpSwagger.URL("http://"+config.Keys.Addr+"/swagger/doc.json")))
	}

	// Secured routes (require authentication)
	s.router.Group(func(secured chi.Router) {
		if !config.Keys.DisableAuthentication {
			secured.Use(func(next http.Handler) http.Handler {
				return authHandle.Auth(
					next,
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
		}

		secured.Handle("/query", graphQLServer)

		secured.HandleFunc("/search", func(rw http.ResponseWriter, r *http.Request) {
			routerConfig.HandleSearchBar(rw, r, buildInfo)
		})

		routerConfig.SetupRoutes(secured, buildInfo)
	})

	// API routes (JWT token auth)
	s.router.Route("/api", func(apiRouter chi.Router) {
		// Main API routes with API auth
		apiRouter.Group(func(securedapi chi.Router) {
			if !config.Keys.DisableAuthentication {
				securedapi.Use(func(next http.Handler) http.Handler {
					return authHandle.AuthAPI(next, onFailureResponse)
				})
			}
			s.restAPIHandle.MountAPIRoutes(securedapi)
		})

		// Metric store API routes with separate auth
		apiRouter.Group(func(metricstoreapi chi.Router) {
			if !config.Keys.DisableAuthentication {
				metricstoreapi.Use(func(next http.Handler) http.Handler {
					return authHandle.AuthMetricStoreAPI(next, onFailureResponse)
				})
			}
			s.restAPIHandle.MountMetricStoreAPIRoutes(metricstoreapi)
		})
	})

	// User API routes
	s.router.Route("/userapi", func(userapi chi.Router) {
		if !config.Keys.DisableAuthentication {
			userapi.Use(func(next http.Handler) http.Handler {
				return authHandle.AuthUserAPI(next, onFailureResponse)
			})
		}
		s.restAPIHandle.MountUserAPIRoutes(userapi)
	})

	// Config API routes (uses Group with full paths to avoid shadowing
	// the /config page route that is registered in the secured group)
	s.router.Group(func(configapi chi.Router) {
		if !config.Keys.DisableAuthentication {
			configapi.Use(func(next http.Handler) http.Handler {
				return authHandle.AuthConfigAPI(next, onFailureResponse)
			})
		}
		s.restAPIHandle.MountConfigAPIRoutes(configapi)
	})

	// Frontend API routes
	s.router.Route("/frontend", func(frontendapi chi.Router) {
		if !config.Keys.DisableAuthentication {
			frontendapi.Use(func(next http.Handler) http.Handler {
				return authHandle.AuthFrontendAPI(next, onFailureResponse)
			})
		}
		s.restAPIHandle.MountFrontendAPIRoutes(frontendapi)
	})

	if config.Keys.APISubjects != nil {
		s.natsAPIHandle = api.NewNatsAPI()
		if err := s.natsAPIHandle.StartSubscriptions(); err != nil {
			return fmt.Errorf("starting NATS subscriptions: %w", err)
		}
	}

	// 404 handler for pages and API routes
	notFoundHandler := func(rw http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/userapi/") ||
			strings.HasPrefix(r.URL.Path, "/frontend/") || strings.HasPrefix(r.URL.Path, "/config/") {
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusNotFound)
			json.NewEncoder(rw).Encode(map[string]string{
				"status": "Resource not found",
				"error":  "the requested endpoint does not exist",
			})
			return
		}
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		rw.WriteHeader(http.StatusNotFound)
		web.RenderTemplate(rw, "404.tmpl", &web.Page{
			Title: "Page Not Found",
			Build: buildInfo,
		})
	}

	if config.Keys.EmbedStaticFiles {
		if i, err := os.Stat("./var/img"); err == nil {
			if i.IsDir() {
				cclog.Info("Use local directory for static images")
				s.router.Handle("/img/*", http.StripPrefix("/img/", http.FileServer(http.Dir("./var/img"))))
			}
		}
		fileServer := http.StripPrefix("/", web.ServeFiles())
		s.router.Handle("/*", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if web.StaticFileExists(r.URL.Path) {
				fileServer.ServeHTTP(rw, r)
				return
			}
			notFoundHandler(rw, r)
		}))
	} else {
		staticDir := http.Dir(config.Keys.StaticFiles)
		fileServer := http.FileServer(staticDir)
		s.router.Handle("/*", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			f, err := staticDir.Open(r.URL.Path)
			if err == nil {
				f.Close()
				fileServer.ServeHTTP(rw, r)
				return
			}
			notFoundHandler(rw, r)
		}))
	}

	return nil
}

// Server timeout defaults (in seconds)
const (
	defaultReadTimeout  = 20
	defaultWriteTimeout = 20
)

func (s *Server) Start(ctx context.Context) error {
	// Use configurable timeouts with defaults
	readTimeout := time.Duration(defaultReadTimeout) * time.Second
	writeTimeout := time.Duration(defaultWriteTimeout) * time.Second

	s.server = &http.Server{
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		Handler:      s.router,
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
	if err := runtime.DropPrivileges(config.Keys.Group, config.Keys.User); err != nil {
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

	nc := nats.GetClient()
	if nc != nil {
		nc.Close()
	}

	// First shut down the server gracefully (waiting for all ongoing requests)
	if err := s.server.Shutdown(shutdownCtx); err != nil {
		cclog.Errorf("Server shutdown error: %v", err)
	}

	// Archive all the metric store data
	metricstore.Shutdown()

	// Shutdown archiver with 10 second timeout for fast shutdown
	if err := archiver.Shutdown(10 * time.Second); err != nil {
		cclog.Warnf("Archiver shutdown: %v", err)
	}
}
