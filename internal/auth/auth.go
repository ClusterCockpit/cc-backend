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
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
)

type AuthSource int

const (
	AuthViaLocalPassword AuthSource = iota
	AuthViaLDAP
	AuthViaToken
)

type User struct {
	Username   string     `json:"username"`
	Password   string     `json:"-"`
	Name       string     `json:"name"`
	Roles      []string   `json:"roles"`
	AuthSource AuthSource `json:"via"`
	Email      string     `json:"email"`
	Projects   []string   `json:"projects"`
	Expiration time.Time
}

type Role int

const (
	RoleAnonymous Role = iota
	RoleApi
	RoleUser
	RoleManager
	RoleSupport
	RoleAdmin
	RoleError
)

func GetRoleString(roleInt Role) string {
	return [6]string{"anonymous", "api", "user", "manager", "support", "admin"}[roleInt]
}

func getRoleEnum(roleStr string) Role {
	switch strings.ToLower(roleStr) {
	case "admin":
		return RoleAdmin
	case "support":
		return RoleSupport
	case "manager":
		return RoleManager
	case "user":
		return RoleUser
	case "api":
		return RoleApi
	case "anonymous":
		return RoleAnonymous
	default:
		return RoleError
	}
}

func isValidRole(role string) bool {
	return getRoleEnum(role) != RoleError
}

func (u *User) HasValidRole(role string) (hasRole bool, isValid bool) {
	if isValidRole(role) {
		for _, r := range u.Roles {
			if r == role {
				return true, true
			}
		}
		return false, true
	}
	return false, false
}

func (u *User) HasRole(role Role) bool {
	for _, r := range u.Roles {
		if r == GetRoleString(role) {
			return true
		}
	}
	return false
}

// Role-Arrays are short: performance not impacted by nested loop
func (u *User) HasAnyRole(queryroles []Role) bool {
	for _, ur := range u.Roles {
		for _, qr := range queryroles {
			if ur == GetRoleString(qr) {
				return true
			}
		}
	}
	return false
}

// Role-Arrays are short: performance not impacted by nested loop
func (u *User) HasAllRoles(queryroles []Role) bool {
	target := len(queryroles)
	matches := 0
	for _, ur := range u.Roles {
		for _, qr := range queryroles {
			if ur == GetRoleString(qr) {
				matches += 1
				break
			}
		}
	}

	if matches == target {
		return true
	} else {
		return false
	}
}

// Role-Arrays are short: performance not impacted by nested loop
func (u *User) HasNotRoles(queryroles []Role) bool {
	matches := 0
	for _, ur := range u.Roles {
		for _, qr := range queryroles {
			if ur == GetRoleString(qr) {
				matches += 1
				break
			}
		}
	}

	if matches == 0 {
		return true
	} else {
		return false
	}
}

// Called by API endpoint '/roles/' from frontend: Only required for admin config -> Check Admin Role
func GetValidRoles(user *User) ([]string, error) {
	var vals []string
	if user.HasRole(RoleAdmin) {
		for i := RoleApi; i < RoleError; i++ {
			vals = append(vals, GetRoleString(i))
		}
		return vals, nil
	}

	return vals, fmt.Errorf("%s: only admins are allowed to fetch a list of roles", user.Username)
}

// Called by routerConfig web.page setup in backend: Only requires known user
func GetValidRolesMap(user *User) (map[string]Role, error) {
	named := make(map[string]Role)
	if user.HasNotRoles([]Role{RoleAnonymous}) {
		for i := RoleApi; i < RoleError; i++ {
			named[GetRoleString(i)] = i
		}
		return named, nil
	}
	return named, fmt.Errorf("only known users are allowed to fetch a list of roles")
}

// Find highest role
func (u *User) GetAuthLevel() Role {
	if u.HasRole(RoleAdmin) {
		return RoleAdmin
	} else if u.HasRole(RoleSupport) {
		return RoleSupport
	} else if u.HasRole(RoleManager) {
		return RoleManager
	} else if u.HasRole(RoleUser) {
		return RoleUser
	} else if u.HasRole(RoleApi) {
		return RoleApi
	} else if u.HasRole(RoleAnonymous) {
		return RoleAnonymous
	} else {
		return RoleError
	}
}

func (u *User) HasProject(project string) bool {
	for _, p := range u.Projects {
		if p == project {
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
	LdapAuth       *LdapAuthenticator
	JwtAuth        *JWTAuthenticator
	LocalAuth      *LocalAuthenticator
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

	auth.LocalAuth = &LocalAuthenticator{}
	if err := auth.LocalAuth.Init(auth, nil); err != nil {
		log.Error("Error while initializing authentication -> localAuth init failed")
		return nil, err
	}
	auth.authenticators = append(auth.authenticators, auth.LocalAuth)

	auth.JwtAuth = &JWTAuthenticator{}
	if err := auth.JwtAuth.Init(auth, configs["jwt"]); err != nil {
		log.Error("Error while initializing authentication -> jwtAuth init failed")
		return nil, err
	}
	auth.authenticators = append(auth.authenticators, auth.JwtAuth)

	if config, ok := configs["ldap"]; ok {
		auth.LdapAuth = &LdapAuthenticator{}
		if err := auth.LdapAuth.Init(auth, config); err != nil {
			log.Error("Error while initializing authentication -> ldapAuth init failed")
			return nil, err
		}
		auth.authenticators = append(auth.authenticators, auth.LdapAuth)
	}

	return auth, nil
}

func (auth *Authentication) AuthViaSession(
	rw http.ResponseWriter,
	r *http.Request) (*User, error) {

	session, err := auth.sessionStore.Get(r, "session")
	if err != nil {
		log.Error("Error while getting session store")
		return nil, err
	}

	if session.IsNew {
		return nil, nil
	}

	// TODO Check if keys are present in session?
	username, _ := session.Values["username"].(string)
	projects, _ := session.Values["projects"].([]string)
	roles, _ := session.Values["roles"].([]string)
	return &User{
		Username:   username,
		Projects:   projects,
		Roles:      roles,
		AuthSource: -1,
	}, nil
}

// Handle a POST request that should log the user in, starting a new session.
func (auth *Authentication) Login(
	onsuccess http.Handler,
	onfailure func(rw http.ResponseWriter, r *http.Request, loginErr error)) http.Handler {

	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		err := errors.New("no authenticator applied")
		username := r.FormValue("username")
		user := (*User)(nil)

		if username != "" {
			user, _ = auth.GetUser(username)
		}

		for _, authenticator := range auth.authenticators {
			if !authenticator.CanLogin(user, rw, r) {
				continue
			}

			user, err = authenticator.Login(user, rw, r)
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

			log.Infof("login successfull: user: %#v (roles: %v, projects: %v)", user.Username, user.Roles, user.Projects)
			ctx := context.WithValue(r.Context(), ContextUserKey, user)
			onsuccess.ServeHTTP(rw, r.WithContext(ctx))
			return
		}

		log.Debugf("login failed: no authenticator applied")
		onfailure(rw, r, err)
	})
}

// Authenticate the user and put a User object in the
// context of the request. If authentication fails,
// do not continue but send client to the login screen.
func (auth *Authentication) Auth(
	onsuccess http.Handler,
	onfailure func(rw http.ResponseWriter, r *http.Request, authErr error)) http.Handler {

	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		for _, authenticator := range auth.authenticators {
			user, err := authenticator.Auth(rw, r)
			if err != nil {
				log.Infof("authentication failed: %s", err.Error())
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

		log.Debugf("authentication failed: %s", "no authenticator applied")
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
