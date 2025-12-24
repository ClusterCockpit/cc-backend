// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package auth implements various authentication methods
package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/ClusterCockpit/cc-lib/v2/util"
	"github.com/gorilla/sessions"
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
	sessionStore   *sessions.CookieStore
	LdapAuth       *LdapAuthenticator
	JwtAuth        *JWTAuthenticator
	LocalAuth      *LocalAuthenticator
	authenticators []Authenticator
	SessionMaxAge  time.Duration
}

func (auth *Authentication) AuthViaSession(
	rw http.ResponseWriter,
	r *http.Request,
) (*schema.User, error) {
	session, err := auth.sessionStore.Get(r, "session")
	if err != nil {
		cclog.Error("Error while getting session store")
		return nil, err
	}

	if session.IsNew {
		return nil, nil
	}

	// Validate session data with proper type checking
	username, ok := session.Values["username"].(string)
	if !ok || username == "" {
		cclog.Warn("Invalid session: missing or invalid username")
		// Invalidate the corrupted session
		session.Options.MaxAge = -1
		_ = auth.sessionStore.Save(r, rw, session)
		return nil, errors.New("invalid session data")
	}

	projects, ok := session.Values["projects"].([]string)
	if !ok {
		cclog.Warn("Invalid session: projects not found or invalid type, using empty list")
		projects = []string{}
	}

	roles, ok := session.Values["roles"].([]string)
	if !ok || len(roles) == 0 {
		cclog.Warn("Invalid session: missing or invalid roles")
		// Invalidate the corrupted session
		session.Options.MaxAge = -1
		_ = auth.sessionStore.Save(r, rw, session)
		return nil, errors.New("invalid session data")
	}

	return &schema.User{
		Username:   username,
		Projects:   projects,
		Roles:      roles,
		AuthType:   schema.AuthSession,
		AuthSource: -1,
	}, nil
}

func Init(authCfg *json.RawMessage) {
	initOnce.Do(func() {
		authInstance = &Authentication{}
		
		// Start background cleanup of rate limiters
		startRateLimiterCleanup()

		sessKey := os.Getenv("SESSION_KEY")
		if sessKey == "" {
			cclog.Warn("environment variable 'SESSION_KEY' not set (will use non-persistent random key)")
			bytes := make([]byte, 32)
			if _, err := rand.Read(bytes); err != nil {
				cclog.Fatal("Error while initializing authentication -> failed to generate random bytes for session key")
			}
			authInstance.sessionStore = sessions.NewCookieStore(bytes)
		} else {
			bytes, err := base64.StdEncoding.DecodeString(sessKey)
			if err != nil {
				cclog.Fatal("Error while initializing authentication -> decoding session key failed")
			}
			authInstance.sessionStore = sessions.NewCookieStore(bytes)
		}

		if d, err := time.ParseDuration(config.Keys.SessionMaxAge); err == nil {
			authInstance.SessionMaxAge = d
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
// This is used for both JWT and OIDC authentication when syncUserOnLogin or updateUserOnLogin is enabled.
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

func (auth *Authentication) SaveSession(rw http.ResponseWriter, r *http.Request, user *schema.User) error {
	session, err := auth.sessionStore.New(r, "session")
	if err != nil {
		cclog.Errorf("session creation failed: %s", err.Error())
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return err
	}

	if auth.SessionMaxAge != 0 {
		session.Options.MaxAge = int(auth.SessionMaxAge.Seconds())
	}
	if config.Keys.HTTPSCertFile == "" && config.Keys.HTTPSKeyFile == "" {
		cclog.Warn("HTTPS not configured - session cookies will not have Secure flag set (insecure for production)")
		session.Options.Secure = false
	}
	session.Options.SameSite = http.SameSiteStrictMode
	session.Values["username"] = user.Username
	session.Values["projects"] = user.Projects
	session.Values["roles"] = user.Roles
	if err := auth.sessionStore.Save(r, rw, session); err != nil {
		cclog.Warnf("session save failed: %s", err.Error())
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return err
	}

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

			if r.FormValue("redirect") != "" {
				http.RedirectHandler(r.FormValue("redirect"), http.StatusFound).ServeHTTP(rw, r.WithContext(ctx))
				return
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

		cclog.Info("auth -> authentication failed")
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
				if user.HasRole(schema.RoleApi) {
					ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
					onsuccess.ServeHTTP(rw, r.WithContext(ctx))
					return
				}
			case len(user.Roles) >= 2:
				if user.HasAllRoles([]schema.Role{schema.RoleAdmin, schema.RoleApi}) {
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
				if user.HasRole(schema.RoleApi) {
					ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
					onsuccess.ServeHTTP(rw, r.WithContext(ctx))
					return
				}
			case len(user.Roles) >= 2:
				if user.HasRole(schema.RoleApi) && user.HasAnyRole([]schema.Role{schema.RoleUser, schema.RoleManager, schema.RoleSupport, schema.RoleAdmin}) {
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
				if user.HasRole(schema.RoleApi) {
					ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
					onsuccess.ServeHTTP(rw, r.WithContext(ctx))
					return
				}
			case len(user.Roles) >= 2:
				if user.HasRole(schema.RoleApi) && user.HasAnyRole([]schema.Role{schema.RoleUser, schema.RoleManager, schema.RoleAdmin}) {
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
		session, err := auth.sessionStore.Get(r, "session")
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		if !session.IsNew {
			session.Options.MaxAge = -1
			if err := auth.sessionStore.Save(r, rw, session); err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}
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

	// If nothing declared in config: deny all request to this api endpoint
	if len(config.Keys.APIAllowedIPs) == 0 {
		return fmt.Errorf("missing configuration key ApiAllowedIPs")
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
