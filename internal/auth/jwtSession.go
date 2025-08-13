// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package auth

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/golang-jwt/jwt/v5"
)

type JWTSessionAuthenticator struct {
	loginTokenKey []byte // HS256 key
}

var _ Authenticator = (*JWTSessionAuthenticator)(nil)

func (ja *JWTSessionAuthenticator) Init() error {
	if pubKey := os.Getenv("CROSS_LOGIN_JWT_HS512_KEY"); pubKey != "" {
		bytes, err := base64.StdEncoding.DecodeString(pubKey)
		if err != nil {
			cclog.Warn("Could not decode cross login JWT HS512 key")
			return err
		}
		ja.loginTokenKey = bytes
	}

	cclog.Info("JWT Session authenticator successfully registered")
	return nil
}

func (ja *JWTSessionAuthenticator) CanLogin(
	user *schema.User,
	username string,
	rw http.ResponseWriter,
	r *http.Request,
) (*schema.User, bool) {
	return user, r.Header.Get("Authorization") != "" ||
		r.URL.Query().Get("login-token") != ""
}

func (ja *JWTSessionAuthenticator) Login(
	user *schema.User,
	rw http.ResponseWriter,
	r *http.Request,
) (*schema.User, error) {
	rawtoken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if rawtoken == "" {
		rawtoken = r.URL.Query().Get("login-token")
	}

	token, err := jwt.Parse(rawtoken, func(t *jwt.Token) (any, error) {
		if t.Method == jwt.SigningMethodHS256 || t.Method == jwt.SigningMethodHS512 {
			return ja.loginTokenKey, nil
		}
		return nil, fmt.Errorf("unkown signing method for login token: %s (known: HS256, HS512, EdDSA)", t.Method.Alg())
	})
	if err != nil {
		cclog.Warn("Error while parsing jwt token")
		return nil, err
	}

	if !token.Valid {
		cclog.Warn("jwt token claims are not valid")
		return nil, errors.New("jwt token claims are not valid")
	}

	claims := token.Claims.(jwt.MapClaims)
	sub, _ := claims["sub"].(string)

	var roles []string
	projects := make([]string, 0)

	if Keys.JwtConfig.ValidateUser {
		var err error
		user, err = repository.GetUserRepository().GetUser(sub)
		if err != nil && err != sql.ErrNoRows {
			cclog.Errorf("Error while loading user '%v'", sub)
		}

		// Deny any logins for unknown usernames
		if user == nil {
			cclog.Warn("Could not find user from JWT in internal database.")
			return nil, errors.New("unknown user")
		}
	} else {
		var name string
		if wrap, ok := claims["name"].(map[string]any); ok {
			if vals, ok := wrap["values"].([]any); ok {
				if len(vals) != 0 {
					name = fmt.Sprintf("%v", vals[0])

					for i := 1; i < len(vals); i++ {
						name += fmt.Sprintf(" %v", vals[i])
					}
				}
			}
		}

		// Extract roles from JWT (if present)
		if rawroles, ok := claims["roles"].([]any); ok {
			for _, rr := range rawroles {
				if r, ok := rr.(string); ok {
					if schema.IsValidRole(r) {
						roles = append(roles, r)
					}
				}
			}
		}

		if rawprojs, ok := claims["projects"].([]any); ok {
			for _, pp := range rawprojs {
				if p, ok := pp.(string); ok {
					projects = append(projects, p)
				}
			}
		} else if rawprojs, ok := claims["projects"]; ok {
			projects = append(projects, rawprojs.([]string)...)
		}

		user = &schema.User{
			Username:   sub,
			Name:       name,
			Roles:      roles,
			Projects:   projects,
			AuthType:   schema.AuthSession,
			AuthSource: schema.AuthViaToken,
		}

		if Keys.JwtConfig.SyncUserOnLogin || Keys.JwtConfig.UpdateUserOnLogin {
			handleTokenUser(user)
		}
	}

	return user, nil
}
