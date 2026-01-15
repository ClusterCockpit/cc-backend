// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"context"
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestAddUser(t *testing.T) {
	_ = setup(t)
	r := GetUserRepository()

	t.Run("add user with all fields", func(t *testing.T) {
		user := &schema.User{
			Username:   "testuser1",
			Name:       "Test User One",
			Email:      "test1@example.com",
			Password:   "testpassword123",
			Roles:      []string{"user"},
			Projects:   []string{"project1", "project2"},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		retrievedUser, err := r.GetUser("testuser1")
		require.NoError(t, err)
		assert.Equal(t, user.Username, retrievedUser.Username)
		assert.Equal(t, user.Name, retrievedUser.Name)
		assert.Equal(t, user.Email, retrievedUser.Email)
		assert.Equal(t, user.Roles, retrievedUser.Roles)
		assert.Equal(t, user.Projects, retrievedUser.Projects)
		assert.NotEmpty(t, retrievedUser.Password)
		err = bcrypt.CompareHashAndPassword([]byte(retrievedUser.Password), []byte("testpassword123"))
		assert.NoError(t, err, "Password should be hashed correctly")

		err = r.DelUser("testuser1")
		require.NoError(t, err)
	})

	t.Run("add user with minimal fields", func(t *testing.T) {
		user := &schema.User{
			Username:   "testuser2",
			Roles:      []string{"user"},
			Projects:   []string{},
			AuthSource: schema.AuthViaLDAP,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		retrievedUser, err := r.GetUser("testuser2")
		require.NoError(t, err)
		assert.Equal(t, user.Username, retrievedUser.Username)
		assert.Equal(t, "", retrievedUser.Name)
		assert.Equal(t, "", retrievedUser.Email)
		assert.Equal(t, "", retrievedUser.Password)

		err = r.DelUser("testuser2")
		require.NoError(t, err)
	})

	t.Run("add duplicate user fails", func(t *testing.T) {
		user := &schema.User{
			Username:   "testuser3",
			Roles:      []string{"user"},
			Projects:   []string{},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		err = r.AddUser(user)
		assert.Error(t, err, "Adding duplicate user should fail")

		err = r.DelUser("testuser3")
		require.NoError(t, err)
	})
}

func TestGetUser(t *testing.T) {
	_ = setup(t)
	r := GetUserRepository()

	t.Run("get existing user", func(t *testing.T) {
		user := &schema.User{
			Username:   "getuser1",
			Name:       "Get User",
			Email:      "getuser@example.com",
			Roles:      []string{"user", "admin"},
			Projects:   []string{"proj1"},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		retrieved, err := r.GetUser("getuser1")
		require.NoError(t, err)
		assert.Equal(t, user.Username, retrieved.Username)
		assert.Equal(t, user.Name, retrieved.Name)
		assert.Equal(t, user.Email, retrieved.Email)
		assert.ElementsMatch(t, user.Roles, retrieved.Roles)
		assert.ElementsMatch(t, user.Projects, retrieved.Projects)

		err = r.DelUser("getuser1")
		require.NoError(t, err)
	})

	t.Run("get non-existent user", func(t *testing.T) {
		_, err := r.GetUser("nonexistent")
		assert.Error(t, err)
	})
}

func TestUpdateUser(t *testing.T) {
	_ = setup(t)
	r := GetUserRepository()

	t.Run("update user name", func(t *testing.T) {
		user := &schema.User{
			Username:   "updateuser1",
			Name:       "Original Name",
			Roles:      []string{"user"},
			Projects:   []string{},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		dbUser, err := r.GetUser("updateuser1")
		require.NoError(t, err)

		updatedUser := &schema.User{
			Username: "updateuser1",
			Name:     "Updated Name",
		}

		err = r.UpdateUser(dbUser, updatedUser)
		require.NoError(t, err)

		retrieved, err := r.GetUser("updateuser1")
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", retrieved.Name)

		err = r.DelUser("updateuser1")
		require.NoError(t, err)
	})

	t.Run("update with no changes", func(t *testing.T) {
		user := &schema.User{
			Username:   "updateuser2",
			Name:       "Same Name",
			Roles:      []string{"user"},
			Projects:   []string{},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		dbUser, err := r.GetUser("updateuser2")
		require.NoError(t, err)

		err = r.UpdateUser(dbUser, dbUser)
		assert.NoError(t, err)

		err = r.DelUser("updateuser2")
		require.NoError(t, err)
	})
}

func TestDelUser(t *testing.T) {
	_ = setup(t)
	r := GetUserRepository()

	t.Run("delete existing user", func(t *testing.T) {
		user := &schema.User{
			Username:   "deluser1",
			Roles:      []string{"user"},
			Projects:   []string{},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		err = r.DelUser("deluser1")
		require.NoError(t, err)

		_, err = r.GetUser("deluser1")
		assert.Error(t, err, "User should not exist after deletion")
	})

	t.Run("delete non-existent user", func(t *testing.T) {
		err := r.DelUser("nonexistent")
		assert.NoError(t, err, "Deleting non-existent user should not error")
	})
}

func TestListUsers(t *testing.T) {
	_ = setup(t)
	r := GetUserRepository()

	user1 := &schema.User{
		Username:   "listuser1",
		Roles:      []string{"user"},
		Projects:   []string{},
		AuthSource: schema.AuthViaLocalPassword,
	}
	user2 := &schema.User{
		Username:   "listuser2",
		Roles:      []string{"admin"},
		Projects:   []string{},
		AuthSource: schema.AuthViaLocalPassword,
	}
	user3 := &schema.User{
		Username:   "listuser3",
		Roles:      []string{"manager"},
		Projects:   []string{"proj1"},
		AuthSource: schema.AuthViaLocalPassword,
	}

	err := r.AddUser(user1)
	require.NoError(t, err)
	err = r.AddUser(user2)
	require.NoError(t, err)
	err = r.AddUser(user3)
	require.NoError(t, err)

	t.Run("list all users", func(t *testing.T) {
		users, err := r.ListUsers(false)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(users), 3)

		usernames := make([]string, len(users))
		for i, u := range users {
			usernames[i] = u.Username
		}
		assert.Contains(t, usernames, "listuser1")
		assert.Contains(t, usernames, "listuser2")
		assert.Contains(t, usernames, "listuser3")
	})

	t.Run("list special users only", func(t *testing.T) {
		users, err := r.ListUsers(true)
		require.NoError(t, err)

		usernames := make([]string, len(users))
		for i, u := range users {
			usernames[i] = u.Username
		}
		assert.Contains(t, usernames, "listuser2")
		assert.Contains(t, usernames, "listuser3")
	})

	err = r.DelUser("listuser1")
	require.NoError(t, err)
	err = r.DelUser("listuser2")
	require.NoError(t, err)
	err = r.DelUser("listuser3")
	require.NoError(t, err)
}

func TestGetLdapUsernames(t *testing.T) {
	_ = setup(t)
	r := GetUserRepository()

	ldapUser := &schema.User{
		Username:   "ldapuser1",
		Roles:      []string{"user"},
		Projects:   []string{},
		AuthSource: schema.AuthViaLDAP,
	}
	localUser := &schema.User{
		Username:   "localuser1",
		Roles:      []string{"user"},
		Projects:   []string{},
		AuthSource: schema.AuthViaLocalPassword,
	}

	err := r.AddUser(ldapUser)
	require.NoError(t, err)
	err = r.AddUser(localUser)
	require.NoError(t, err)

	usernames, err := r.GetLdapUsernames()
	require.NoError(t, err)
	assert.Contains(t, usernames, "ldapuser1")
	assert.NotContains(t, usernames, "localuser1")

	err = r.DelUser("ldapuser1")
	require.NoError(t, err)
	err = r.DelUser("localuser1")
	require.NoError(t, err)
}

func TestAddRole(t *testing.T) {
	_ = setup(t)
	r := GetUserRepository()
	ctx := context.Background()

	t.Run("add valid role", func(t *testing.T) {
		user := &schema.User{
			Username:   "roleuser1",
			Roles:      []string{"user"},
			Projects:   []string{},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		err = r.AddRole(ctx, "roleuser1", "admin")
		require.NoError(t, err)

		retrieved, err := r.GetUser("roleuser1")
		require.NoError(t, err)
		assert.Contains(t, retrieved.Roles, "admin")
		assert.Contains(t, retrieved.Roles, "user")

		err = r.DelUser("roleuser1")
		require.NoError(t, err)
	})

	t.Run("add duplicate role", func(t *testing.T) {
		user := &schema.User{
			Username:   "roleuser2",
			Roles:      []string{"user"},
			Projects:   []string{},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		err = r.AddRole(ctx, "roleuser2", "user")
		assert.Error(t, err, "Adding duplicate role should fail")
		assert.Contains(t, err.Error(), "already has role")

		err = r.DelUser("roleuser2")
		require.NoError(t, err)
	})

	t.Run("add invalid role", func(t *testing.T) {
		user := &schema.User{
			Username:   "roleuser3",
			Roles:      []string{"user"},
			Projects:   []string{},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		err = r.AddRole(ctx, "roleuser3", "invalidrole")
		assert.Error(t, err, "Adding invalid role should fail")
		assert.Contains(t, err.Error(), "no valid option")

		err = r.DelUser("roleuser3")
		require.NoError(t, err)
	})
}

func TestRemoveRole(t *testing.T) {
	_ = setup(t)
	r := GetUserRepository()
	ctx := context.Background()

	t.Run("remove existing role", func(t *testing.T) {
		user := &schema.User{
			Username:   "rmroleuser1",
			Roles:      []string{"user", "admin"},
			Projects:   []string{},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		err = r.RemoveRole(ctx, "rmroleuser1", "admin")
		require.NoError(t, err)

		retrieved, err := r.GetUser("rmroleuser1")
		require.NoError(t, err)
		assert.NotContains(t, retrieved.Roles, "admin")
		assert.Contains(t, retrieved.Roles, "user")

		err = r.DelUser("rmroleuser1")
		require.NoError(t, err)
	})

	t.Run("remove non-existent role", func(t *testing.T) {
		user := &schema.User{
			Username:   "rmroleuser2",
			Roles:      []string{"user"},
			Projects:   []string{},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		err = r.RemoveRole(ctx, "rmroleuser2", "admin")
		assert.Error(t, err, "Removing non-existent role should fail")
		assert.Contains(t, err.Error(), "already deleted")

		err = r.DelUser("rmroleuser2")
		require.NoError(t, err)
	})

	t.Run("remove manager role with projects", func(t *testing.T) {
		user := &schema.User{
			Username:   "rmroleuser3",
			Roles:      []string{"manager"},
			Projects:   []string{"proj1", "proj2"},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		err = r.RemoveRole(ctx, "rmroleuser3", "manager")
		assert.Error(t, err, "Removing manager role with projects should fail")
		assert.Contains(t, err.Error(), "still has assigned project")

		err = r.DelUser("rmroleuser3")
		require.NoError(t, err)
	})
}

func TestAddProject(t *testing.T) {
	_ = setup(t)
	r := GetUserRepository()
	ctx := context.Background()

	t.Run("add project to manager", func(t *testing.T) {
		user := &schema.User{
			Username:   "projuser1",
			Roles:      []string{"manager"},
			Projects:   []string{},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		err = r.AddProject(ctx, "projuser1", "newproject")
		require.NoError(t, err)

		retrieved, err := r.GetUser("projuser1")
		require.NoError(t, err)
		assert.Contains(t, retrieved.Projects, "newproject")

		err = r.DelUser("projuser1")
		require.NoError(t, err)
	})

	t.Run("add project to non-manager", func(t *testing.T) {
		user := &schema.User{
			Username:   "projuser2",
			Roles:      []string{"user"},
			Projects:   []string{},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		err = r.AddProject(ctx, "projuser2", "newproject")
		assert.Error(t, err, "Adding project to non-manager should fail")
		assert.Contains(t, err.Error(), "not a manager")

		err = r.DelUser("projuser2")
		require.NoError(t, err)
	})

	t.Run("add duplicate project", func(t *testing.T) {
		user := &schema.User{
			Username:   "projuser3",
			Roles:      []string{"manager"},
			Projects:   []string{"existingproject"},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		err = r.AddProject(ctx, "projuser3", "existingproject")
		assert.Error(t, err, "Adding duplicate project should fail")
		assert.Contains(t, err.Error(), "already manages")

		err = r.DelUser("projuser3")
		require.NoError(t, err)
	})
}

func TestRemoveProject(t *testing.T) {
	_ = setup(t)
	r := GetUserRepository()
	ctx := context.Background()

	t.Run("remove existing project", func(t *testing.T) {
		user := &schema.User{
			Username:   "rmprojuser1",
			Roles:      []string{"manager"},
			Projects:   []string{"proj1", "proj2"},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		err = r.RemoveProject(ctx, "rmprojuser1", "proj1")
		require.NoError(t, err)

		retrieved, err := r.GetUser("rmprojuser1")
		require.NoError(t, err)
		assert.NotContains(t, retrieved.Projects, "proj1")
		assert.Contains(t, retrieved.Projects, "proj2")

		err = r.DelUser("rmprojuser1")
		require.NoError(t, err)
	})

	t.Run("remove non-existent project", func(t *testing.T) {
		user := &schema.User{
			Username:   "rmprojuser2",
			Roles:      []string{"manager"},
			Projects:   []string{"proj1"},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		err = r.RemoveProject(ctx, "rmprojuser2", "nonexistent")
		assert.Error(t, err, "Removing non-existent project should fail")

		err = r.DelUser("rmprojuser2")
		require.NoError(t, err)
	})

	t.Run("remove project from non-manager", func(t *testing.T) {
		user := &schema.User{
			Username:   "rmprojuser3",
			Roles:      []string{"user"},
			Projects:   []string{},
			AuthSource: schema.AuthViaLocalPassword,
		}

		err := r.AddUser(user)
		require.NoError(t, err)

		err = r.RemoveProject(ctx, "rmprojuser3", "proj1")
		assert.Error(t, err, "Removing project from non-manager should fail")
		assert.Contains(t, err.Error(), "not a manager")

		err = r.DelUser("rmprojuser3")
		require.NoError(t, err)
	})
}

func TestGetUserFromContext(t *testing.T) {
	t.Run("get user from context", func(t *testing.T) {
		user := &schema.User{
			Username: "contextuser",
			Roles:    []string{"user"},
		}

		ctx := context.WithValue(context.Background(), ContextUserKey, user)
		retrieved := GetUserFromContext(ctx)

		require.NotNil(t, retrieved)
		assert.Equal(t, user.Username, retrieved.Username)
	})

	t.Run("get user from empty context", func(t *testing.T) {
		ctx := context.Background()
		retrieved := GetUserFromContext(ctx)

		assert.Nil(t, retrieved)
	})
}
