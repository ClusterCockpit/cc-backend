// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package auth implements various authentication methods
package auth

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/ClusterCockpit/cc-lib/v2/util"
	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"
)

// Authenticator is the interface for all authentication methods.
// Each authenticator determines if it can handle a login request (CanLogin)
// and performs the actual authentication (Login).
type Authenticator interface {
	// CanLogin determines if this authenticator can handle the login request.
	// It returns the user object if available and a boolean indicating if this
	// authenticator should attempt the login. This method should not perform
	// expensive operations or actual authentication.
	CanLogin(user *schema.User, username string, rw http.ResponseWriter, r *http.Request) (*schema.User, bool)

	// Login performs the actually authentication for the user.
	// It returns the authenticated user or an error if authentication fails.
	// The user parameter may be nil if the user doesn't exist in the database yet.
	Login(user *schema.User, rw http.ResponseWriter, r *http.Request) (*schema.User, error)
}

var (
	initOnce     sync.Once
	authInstance *Authentication
)

// rateLimiterEntry tracks a rate limiter and its last use time for cleanup
type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastUsed time.Time
}

var ipUserLimiters sync.Map

// getIPUserLimiter returns a rate limiter for the given IP and username combination.
// Rate limiters are created on demand and track 5 attempts per 15 minutes.
func getIPUserLimiter(ip, username string) *rate.Limiter {
	key := ip + ":" + username
	now := time.Now()

	if entry, ok := ipUserLimiters.Load(key); ok {
		rle := entry.(*rateLimiterEntry)
		rle.lastUsed = now
		return rle.limiter
	}

	// More aggressive rate limiting: 5 attempts per 15 minutes
	newLimiter := rate.NewLimiter(rate.Every(15*time.Minute/5), 5)
	ipUserLimiters.Store(key, &rateLimiterEntry{
		limiter:  newLimiter,
		lastUsed: now,
	})
	return newLimiter
}

// cleanupOldRateLimiters removes rate limiters that haven't been used recently
func cleanupOldRateLimiters(olderThan time.Time) {
	ipUserLimiters.Range(func(key, value any) bool {
		entry := value.(*rateLimiterEntry)
		if entry.lastUsed.Before(olderThan) {
			ipUserLimiters.Delete(key)
			cclog.Debugf("Cleaned up rate limiter for %v", key)
		}
		return true
	})
}

// startRateLimiterCleanup starts a background goroutine to clean up old rate limiters
func startRateLimiterCleanup() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			// Clean up limiters not used in the last 24 hours
			cleanupOldRateLimiters(time.Now().Add(-24 * time.Hour))
		}
	}()
}

// AuthConfig contains configuration for all authentication methods
type AuthConfig struct {
	LdapConfig   *LdapConfig    `json:"ldap"`
	JwtConfig    *JWTAuthConfig `json:"jwts"`
	OpenIDConfig *OpenIDConfig  `json:"oidc"`
}

// Keys holds the global authentication configuration
var Keys AuthConfig

// Authentication manages all authentication methods and session handling
type Authentication struct {
	sessionManager *scs.SessionManager
	LdapAuth       *LdapAuthenticator
	JwtAuth        *JWTAuthenticator
	LocalAuth      *LocalAuthenticator
	authenticators []Authenticator
	SessionMaxAge  time.Duration
}

// SessionManager exposes the scs session manager so the HTTP router can install
// the session middleware (LoadAndSave on write paths, LoadSession on read paths).
func (auth *Authentication) SessionManager() *scs.SessionManager {
	return auth.sessionManager
}

// LoadSession is a non-buffering, read-only session middleware. It loads any
// existing session into the request context so AuthViaSession can read it, but
// (unlike scs.LoadAndSave) it never wraps the ResponseWriter and never commits,
// so large responses (e.g. the GraphQL /query endpoint) stream without being
// buffered in memory. Use it on routes that only read the session to
// authenticate; use scs.LoadAndSave on the login/logout routes that mutate it.
func (auth *Authentication) LoadSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var token string
		if c, err := r.Cookie(auth.sessionManager.Cookie.Name); err == nil {
			token = c.Value
		}
		ctx, err := auth.sessionManager.Load(r.Context(), token)
		if err != nil {
			cclog.Errorf("session load failed: %s", err.Error())
			http.Error(rw, "internal server error", http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(rw, r.WithContext(ctx))
	})
}

// expireSessionCookie clears a (corrupted) session cookie on the client. Used on
// read paths where there is no commit to drive the deletion.
func expireSessionCookie(rw http.ResponseWriter) {
	http.SetCookie(rw, &http.Cookie{
		Name:     "session",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (auth *Authentication) AuthViaSession(
	rw http.ResponseWriter,
	r *http.Request,
) (*schema.User, error) {
	// The session data was loaded into the request context by the LoadSession
	// middleware. No active session cookie => not logged in (mirrors session.IsNew).
	ctx := r.Context()
	if !auth.sessionManager.Exists(ctx, "username") {
		return nil, nil
	}

	// Validate session data with proper type checking
	username := auth.sessionManager.GetString(ctx, "username")
	if username == "" {
		cclog.Warn("Invalid session: missing or invalid username")
		expireSessionCookie(rw)
		return nil, errors.New("invalid session data")
	}

	projects, ok := auth.sessionManager.Get(ctx, "projects").([]string)
	if !ok {
		cclog.Warn("Invalid session: projects not found or invalid type, using empty list")
		projects = []string{}
	}

	roles, ok := auth.sessionManager.Get(ctx, "roles").([]string)
	if !ok || len(roles) == 0 {
		cclog.Warn("Invalid session: missing or invalid roles")
		expireSessionCookie(rw)
		return nil, errors.New("invalid session data")
	}

	// GetInt returns 0 (== schema.AuthViaLocalPassword) when the key is absent.
	authSourceInt := auth.sessionManager.GetInt(ctx, "authSource")

	return &schema.User{
		Username:   username,
		Projects:   projects,
		Roles:      roles,
		AuthType:   schema.AuthSession,
		AuthSource: schema.AuthSource(authSourceInt),
	}, nil
}

func Init(authCfg *json.RawMessage) {
	initOnce.Do(func() {
		authInstance = &Authentication{}

		// Start background cleanup of rate limiters
		startRateLimiterCleanup()

		// Server-side sessions via scs, persisted in the existing SQLite DB so
		// sessions survive restarts. Only an opaque random token is stored in the
		// cookie, so no secret signing key (the former SESSION_KEY) is required.
		gob.Register([]string{}) // user.Projects / user.Roles are stored as []string
		sm := scs.New()
		sm.Store = sqlite3store.New(repository.GetConnection().DB.DB)
		sm.Cookie.Name = "session"
		sm.Cookie.Path = "/"
		sm.Cookie.HttpOnly = true
		sm.Cookie.SameSite = http.SameSiteLaxMode
		// scs sets Secure globally (no per-request option). Enable it when this
		// process terminates TLS itself. Deployments terminating TLS at a reverse
		// proxy can set this via a future config flag if needed.
		sm.Cookie.Secure = config.Keys.HTTPSCertFile != ""

		if d, err := time.ParseDuration(config.Keys.SessionMaxAge); err == nil && d != 0 {
			sm.Lifetime = d
			authInstance.SessionMaxAge = d
		} else {
			// SessionMaxAge of 0/empty means "do not expire": approximate with a
			// long absolute lifetime (the cookie remains persistent).
			sm.Lifetime = 10 * 365 * 24 * time.Hour
		}
		authInstance.sessionManager = sm

		// When authentication is disabled no authenticators are required; the
		// session store created above is enough for the server to run with a
		// valid (non-nil) auth instance.
		if config.Keys.DisableAuthentication {
			return
		}

		if authCfg == nil {
			return
		}

		config.Validate(configSchema, *authCfg)
		dec := json.NewDecoder(bytes.NewReader(*authCfg))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&Keys); err != nil {
			cclog.Errorf("error while decoding ldap config: %v", err)
		}

		if Keys.LdapConfig != nil {
			ldapAuth := &LdapAuthenticator{}
			if err := ldapAuth.Init(); err != nil {
				cclog.Warn("Error while initializing authentication -> ldapAuth init failed")
			} else {
				authInstance.LdapAuth = ldapAuth
				authInstance.authenticators = append(authInstance.authenticators, authInstance.LdapAuth)
			}
		} else {
			cclog.Info("Missing LDAP configuration: No LDAP support!")
		}

		if Keys.JwtConfig != nil {
			authInstance.JwtAuth = &JWTAuthenticator{}
			if err := authInstance.JwtAuth.Init(); err != nil {
				cclog.Fatal("Error while initializing authentication -> jwtAuth init failed")
			}

			jwtSessionAuth := &JWTSessionAuthenticator{}
			if err := jwtSessionAuth.Init(); err != nil {
				cclog.Info("jwtSessionAuth init failed: No JWT login support!")
			} else {
				authInstance.authenticators = append(authInstance.authenticators, jwtSessionAuth)
			}

			jwtCookieSessionAuth := &JWTCookieSessionAuthenticator{}
			if err := jwtCookieSessionAuth.Init(); err != nil {
				cclog.Info("jwtCookieSessionAuth init failed: No JWT cookie login support!")
			} else {
				authInstance.authenticators = append(authInstance.authenticators, jwtCookieSessionAuth)
			}
		} else {
			cclog.Info("Missing JWT configuration: No JWT token support!")
		}

		authInstance.LocalAuth = &LocalAuthenticator{}
		if err := authInstance.LocalAuth.Init(); err != nil {
			cclog.Fatal("Error while initializing authentication -> localAuth init failed")
		}
		authInstance.authenticators = append(authInstance.authenticators, authInstance.LocalAuth)
	})
}

func GetAuthInstance() *Authentication {
	if authInstance == nil {
		cclog.Fatal("Authentication module not initialized!")
	}

	return authInstance
}

// handleUserSync syncs or updates a user in the database based on configuration.
// This is used for LDAP, JWT and OIDC authentications when syncUserOnLogin or updateUserOnLogin is enabled.
func handleUserSync(user *schema.User, syncUserOnLogin, updateUserOnLogin bool) {
	r := repository.GetUserRepository()
	dbUser, err := r.GetUser(user.Username)

	if err != nil && err != sql.ErrNoRows {
		cclog.Errorf("Error while loading user '%s': %v", user.Username, err)
		return
	}

	if err == sql.ErrNoRows && syncUserOnLogin { // Add new user
		if err := r.AddUser(user); err != nil {
			cclog.Errorf("Error while adding user '%s' to DB: %v", user.Username, err)
		}
	} else if err == nil && updateUserOnLogin { // Update existing user
		if err := r.UpdateUser(dbUser, user); err != nil {
			cclog.Errorf("Error while updating user '%s' in DB: %v", dbUser.Username, err)
		}
	}
}

// handleTokenUser syncs JWT token user with database
func handleTokenUser(tokenUser *schema.User) {
	handleUserSync(tokenUser, Keys.JwtConfig.SyncUserOnLogin, Keys.JwtConfig.UpdateUserOnLogin)
}

// handleOIDCUser syncs OIDC user with database
func handleOIDCUser(OIDCUser *schema.User) {
	handleUserSync(OIDCUser, Keys.OpenIDConfig.SyncUserOnLogin, Keys.OpenIDConfig.UpdateUserOnLogin)
}

// handleLdapUser syncs LDAP user with database
func handleLdapUser(ldapUser *schema.User) {
	handleUserSync(ldapUser, Keys.LdapConfig.SyncUserOnLogin, Keys.LdapConfig.UpdateUserOnLogin)
}

func (auth *Authentication) SaveSession(rw http.ResponseWriter, r *http.Request, user *schema.User) error {
	// The login routes are wrapped by scs.LoadAndSave, which loaded the session
	// into the request context and will commit it (persist to the store and write
	// the Set-Cookie header) after the handler returns.
	ctx := r.Context()

	// Generate a new session token to prevent session fixation.
	if err := auth.sessionManager.RenewToken(ctx); err != nil {
		cclog.Errorf("session renew failed: %s", err.Error())
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return err
	}

	auth.sessionManager.Put(ctx, "username", user.Username)
	auth.sessionManager.Put(ctx, "projects", user.Projects)
	auth.sessionManager.Put(ctx, "roles", user.Roles)
	auth.sessionManager.Put(ctx, "authSource", int(user.AuthSource))

	return nil
}

func (auth *Authentication) Login(
	onfailure func(rw http.ResponseWriter, r *http.Request, loginErr error),
) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}

		username := r.FormValue("username")

		limiter := getIPUserLimiter(ip, username)
		if !limiter.Allow() {
			cclog.Warnf("AUTH/RATE > Too many login attempts for combination IP: %s, Username: %s", ip, username)
			onfailure(rw, r, errors.New("too many login attempts, try again in a few minutes"))
			return
		}

		var dbUser *schema.User
		if username != "" {
			var err error
			dbUser, err = repository.GetUserRepository().GetUser(username)
			if err != nil && err != sql.ErrNoRows {
				cclog.Errorf("Error while loading user '%v'", username)
			}
		}

		for _, authenticator := range auth.authenticators {
			var ok bool
			var user *schema.User
			if user, ok = authenticator.CanLogin(dbUser, username, rw, r); !ok {
				continue
			} else {
				cclog.Debugf("Can login with user %v", user)
			}

			user, err := authenticator.Login(user, rw, r)
			if err != nil {
				cclog.Warnf("user login failed: %s", err.Error())
				onfailure(rw, r, err)
				return
			}

			if err := auth.SaveSession(rw, r, user); err != nil {
				return
			}

			cclog.Infof("login successfull: user: %#v (roles: %v, projects: %v)", user.Username, user.Roles, user.Projects)
			ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)

			if redirect := r.FormValue("redirect"); redirect != "" {
				if u, perr := url.Parse(redirect); perr == nil && u.Scheme == "" && u.Host == "" {
					http.RedirectHandler(redirect, http.StatusFound).ServeHTTP(rw, r.WithContext(ctx))
					return
				}
				cclog.Warnf("login redirect rejected (not a relative path): %q", redirect)
			}

			http.RedirectHandler("/", http.StatusFound).ServeHTTP(rw, r.WithContext(ctx))
			return
		}

		cclog.Debugf("login failed: no authenticator applied")
		onfailure(rw, r, errors.New("no authenticator applied"))
	})
}

func (auth *Authentication) Auth(
	onsuccess http.Handler,
	onfailure func(rw http.ResponseWriter, r *http.Request, authErr error),
) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		user, err := auth.JwtAuth.AuthViaJWT(rw, r)
		if err != nil {
			cclog.Infof("auth -> authentication failed: %s", err.Error())
			http.Error(rw, err.Error(), http.StatusUnauthorized)
			return
		}
		if user == nil {
			user, err = auth.AuthViaSession(rw, r)
			if err != nil {
				cclog.Infof("auth -> authentication failed: %s", err.Error())
				http.Error(rw, err.Error(), http.StatusUnauthorized)
				return
			}
		}
		if user != nil {
			ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
			onsuccess.ServeHTTP(rw, r.WithContext(ctx))
			return
		}

		cclog.Infof("auth -> authentication failed: no valid session or JWT for %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		onfailure(rw, r, errors.New("unauthorized (please login first)"))
	})
}

func (auth *Authentication) AuthAPI(
	onsuccess http.Handler,
	onfailure func(rw http.ResponseWriter, r *http.Request, authErr error),
) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		user, err := auth.JwtAuth.AuthViaJWT(rw, r)
		if err != nil {
			cclog.Infof("auth api -> authentication failed: %s", err.Error())
			onfailure(rw, r, err)
			return
		}

		ipErr := securedCheck(user, r)
		if ipErr != nil {
			cclog.Infof("auth api -> secured check failed: %s", ipErr.Error())
			onfailure(rw, r, ipErr)
			return
		}

		if user != nil {
			switch {
			case len(user.Roles) == 1:
				if user.HasRole(schema.RoleAPI) {
					ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
					onsuccess.ServeHTTP(rw, r.WithContext(ctx))
					return
				}
			case len(user.Roles) >= 2:
				if user.HasAllRoles([]schema.Role{schema.RoleAdmin, schema.RoleAPI}) {
					ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
					onsuccess.ServeHTTP(rw, r.WithContext(ctx))
					return
				}
			default:
				cclog.Info("auth api -> authentication failed: missing role")
				onfailure(rw, r, errors.New("unauthorized"))
			}
		}
		cclog.Info("auth api -> authentication failed: no auth")
		onfailure(rw, r, errors.New("unauthorized"))
	})
}

func (auth *Authentication) AuthUserAPI(
	onsuccess http.Handler,
	onfailure func(rw http.ResponseWriter, r *http.Request, authErr error),
) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		user, err := auth.JwtAuth.AuthViaJWT(rw, r)
		if err != nil {
			cclog.Infof("auth user api -> authentication failed: %s", err.Error())
			onfailure(rw, r, err)
			return
		}

		if user != nil {
			switch {
			case len(user.Roles) == 1:
				if user.HasRole(schema.RoleAPI) {
					ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
					onsuccess.ServeHTTP(rw, r.WithContext(ctx))
					return
				}
			case len(user.Roles) >= 2:
				if user.HasRole(schema.RoleAPI) && user.HasAnyRole([]schema.Role{schema.RoleUser, schema.RoleManager, schema.RoleSupport, schema.RoleAdmin}) {
					ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
					onsuccess.ServeHTTP(rw, r.WithContext(ctx))
					return
				}
			default:
				cclog.Info("auth user api -> authentication failed: missing role")
				onfailure(rw, r, errors.New("unauthorized"))
			}
		}
		cclog.Info("auth user api -> authentication failed: no auth")
		onfailure(rw, r, errors.New("unauthorized"))
	})
}

func (auth *Authentication) AuthMetricStoreAPI(
	onsuccess http.Handler,
	onfailure func(rw http.ResponseWriter, r *http.Request, authErr error),
) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		user, err := auth.JwtAuth.AuthViaJWT(rw, r)
		if err != nil {
			cclog.Infof("auth metricstore api -> authentication failed: %s", err.Error())
			onfailure(rw, r, err)
			return
		}

		if user != nil {
			switch {
			case len(user.Roles) == 1:
				if user.HasRole(schema.RoleAPI) {
					ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
					onsuccess.ServeHTTP(rw, r.WithContext(ctx))
					return
				}
			case len(user.Roles) >= 2:
				if user.HasRole(schema.RoleAPI) && user.HasAnyRole([]schema.Role{schema.RoleUser, schema.RoleManager, schema.RoleAdmin}) {
					ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
					onsuccess.ServeHTTP(rw, r.WithContext(ctx))
					return
				}
			default:
				cclog.Info("auth metricstore api -> authentication failed: missing role")
				onfailure(rw, r, errors.New("unauthorized"))
			}
		}
		cclog.Info("auth metricstore api -> authentication failed: no auth")
		onfailure(rw, r, errors.New("unauthorized"))
	})
}

func (auth *Authentication) AuthConfigAPI(
	onsuccess http.Handler,
	onfailure func(rw http.ResponseWriter, r *http.Request, authErr error),
) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		user, err := auth.AuthViaSession(rw, r)
		if err != nil {
			cclog.Infof("auth config api -> authentication failed: %s", err.Error())
			onfailure(rw, r, err)
			return
		}
		if user != nil && user.HasRole(schema.RoleAdmin) {
			ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
			onsuccess.ServeHTTP(rw, r.WithContext(ctx))
			return
		}
		cclog.Info("auth config api -> authentication failed: no auth")
		onfailure(rw, r, errors.New("unauthorized"))
	})
}

func (auth *Authentication) AuthFrontendAPI(
	onsuccess http.Handler,
	onfailure func(rw http.ResponseWriter, r *http.Request, authErr error),
) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		user, err := auth.AuthViaSession(rw, r)
		if err != nil {
			cclog.Infof("auth frontend api -> authentication failed: %s", err.Error())
			onfailure(rw, r, err)
			return
		}
		if user != nil {
			ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
			onsuccess.ServeHTTP(rw, r.WithContext(ctx))
			return
		}
		cclog.Info("auth frontend api -> authentication failed: no auth")
		onfailure(rw, r, errors.New("unauthorized"))
	})
}

func (auth *Authentication) Logout(onsuccess http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// The logout route is wrapped by scs.LoadAndSave: Destroy removes the
		// session from the store and the middleware clears the cookie on the way out.
		if err := auth.sessionManager.Destroy(r.Context()); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		onsuccess.ServeHTTP(rw, r)
	})
}

// Helper Moved To MiddleWare Auth Handlers
func securedCheck(user *schema.User, r *http.Request) error {
	if user == nil {
		return fmt.Errorf("no user for secured check")
	}

	// extract IP address for checking
	IPAddress := r.Header.Get("X-Real-Ip")
	if IPAddress == "" {
		IPAddress = r.Header.Get("X-Forwarded-For")
	}
	if IPAddress == "" {
		IPAddress = r.RemoteAddr
	}

	// Handle both IPv4 and IPv6 addresses properly
	// For IPv6, this will strip the port and brackets
	// For IPv4, this will strip the port
	if host, _, err := net.SplitHostPort(IPAddress); err == nil {
		IPAddress = host
	}
	// If SplitHostPort fails, IPAddress is already just a host (no port)

	// If nothing declared in config: Continue // FIXME: Allow All If Not Declared?
	if len(config.Keys.APIAllowedIPs) == 0 {
		return nil
	}
	// If wildcard declared in config: Continue
	if config.Keys.APIAllowedIPs[0] == "*" {
		return nil
	}
	// check if IP is allowed
	if !util.Contains(config.Keys.APIAllowedIPs, IPAddress) {
		return fmt.Errorf("unknown ip: %v", IPAddress)
	}

	return nil
}
