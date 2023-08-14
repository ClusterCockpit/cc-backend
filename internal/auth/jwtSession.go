// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package auth

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/golang-jwt/jwt/v4"
)

type JWTSessionAuthenticator struct {
	auth *Authentication

	loginTokenKey []byte // HS256 key
}

var _ Authenticator = (*JWTSessionAuthenticator)(nil)

func (ja *JWTSessionAuthenticator) Init(auth *Authentication, conf interface{}) error {

	ja.auth = auth

	if pubKey := os.Getenv("CROSS_LOGIN_JWT_HS512_KEY"); pubKey != "" {
		bytes, err := base64.StdEncoding.DecodeString(pubKey)
		if err != nil {
			log.Warn("Could not decode cross login JWT HS512 key")
			return err
		}
		ja.loginTokenKey = bytes
	}

	return nil
}

func (ja *JWTSessionAuthenticator) CanLogin(
	user *User,
	rw http.ResponseWriter,
	r *http.Request) bool {

	return r.Header.Get("Authorization") != "" || r.URL.Query().Get("login-token") != ""
}

func (ja *JWTSessionAuthenticator) Login(
	user *User,
	rw http.ResponseWriter,
	r *http.Request) (*User, error) {

	rawtoken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if rawtoken == "" {
		rawtoken = r.URL.Query().Get("login-token")
	}

	token, err := jwt.Parse(rawtoken, func(t *jwt.Token) (interface{}, error) {
		if t.Method == jwt.SigningMethodHS256 || t.Method == jwt.SigningMethodHS512 {
			return ja.loginTokenKey, nil
		}
		return nil, fmt.Errorf("AUTH/JWT > unkown signing method for login token: %s (known: HS256, HS512, EdDSA)", t.Method.Alg())
	})
	if err != nil {
		log.Warn("Error while parsing jwt token")
		return nil, err
	}

	if err = token.Claims.Valid(); err != nil {
		log.Warn("jwt token claims are not valid")
		return nil, err
	}

	claims := token.Claims.(jwt.MapClaims)
	sub, _ := claims["sub"].(string)
	exp, _ := claims["exp"].(float64)

	var name string
	// Java/Grails Issued Token
	if wrap, ok := claims["name"].(map[string]interface{}); ok {
		if vals, ok := wrap["values"].([]interface{}); ok {
			name = fmt.Sprintf("%v %v", vals[0], vals[1])
		}
	} else if val, ok := claims["name"]; ok {
		name, _ = val.(string)
	}

	var roles []string
	// Java/Grails Issued Token
	if rawroles, ok := claims["roles"].([]interface{}); ok {
		for _, rr := range rawroles {
			if r, ok := rr.(string); ok {
				if isValidRole(r) {
					roles = append(roles, r)
				}
			}
		}
	} else if rawroles, ok := claims["roles"]; ok {
		for _, r := range rawroles.([]string) {
			if isValidRole(r) {
				roles = append(roles, r)
			}
		}
	}

	projects := make([]string, 0)
	// Java/Grails Issued Token
	// if rawprojs, ok := claims["projects"].([]interface{}); ok {
	// 	for _, pp := range rawprojs {
	// 		if p, ok := pp.(string); ok {
	// 			projects = append(projects, p)
	// 		}
	// 	}
	// } else if rawprojs, ok := claims["projects"]; ok {
	// 	for _, p := range rawprojs.([]string) {
	// 		projects = append(projects, p)
	// 	}
	// }

	if user == nil {
		user = &User{
			Username:   sub,
			Name:       name,
			Roles:      roles,
			Projects:   projects,
			AuthType:   AuthSession,
			AuthSource: AuthViaToken,
		}
	}

	user.Expiration = time.Unix(int64(exp), 0)
	return user, nil
}
