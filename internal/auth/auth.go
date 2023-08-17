// Copyright (C) 2023 NHR@FAU, University Erlangen-Nuremberg.
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

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
)

type Authenticator interface {
	Init(auth *Authentication, config interface{}) error
	CanLogin(user *schema.User, username string, rw http.ResponseWriter, r *http.Request) bool
	Login(user *schema.User, rw http.ResponseWriter, r *http.Request) (*schema.User, error)
}

type Authentication struct {
	db            *sqlx.DB
	sessionStore  *sessions.CookieStore
	SessionMaxAge time.Duration

	authenticators []Authenticator
	LdapAuth       *LdapAuthenticator
	JwtAuth        *JWTAuthenticator
	LocalAuth      *LocalAuthenticator
}

func (auth *Authentication) AuthViaSession(
	rw http.ResponseWriter,
	r *http.Request) (*schema.User, error) {
	session, err := auth.sessionStore.Get(r, "session")
	if err != nil {
		log.Error("Error while getting session store")
		return nil, err
	}

	if session.IsNew {
		return nil, nil
	}
	//
	// var username string
	// var projects, roles []string
	//
	// if val, ok := session.Values["username"]; ok {
	// 	username, _ = val.(string)
	// } else {
	// 	return nil, errors.New("no key username in session")
	// }
	// if val, ok := session.Values["projects"]; ok {
	// 	projects, _ = val.([]string)
	// } else {
	// 	return nil, errors.New("no key projects in session")
	// }
	// if val, ok := session.Values["projects"]; ok {
	// 	roles, _ = val.([]string)
	// } else {
	// 	return nil, errors.New("no key roles in session")
	// }
	//
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

func Init(db *sqlx.DB,
	configs map[string]interface{}) (*Authentication, error) {
	auth := &Authentication{}
	auth.db = db

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

	auth.JwtAuth = &JWTAuthenticator{}
	if err := auth.JwtAuth.Init(auth, configs["jwt"]); err != nil {
		log.Error("Error while initializing authentication -> jwtAuth init failed")
		return nil, err
	}

	if config, ok := configs["ldap"]; ok {
		ldapAuth := &LdapAuthenticator{}
		if err := ldapAuth.Init(auth, config); err != nil {
			log.Warn("Error while initializing authentication -> ldapAuth init failed")
		} else {
			auth.LdapAuth = ldapAuth
			auth.authenticators = append(auth.authenticators, auth.LdapAuth)
		}
	}

	jwtSessionAuth := &JWTSessionAuthenticator{}
	if err := jwtSessionAuth.Init(auth, configs["jwt"]); err != nil {
		log.Warn("Error while initializing authentication -> jwtSessionAuth init failed")
	} else {
		auth.authenticators = append(auth.authenticators, jwtSessionAuth)
	}

	jwtCookieSessionAuth := &JWTCookieSessionAuthenticator{}
	if err := jwtCookieSessionAuth.Init(auth, configs["jwt"]); err != nil {
		log.Warn("Error while initializing authentication -> jwtCookieSessionAuth init failed")
	} else {
		auth.authenticators = append(auth.authenticators, jwtCookieSessionAuth)
	}

	auth.LocalAuth = &LocalAuthenticator{}
	if err := auth.LocalAuth.Init(auth, nil); err != nil {
		log.Error("Error while initializing authentication -> localAuth init failed")
		return nil, err
	}
	auth.authenticators = append(auth.authenticators, auth.LocalAuth)

	return auth, nil
}

func (auth *Authentication) Login(
	onsuccess http.Handler,
	onfailure func(rw http.ResponseWriter, r *http.Request, loginErr error)) http.Handler {

	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ur := repository.GetUserRepository()
		err := errors.New("no authenticator applied")
		username := r.FormValue("username")
		dbUser := (*schema.User)(nil)

		if username != "" {
			dbUser, err = ur.GetUser(username)
			if err != nil && err != sql.ErrNoRows {
				log.Errorf("Error while loading user '%v'", username)
			}
		}

		for _, authenticator := range auth.authenticators {
			if !authenticator.CanLogin(dbUser, username, rw, r) {
				continue
			}
			dbUser, err = ur.GetUser(username)
			if err != nil && err != sql.ErrNoRows {
				log.Errorf("Error while loading user '%v'", username)
			}

			user, err := authenticator.Login(dbUser, rw, r)
			if err != nil {
				log.Warnf("user login failed: %s", err.Error())
				onfailure(rw, r, err)
				return
			}

			session, err := auth.sessionStore.New(r, "session")
			if err != nil {
				log.Errorf("session creation failed: %s", err.Error())
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
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
				return
			}

			if dbUser == nil {
				if err := ur.AddUser(user); err != nil {
					// TODO Add AuthSource
					log.Errorf("Error while adding user '%v' to auth from XX",
						user.Username)
				}
			}

			log.Infof("login successfull: user: %#v (roles: %v, projects: %v)", user.Username, user.Roles, user.Projects)
			ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
			onsuccess.ServeHTTP(rw, r.WithContext(ctx))
			return
		}

		log.Debugf("login failed: no authenticator applied")
		onfailure(rw, r, err)
	})
}

func (auth *Authentication) Auth(
	onsuccess http.Handler,
	onfailure func(rw http.ResponseWriter, r *http.Request, authErr error)) http.Handler {

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
