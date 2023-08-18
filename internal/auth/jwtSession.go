// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package auth

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/golang-jwt/jwt/v4"
)

type JWTSessionAuthenticator struct {
	loginTokenKey []byte // HS256 key
}

var _ Authenticator = (*JWTSessionAuthenticator)(nil)

func (ja *JWTSessionAuthenticator) Init() error {
	if pubKey := os.Getenv("CROSS_LOGIN_JWT_HS512_KEY"); pubKey != "" {
		bytes, err := base64.StdEncoding.DecodeString(pubKey)
		if err != nil {
			log.Warn("Could not decode cross login JWT HS512 key")
			return err
		}
		ja.loginTokenKey = bytes
	}

	log.Info("JWT Session authenticator successfully registered")
	return nil
}

func (ja *JWTSessionAuthenticator) CanLogin(
	user *schema.User,
	username string,
	rw http.ResponseWriter,
	r *http.Request) (*schema.User, bool) {

	return user, r.Header.Get("Authorization") != "" ||
		r.URL.Query().Get("login-token") != ""
}

func (ja *JWTSessionAuthenticator) Login(
	user *schema.User,
	rw http.ResponseWriter,
	r *http.Request) (*schema.User, error) {

	rawtoken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if rawtoken == "" {
		rawtoken = r.URL.Query().Get("login-token")
	}

	token, err := jwt.Parse(rawtoken, func(t *jwt.Token) (interface{}, error) {
		if t.Method == jwt.SigningMethodHS256 || t.Method == jwt.SigningMethodHS512 {
			return ja.loginTokenKey, nil
		}
		return nil, fmt.Errorf("unkown signing method for login token: %s (known: HS256, HS512, EdDSA)", t.Method.Alg())
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

	var name string
	if wrap, ok := claims["name"].(map[string]interface{}); ok {
		if vals, ok := wrap["values"].([]interface{}); ok {
			if len(vals) != 0 {
				name = fmt.Sprintf("%v", vals[0])

				for i := 1; i < len(vals); i++ {
					name += fmt.Sprintf(" %v", vals[i])
				}
			}
		}
	}

	var roles []string

	if config.Keys.JwtConfig.ValidateUser {
		// Deny any logins for unknown usernames
		if user == nil {
			log.Warn("Could not find user from JWT in internal database.")
			return nil, errors.New("unknown user")
		}

		// Take user roles from database instead of trusting the JWT
		roles = user.Roles
	} else {
		// Extract roles from JWT (if present)
		if rawroles, ok := claims["roles"].([]interface{}); ok {
			for _, rr := range rawroles {
				if r, ok := rr.(string); ok {
					if schema.IsValidRole(r) {
						roles = append(roles, r)
					}
				}
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
		user = &schema.User{
			Username:   sub,
			Name:       name,
			Roles:      roles,
			Projects:   projects,
			AuthType:   schema.AuthSession,
			AuthSource: schema.AuthViaToken,
		}

		if config.Keys.JwtConfig.SyncUserOnLogin {
			if err := repository.GetUserRepository().AddUser(user); err != nil {
				log.Errorf("Error while adding user '%s' to DB", user.Username)
			}
		}
	}

	return user, nil
}
