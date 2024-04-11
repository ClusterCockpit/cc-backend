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

func Init() (*Authentication, error) {
	auth := &Authentication{}

	sessKey := os.Getenv("SESSION_KEY")
	if sessKey == "" {
		log.Warn("environment variable 'SESSION_KEY' not set (will use non-persistent random key)")
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			log.Error("Error while initializing authentication -> failed to generate random bytes for session key")
			return nil, err
		}
		auth.sessionStore = sessions.NewCookieStore(bytes)
	} else {
		bytes, err := base64.StdEncoding.DecodeString(sessKey)
		if err != nil {
			log.Error("Error while initializing authentication -> decoding session key failed")
			return nil, err
		}
		auth.sessionStore = sessions.NewCookieStore(bytes)
	}

	if config.Keys.LdapConfig != nil {
		ldapAuth := &LdapAuthenticator{}
		if err := ldapAuth.Init(); err != nil {
			log.Warn("Error while initializing authentication -> ldapAuth init failed")
		} else {
			auth.LdapAuth = ldapAuth
			auth.authenticators = append(auth.authenticators, auth.LdapAuth)
		}
	} else {
		log.Info("Missing LDAP configuration: No LDAP support!")
	}

	if config.Keys.JwtConfig != nil {
		auth.JwtAuth = &JWTAuthenticator{}
		if err := auth.JwtAuth.Init(); err != nil {
			log.Error("Error while initializing authentication -> jwtAuth init failed")
			return nil, err
		}

		jwtSessionAuth := &JWTSessionAuthenticator{}
		if err := jwtSessionAuth.Init(); err != nil {
			log.Info("jwtSessionAuth init failed: No JWT login support!")
		} else {
			auth.authenticators = append(auth.authenticators, jwtSessionAuth)
		}

		jwtCookieSessionAuth := &JWTCookieSessionAuthenticator{}
		if err := jwtCookieSessionAuth.Init(); err != nil {
			log.Info("jwtCookieSessionAuth init failed: No JWT cookie login support!")
		} else {
			auth.authenticators = append(auth.authenticators, jwtCookieSessionAuth)
		}
	} else {
		log.Info("Missing JWT configuration: No JWT token support!")
	}

	auth.LocalAuth = &LocalAuthenticator{}
	if err := auth.LocalAuth.Init(); err != nil {
		log.Error("Error while initializing authentication -> localAuth init failed")
		return nil, err
	}
	auth.authenticators = append(auth.authenticators, auth.LocalAuth)

	return auth, nil
}

func persistUser(user *schema.User) {
	r := repository.GetUserRepository()
	_, err := r.GetUser(user.Username)

	if err != nil && err != sql.ErrNoRows {
		log.Errorf("Error while loading user '%s': %v", user.Username, err)
	} else if err == sql.ErrNoRows {
		if err := r.AddUser(user); err != nil {
			log.Errorf("Error while adding user '%s' to DB: %v", user.Username, err)
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
	onsuccess http.Handler,
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
			onsuccess.ServeHTTP(rw, r.WithContext(ctx))
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
			log.Infof("authentication failed: %s", err.Error())
			http.Error(rw, err.Error(), http.StatusUnauthorized)
			return
		}

		if user == nil {
			user, err = auth.AuthViaSession(rw, r)
			if err != nil {
				log.Infof("authentication failed: %s", err.Error())
				http.Error(rw, err.Error(), http.StatusUnauthorized)
				return
			}
		}

		if user != nil {
			ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
			onsuccess.ServeHTTP(rw, r.WithContext(ctx))
			return
		}

		log.Debug("authentication failed")
		onfailure(rw, r, errors.New("unauthorized (please login first)"))
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
