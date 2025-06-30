// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/ClusterCockpit/cc-lib/util"
	"github.com/gorilla/sessions"
)

type Authenticator interface {
	CanLogin(user *schema.User, username string, rw http.ResponseWriter, r *http.Request) (*schema.User, bool)
	Login(user *schema.User, rw http.ResponseWriter, r *http.Request) (*schema.User, error)
}

var (
	initOnce     sync.Once
	authInstance *Authentication
)

var ipUserLimiters sync.Map

func getIPUserLimiter(ip, username string) *rate.Limiter {
	key := ip + ":" + username
	limiter, ok := ipUserLimiters.Load(key)
	if !ok {
		newLimiter := rate.NewLimiter(rate.Every(time.Hour/10), 10)
		ipUserLimiters.Store(key, newLimiter)
		return newLimiter
	}
	return limiter.(*rate.Limiter)
}

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

	// TODO: Check if session keys exist
	username, _ := session.Values["username"].(string)
	projects, _ := session.Values["projects"].([]string)
	roles, _ := session.Values["roles"].([]string)
	return &schema.User{
		Username:   username,
		Projects:   projects,
		Roles:      roles,
		AuthType:   schema.AuthSession,
		AuthSource: -1,
	}, nil
}

func Init() {
	initOnce.Do(func() {
		authInstance = &Authentication{}

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

		if config.Keys.LdapConfig != nil {
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

		if config.Keys.JwtConfig != nil {
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

func handleTokenUser(tokenUser *schema.User) {
	r := repository.GetUserRepository()
	dbUser, err := r.GetUser(tokenUser.Username)

	if err != nil && err != sql.ErrNoRows {
		cclog.Errorf("Error while loading user '%s': %v", tokenUser.Username, err)
	} else if err == sql.ErrNoRows && config.Keys.JwtConfig.SyncUserOnLogin { // Adds New User
		if err := r.AddUser(tokenUser); err != nil {
			cclog.Errorf("Error while adding user '%s' to DB: %v", tokenUser.Username, err)
		}
	} else if err == nil && config.Keys.JwtConfig.UpdateUserOnLogin { // Update Existing User
		if err := r.UpdateUser(dbUser, tokenUser); err != nil {
			cclog.Errorf("Error while updating user '%s' to DB: %v", dbUser.Username, err)
		}
	}
}

func handleOIDCUser(OIDCUser *schema.User) {
	r := repository.GetUserRepository()
	dbUser, err := r.GetUser(OIDCUser.Username)

	if err != nil && err != sql.ErrNoRows {
		cclog.Errorf("Error while loading user '%s': %v", OIDCUser.Username, err)
	} else if err == sql.ErrNoRows && config.Keys.OpenIDConfig.SyncUserOnLogin { // Adds New User
		if err := r.AddUser(OIDCUser); err != nil {
			cclog.Errorf("Error while adding user '%s' to DB: %v", OIDCUser.Username, err)
		}
	} else if err == nil && config.Keys.OpenIDConfig.UpdateUserOnLogin { // Update Existing User
		if err := r.UpdateUser(dbUser, OIDCUser); err != nil {
			cclog.Errorf("Error while updating user '%s' to DB: %v", dbUser.Username, err)
		}
	}
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
	if config.Keys.HttpsCertFile == "" && config.Keys.HttpsKeyFile == "" {
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

func (auth *Authentication) AuthApi(
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

func (auth *Authentication) AuthUserApi(
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
				if user.HasRole(schema.RoleApi) && user.HasAnyRole([]schema.Role{schema.RoleUser, schema.RoleManager, schema.RoleAdmin}) {
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

func (auth *Authentication) AuthConfigApi(
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

func (auth *Authentication) AuthFrontendApi(
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

	if strings.Contains(IPAddress, ":") {
		IPAddress = strings.Split(IPAddress, ":")[0]
	}

	// If nothing declared in config: deny all request to this api endpoint
	if len(config.Keys.ApiAllowedIPs) == 0 {
		return fmt.Errorf("missing configuration key ApiAllowedIPs")
	}
	// If wildcard declared in config: Continue
	if config.Keys.ApiAllowedIPs[0] == "*" {
		return nil
	}
	// check if IP is allowed
	if !util.Contains(config.Keys.ApiAllowedIPs, IPAddress) {
		return fmt.Errorf("unknown ip: %v", IPAddress)
	}

	return nil
}
