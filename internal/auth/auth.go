// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
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
		log.Error("Error while getting session store")
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
			log.Warn("environment variable 'SESSION_KEY' not set (will use non-persistent random key)")
			bytes := make([]byte, 32)
			if _, err := rand.Read(bytes); err != nil {
				log.Fatal("Error while initializing authentication -> failed to generate random bytes for session key")
			}
			authInstance.sessionStore = sessions.NewCookieStore(bytes)
		} else {
			bytes, err := base64.StdEncoding.DecodeString(sessKey)
			if err != nil {
				log.Fatal("Error while initializing authentication -> decoding session key failed")
			}
			authInstance.sessionStore = sessions.NewCookieStore(bytes)
		}

		if d, err := time.ParseDuration(config.Keys.SessionMaxAge); err == nil {
			authInstance.SessionMaxAge = d
		}

		if config.Keys.LdapConfig != nil {
			ldapAuth := &LdapAuthenticator{}
			if err := ldapAuth.Init(); err != nil {
				log.Warn("Error while initializing authentication -> ldapAuth init failed")
			} else {
				authInstance.LdapAuth = ldapAuth
				authInstance.authenticators = append(authInstance.authenticators, authInstance.LdapAuth)
			}
		} else {
			log.Info("Missing LDAP configuration: No LDAP support!")
		}

		if config.Keys.JwtConfig != nil {
			authInstance.JwtAuth = &JWTAuthenticator{}
			if err := authInstance.JwtAuth.Init(); err != nil {
				log.Fatal("Error while initializing authentication -> jwtAuth init failed")
			}

			jwtSessionAuth := &JWTSessionAuthenticator{}
			if err := jwtSessionAuth.Init(); err != nil {
				log.Info("jwtSessionAuth init failed: No JWT login support!")
			} else {
				authInstance.authenticators = append(authInstance.authenticators, jwtSessionAuth)
			}

			jwtCookieSessionAuth := &JWTCookieSessionAuthenticator{}
			if err := jwtCookieSessionAuth.Init(); err != nil {
				log.Info("jwtCookieSessionAuth init failed: No JWT cookie login support!")
			} else {
				authInstance.authenticators = append(authInstance.authenticators, jwtCookieSessionAuth)
			}
		} else {
			log.Info("Missing JWT configuration: No JWT token support!")
		}

		authInstance.LocalAuth = &LocalAuthenticator{}
		if err := authInstance.LocalAuth.Init(); err != nil {
			log.Fatal("Error while initializing authentication -> localAuth init failed")
		}
		authInstance.authenticators = append(authInstance.authenticators, authInstance.LocalAuth)
	})
}

func GetAuthInstance() *Authentication {
	if authInstance == nil {
		log.Fatal("Authentication module not initialized!")
	}

	return authInstance
}

func handleTokenUser(tokenUser *schema.User) {
	r := repository.GetUserRepository()
	dbUser, err := r.GetUser(tokenUser.Username)

	if err != nil && err != sql.ErrNoRows {
		log.Errorf("Error while loading user '%s': %v", tokenUser.Username, err)
	} else if err == sql.ErrNoRows && config.Keys.JwtConfig.SyncUserOnLogin { // Adds New User
		if err := r.AddUser(tokenUser); err != nil {
			log.Errorf("Error while adding user '%s' to DB: %v", tokenUser.Username, err)
		}
	} else if err == nil && config.Keys.JwtConfig.UpdateUserOnLogin { // Update Existing User
		if err := r.UpdateUser(dbUser, tokenUser); err != nil {
			log.Errorf("Error while updating user '%s' to DB: %v", dbUser.Username, err)
		}
	}
}

func handleOIDCUser(OIDCUser *schema.User) {
	r := repository.GetUserRepository()
	dbUser, err := r.GetUser(OIDCUser.Username)

	if err != nil && err != sql.ErrNoRows {
		log.Errorf("Error while loading user '%s': %v", OIDCUser.Username, err)
	} else if err == sql.ErrNoRows && config.Keys.OpenIDConfig.SyncUserOnLogin { // Adds New User
		if err := r.AddUser(OIDCUser); err != nil {
			log.Errorf("Error while adding user '%s' to DB: %v", OIDCUser.Username, err)
		}
	} else if err == nil && config.Keys.OpenIDConfig.UpdateUserOnLogin { // Update Existing User
		if err := r.UpdateUser(dbUser, OIDCUser); err != nil {
			log.Errorf("Error while updating user '%s' to DB: %v", dbUser.Username, err)
		}
	}
}

func (auth *Authentication) SaveSession(rw http.ResponseWriter, r *http.Request, user *schema.User) error {
	session, err := auth.sessionStore.New(r, "session")
	if err != nil {
		log.Errorf("session creation failed: %s", err.Error())
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
		log.Warnf("session save failed: %s", err.Error())
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return err
	}

	return nil
}

func (auth *Authentication) Login(
	onfailure func(rw http.ResponseWriter, r *http.Request, loginErr error),
) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		username := r.FormValue("username")
		var dbUser *schema.User

		if username != "" {
			var err error
			dbUser, err = repository.GetUserRepository().GetUser(username)
			if err != nil && err != sql.ErrNoRows {
				log.Errorf("Error while loading user '%v'", username)
			}
		}

		for _, authenticator := range auth.authenticators {
			var ok bool
			var user *schema.User
			if user, ok = authenticator.CanLogin(dbUser, username, rw, r); !ok {
				continue
			} else {
				log.Debugf("Can login with user %v", user)
			}

			user, err := authenticator.Login(user, rw, r)
			if err != nil {
				log.Warnf("user login failed: %s", err.Error())
				onfailure(rw, r, err)
				return
			}

			if err := auth.SaveSession(rw, r, user); err != nil {
				return
			}

			log.Infof("login successfull: user: %#v (roles: %v, projects: %v)", user.Username, user.Roles, user.Projects)
			ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)

			if r.FormValue("redirect") != "" {
				http.RedirectHandler(r.FormValue("redirect"), http.StatusFound).ServeHTTP(rw, r.WithContext(ctx))
				return
			}

			http.RedirectHandler("/", http.StatusFound).ServeHTTP(rw, r.WithContext(ctx))
			return
		}

		log.Debugf("login failed: no authenticator applied")
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
			log.Infof("auth -> authentication failed: %s", err.Error())
			http.Error(rw, err.Error(), http.StatusUnauthorized)
			return
		}
		if user == nil {
			user, err = auth.AuthViaSession(rw, r)
			if err != nil {
				log.Infof("auth -> authentication failed: %s", err.Error())
				http.Error(rw, err.Error(), http.StatusUnauthorized)
				return
			}
		}
		if user != nil {
			ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
			onsuccess.ServeHTTP(rw, r.WithContext(ctx))
			return
		}

		log.Info("auth -> authentication failed")
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
			log.Infof("auth api -> authentication failed: %s", err.Error())
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
				if user.HasAllRoles([]schema.Role{schema.RoleAdmin, schema.RoleApi}) {
					ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
					onsuccess.ServeHTTP(rw, r.WithContext(ctx))
					return
				}
			default:
				log.Info("auth api -> authentication failed: missing role")
				onfailure(rw, r, errors.New("unauthorized"))
			}
		}
		log.Info("auth api -> authentication failed: no auth")
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
			log.Infof("auth user api -> authentication failed: %s", err.Error())
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
				log.Info("auth user api -> authentication failed: missing role")
				onfailure(rw, r, errors.New("unauthorized"))
			}
		}
		log.Info("auth user api -> authentication failed: no auth")
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
			log.Infof("auth config api -> authentication failed: %s", err.Error())
			onfailure(rw, r, err)
			return
		}
		if user != nil && user.HasRole(schema.RoleAdmin) {
			ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
			onsuccess.ServeHTTP(rw, r.WithContext(ctx))
			return
		}
		log.Info("auth config api -> authentication failed: no auth")
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
			log.Infof("auth frontend api -> authentication failed: %s", err.Error())
			onfailure(rw, r, err)
			return
		}
		if user != nil {
			ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
			onsuccess.ServeHTTP(rw, r.WithContext(ctx))
			return
		}
		log.Info("auth frontend api -> authentication failed: no auth")
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
