// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package auth

import (
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/golang-jwt/jwt/v4"
)

type JWTAuthenticator struct {
	auth *Authentication

	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey

	loginTokenKey []byte // HS256 key

	config *schema.JWTAuthConfig
}

var _ Authenticator = (*JWTAuthenticator)(nil)

func (ja *JWTAuthenticator) Init(auth *Authentication, conf interface{}) error {

	ja.auth = auth
	ja.config = conf.(*schema.JWTAuthConfig)

	pubKey, privKey := os.Getenv("JWT_PUBLIC_KEY"), os.Getenv("JWT_PRIVATE_KEY")
	if pubKey == "" || privKey == "" {
		log.Warn("environment variables 'JWT_PUBLIC_KEY' or 'JWT_PRIVATE_KEY' not set (token based authentication will not work)")
	} else {
		bytes, err := base64.StdEncoding.DecodeString(pubKey)
		if err != nil {
			return err
		}
		ja.publicKey = ed25519.PublicKey(bytes)
		bytes, err = base64.StdEncoding.DecodeString(privKey)
		if err != nil {
			return err
		}
		ja.privateKey = ed25519.PrivateKey(bytes)
	}

	if pubKey = os.Getenv("CROSS_LOGIN_JWT_HS512_KEY"); pubKey != "" {
		bytes, err := base64.StdEncoding.DecodeString(pubKey)
		if err != nil {
			return err
		}
		ja.loginTokenKey = bytes
	}

	return nil
}

func (ja *JWTAuthenticator) CanLogin(
	user *User,
	rw http.ResponseWriter,
	r *http.Request) bool {

	return (user != nil && user.AuthSource == AuthViaToken) || r.Header.Get("Authorization") != "" || r.URL.Query().Get("login-token") != ""
}

func (ja *JWTAuthenticator) Login(
	user *User,
	rw http.ResponseWriter,
	r *http.Request) (*User, error) {

	rawtoken := r.Header.Get("X-Auth-Token")
	if rawtoken == "" {
		rawtoken = r.Header.Get("Authorization")
		rawtoken = strings.TrimPrefix(rawtoken, "Bearer ")
		if rawtoken == "" {
			rawtoken = r.URL.Query().Get("login-token")
		}
	}

	token, err := jwt.Parse(rawtoken, func(t *jwt.Token) (interface{}, error) {
		if t.Method == jwt.SigningMethodEdDSA {
			return ja.publicKey, nil
		}
		if t.Method == jwt.SigningMethodHS256 || t.Method == jwt.SigningMethodHS512 {
			return ja.loginTokenKey, nil
		}
		return nil, fmt.Errorf("unkown signing method for login token: %s (known: HS256, HS512, EdDSA)", t.Method.Alg())
	})
	if err != nil {
		return nil, err
	}

	if err := token.Claims.Valid(); err != nil {
		return nil, err
	}

	claims := token.Claims.(jwt.MapClaims)
	sub, _ := claims["sub"].(string)
	exp, _ := claims["exp"].(float64)
	var roles []string
	if rawroles, ok := claims["roles"].([]interface{}); ok {
		for _, rr := range rawroles {
			if r, ok := rr.(string); ok {
				roles = append(roles, r)
			}
		}
	}
	if rawrole, ok := claims["roles"].(string); ok {
		roles = append(roles, rawrole)
	}

	if user == nil {
		user, err = ja.auth.GetUser(sub)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		} else if user == nil {
			user = &User{
				Username:   sub,
				Roles:      roles,
				AuthSource: AuthViaToken,
			}
			if err := ja.auth.AddUser(user); err != nil {
				return nil, err
			}
		}
	}

	user.Expiration = time.Unix(int64(exp), 0)
	return user, nil
}

func (ja *JWTAuthenticator) Auth(
	rw http.ResponseWriter,
	r *http.Request) (*User, error) {

	rawtoken := r.Header.Get("X-Auth-Token")
	if rawtoken == "" {
		rawtoken = r.Header.Get("Authorization")
		rawtoken = strings.TrimPrefix(rawtoken, "Bearer ")
	}

	// Because a user can also log in via a token, the
	// session cookie must be checked here as well:
	if rawtoken == "" {
		return ja.auth.AuthViaSession(rw, r)
	}

	token, err := jwt.Parse(rawtoken, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodEdDSA {
			return nil, errors.New("only Ed25519/EdDSA supported")
		}
		return ja.publicKey, nil
	})
	if err != nil {
		return nil, err
	}

	if err := token.Claims.Valid(); err != nil {
		return nil, err
	}

	claims := token.Claims.(jwt.MapClaims)
	sub, _ := claims["sub"].(string)

	var roles []string
	if rawroles, ok := claims["roles"].([]interface{}); ok {
		for _, rr := range rawroles {
			if r, ok := rr.(string); ok {
				roles = append(roles, r)
			}
		}
	}

	return &User{
		Username:   sub,
		Roles:      roles,
		AuthSource: AuthViaToken,
	}, nil
}

// Generate a new JWT that can be used for authentication
func (ja *JWTAuthenticator) ProvideJWT(user *User) (string, error) {

	if ja.privateKey == nil {
		return "", errors.New("environment variable 'JWT_PRIVATE_KEY' not set")
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   user.Username,
		"roles": user.Roles,
		"iat":   now.Unix(),
	}
	if ja.config != nil && ja.config.MaxAge != 0 {
		claims["exp"] = now.Add(time.Duration(ja.config.MaxAge)).Unix()
	}

	return jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(ja.privateKey)
}
