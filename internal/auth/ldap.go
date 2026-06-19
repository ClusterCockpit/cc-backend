// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"fmt"
	"net"
	"net/http"
	"slices"
	"sort"
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
	UIDAttr         string `json:"uid-attr"`
	SyncInterval    string `json:"sync-interval"` // Parsed using time.ParseDuration.
	SyncDelOldUsers bool   `json:"sync-del-old-users"`

	// Should a non-existent user be added to the DB if user exists in ldap directory
	SyncUserOnLogin   bool `json:"sync-user-on-login"`
	UpdateUserOnLogin bool `json:"update-user-on-login"`

	// Password for the LDAP admin account used for syncing (optional).
	// Overridden by the LDAP_ADMIN_PASSWORD environment variable when set.
	SyncPassword string `json:"sync-password"`

	// Maps an elevated role (admin/support/api/manager) to an LDAP filter.
	// An account matching the filter is granted that role; LDAP is authoritative
	// for every role listed here (it is both added and removed to match group
	// membership). Roles not listed here are never touched. Empty/absent means
	// roles are never modified by LDAP.
	RoleFilters map[string]string `json:"role-filters"`
}

type LdapAuthenticator struct {
	syncPassword string
	UserAttr     string
	UIDAttr      string

	// roleFilters holds the validated subset of LdapConfig.RoleFilters.
	roleFilters map[string]string
	// managedRoles is the sorted list of roles LDAP is authoritative for
	// (the keys of roleFilters). Empty when no role filters are configured.
	managedRoles []string
}

var _ Authenticator = (*LdapAuthenticator)(nil)

func (la *LdapAuthenticator) Init() error {
	la.syncPassword = secretFromEnv("LDAP_ADMIN_PASSWORD", Keys.LdapConfig.SyncPassword)
	if la.syncPassword == "" {
		cclog.Warn("LDAP admin password not configured ('sync-password' in config or 'LDAP_ADMIN_PASSWORD' env): ldap sync will not work")
	}

	if Keys.LdapConfig.UserAttr != "" {
		la.UserAttr = Keys.LdapConfig.UserAttr
	} else {
		la.UserAttr = "gecos"
	}

	if Keys.LdapConfig.UIDAttr != "" {
		la.UIDAttr = Keys.LdapConfig.UIDAttr
	} else {
		la.UIDAttr = "uid"
	}

	// Validate the optional role filters. Invalid keys are dropped with a
	// warning rather than failing startup. The baseline "user" role cannot be
	// LDAP-managed (it is always granted), and an empty filter is meaningless.
	la.roleFilters = make(map[string]string)
	for role, filter := range Keys.LdapConfig.RoleFilters {
		role = strings.ToLower(role)
		if !schema.IsValidRole(role) || role == schema.GetRoleString(schema.RoleUser) ||
			role == schema.GetRoleString(schema.RoleAnonymous) {
			cclog.Warnf("LDAP: ignoring role-filter for invalid or non-assignable role '%s'", role)
			continue
		}
		if strings.TrimSpace(filter) == "" {
			cclog.Warnf("LDAP: ignoring empty role-filter for role '%s'", role)
			continue
		}
		la.roleFilters[role] = filter
		la.managedRoles = append(la.managedRoles, role)
	}
	sort.Strings(la.managedRoles)

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
			// Refresh elevated roles from LDAP when role filters are configured
			// and role updates on login are enabled. Without role filters this
			// stays a fast path with no extra LDAP query.
			if len(la.managedRoles) > 0 && lc.UpdateUserOnLogin {
				if l, err := la.getLdapConnection(true); err != nil {
					cclog.Warnf("LDAP: skipping role refresh for user '%s': connection error", user.Username)
				} else {
					defer l.Close()
					if matched, err := la.matchRoles(l, user.Username); err == nil {
						roles := mergeLdapRoles(user.Roles, la.managedRoles, matched, user.Projects)
						current := append([]string{}, user.Roles...)
						sort.Strings(current)
						if !slices.Equal(roles, current) {
							user.Roles = roles
							handleLdapUser(user)
						}
					}
				}
			}
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
			fmt.Sprintf("(&%s(%s=%s))", lc.UserFilter, la.UIDAttr, ldap.EscapeFilter(username)),
			[]string{"dn", la.UIDAttr, la.UserAttr}, nil)

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

		roles := []string{schema.GetRoleString(schema.RoleUser)}
		if len(la.managedRoles) > 0 {
			if matched, err := la.matchRoles(l, username); err == nil {
				roles = mergeLdapRoles(nil, la.managedRoles, matched, nil)
			}
		}

		user = &schema.User{
			Username:   username,
			Name:       entry.GetAttributeValue(la.UserAttr),
			Roles:      roles,
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
		[]string{"dn", la.UIDAttr, la.UserAttr}, nil))
	if err != nil {
		cclog.Warn("LDAP search error")
		return err
	}

	newnames := map[string]string{}
	for _, entry := range ldapResults.Entries {
		username := entry.GetAttributeValue(la.UIDAttr)
		if username == "" {
			return fmt.Errorf("no attribute '%s'", la.UIDAttr)
		}

		_, ok := users[username]
		if !ok {
			users[username] = InLdap
			newnames[username] = entry.GetAttributeValue(la.UserAttr)
		} else {
			users[username] = InBoth
		}
	}

	// Evaluate configured role filters once over the whole base. Empty when no
	// role filters are configured, in which case role handling is a no-op and
	// behaviour is identical to before.
	matched, err := la.matchRolesBulk(l)
	if err != nil {
		return err
	}

	// Current roles/projects of users that already hold a non-default role, so
	// existing users can be reconciled without a per-user lookup.
	currentRoles := map[string]*schema.User{}
	if len(la.managedRoles) > 0 {
		specials, err := ur.ListUsers(true)
		if err != nil {
			return err
		}
		for _, u := range specials {
			currentRoles[u.Username] = u
		}
	}

	userRole := schema.GetRoleString(schema.RoleUser)
	for username, where := range users {
		if where == InDB && lc.SyncDelOldUsers {
			if err := ur.DelUser(username); err != nil {
				cclog.Errorf("User '%s' LDAP: Delete from DB failed: %v", username, err)
				return err
			}
			cclog.Debugf("sync: remove %v (does not show up in LDAP anymore)", username)
		} else if where == InLdap {
			name := newnames[username]

			roles := []string{userRole}
			if len(la.managedRoles) > 0 {
				roles = mergeLdapRoles(nil, la.managedRoles, matched[username], nil)
			}

			user := &schema.User{
				Username:   username,
				Name:       name,
				Roles:      roles,
				Projects:   make([]string, 0),
				AuthSource: schema.AuthViaLDAP,
			}

			cclog.Debugf("sync: add %v (name: %v, roles: %v, ldap: true)", username, name, roles)
			if err := ur.AddUserIfNotExists(user); err != nil {
				cclog.Errorf("User '%s' LDAP: Insert into DB failed", username)
				return err
			}
		} else if where == InBoth && len(la.managedRoles) > 0 {
			// Reconcile elevated roles for existing users: LDAP is authoritative
			// for the managed roles, all other roles are preserved.
			cur := []string{userRole}
			var projects []string
			if u, ok := currentRoles[username]; ok {
				cur = u.Roles
				projects = u.Projects
			}

			roles := mergeLdapRoles(cur, la.managedRoles, matched[username], projects)
			sortedCur := append([]string{}, cur...)
			sort.Strings(sortedCur)
			if !slices.Equal(roles, sortedCur) {
				cclog.Debugf("sync: update %v roles %v -> %v", username, cur, roles)
				if err := ur.UpdateRoles(username, roles); err != nil {
					cclog.Errorf("User '%s' LDAP: role update failed: %v", username, err)
					return err
				}
			}
		}
	}

	return nil
}

// mergeLdapRoles computes the role set for an LDAP user. Every current role that
// LDAP does not manage is preserved, the baseline "user" role is always present,
// and the matched managed roles are added. managed is the set of roles LDAP is
// authoritative for; matched is the subset of those the account currently
// qualifies for. projects is used to guard manager removal: a manager that still
// has assigned projects keeps the role even if it is no longer matched (mirrors
// UserRepository.RemoveRole). The result is deduplicated and sorted.
func mergeLdapRoles(current, managed, matched, projects []string) []string {
	managedSet := make(map[string]bool, len(managed))
	for _, r := range managed {
		managedSet[r] = true
	}
	matchedSet := make(map[string]bool, len(matched))
	for _, r := range matched {
		matchedSet[r] = true
	}

	result := map[string]bool{schema.GetRoleString(schema.RoleUser): true}

	// Preserve roles LDAP does not manage (e.g. a manually granted manager).
	for _, r := range current {
		if !managedSet[r] {
			result[r] = true
		}
	}

	// Add managed roles the account currently qualifies for.
	for _, r := range managed {
		if matchedSet[r] {
			result[r] = true
		}
	}

	// Guard: do not strip a manager that still has assigned projects.
	managerRole := schema.GetRoleString(schema.RoleManager)
	if managedSet[managerRole] && !matchedSet[managerRole] && len(projects) > 0 &&
		slices.Contains(current, managerRole) {
		cclog.Warnf("LDAP: keeping role 'manager' despite no filter match: user still has assigned project(s): %v", projects)
		result[managerRole] = true
	}

	roles := make([]string, 0, len(result))
	for r := range result {
		roles = append(roles, r)
	}
	sort.Strings(roles)
	return roles
}

// matchRoles evaluates all configured role filters for a single user and returns
// the managed roles the account qualifies for. Used on the login path where only
// one user is inspected.
func (la *LdapAuthenticator) matchRoles(l *ldap.Conn, username string) ([]string, error) {
	if len(la.managedRoles) == 0 {
		return nil, nil
	}

	matched := make([]string, 0, len(la.managedRoles))
	for _, role := range la.managedRoles {
		filter := fmt.Sprintf("(&(%s=%s)%s)", la.UIDAttr, ldap.EscapeFilter(username), la.roleFilters[role])
		sr, err := l.Search(ldap.NewSearchRequest(
			Keys.LdapConfig.UserBase,
			ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
			filter,
			[]string{la.UIDAttr}, nil))
		if err != nil {
			cclog.Warnf("LDAP: role filter search for role '%s' failed: %v", role, err)
			return nil, err
		}
		if len(sr.Entries) > 0 {
			matched = append(matched, role)
		}
	}
	return matched, nil
}

// matchRolesBulk evaluates every configured role filter once over the whole user
// base and returns a username -> matched managed roles mapping. Used by Sync,
// it costs one LDAP search per configured role rather than one per user.
func (la *LdapAuthenticator) matchRolesBulk(l *ldap.Conn) (map[string][]string, error) {
	matched := map[string][]string{}
	if len(la.managedRoles) == 0 {
		return matched, nil
	}

	lc := Keys.LdapConfig
	for _, role := range la.managedRoles {
		filter := fmt.Sprintf("(&%s%s)", lc.UserFilter, la.roleFilters[role])
		sr, err := l.Search(ldap.NewSearchRequest(
			lc.UserBase,
			ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
			filter,
			[]string{la.UIDAttr}, nil))
		if err != nil {
			cclog.Warnf("LDAP: role filter search for role '%s' failed: %v", role, err)
			return nil, err
		}
		for _, entry := range sr.Entries {
			if username := entry.GetAttributeValue(la.UIDAttr); username != "" {
				matched[username] = append(matched[username], role)
			}
		}
	}
	return matched, nil
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
