// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package auth

import (
	"fmt"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

type LocalAuthenticator struct {
	auth *Authentication
}

var _ Authenticator = (*LocalAuthenticator)(nil)

func (la *LocalAuthenticator) Init(
	auth *Authentication,
	_ interface{}) error {

	la.auth = auth
	return nil
}

func (la *LocalAuthenticator) CanLogin(
	user *User,
	rw http.ResponseWriter,
	r *http.Request) bool {

	return user != nil && user.AuthSource == AuthViaLocalPassword
}

func (la *LocalAuthenticator) Login(
	user *User,
	rw http.ResponseWriter,
	r *http.Request) (*User, error) {

	if e := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(r.FormValue("password"))); e != nil {
		return nil, fmt.Errorf("user '%s' provided the wrong password (%w)", user.Username, e)
	}

	return user, nil
}

func (la *LocalAuthenticator) Auth(
	rw http.ResponseWriter,
	r *http.Request) (*User, error) {

	return la.auth.AuthViaSession(rw, r)
}
