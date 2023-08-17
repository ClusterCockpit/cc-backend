// Copyright (C) 2023 NHR@FAU, University Erlangen-Nuremberg.
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

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/golang-jwt/jwt/v4"
)

type JWTCookieSessionAuthenticator struct {
	publicKey           ed25519.PublicKey
	privateKey          ed25519.PrivateKey
	publicKeyCrossLogin ed25519.PublicKey // For accepting externally generated JWTs

	config *schema.JWTAuthConfig
}

var _ Authenticator = (*JWTCookieSessionAuthenticator)(nil)

func (ja *JWTCookieSessionAuthenticator) Init(conf interface{}) error {
	ja.config = conf.(*schema.JWTAuthConfig)

	pubKey, privKey := os.Getenv("JWT_PUBLIC_KEY"), os.Getenv("JWT_PRIVATE_KEY")
	if pubKey == "" || privKey == "" {
		log.Warn("environment variables 'JWT_PUBLIC_KEY' or 'JWT_PRIVATE_KEY' not set (token based authentication will not work)")
		return errors.New("environment variables 'JWT_PUBLIC_KEY' or 'JWT_PRIVATE_KEY' not set (token based authentication will not work)")
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

	// Look for external public keys
	pubKeyCrossLogin, keyFound := os.LookupEnv("CROSS_LOGIN_JWT_PUBLIC_KEY")
	if keyFound && pubKeyCrossLogin != "" {
		bytes, err := base64.StdEncoding.DecodeString(pubKeyCrossLogin)
		if err != nil {
			log.Warn("Could not decode cross login JWT public key")
			return err
		}
		ja.publicKeyCrossLogin = ed25519.PublicKey(bytes)
	} else {
		ja.publicKeyCrossLogin = nil
		log.Debug("environment variable 'CROSS_LOGIN_JWT_PUBLIC_KEY' not set (cross login token based authentication will not work)")
		return errors.New("environment variable 'CROSS_LOGIN_JWT_PUBLIC_KEY' not set (cross login token based authentication will not work)")
	}

	// Warn if other necessary settings are not configured
	if ja.config != nil {
		if ja.config.CookieName == "" {
			log.Warn("cookieName for JWTs not configured (cross login via JWT cookie will fail)")
			return errors.New("cookieName for JWTs not configured (cross login via JWT cookie will fail)")
		}
		if !ja.config.ValidateUser {
			log.Warn("forceJWTValidationViaDatabase not set to true: CC will accept users and roles defined in JWTs regardless of its own database!")
		}
		if ja.config.TrustedIssuer == "" {
			log.Warn("trustedExternalIssuer for JWTs not configured (cross login via JWT cookie will fail)")
			return errors.New("trustedExternalIssuer for JWTs not configured (cross login via JWT cookie will fail)")
		}
	} else {
		log.Warn("config for JWTs not configured (cross login via JWT cookie will fail)")
		return errors.New("config for JWTs not configured (cross login via JWT cookie will fail)")
	}

	return nil
}

func (ja *JWTCookieSessionAuthenticator) CanLogin(
	user *schema.User,
	username string,
	rw http.ResponseWriter,
	r *http.Request) (*schema.User, bool) {

	cookieName := ""
	if ja.config != nil && ja.config.CookieName != "" {
		cookieName = ja.config.CookieName
	}

	// Try to read the JWT cookie
	if cookieName != "" {
		jwtCookie, err := r.Cookie(cookieName)

		if err == nil && jwtCookie.Value != "" {
			return user, true
		}
	}

	return nil, false
}

func (ja *JWTCookieSessionAuthenticator) Login(
	user *schema.User,
	rw http.ResponseWriter,
	r *http.Request) (*schema.User, error) {

	jwtCookie, err := r.Cookie(ja.config.CookieName)
	var rawtoken string

	if err == nil && jwtCookie.Value != "" {
		rawtoken = jwtCookie.Value
	}

	token, err := jwt.Parse(rawtoken, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodEdDSA {
			return nil, errors.New("only Ed25519/EdDSA supported")
		}

		unvalidatedIssuer, success := t.Claims.(jwt.MapClaims)["iss"].(string)
		if success && unvalidatedIssuer == ja.config.TrustedIssuer {
			// The (unvalidated) issuer seems to be the expected one,
			// use public cross login key from config
			return ja.publicKeyCrossLogin, nil
		}

		// No cross login key configured or issuer not expected
		// Try own key
		return ja.publicKey, nil
	})
	if err != nil {
		log.Warn("error while parsing token")
		return nil, err
	}

	// Check token validity and extract paypload
	if err := token.Claims.Valid(); err != nil {
		log.Warn("jwt token claims are not valid")
		return nil, err
	}

	claims := token.Claims.(jwt.MapClaims)
	sub, _ := claims["sub"].(string)

	var name string
	if val, ok := claims["name"]; ok {
		name, _ = val.(string)
	}

	var roles []string

	if ja.config.ValidateUser {
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
					roles = append(roles, r)
				}
			}
		}
	}

	// (Ask browser to) Delete JWT cookie
	deletedCookie := &http.Cookie{
		Name:     ja.config.CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	}
	http.SetCookie(rw, deletedCookie)

	if user == nil {
		user = &schema.User{
			Username:   sub,
			Name:       name,
			Roles:      roles,
			AuthType:   schema.AuthSession,
			AuthSource: schema.AuthViaToken,
		}

		if ja.config.SyncUserOnLogin {
			if err := repository.GetUserRepository().AddUser(user); err != nil {
				log.Errorf("Error while adding user '%s' to DB", user.Username)
			}
		}
	}

	return user, nil
}
