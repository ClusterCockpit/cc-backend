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
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/gorilla/mux"
)

type ApiReturnedUser struct {
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
// @success     200     {array} api.ApiReturnedUser "List of users returned successfully"
// @failure     400     {string} string             "Bad Request"
// @failure     401     {string} string             "Unauthorized"
// @failure     403     {string} string             "Forbidden"
// @failure     500     {string} string             "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/users/ [get]
func (api *RestApi) getUsers(rw http.ResponseWriter, r *http.Request) {
	// SecuredCheck() only worked with TokenAuth: Removed

	if user := repository.GetUserFromContext(r.Context()); !user.HasRole(schema.RoleAdmin) {
		http.Error(rw, "Only admins are allowed to fetch a list of users", http.StatusForbidden)
		return
	}

	users, err := repository.GetUserRepository().ListUsers(r.URL.Query().Get("not-just-user") == "true")
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(rw).Encode(users)
}

func (api *RestApi) updateUser(rw http.ResponseWriter, r *http.Request) {
	// SecuredCheck() only worked with TokenAuth: Removed

	if user := repository.GetUserFromContext(r.Context()); !user.HasRole(schema.RoleAdmin) {
		http.Error(rw, "Only admins are allowed to update a user", http.StatusForbidden)
		return
	}

	// Get Values
	newrole := r.FormValue("add-role")
	delrole := r.FormValue("remove-role")
	newproj := r.FormValue("add-project")
	delproj := r.FormValue("remove-project")

	// TODO: Handle anything but roles...
	if newrole != "" {
		if err := repository.GetUserRepository().AddRole(r.Context(), mux.Vars(r)["id"], newrole); err != nil {
			http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		rw.Write([]byte("Add Role Success"))
	} else if delrole != "" {
		if err := repository.GetUserRepository().RemoveRole(r.Context(), mux.Vars(r)["id"], delrole); err != nil {
			http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		rw.Write([]byte("Remove Role Success"))
	} else if newproj != "" {
		if err := repository.GetUserRepository().AddProject(r.Context(), mux.Vars(r)["id"], newproj); err != nil {
			http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		rw.Write([]byte("Add Project Success"))
	} else if delproj != "" {
		if err := repository.GetUserRepository().RemoveProject(r.Context(), mux.Vars(r)["id"], delproj); err != nil {
			http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		rw.Write([]byte("Remove Project Success"))
	} else {
		http.Error(rw, "Not Add or Del [role|project]?", http.StatusInternalServerError)
	}
}

func (api *RestApi) createUser(rw http.ResponseWriter, r *http.Request) {
	// SecuredCheck() only worked with TokenAuth: Removed

	rw.Header().Set("Content-Type", "text/plain")
	me := repository.GetUserFromContext(r.Context())
	if !me.HasRole(schema.RoleAdmin) {
		http.Error(rw, "Only admins are allowed to create new users", http.StatusForbidden)
		return
	}

	username, password, role, name, email, project := r.FormValue("username"),
		r.FormValue("password"), r.FormValue("role"), r.FormValue("name"),
		r.FormValue("email"), r.FormValue("project")

	if len(password) == 0 && role != schema.GetRoleString(schema.RoleApi) {
		http.Error(rw, "Only API users are allowed to have a blank password (login will be impossible)", http.StatusBadRequest)
		return
	}

	if len(project) != 0 && role != schema.GetRoleString(schema.RoleManager) {
		http.Error(rw, "only managers require a project (can be changed later)",
			http.StatusBadRequest)
		return
	} else if len(project) == 0 && role == schema.GetRoleString(schema.RoleManager) {
		http.Error(rw, "managers require a project to manage (can be changed later)",
			http.StatusBadRequest)
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
		http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	fmt.Fprintf(rw, "User %v successfully created!\n", username)
}

func (api *RestApi) deleteUser(rw http.ResponseWriter, r *http.Request) {
	// SecuredCheck() only worked with TokenAuth: Removed

	if user := repository.GetUserFromContext(r.Context()); !user.HasRole(schema.RoleAdmin) {
		http.Error(rw, "Only admins are allowed to delete a user", http.StatusForbidden)
		return
	}

	username := r.FormValue("username")
	if err := repository.GetUserRepository().DelUser(username); err != nil {
		http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	rw.WriteHeader(http.StatusOK)
}
