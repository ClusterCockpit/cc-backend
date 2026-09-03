// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/golang-jwt/jwt/v5"
)

type JWTAuthConfig struct {
	// Specifies for how long a JWT token shall be valid
	// as a string parsable by time.ParseDuration().
	MaxAge string `json:"max-age"`

	// Specifies which cookie should be checked for a JWT token (if no authorization header is present)
	CookieName string `json:"cookie-name"`

	// Deny login for users not in database (but defined in JWT).
	// Ignore user roles defined in JWTs ('roles' claim), get them from db.
	ValidateUser bool `json:"validate-user"`

	// Specifies which issuer should be accepted when validating external JWTs ('iss' claim)
	TrustedIssuer string `json:"trusted-issuer"`

	// Should an non-existent user be added to the DB based on the information in the token
	SyncUserOnLogin bool `json:"sync-user-on-login"`

	// Should an existent user be updated in the DB based on the information in the token
	UpdateUserOnLogin bool `json:"update-user-on-login"`

	// Base64 encoded Ed25519 public key used to validate JWTs.
	// Overridden by the JWT_PUBLIC_KEY environment variable when set.
	PublicKey string `json:"public-key"`

	// Base64 encoded Ed25519 private key used to sign JWTs.
	// Overridden by the JWT_PRIVATE_KEY environment variable when set.
	PrivateKey string `json:"private-key"`

	// Base64 encoded Ed25519 public key for accepting externally generated JWTs.
	// Overridden by the CROSS_LOGIN_JWT_PUBLIC_KEY environment variable when set.
	CrossLoginPublicKey string `json:"cross-login-public-key"`

	// Base64 encoded HMAC (HS256/HS512) key for accepting externally generated
	// session login tokens.
	// Overridden by the CROSS_LOGIN_JWT_HS512_KEY environment variable when set.
	CrossLoginHS512Key string `json:"cross-login-hs512-key"`
}

type JWTAuthenticator struct {
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
}

func (ja *JWTAuthenticator) Init() error {
	pubKey := secretFromEnv("JWT_PUBLIC_KEY", Keys.JwtConfig.PublicKey)
	privKey := secretFromEnv("JWT_PRIVATE_KEY", Keys.JwtConfig.PrivateKey)
	if pubKey == "" || privKey == "" {
		cclog.Warn("JWT public/private key not configured ('public-key'/'private-key' in config or 'JWT_PUBLIC_KEY'/'JWT_PRIVATE_KEY' env): token based authentication will not work")
	} else {
		bytes, err := base64.StdEncoding.DecodeString(pubKey)
		if err != nil {
			cclog.Warn("Could not decode JWT public key")
			return err
		}
		ja.publicKey = ed25519.PublicKey(bytes)
		bytes, err = base64.StdEncoding.DecodeString(privKey)
		if err != nil {
			cclog.Warn("Could not decode JWT private key")
			return err
		}
		ja.privateKey = ed25519.PrivateKey(bytes)
	}

	return nil
}

func (ja *JWTAuthenticator) AuthViaJWT(
	rw http.ResponseWriter,
	r *http.Request,
) (*schema.User, error) {
	rawtoken := r.Header.Get("X-Auth-Token")
	if rawtoken == "" {
		rawtoken = r.Header.Get("Authorization")
		rawtoken = strings.TrimPrefix(rawtoken, "Bearer ")
	}

	// there is no token
	if rawtoken == "" {
		return nil, nil
	}

	token, err := jwt.Parse(rawtoken, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodEdDSA {
			return nil, errors.New("JWT Auth: only Ed25519/EdDSA supported")
		}

		return ja.publicKey, nil
	})
	if err != nil {
		cclog.Warnf("JWT Auth: error while parsing token: %s", err.Error())
		return nil, err
	}
	if !token.Valid {
		cclog.Warn("JWT Auth: token claims are not valid")
		return nil, errors.New("JWT Auth: token claims are not valid")
	}

	// Token is valid, extract payload
	claims := token.Claims.(jwt.MapClaims)

	// Use shared helper to get user from JWT claims
	var user *schema.User
	user, err = getUserFromJWT(claims, Keys.JwtConfig.ValidateUser, schema.AuthToken, -1)
	if err != nil {
		return nil, err
	}

	// If not validating user, we only get roles from JWT (no projects for this auth method)
	if !Keys.JwtConfig.ValidateUser {
		user.Roles = extractRolesFromClaims(claims, false)
		user.Projects = nil // Standard JWT auth doesn't include projects
	}

	return user, nil
}

// ProvideJWT generates a new JWT that can be used for authentication
func (ja *JWTAuthenticator) ProvideJWT(user *schema.User) (string, error) {
	if ja.privateKey == nil {
		return "", errors.New("JWT private key not configured ('private-key' in config or 'JWT_PRIVATE_KEY' env)")
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   user.Username,
		"roles": user.Roles,
		"iat":   now.Unix(),
	}
	if Keys.JwtConfig.MaxAge != "" {
		d, err := time.ParseDuration(Keys.JwtConfig.MaxAge)
		if err != nil {
			return "", errors.New("cannot parse max-age config key")
		}
		claims["exp"] = now.Add(d).Unix()
	}

	return jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(ja.privateKey)
}
