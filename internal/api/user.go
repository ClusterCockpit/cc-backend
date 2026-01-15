// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/gorilla/mux"
)

type APIReturnedUser struct {
	Username string   `json:"username"`
	Name     string   `json:"name"`
	Roles    []string `json:"roles"`
	Email    string   `json:"email"`
	Projects []string `json:"projects"`
}

// getUsers godoc
// @summary     Returns a list of users
// @tags User
// @description Returns a JSON-encoded list of users.
// @description Required query-parameter defines if all users or only users with additional special roles are returned.
// @produce     json
// @param       not-just-user query bool true "If returned list should contain all users or only users with additional special roles"
// @success     200     {array} api.APIReturnedUser "List of users returned successfully"
// @failure     400     {string} string             "Bad Request"
// @failure     401     {string} string             "Unauthorized"
// @failure     403     {string} string             "Forbidden"
// @failure     500     {string} string             "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/users/ [get]
func (api *RestAPI) getUsers(rw http.ResponseWriter, r *http.Request) {
	// SecuredCheck() only worked with TokenAuth: Removed

	if user := repository.GetUserFromContext(r.Context()); !user.HasRole(schema.RoleAdmin) {
		handleError(fmt.Errorf("only admins are allowed to fetch a list of users"), http.StatusForbidden, rw)
		return
	}

	users, err := repository.GetUserRepository().ListUsers(r.URL.Query().Get("not-just-user") == "true")
	if err != nil {
		handleError(fmt.Errorf("listing users failed: %w", err), http.StatusInternalServerError, rw)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(rw).Encode(users); err != nil {
		cclog.Errorf("Failed to encode users response: %v", err)
	}
}

// updateUser godoc
// @summary     Update user roles and projects
// @tags User
// @description Allows admins to add/remove roles and projects for a user
// @produce     plain
// @param       id          path   string  true  "Username"
// @param       add-role    formData string false "Role to add"
// @param       remove-role formData string false "Role to remove"
// @param       add-project formData string false "Project to add"
// @param       remove-project formData string false "Project to remove"
// @success     200     {string} string "Success message"
// @failure     403     {object} api.ErrorResponse "Forbidden"
// @failure     422     {object} api.ErrorResponse "Unprocessable Entity"
// @security    ApiKeyAuth
// @router      /api/user/{id} [post]
func (api *RestAPI) updateUser(rw http.ResponseWriter, r *http.Request) {
	// SecuredCheck() only worked with TokenAuth: Removed

	if user := repository.GetUserFromContext(r.Context()); !user.HasRole(schema.RoleAdmin) {
		handleError(fmt.Errorf("only admins are allowed to update a user"), http.StatusForbidden, rw)
		return
	}

	// Get Values
	newrole := r.FormValue("add-role")
	delrole := r.FormValue("remove-role")
	newproj := r.FormValue("add-project")
	delproj := r.FormValue("remove-project")

	rw.Header().Set("Content-Type", "application/json")

	// Handle role updates
	if newrole != "" {
		if err := repository.GetUserRepository().AddRole(r.Context(), mux.Vars(r)["id"], newrole); err != nil {
			handleError(fmt.Errorf("adding role failed: %w", err), http.StatusUnprocessableEntity, rw)
			return
		}
		if err := json.NewEncoder(rw).Encode(DefaultAPIResponse{Message: "Add Role Success"}); err != nil {
			cclog.Errorf("Failed to encode response: %v", err)
		}
	} else if delrole != "" {
		if err := repository.GetUserRepository().RemoveRole(r.Context(), mux.Vars(r)["id"], delrole); err != nil {
			handleError(fmt.Errorf("removing role failed: %w", err), http.StatusUnprocessableEntity, rw)
			return
		}
		if err := json.NewEncoder(rw).Encode(DefaultAPIResponse{Message: "Remove Role Success"}); err != nil {
			cclog.Errorf("Failed to encode response: %v", err)
		}
	} else if newproj != "" {
		if err := repository.GetUserRepository().AddProject(r.Context(), mux.Vars(r)["id"], newproj); err != nil {
			handleError(fmt.Errorf("adding project failed: %w", err), http.StatusUnprocessableEntity, rw)
			return
		}
		if err := json.NewEncoder(rw).Encode(DefaultAPIResponse{Message: "Add Project Success"}); err != nil {
			cclog.Errorf("Failed to encode response: %v", err)
		}
	} else if delproj != "" {
		if err := repository.GetUserRepository().RemoveProject(r.Context(), mux.Vars(r)["id"], delproj); err != nil {
			handleError(fmt.Errorf("removing project failed: %w", err), http.StatusUnprocessableEntity, rw)
			return
		}
		if err := json.NewEncoder(rw).Encode(DefaultAPIResponse{Message: "Remove Project Success"}); err != nil {
			cclog.Errorf("Failed to encode response: %v", err)
		}
	} else {
		handleError(fmt.Errorf("no operation specified: must provide add-role, remove-role, add-project, or remove-project"), http.StatusBadRequest, rw)
	}
}

// createUser godoc
// @summary     Create a new user
// @tags User
// @description Creates a new user with specified credentials and role
// @produce     plain
// @param       username formData string true  "Username"
// @param       password formData string false "Password (not required for API users)"
// @param       role     formData string true  "User role"
// @param       name     formData string false "Full name"
// @param       email    formData string false "Email address"
// @param       project  formData string false "Project (required for managers)"
// @success     200     {string} string "Success message"
// @failure     400     {object} api.ErrorResponse "Bad Request"
// @failure     403     {object} api.ErrorResponse "Forbidden"
// @failure     422     {object} api.ErrorResponse "Unprocessable Entity"
// @security    ApiKeyAuth
// @router      /api/users/ [post]
func (api *RestAPI) createUser(rw http.ResponseWriter, r *http.Request) {
	// SecuredCheck() only worked with TokenAuth: Removed

	rw.Header().Set("Content-Type", "text/plain")
	me := repository.GetUserFromContext(r.Context())
	if !me.HasRole(schema.RoleAdmin) {
		handleError(fmt.Errorf("only admins are allowed to create new users"), http.StatusForbidden, rw)
		return
	}

	username, password, role, name, email, project := r.FormValue("username"),
		r.FormValue("password"), r.FormValue("role"), r.FormValue("name"),
		r.FormValue("email"), r.FormValue("project")

	// Validate username length
	if len(username) == 0 || len(username) > 100 {
		handleError(fmt.Errorf("username must be between 1 and 100 characters"), http.StatusBadRequest, rw)
		return
	}

	if len(password) == 0 && role != schema.GetRoleString(schema.RoleApi) {
		handleError(fmt.Errorf("only API users are allowed to have a blank password (login will be impossible)"), http.StatusBadRequest, rw)
		return
	}

	if len(project) != 0 && role != schema.GetRoleString(schema.RoleManager) {
		handleError(fmt.Errorf("only managers require a project (can be changed later)"), http.StatusBadRequest, rw)
		return
	} else if len(project) == 0 && role == schema.GetRoleString(schema.RoleManager) {
		handleError(fmt.Errorf("managers require a project to manage (can be changed later)"), http.StatusBadRequest, rw)
		return
	}

	if err := repository.GetUserRepository().AddUser(&schema.User{
		Username: username,
		Name:     name,
		Password: password,
		Email:    email,
		Projects: []string{project},
		Roles:    []string{role},
	}); err != nil {
		handleError(fmt.Errorf("adding user failed: %w", err), http.StatusUnprocessableEntity, rw)
		return
	}

	fmt.Fprintf(rw, "User %v successfully created!\n", username)
}

// deleteUser godoc
// @summary     Delete a user
// @tags User
// @description Deletes a user from the system
// @produce     plain
// @param       username formData string true "Username to delete"
// @success     200     {string} string "Success"
// @failure     403     {object} api.ErrorResponse "Forbidden"
// @failure     422     {object} api.ErrorResponse "Unprocessable Entity"
// @security    ApiKeyAuth
// @router      /api/users/ [delete]
func (api *RestAPI) deleteUser(rw http.ResponseWriter, r *http.Request) {
	// SecuredCheck() only worked with TokenAuth: Removed

	if user := repository.GetUserFromContext(r.Context()); !user.HasRole(schema.RoleAdmin) {
		handleError(fmt.Errorf("only admins are allowed to delete a user"), http.StatusForbidden, rw)
		return
	}

	username := r.FormValue("username")
	if err := repository.GetUserRepository().DelUser(username); err != nil {
		handleError(fmt.Errorf("deleting user failed: %w", err), http.StatusUnprocessableEntity, rw)
		return
	}

	rw.WriteHeader(http.StatusOK)
}
