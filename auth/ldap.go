package auth

import (
	"crypto/tls"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/log"

	"github.com/go-ldap/ldap/v3"
)

type LdapConfig struct {
	Url        string `json:"url"`
	UserBase   string `json:"user_base"`
	SearchDN   string `json:"search_dn"`
	UserBind   string `json:"user_bind"`
	UserFilter string `json:"user_filter"`
	TLS        bool   `json:"tls"`

	// Parsed using time.ParseDuration.
	SyncInterval    string `json:"sync_interval"`
	SyncDelOldUsers bool   `json:"sync_del_old_users"`
}

func (auth *Authentication) initLdap() error {
	auth.ldapSyncUserPassword = os.Getenv("LDAP_ADMIN_PASSWORD")
	if auth.ldapSyncUserPassword == "" {
		log.Warn("environment variable 'LDAP_ADMIN_PASSWORD' not set (ldap sync or authentication will not work)")
	}

	if auth.ldapConfig.SyncInterval != "" {
		interval, err := time.ParseDuration(auth.ldapConfig.SyncInterval)
		if err != nil {
			return err
		}

		if interval == 0 {
			return nil
		}

		go func() {
			ticker := time.NewTicker(interval)
			for t := range ticker.C {
				log.Printf("LDAP sync started at %s", t.Format(time.RFC3339))
				if err := auth.SyncWithLDAP(auth.ldapConfig.SyncDelOldUsers); err != nil {
					log.Errorf("LDAP sync failed: %s", err.Error())
				}
				log.Print("LDAP sync done")
			}
		}()
	}

	return nil
}

// TODO: Add a connection pool or something like
// that so that connections can be reused/cached.
func (auth *Authentication) getLdapConnection(admin bool) (*ldap.Conn, error) {
	conn, err := ldap.DialURL(auth.ldapConfig.Url)
	if err != nil {
		return nil, err
	}

	if auth.ldapConfig.TLS {
		if err := conn.StartTLS(&tls.Config{InsecureSkipVerify: true}); err != nil {
			conn.Close()
			return nil, err
		}
	}

	if admin {
		if err := conn.Bind(auth.ldapConfig.SearchDN, auth.ldapSyncUserPassword); err != nil {
			conn.Close()
			return nil, err
		}
	}

	return conn, nil
}

func (auth *Authentication) loginViaLdap(user *User, password string) error {
	l, err := auth.getLdapConnection(false)
	if err != nil {
		return err
	}
	defer l.Close()

	userDn := strings.Replace(auth.ldapConfig.UserBind, "{username}", user.Username, -1)
	if err := l.Bind(userDn, password); err != nil {
		return err
	}

	user.ViaLdap = true
	return nil
}

// Delete users where user.ldap is 1 and that do not show up in the ldap search results.
// Add users to the users table that are new in the ldap search results.
func (auth *Authentication) SyncWithLDAP(deleteOldUsers bool) error {
	if auth.ldapConfig == nil {
		return errors.New("ldap not enabled")
	}

	const IN_DB int = 1
	const IN_LDAP int = 2
	const IN_BOTH int = 3

	users := map[string]int{}
	rows, err := auth.db.Query(`SELECT username FROM user WHERE user.ldap = 1`)
	if err != nil {
		return err
	}

	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return err
		}

		users[username] = IN_DB
	}

	l, err := auth.getLdapConnection(true)
	if err != nil {
		return err
	}
	defer l.Close()

	ldapResults, err := l.Search(ldap.NewSearchRequest(
		auth.ldapConfig.UserBase, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		auth.ldapConfig.UserFilter, []string{"dn", "uid", "gecos"}, nil))
	if err != nil {
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
		if where == IN_DB && deleteOldUsers {
			log.Infof("ldap-sync: remove %#v (does not show up in LDAP anymore)", username)
			if _, err := auth.db.Exec(`DELETE FROM user WHERE user.username = ?`, username); err != nil {
				return err
			}
		} else if where == IN_LDAP {
			name := newnames[username]
			log.Infof("ldap-sync: add %#v (name: %#v, roles: [], ldap: true)", username, name)
			if _, err := auth.db.Exec(`INSERT INTO user (username, ldap, name, roles) VALUES (?, ?, ?, ?)`,
				username, 1, name, "[]"); err != nil {
				return err
			}
		}
	}

	return nil
}
