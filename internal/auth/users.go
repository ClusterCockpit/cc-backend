// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
)

func (auth *Authentication) GetUser(username string) (*User, error) {

	user := &User{Username: username}
	var hashedPassword, name, rawRoles, email sql.NullString
	if err := sq.Select("password", "ldap", "name", "roles", "email").From("user").
		Where("user.username = ?", username).RunWith(auth.db).
		QueryRow().Scan(&hashedPassword, &user.AuthSource, &name, &rawRoles, &email); err != nil {
		return nil, err
	}

	user.Password = hashedPassword.String
	user.Name = name.String
	user.Email = email.String
	if rawRoles.Valid {
		if err := json.Unmarshal([]byte(rawRoles.String), &user.Roles); err != nil {
			return nil, err
		}
	}

	return user, nil
}

func (auth *Authentication) AddUser(user *User) error {

	rolesJson, _ := json.Marshal(user.Roles)

	cols := []string{"username", "roles"}
	vals := []interface{}{user.Username, string(rolesJson)}
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
			return err
		}
		cols = append(cols, "password")
		vals = append(vals, string(password))
	}

	if _, err := sq.Insert("user").Columns(cols...).Values(vals...).RunWith(auth.db).Exec(); err != nil {
		return err
	}

	log.Infof("new user %#v created (roles: %s, auth-source: %d)", user.Username, rolesJson, user.AuthSource)
	return nil
}

func (auth *Authentication) DelUser(username string) error {

	_, err := auth.db.Exec(`DELETE FROM user WHERE user.username = ?`, username)
	return err
}

func (auth *Authentication) ListUsers(specialsOnly bool) ([]*User, error) {

	q := sq.Select("username", "name", "email", "roles").From("user")
	if specialsOnly {
		q = q.Where("(roles != '[\"user\"]' AND roles != '[]')")
	}

	rows, err := q.RunWith(auth.db).Query()
	if err != nil {
		return nil, err
	}

	users := make([]*User, 0)
	defer rows.Close()
	for rows.Next() {
		rawroles := ""
		user := &User{}
		var name, email sql.NullString
		if err := rows.Scan(&user.Username, &name, &email, &rawroles); err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(rawroles), &user.Roles); err != nil {
			return nil, err
		}

		user.Name = name.String
		user.Email = email.String
		users = append(users, user)
	}
	return users, nil
}

func (auth *Authentication) AddRole(
	ctx context.Context,
	username string,
	role string) error {

	user, err := auth.GetUser(username)
	if err != nil {
		return err
	}

	if role != RoleAdmin && role != RoleApi && role != RoleUser && role != RoleSupport {
		return fmt.Errorf("invalid user role: %#v", role)
	}

	for _, r := range user.Roles {
		if r == role {
			return fmt.Errorf("user %#v already has role %#v", username, role)
		}
	}

	roles, _ := json.Marshal(append(user.Roles, role))
	if _, err := sq.Update("user").Set("roles", roles).Where("user.username = ?", username).RunWith(auth.db).Exec(); err != nil {
		return err
	}
	return nil
}

func (auth *Authentication) RemoveRole(ctx context.Context, username string, role string) error {
	user, err := auth.GetUser(username)
	if err != nil {
		return err
	}

	if role != RoleAdmin && role != RoleApi && role != RoleUser {
		return fmt.Errorf("invalid user role: %#v", role)
	}

	var exists bool
	var newroles []string
	for _, r := range user.Roles {
		if r != role {
			newroles = append(newroles, r) // Append all roles not matching requested delete role
		} else {
			exists = true
		}
	}

	if (exists == true) {
		var mroles, _ = json.Marshal(newroles)
		if _, err := sq.Update("user").Set("roles", mroles).Where("user.username = ?", username).RunWith(auth.db).Exec(); err != nil {
			return err
		}
		return nil
	} else {
		return fmt.Errorf("user %#v already does not have role %#v", username, role)
	}
}

func FetchUser(ctx context.Context, db *sqlx.DB, username string) (*model.User, error) {
	me := GetUser(ctx)
	if me != nil && !me.HasRole(RoleAdmin) && !me.HasRole(RoleSupport) && me.Username != username {
		return nil, errors.New("forbidden")
	}

	user := &model.User{Username: username}
	var name, email sql.NullString
	if err := sq.Select("name", "email").From("user").Where("user.username = ?", username).
		RunWith(db).QueryRow().Scan(&name, &email); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}

		return nil, err
	}

	user.Name = name.String
	user.Email = email.String
	return user, nil
}
