package auth

import (
	"crypto/tls"
	"errors"
	"os"
	"strings"

	"github.com/ClusterCockpit/cc-backend/log"

	"github.com/go-ldap/ldap/v3"
	"github.com/jmoiron/sqlx"
)

type LdapConfig struct {
	Url        string `json:"url"`
	UserBase   string `json:"user_base"`
	SearchDN   string `json:"search_dn"`
	UserBind   string `json:"user_bind"`
	UserFilter string `json:"user_filter"`
	TLS        bool   `json:"tls"`
}

var ldapAuthEnabled bool = false
var ldapConfig *LdapConfig
var ldapAdminPassword string

func initLdap(config *LdapConfig) error {
	ldapAdminPassword = os.Getenv("LDAP_ADMIN_PASSWORD")
	if ldapAdminPassword == "" {
		log.Warn("environment variable 'LDAP_ADMIN_PASSWORD' not set (ldap sync or authentication will not work)")
	}

	ldapConfig = config
	ldapAuthEnabled = true
	return nil
}

// TODO: Add a connection pool or something like
// that so that connections can be reused/cached.
func getLdapConnection(admin bool) (*ldap.Conn, error) {
	conn, err := ldap.DialURL(ldapConfig.Url)
	if err != nil {
		return nil, err
	}

	if ldapConfig.TLS {
		if err := conn.StartTLS(&tls.Config{InsecureSkipVerify: true}); err != nil {
			conn.Close()
			return nil, err
		}
	}

	if admin {
		if err := conn.Bind(ldapConfig.SearchDN, ldapAdminPassword); err != nil {
			conn.Close()
			return nil, err
		}
	}

	return conn, nil
}

func releaseConnection(conn *ldap.Conn) {
	conn.Close()
}

func loginViaLdap(user *User, password string) error {
	l, err := getLdapConnection(false)
	if err != nil {
		return err
	}
	defer releaseConnection(l)

	userDn := strings.Replace(ldapConfig.UserBind, "{username}", user.Username, -1)
	if err := l.Bind(userDn, password); err != nil {
		return err
	}

	user.ViaLdap = true
	return nil
}

// Delete users where user.ldap is 1 and that do not show up in the ldap search results.
// Add users to the users table that are new in the ldap search results.
func SyncWithLDAP(db *sqlx.DB) error {
	if !ldapAuthEnabled {
		return errors.New("ldap not enabled")
	}

	const IN_DB int = 1
	const IN_LDAP int = 2
	const IN_BOTH int = 3

	users := map[string]int{}
	rows, err := db.Query(`SELECT username FROM user WHERE user.ldap = 1`)
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

	l, err := getLdapConnection(true)
	if err != nil {
		return err
	}
	defer releaseConnection(l)

	ldapResults, err := l.Search(ldap.NewSearchRequest(
		ldapConfig.UserBase, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		ldapConfig.UserFilter, []string{"dn", "uid", "gecos"}, nil))
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
		if where == IN_DB {
			log.Infof("ldap-sync: remove %#v (does not show up in LDAP anymore)", username)
			if _, err := db.Exec(`DELETE FROM user WHERE user.username = ?`, username); err != nil {
				return err
			}
		} else if where == IN_LDAP {
			name := newnames[username]
			log.Infof("ldap-sync: add %#v (name: %#v, roles: [], ldap: true)", username, name)
			if _, err := db.Exec(`INSERT INTO user (username, ldap, name, roles) VALUES (?, ?, ?, ?)`,
				username, 1, name, "[]"); err != nil {
				return err
			}
		}
	}

	return nil
}
