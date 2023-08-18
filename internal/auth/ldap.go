// Copyright (C) 2023 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package auth

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/go-ldap/ldap/v3"
)

type LdapAuthenticator struct {
	config       *schema.LdapConfig
	syncPassword string
}

var _ Authenticator = (*LdapAuthenticator)(nil)

func (la *LdapAuthenticator) Init(conf interface{}) error {

	la.config = conf.(*schema.LdapConfig)

	la.syncPassword = os.Getenv("LDAP_ADMIN_PASSWORD")
	if la.syncPassword == "" {
		log.Warn("environment variable 'LDAP_ADMIN_PASSWORD' not set (ldap sync will not work)")
	}

	if la.config != nil && la.config.SyncInterval != "" {
		interval, err := time.ParseDuration(la.config.SyncInterval)
		if err != nil {
			log.Warnf("Could not parse duration for sync interval: %v", la.config.SyncInterval)
			return err
		}

		if interval == 0 {
			log.Info("Sync interval is zero")
			return nil
		}

		go func() {
			ticker := time.NewTicker(interval)
			for t := range ticker.C {
				log.Printf("sync started at %s", t.Format(time.RFC3339))
				if err := la.Sync(); err != nil {
					log.Errorf("sync failed: %s", err.Error())
				}
				log.Print("sync done")
			}
		}()
	} else {
		log.Info("Missing LDAP configuration key sync_interval")
	}

	return nil
}

func (la *LdapAuthenticator) CanLogin(
	user *schema.User,
	username string,
	rw http.ResponseWriter,
	r *http.Request) (*schema.User, bool) {

	if user != nil {
		if user.AuthSource == schema.AuthViaLDAP {
			return user, true
		}
	} else {
		if la.config.SyncUserOnLogin {
			l, err := la.getLdapConnection(true)
			if err != nil {
				log.Error("LDAP connection error")
			}
			defer l.Close()

			// Search for the given username
			searchRequest := ldap.NewSearchRequest(
				la.config.UserBase,
				ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
				fmt.Sprintf("(&%s(uid=%s))", la.config.UserFilter, username),
				[]string{"dn", "uid", "gecos"}, nil)

			sr, err := l.Search(searchRequest)
			if err != nil {
				log.Warn(err)
				return nil, false
			}

			if len(sr.Entries) != 1 {
				log.Warn("LDAP: User does not exist or too many entries returned")
				return nil, false
			}

			entry := sr.Entries[0]
			name := entry.GetAttributeValue("gecos")
			var roles []string
			roles = append(roles, schema.GetRoleString(schema.RoleUser))
			projects := make([]string, 0)

			user = &schema.User{
				Username:   username,
				Name:       name,
				Roles:      roles,
				Projects:   projects,
				AuthType:   schema.AuthSession,
				AuthSource: schema.AuthViaLDAP,
			}

			if err := repository.GetUserRepository().AddUser(user); err != nil {
				log.Errorf("User '%s' LDAP: Insert into DB failed", username)
				return nil, false
			}

			return user, true
		}
	}

	return nil, false
}

func (la *LdapAuthenticator) Login(
	user *schema.User,
	rw http.ResponseWriter,
	r *http.Request) (*schema.User, error) {

	l, err := la.getLdapConnection(false)
	if err != nil {
		log.Warn("Error while getting ldap connection")
		return nil, err
	}
	defer l.Close()

	userDn := strings.Replace(la.config.UserBind, "{username}", user.Username, -1)
	if err := l.Bind(userDn, r.FormValue("password")); err != nil {
		log.Errorf("AUTH/LDAP > Authentication for user %s failed: %v",
			user.Username, err)
		return nil, fmt.Errorf("Authentication failed")
	}

	return user, nil
}

func (la *LdapAuthenticator) Sync() error {
	const IN_DB int = 1
	const IN_LDAP int = 2
	const IN_BOTH int = 3
	ur := repository.GetUserRepository()

	users := map[string]int{}
	usernames, err := ur.GetLdapUsernames()
	if err != nil {
		return err
	}

	for _, username := range usernames {
		users[username] = IN_DB
	}

	l, err := la.getLdapConnection(true)
	if err != nil {
		log.Error("LDAP connection error")
		return err
	}
	defer l.Close()

	ldapResults, err := l.Search(ldap.NewSearchRequest(
		la.config.UserBase,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		la.config.UserFilter,
		[]string{"dn", "uid", "gecos"}, nil))
	if err != nil {
		log.Warn("LDAP search error")
		return err
	}

	newnames := map[string]string{}
	for _, entry := range ldapResults.Entries {
		username := entry.GetAttributeValue("uid")
		if username == "" {
			return errors.New("no attribute 'uid'")
		}

		_, ok := users[username]
		if !ok {
			users[username] = IN_LDAP
			newnames[username] = entry.GetAttributeValue("gecos")
		} else {
			users[username] = IN_BOTH
		}
	}

	for username, where := range users {
		if where == IN_DB && la.config.SyncDelOldUsers {
			ur.DelUser(username)
			log.Debugf("sync: remove %v (does not show up in LDAP anymore)", username)
		} else if where == IN_LDAP {
			name := newnames[username]

			var roles []string
			roles = append(roles, schema.GetRoleString(schema.RoleUser))
			projects := make([]string, 0)

			user := &schema.User{
				Username:   username,
				Name:       name,
				Roles:      roles,
				Projects:   projects,
				AuthSource: schema.AuthViaLDAP,
			}

			log.Debugf("sync: add %v (name: %v, roles: [user], ldap: true)", username, name)
			if err := ur.AddUser(user); err != nil {
				log.Errorf("User '%s' LDAP: Insert into DB failed", username)
				return err
			}
		}
	}

	return nil
}

// TODO: Add a connection pool or something like
// that so that connections can be reused/cached.
func (la *LdapAuthenticator) getLdapConnection(admin bool) (*ldap.Conn, error) {

	conn, err := ldap.DialURL(la.config.Url)
	if err != nil {
		log.Warn("LDAP URL dial failed")
		return nil, err
	}

	if admin {
		if err := conn.Bind(la.config.SearchDN, la.syncPassword); err != nil {
			conn.Close()
			log.Warn("LDAP connection bind failed")
			return nil, err
		}
	}

	return conn, nil
}
