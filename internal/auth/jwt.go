// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/golang-jwt/jwt/v4"
)

type JWTAuthenticator struct {
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
}

func (ja *JWTAuthenticator) Init() error {
	pubKey, privKey := os.Getenv("JWT_PUBLIC_KEY"), os.Getenv("JWT_PRIVATE_KEY")
	if pubKey == "" || privKey == "" {
		log.Warn("environment variables 'JWT_PUBLIC_KEY' or 'JWT_PRIVATE_KEY' not set (token based authentication will not work)")
	} else {
		bytes, err := base64.StdEncoding.DecodeString(pubKey)
		if err != nil {
			log.Warn("Could not decode JWT public key")
			return err
		}
		ja.publicKey = ed25519.PublicKey(bytes)
		bytes, err = base64.StdEncoding.DecodeString(privKey)
		if err != nil {
			log.Warn("Could not decode JWT private key")
			return err
		}
		ja.privateKey = ed25519.PrivateKey(bytes)
	}

	return nil
}

func (ja *JWTAuthenticator) AuthViaJWT(
	rw http.ResponseWriter,
	r *http.Request) (*schema.User, error) {

	rawtoken := r.Header.Get("X-Auth-Token")
	if rawtoken == "" {
		rawtoken = r.Header.Get("Authorization")
		rawtoken = strings.TrimPrefix(rawtoken, "Bearer ")
	}

	// there is no token
	if rawtoken == "" {
		return nil, nil
	}

	token, err := jwt.Parse(rawtoken, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodEdDSA {
			return nil, errors.New("only Ed25519/EdDSA supported")
		}

		return ja.publicKey, nil
	})
	if err != nil {
		log.Warn("Error while parsing JWT token")
		return nil, err
	}
	if err := token.Claims.Valid(); err != nil {
		log.Warn("jwt token claims are not valid")
		return nil, err
	}

	// Token is valid, extract payload
	claims := token.Claims.(jwt.MapClaims)
	sub, _ := claims["sub"].(string)

	var roles []string

	// Validate user + roles from JWT against database?
	if config.Keys.JwtConfig.ValidateUser {
		ur := repository.GetUserRepository()
		user, err := ur.GetUser(sub)

		// Deny any logins for unknown usernames
		if err != nil {
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
					roles = append(roles, r)
				}
			}
		}
	}

	return &schema.User{
		Username:   sub,
		Roles:      roles,
		AuthType:   schema.AuthToken,
		AuthSource: -1,
	}, nil
}

// Generate a new JWT that can be used for authentication
func (ja *JWTAuthenticator) ProvideJWT(user *schema.User) (string, error) {

	if ja.privateKey == nil {
		return "", errors.New("environment variable 'JWT_PRIVATE_KEY' not set")
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   user.Username,
		"roles": user.Roles,
		"iat":   now.Unix(),
	}
	if config.Keys.JwtConfig.MaxAge != "" {
		d, err := time.ParseDuration(config.Keys.JwtConfig.MaxAge)
		if err != nil {
			return "", errors.New("cannot parse max-age config key")
		}
		claims["exp"] = now.Add(d).Unix()
	}

	return jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(ja.privateKey)
}
