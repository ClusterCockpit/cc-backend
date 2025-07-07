// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package auth

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/go-ldap/ldap/v3"
)

type LdapConfig struct {
	Url             string `json:"url"`
	UserBase        string `json:"user_base"`
	SearchDN        string `json:"search_dn"`
	UserBind        string `json:"user_bind"`
	UserFilter      string `json:"user_filter"`
	UserAttr        string `json:"username_attr"`
	SyncInterval    string `json:"sync_interval"` // Parsed using time.ParseDuration.
	SyncDelOldUsers bool   `json:"sync_del_old_users"`

	// Should an non-existent user be added to the DB if user exists in ldap directory
	SyncUserOnLogin bool `json:"syncUserOnLogin"`
}

type LdapAuthenticator struct {
	syncPassword string
	UserAttr     string
}

var _ Authenticator = (*LdapAuthenticator)(nil)

func (la *LdapAuthenticator) Init() error {
	la.syncPassword = os.Getenv("LDAP_ADMIN_PASSWORD")
	if la.syncPassword == "" {
		cclog.Warn("environment variable 'LDAP_ADMIN_PASSWORD' not set (ldap sync will not work)")
	}

	if Keys.LdapConfig.UserAttr != "" {
		la.UserAttr = Keys.LdapConfig.UserAttr
	} else {
		la.UserAttr = "gecos"
	}

	return nil
}

func (la *LdapAuthenticator) CanLogin(
	user *schema.User,
	username string,
	rw http.ResponseWriter,
	r *http.Request,
) (*schema.User, bool) {
	lc := Keys.LdapConfig

	if user != nil {
		if user.AuthSource == schema.AuthViaLDAP {
			return user, true
		}
	} else {
		if lc.SyncUserOnLogin {
			l, err := la.getLdapConnection(true)
			if err != nil {
				cclog.Error("LDAP connection error")
			}
			defer l.Close()

			// Search for the given username
			searchRequest := ldap.NewSearchRequest(
				lc.UserBase,
				ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
				fmt.Sprintf("(&%s(uid=%s))", lc.UserFilter, username),
				[]string{"dn", "uid", la.UserAttr}, nil)

			sr, err := l.Search(searchRequest)
			if err != nil {
				cclog.Warn(err)
				return nil, false
			}

			if len(sr.Entries) != 1 {
				cclog.Warn("LDAP: User does not exist or too many entries returned")
				return nil, false
			}

			entry := sr.Entries[0]
			name := entry.GetAttributeValue(la.UserAttr)
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
				cclog.Errorf("User '%s' LDAP: Insert into DB failed", username)
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
	r *http.Request,
) (*schema.User, error) {
	l, err := la.getLdapConnection(false)
	if err != nil {
		cclog.Warn("Error while getting ldap connection")
		return nil, err
	}
	defer l.Close()

	userDn := strings.Replace(Keys.LdapConfig.UserBind, "{username}", user.Username, -1)
	if err := l.Bind(userDn, r.FormValue("password")); err != nil {
		cclog.Errorf("AUTH/LDAP > Authentication for user %s failed: %v",
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
	lc := Keys.LdapConfig

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
		cclog.Error("LDAP connection error")
		return err
	}
	defer l.Close()

	ldapResults, err := l.Search(ldap.NewSearchRequest(
		lc.UserBase,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		lc.UserFilter,
		[]string{"dn", "uid", la.UserAttr}, nil))
	if err != nil {
		cclog.Warn("LDAP search error")
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
			newnames[username] = entry.GetAttributeValue(la.UserAttr)
		} else {
			users[username] = IN_BOTH
		}
	}

	for username, where := range users {
		if where == IN_DB && lc.SyncDelOldUsers {
			ur.DelUser(username)
			cclog.Debugf("sync: remove %v (does not show up in LDAP anymore)", username)
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

			cclog.Debugf("sync: add %v (name: %v, roles: [user], ldap: true)", username, name)
			if err := ur.AddUser(user); err != nil {
				cclog.Errorf("User '%s' LDAP: Insert into DB failed", username)
				return err
			}
		}
	}

	return nil
}

func (la *LdapAuthenticator) getLdapConnection(admin bool) (*ldap.Conn, error) {
	lc := Keys.LdapConfig
	conn, err := ldap.DialURL(lc.Url)
	if err != nil {
		cclog.Warn("LDAP URL dial failed")
		return nil, err
	}

	if admin {
		if err := conn.Bind(lc.SearchDN, la.syncPassword); err != nil {
			conn.Close()
			cclog.Warn("LDAP connection bind failed")
			return nil, err
		}
	}

	return conn, nil
}
