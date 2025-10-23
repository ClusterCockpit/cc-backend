// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package auth

import (
	"fmt"
	"net/http"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"golang.org/x/crypto/bcrypt"
)

type LocalAuthenticator struct {
	auth *Authentication
}

var _ Authenticator = (*LocalAuthenticator)(nil)

func (la *LocalAuthenticator) Init() error {
	return nil
}

func (la *LocalAuthenticator) CanLogin(
	user *schema.User,
	username string,
	rw http.ResponseWriter,
	r *http.Request) (*schema.User, bool) {

	return user, user != nil && user.AuthSource == schema.AuthViaLocalPassword
}

func (la *LocalAuthenticator) Login(
	user *schema.User,
	rw http.ResponseWriter,
	r *http.Request) (*schema.User, error) {

	if e := bcrypt.CompareHashAndPassword([]byte(user.Password),
		[]byte(r.FormValue("password"))); e != nil {
		log.Errorf("AUTH/LOCAL > Authentication for user %s failed!", user.Username)
		return nil, fmt.Errorf("Authentication failed")
	}

	return user, nil
}
