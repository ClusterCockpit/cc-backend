// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/go-ldap/ldap/v3"
)

type LdapConfig struct {
	URL             string `json:"url"`
	UserBase        string `json:"user-base"`
	SearchDN        string `json:"search-dn"`
	UserBind        string `json:"user-bind"`
	UserFilter      string `json:"user-filter"`
	UserAttr        string `json:"username-attr"`
	UidAttr         string `json:"uid-attr"`
	SyncInterval    string `json:"sync-interval"` // Parsed using time.ParseDuration.
	SyncDelOldUsers bool   `json:"sync-del-old-users"`

	// Should a non-existent user be added to the DB if user exists in ldap directory
	SyncUserOnLogin   bool `json:"sync-user-on-login"`
	UpdateUserOnLogin bool `json:"update-user-on-login"`
}

type LdapAuthenticator struct {
	syncPassword string
	UserAttr     string
	UidAttr      string
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

	if Keys.LdapConfig.UidAttr != "" {
		la.UidAttr = Keys.LdapConfig.UidAttr
	} else {
		la.UidAttr = "uid"
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
	} else if lc.SyncUserOnLogin {
		l, err := la.getLdapConnection(true)
		if err != nil {
			cclog.Error("LDAP connection error")
			return nil, false
		}
		defer l.Close()

		// Search for the given username
		searchRequest := ldap.NewSearchRequest(
			lc.UserBase,
			ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
			fmt.Sprintf("(&%s(%s=%s))", lc.UserFilter, la.UidAttr, ldap.EscapeFilter(username)),
			[]string{"dn", la.UidAttr, la.UserAttr}, nil)

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
		user = &schema.User{
			Username:   username,
			Name:       entry.GetAttributeValue(la.UserAttr),
			Roles:      []string{schema.GetRoleString(schema.RoleUser)},
			Projects:   make([]string, 0),
			AuthType:   schema.AuthSession,
			AuthSource: schema.AuthViaLDAP,
		}

		handleLdapUser(user)
		return user, true
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

	userDn := strings.ReplaceAll(Keys.LdapConfig.UserBind, "{username}", ldap.EscapeDN(user.Username))
	if err := l.Bind(userDn, r.FormValue("password")); err != nil {
		cclog.Errorf("AUTH/LDAP > Authentication for user %s failed: %v",
			user.Username, err)
		return nil, fmt.Errorf("Authentication failed")
	}

	return user, nil
}

func (la *LdapAuthenticator) Sync() error {
	const InDB int = 1
	const InLdap int = 2
	const InBoth int = 3
	ur := repository.GetUserRepository()
	lc := Keys.LdapConfig

	users := map[string]int{}
	usernames, err := ur.GetLdapUsernames()
	if err != nil {
		return err
	}

	for _, username := range usernames {
		users[username] = InDB
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
		[]string{"dn", la.UidAttr, la.UserAttr}, nil))
	if err != nil {
		cclog.Warn("LDAP search error")
		return err
	}

	newnames := map[string]string{}
	for _, entry := range ldapResults.Entries {
		username := entry.GetAttributeValue(la.UidAttr)
		if username == "" {
			return fmt.Errorf("no attribute '%s'", la.UidAttr)
		}

		_, ok := users[username]
		if !ok {
			users[username] = InLdap
			newnames[username] = entry.GetAttributeValue(la.UserAttr)
		} else {
			users[username] = InBoth
		}
	}

	for username, where := range users {
		if where == InDB && lc.SyncDelOldUsers {
			if err := ur.DelUser(username); err != nil {
				cclog.Errorf("User '%s' LDAP: Delete from DB failed: %v", username, err)
				return err
			}
			cclog.Debugf("sync: remove %v (does not show up in LDAP anymore)", username)
		} else if where == InLdap {
			name := newnames[username]

			user := &schema.User{
				Username:   username,
				Name:       name,
				Roles:      []string{schema.GetRoleString(schema.RoleUser)},
				Projects:   make([]string, 0),
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
	conn, err := ldap.DialURL(lc.URL,
		ldap.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}))
	if err != nil {
		cclog.Warn("LDAP URL dial failed")
		return nil, err
	}
	conn.SetTimeout(30 * time.Second)

	if admin {
		if err := conn.Bind(lc.SearchDN, la.syncPassword); err != nil {
			conn.Close()
			cclog.Warn("LDAP connection bind failed")
			return nil, err
		}
	}

	return conn, nil
}
