// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
)

const (
	RoleAdmin string = "admin"
	RoleApi   string = "api"
	RoleUser  string = "user"
)

const (
	AuthViaLocalPassword int8 = 0
	AuthViaLDAP          int8 = 1
	AuthViaToken         int8 = 2
)

type User struct {
	Username   string   `json:"username"`
	Password   string   `json:"-"`
	Name       string   `json:"name"`
	Roles      []string `json:"roles"`
	AuthSource int8     `json:"via"`
	Email      string   `json:"email"`
	Expiration time.Time
}

func (u *User) HasRole(role string) bool {
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

func GetUser(ctx context.Context) *User {
	x := ctx.Value(ContextUserKey)
	if x == nil {
		return nil
	}

	return x.(*User)
}

type Authenticator interface {
	Init(auth *Authentication, config interface{}) error
	CanLogin(user *User, rw http.ResponseWriter, r *http.Request) bool
	Login(user *User, rw http.ResponseWriter, r *http.Request) (*User, error)
	Auth(rw http.ResponseWriter, r *http.Request) (*User, error)
}

type ContextKey string

const ContextUserKey ContextKey = "user"

type Authentication struct {
	db            *sqlx.DB
	sessionStore  *sessions.CookieStore
	SessionMaxAge time.Duration

	authenticators []Authenticator
	LdapAuth       *LdapAutnenticator
	JwtAuth        *JWTAuthenticator
	LocalAuth      *LocalAuthenticator
}

func Init(db *sqlx.DB,
	configs map[string]interface{}) (*Authentication, error) {
	auth := &Authentication{}
	auth.db = db
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS user (
		username varchar(255) PRIMARY KEY NOT NULL,
		password varchar(255) DEFAULT NULL,
		ldap     tinyint      NOT NULL DEFAULT 0, /* col called "ldap" for historic reasons, fills the "AuthSource" */
		name     varchar(255) DEFAULT NULL,
		roles    varchar(255) NOT NULL DEFAULT "[]",
		email    varchar(255) DEFAULT NULL);`)
	if err != nil {
		return nil, err
	}

	sessKey := os.Getenv("SESSION_KEY")
	if sessKey == "" {
		log.Warn("environment variable 'SESSION_KEY' not set (will use non-persistent random key)")
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			return nil, err
		}
		auth.sessionStore = sessions.NewCookieStore(bytes)
	} else {
		bytes, err := base64.StdEncoding.DecodeString(sessKey)
		if err != nil {
			return nil, err
		}
		auth.sessionStore = sessions.NewCookieStore(bytes)
	}

	auth.LocalAuth = &LocalAuthenticator{}
	if err := auth.LocalAuth.Init(auth, nil); err != nil {
		return nil, err
	}
	auth.authenticators = append(auth.authenticators, auth.LocalAuth)

	auth.JwtAuth = &JWTAuthenticator{}
	if err := auth.JwtAuth.Init(auth, configs["jwt"]); err != nil {
		return nil, err
	}
	auth.authenticators = append(auth.authenticators, auth.JwtAuth)

	if config, ok := configs["ldap"]; ok {
		auth.LdapAuth = &LdapAutnenticator{}
		if err := auth.LdapAuth.Init(auth, config); err != nil {
			return nil, err
		}
		auth.authenticators = append(auth.authenticators, auth.LdapAuth)
	}

	return auth, nil
}

func (auth *Authentication) AuthViaSession(rw http.ResponseWriter, r *http.Request) (*User, error) {
	session, err := auth.sessionStore.Get(r, "session")
	if err != nil {
		return nil, err
	}

	if session.IsNew {
		return nil, nil
	}

	username, _ := session.Values["username"].(string)
	roles, _ := session.Values["roles"].([]string)
	return &User{
		Username:   username,
		Roles:      roles,
		AuthSource: -1,
	}, nil
}

// Handle a POST request that should log the user in, starting a new session.
func (auth *Authentication) Login(onsuccess http.Handler, onfailure func(rw http.ResponseWriter, r *http.Request, loginErr error)) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var err error = errors.New("no authenticator applied")
		username := r.FormValue("username")
		user := (*User)(nil)
		if username != "" {
			if user, _ = auth.GetUser(username); err != nil {
				// log.Warnf("login of unkown user %#v", username)
				_ = err
			}
		}

		for _, authenticator := range auth.authenticators {
			if !authenticator.CanLogin(user, rw, r) {
				continue
			}

			user, err = authenticator.Login(user, rw, r)
			if err != nil {
				log.Warnf("login failed: %s", err.Error())
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
			session.Values["roles"] = user.Roles
			if err := auth.sessionStore.Save(r, rw, session); err != nil {
				log.Errorf("session save failed: %s", err.Error())
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			log.Infof("login successfull: user: %#v (roles: %v)", user.Username, user.Roles)
			ctx := context.WithValue(r.Context(), ContextUserKey, user)
			onsuccess.ServeHTTP(rw, r.WithContext(ctx))
			return
		}

		log.Warn("login failed: no authenticator applied")
		onfailure(rw, r, err)
	})
}

// Authenticate the user and put a User object in the
// context of the request. If authentication fails,
// do not continue but send client to the login screen.
func (auth *Authentication) Auth(onsuccess http.Handler, onfailure func(rw http.ResponseWriter, r *http.Request, authErr error)) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		for _, authenticator := range auth.authenticators {
			user, err := authenticator.Auth(rw, r)
			if err != nil {
				log.Warnf("authentication failed: %s", err.Error())
				http.Error(rw, err.Error(), http.StatusUnauthorized)
				return
			}
			if user == nil {
				continue
			}

			ctx := context.WithValue(r.Context(), ContextUserKey, user)
			onsuccess.ServeHTTP(rw, r.WithContext(ctx))
			return
		}

		log.Warnf("authentication failed: %s", "no authenticator applied")
		// http.Error(rw, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		onfailure(rw, r, errors.New("unauthorized (login first or use a token)"))
	})
}

// Clears the session cookie
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
