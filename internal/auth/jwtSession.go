// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
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

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/golang-jwt/jwt/v5"
)

type JWTSessionAuthenticator struct {
	loginTokenKey []byte // HS256 key
}

var _ Authenticator = (*JWTSessionAuthenticator)(nil)

func (ja *JWTSessionAuthenticator) Init() error {
	pubKey := os.Getenv("CROSS_LOGIN_JWT_HS512_KEY")
	if pubKey == "" {
		// Without a configured key the HMAC verification below would run against
		// an empty key, which lets anyone forge a valid token. Refuse to register
		// the authenticator in that case so JWT session login is simply disabled.
		return errors.New("CROSS_LOGIN_JWT_HS512_KEY not set: JWT session login disabled")
	}

	bytes, err := base64.StdEncoding.DecodeString(pubKey)
	if err != nil {
		cclog.Warn("Could not decode cross login JWT HS512 key")
		return err
	}
	ja.loginTokenKey = bytes

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
			// Defense in depth: an empty key would verify any HMAC signature.
			// Init() already refuses to register without a key, so this should
			// never trigger, but guard explicitly rather than trust the chain.
			if len(ja.loginTokenKey) == 0 {
				return nil, errors.New("HS login key not configured")
			}
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

	// Use shared helper to get user from JWT claims
	user, err = getUserFromJWT(claims, Keys.JwtConfig.ValidateUser, schema.AuthSession, schema.AuthViaToken)
	if err != nil {
		return nil, err
	}

	// Sync or update user if configured
	if !Keys.JwtConfig.ValidateUser && (Keys.JwtConfig.SyncUserOnLogin || Keys.JwtConfig.UpdateUserOnLogin) {
		handleTokenUser(user)
	}

	return user, nil
}
