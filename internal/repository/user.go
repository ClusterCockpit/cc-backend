// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
)

// Authentication and Role System:
//
// ClusterCockpit supports multiple authentication sources:
//   - Local: Username/password stored in database (password hashed with bcrypt)
//   - LDAP: External LDAP/Active Directory authentication
//   - JWT: Token-based authentication for API access
//
// Role Hierarchy (from highest to lowest privilege):
//   1. "admin" - Full system access, can manage all users and jobs
//   2. "support" - Can view all jobs but limited management capabilities
//   3. "manager" - Can manage specific projects and their users
//   4. "api" - Programmatic access for job submission/management
//   5. "user" - Default role, can only view own jobs
//
// Project Association:
//   - Managers have a list of projects they oversee
//   - Regular users' project membership is determined by job data
//   - Managers can view/manage all jobs within their projects

var (
	userRepoOnce     sync.Once
	userRepoInstance *UserRepository
)

type UserRepository struct {
	DB     *sqlx.DB
	driver string
}

func GetUserRepository() *UserRepository {
	userRepoOnce.Do(func() {
		db := GetConnection()

		userRepoInstance = &UserRepository{
			DB:     db.DB,
			driver: db.Driver,
		}
	})
	return userRepoInstance
}

// GetUser retrieves a user by username from the database.
// Returns the complete user record including hashed password, roles, and projects.
// Password field contains bcrypt hash for local auth users, empty for LDAP users.
func (r *UserRepository) GetUser(username string) (*schema.User, error) {
	user := &schema.User{Username: username}
	var hashedPassword, name, rawRoles, email, rawProjects sql.NullString
	if err := sq.Select("password", "ldap", "name", "roles", "email", "projects").From("hpc_user").
		Where("hpc_user.username = ?", username).RunWith(r.DB).
		QueryRow().Scan(&hashedPassword, &user.AuthSource, &name, &rawRoles, &email, &rawProjects); err != nil {
		cclog.Warnf("Error while querying user '%v' from database", username)
		return nil, err
	}

	user.Password = hashedPassword.String
	user.Name = name.String
	user.Email = email.String
	if rawRoles.Valid {
		if err := json.Unmarshal([]byte(rawRoles.String), &user.Roles); err != nil {
			cclog.Warn("Error while unmarshaling raw roles from DB")
			return nil, err
		}
	}
	if rawProjects.Valid {
		if err := json.Unmarshal([]byte(rawProjects.String), &user.Projects); err != nil {
			return nil, err
		}
	}

	return user, nil
}

func (r *UserRepository) GetLdapUsernames() ([]string, error) {
	var users []string
	rows, err := r.DB.Query(`SELECT username FROM hpc_user WHERE hpc_user.ldap = 1`)
	if err != nil {
		cclog.Warn("Error while querying usernames")
		return nil, err
	}

	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			cclog.Warnf("Error while scanning for user '%s'", username)
			return nil, err
		}

		users = append(users, username)
	}

	return users, nil
}

// AddUser creates a new user in the database.
// Passwords are automatically hashed with bcrypt before storage.
// Auth source determines authentication method (local, LDAP, etc.).
//
// Required fields: Username, Roles
// Optional fields: Name, Email, Password, Projects, AuthSource
func (r *UserRepository) AddUser(user *schema.User) error {
	rolesJson, _ := json.Marshal(user.Roles)
	projectsJson, _ := json.Marshal(user.Projects)

	cols := []string{"username", "roles", "projects"}
	vals := []any{user.Username, string(rolesJson), string(projectsJson)}

	if user.Name != "" {
		cols = append(cols, "name")
		vals = append(vals, user.Name)
	}
	if user.Email != "" {
		cols = append(cols, "email")
		vals = append(vals, user.Email)
	}
	if user.Password != "" {
		password, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
		if err != nil {
			cclog.Error("Error while encrypting new user password")
			return err
		}
		cols = append(cols, "password")
		vals = append(vals, string(password))
	}
	if user.AuthSource != -1 {
		cols = append(cols, "ldap")
		vals = append(vals, int(user.AuthSource))
	}

	if _, err := sq.Insert("hpc_user").Columns(cols...).Values(vals...).RunWith(r.DB).Exec(); err != nil {
		cclog.Errorf("Error while inserting new user '%v' into DB", user.Username)
		return err
	}

	cclog.Infof("new user %#v created (roles: %s, auth-source: %d, projects: %s)", user.Username, rolesJson, user.AuthSource, projectsJson)

	// DEPRECATED: SUPERSEDED BY NEW USER CONFIG - userConfig.go / web.go
	defaultMetricsCfg, err := config.LoadDefaultMetricsConfig()
	if err != nil {
		cclog.Errorf("Error loading default metrics config: %v", err)
	} else if defaultMetricsCfg != nil {
		for _, cluster := range defaultMetricsCfg.Clusters {
			metricsArray := config.ParseMetricsString(cluster.DefaultMetrics)
			metricsJSON, err := json.Marshal(metricsArray)
			if err != nil {
				cclog.Errorf("Error marshaling default metrics for cluster %s: %v", cluster.Name, err)
				continue
			}
			// Note: StatisticsTable now has different key (metricConfig_jobViewTableMetrics): Not updated here.
			confKey := "metricConfig_jobViewPlotMetrics:" + cluster.Name
			if _, err := sq.Insert("configuration").
				Columns("username", "confkey", "value").
				Values(user.Username, confKey, string(metricsJSON)).
				RunWith(r.DB).Exec(); err != nil {
				cclog.Errorf("Error inserting default job view metrics for user %s and cluster %s: %v", user.Username, cluster.Name, err)
			} else {
				cclog.Infof("Default job view metrics for user %s and cluster %s set to %s", user.Username, cluster.Name, string(metricsJSON))
			}
		}
	}
	// END DEPRECATION

	return nil
}

func (r *UserRepository) UpdateUser(dbUser *schema.User, user *schema.User) error {
	// user contains updated info -> Apply to dbUser
	// --- Simple Name Update ---
	if dbUser.Name != user.Name {
		if _, err := sq.Update("hpc_user").Set("name", user.Name).Where("hpc_user.username = ?", dbUser.Username).RunWith(r.DB).Exec(); err != nil {
			cclog.Errorf("error while updating name of user '%s'", user.Username)
			return err
		}
	}

	// --- Def Helpers ---
	// Helper to update roles
	updateRoles := func(roles []string) error {
		rolesJSON, _ := json.Marshal(roles)
		_, err := sq.Update("hpc_user").Set("roles", rolesJSON).Where("hpc_user.username = ?", dbUser.Username).RunWith(r.DB).Exec()
		return err
	}

	// Helper to update projects
	updateProjects := func(projects []string) error {
		projectsJSON, _ := json.Marshal(projects)
		_, err := sq.Update("hpc_user").Set("projects", projectsJSON).Where("hpc_user.username = ?", dbUser.Username).RunWith(r.DB).Exec()
		return err
	}

	// Helper to clear projects
	clearProjects := func() error {
		_, err := sq.Update("hpc_user").Set("projects", "[]").Where("hpc_user.username = ?", dbUser.Username).RunWith(r.DB).Exec()
		return err
	}

	// --- Manager Role Handling ---
	if dbUser.HasRole(schema.RoleManager) && user.HasRole(schema.RoleManager) && !reflect.DeepEqual(dbUser.Projects, user.Projects) {
		// Existing Manager: update projects
		if err := updateProjects(user.Projects); err != nil {
			return err
		}
	} else if dbUser.HasRole(schema.RoleUser) && user.HasRole(schema.RoleManager) && user.HasNotRoles([]schema.Role{schema.RoleAdmin}) {
		// New Manager: update roles and projects
		if err := updateRoles(user.Roles); err != nil {
			return err
		}
		if err := updateProjects(user.Projects); err != nil {
			return err
		}
	} else if dbUser.HasRole(schema.RoleManager) && user.HasNotRoles([]schema.Role{schema.RoleAdmin, schema.RoleManager}) {
		// Remove Manager: update roles and clear projects
		if err := updateRoles(user.Roles); err != nil {
			return err
		}
		if err := clearProjects(); err != nil {
			return err
		}
	}

	// --- Support Role Handling ---
	if dbUser.HasRole(schema.RoleUser) && dbUser.HasNotRoles([]schema.Role{schema.RoleSupport}) &&
		user.HasRole(schema.RoleSupport) && user.HasNotRoles([]schema.Role{schema.RoleAdmin}) {
		// New Support: update roles
		if err := updateRoles(user.Roles); err != nil {
			return err
		}
	} else if dbUser.HasRole(schema.RoleSupport) && user.HasNotRoles([]schema.Role{schema.RoleAdmin, schema.RoleSupport}) {
		// Remove Support: update roles
		if err := updateRoles(user.Roles); err != nil {
			return err
		}
	}

	return nil
}

func (r *UserRepository) DelUser(username string) error {
	_, err := r.DB.Exec(`DELETE FROM hpc_user WHERE hpc_user.username = ?`, username)
	if err != nil {
		cclog.Errorf("Error while deleting user '%s' from DB", username)
		return err
	}
	cclog.Infof("deleted user '%s' from DB", username)
	return nil
}

func (r *UserRepository) ListUsers(specialsOnly bool) ([]*schema.User, error) {
	q := sq.Select("username", "name", "email", "roles", "projects").From("hpc_user")
	if specialsOnly {
		q = q.Where("(roles != '[\"user\"]' AND roles != '[]')")
	}

	rows, err := q.RunWith(r.DB).Query()
	if err != nil {
		cclog.Warn("Error while querying user list")
		return nil, err
	}

	users := make([]*schema.User, 0)
	defer rows.Close()
	for rows.Next() {
		rawroles := ""
		rawprojects := ""
		user := &schema.User{}
		var name, email sql.NullString
		if err := rows.Scan(&user.Username, &name, &email, &rawroles, &rawprojects); err != nil {
			cclog.Warn("Error while scanning user list")
			return nil, err
		}

		if err := json.Unmarshal([]byte(rawroles), &user.Roles); err != nil {
			cclog.Warn("Error while unmarshaling raw role list")
			return nil, err
		}

		if err := json.Unmarshal([]byte(rawprojects), &user.Projects); err != nil {
			return nil, err
		}

		user.Name = name.String
		user.Email = email.String
		users = append(users, user)
	}
	return users, nil
}

// AddRole adds a role to a user's role list.
// Role string is automatically lowercased.
// Valid roles: admin, support, manager, api, user
//
// Returns error if:
//   - User doesn't exist
//   - Role is invalid
//   - User already has the role
func (r *UserRepository) AddRole(
	ctx context.Context,
	username string,
	queryrole string,
) error {
	newRole := strings.ToLower(queryrole)
	user, err := r.GetUser(username)
	if err != nil {
		cclog.Warnf("Could not load user '%s'", username)
		return err
	}

	exists, valid := user.HasValidRole(newRole)

	if !valid {
		return fmt.Errorf("supplied role is no valid option : %v", newRole)
	}
	if exists {
		return fmt.Errorf("user %v already has role %v", username, newRole)
	}

	roles, _ := json.Marshal(append(user.Roles, newRole))
	if _, err := sq.Update("hpc_user").Set("roles", roles).Where("hpc_user.username = ?", username).RunWith(r.DB).Exec(); err != nil {
		cclog.Errorf("error while adding new role for user '%s'", user.Username)
		return err
	}
	return nil
}

// RemoveRole removes a role from a user's role list.
//
// Special rules:
//   - Cannot remove "manager" role while user has assigned projects
//   - Must remove all projects first before removing manager role
func (r *UserRepository) RemoveRole(ctx context.Context, username string, queryrole string) error {
	oldRole := strings.ToLower(queryrole)
	user, err := r.GetUser(username)
	if err != nil {
		cclog.Warnf("Could not load user '%s'", username)
		return err
	}

	exists, valid := user.HasValidRole(oldRole)

	if !valid {
		return fmt.Errorf("supplied role is no valid option : %v", oldRole)
	}
	if !exists {
		return fmt.Errorf("role already deleted for user '%v': %v", username, oldRole)
	}

	if oldRole == schema.GetRoleString(schema.RoleManager) && len(user.Projects) != 0 {
		return fmt.Errorf("cannot remove role 'manager' while user %s still has assigned project(s) : %v", username, user.Projects)
	}

	var newroles []string
	for _, r := range user.Roles {
		if r != oldRole {
			newroles = append(newroles, r) // Append all roles not matching requested to be deleted role
		}
	}

	mroles, _ := json.Marshal(newroles)
	if _, err := sq.Update("hpc_user").Set("roles", mroles).Where("hpc_user.username = ?", username).RunWith(r.DB).Exec(); err != nil {
		cclog.Errorf("Error while removing role for user '%s'", user.Username)
		return err
	}
	return nil
}

// AddProject assigns a project to a manager user.
// Only users with the "manager" role can have assigned projects.
//
// Returns error if:
//   - User doesn't have manager role
//   - User already manages the project
func (r *UserRepository) AddProject(
	ctx context.Context,
	username string,
	project string,
) error {
	user, err := r.GetUser(username)
	if err != nil {
		return err
	}

	if !user.HasRole(schema.RoleManager) {
		return fmt.Errorf("user '%s' is not a manager", username)
	}

	if user.HasProject(project) {
		return fmt.Errorf("user '%s' already manages project '%s'", username, project)
	}

	projects, _ := json.Marshal(append(user.Projects, project))
	if _, err := sq.Update("hpc_user").Set("projects", projects).Where("hpc_user.username = ?", username).RunWith(r.DB).Exec(); err != nil {
		return err
	}

	return nil
}

func (r *UserRepository) RemoveProject(ctx context.Context, username string, project string) error {
	user, err := r.GetUser(username)
	if err != nil {
		return err
	}

	if !user.HasRole(schema.RoleManager) {
		return fmt.Errorf("user '%#v' is not a manager", username)
	}

	if !user.HasProject(project) {
		return fmt.Errorf("user '%#v': Cannot remove project '%#v' - Does not match", username, project)
	}

	var exists bool
	var newprojects []string
	for _, p := range user.Projects {
		if p != project {
			newprojects = append(newprojects, p) // Append all projects not matching requested to be deleted project
		} else {
			exists = true
		}
	}

	if exists {
		var result any
		if len(newprojects) == 0 {
			result = "[]"
		} else {
			result, _ = json.Marshal(newprojects)
		}
		if _, err := sq.Update("hpc_user").Set("projects", result).Where("hpc_user.username = ?", username).RunWith(r.DB).Exec(); err != nil {
			return err
		}
		return nil
	} else {
		return fmt.Errorf("user %s already does not manage project %s", username, project)
	}
}

type ContextKey string

const ContextUserKey ContextKey = "user"

func GetUserFromContext(ctx context.Context) *schema.User {
	x := ctx.Value(ContextUserKey)
	if x == nil {
		cclog.Warnf("no user retrieved from context")
		return nil
	}
	// cclog.Infof("user retrieved from context: %v", x.(*schema.User))
	return x.(*schema.User)
}

func (r *UserRepository) FetchUserInCtx(ctx context.Context, username string) (*model.User, error) {
	me := GetUserFromContext(ctx)
	if me != nil && me.Username != username &&
		me.HasNotRoles([]schema.Role{schema.RoleAdmin, schema.RoleSupport, schema.RoleManager}) {
		return nil, errors.New("forbidden")
	}

	user := &model.User{Username: username}
	var name, email sql.NullString
	if err := sq.Select("name", "email").From("hpc_user").Where("hpc_user.username = ?", username).
		RunWith(r.DB).QueryRow().Scan(&name, &email); err != nil {
		if err == sql.ErrNoRows {
			/* This warning will be logged *often* for non-local users, i.e. users mentioned only in job-table or archive, */
			/* since FetchUser will be called to retrieve full name and mail for every job in query/list									 */
			// cclog.Warnf("User '%s' Not found in DB", username)
			return nil, nil
		}

		cclog.Warnf("Error while fetching user '%s'", username)
		return nil, err
	}

	user.Name = name.String
	user.Email = email.String
	return user, nil
}
