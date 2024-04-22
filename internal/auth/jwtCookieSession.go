// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
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

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/golang-jwt/jwt/v5"
)

type JWTCookieSessionAuthenticator struct {
	publicKey           ed25519.PublicKey
	privateKey          ed25519.PrivateKey
	publicKeyCrossLogin ed25519.PublicKey // For accepting externally generated JWTs
}

var _ Authenticator = (*JWTCookieSessionAuthenticator)(nil)

func (ja *JWTCookieSessionAuthenticator) Init() error {
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

	jc := config.Keys.JwtConfig
	// Warn if other necessary settings are not configured
	if jc != nil {
		if jc.CookieName == "" {
			log.Info("cookieName for JWTs not configured (cross login via JWT cookie will fail)")
			return errors.New("cookieName for JWTs not configured (cross login via JWT cookie will fail)")
		}
		if !jc.ValidateUser {
			log.Info("forceJWTValidationViaDatabase not set to true: CC will accept users and roles defined in JWTs regardless of its own database!")
		}
		if jc.TrustedIssuer == "" {
			log.Info("trustedExternalIssuer for JWTs not configured (cross login via JWT cookie will fail)")
			return errors.New("trustedExternalIssuer for JWTs not configured (cross login via JWT cookie will fail)")
		}
	} else {
		log.Warn("config for JWTs not configured (cross login via JWT cookie will fail)")
		return errors.New("config for JWTs not configured (cross login via JWT cookie will fail)")
	}

	log.Info("JWT Cookie Session authenticator successfully registered")
	return nil
}

func (ja *JWTCookieSessionAuthenticator) CanLogin(
	user *schema.User,
	username string,
	rw http.ResponseWriter,
	r *http.Request,
) (*schema.User, bool) {
	jc := config.Keys.JwtConfig
	cookieName := ""
	if jc.CookieName != "" {
		cookieName = jc.CookieName
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
	r *http.Request,
) (*schema.User, error) {
	jc := config.Keys.JwtConfig
	jwtCookie, err := r.Cookie(jc.CookieName)
	var rawtoken string

	if err == nil && jwtCookie.Value != "" {
		rawtoken = jwtCookie.Value
	}

	token, err := jwt.Parse(rawtoken, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodEdDSA {
			return nil, errors.New("only Ed25519/EdDSA supported")
		}

		unvalidatedIssuer, success := t.Claims.(jwt.MapClaims)["iss"].(string)
		if success && unvalidatedIssuer == jc.TrustedIssuer {
			// The (unvalidated) issuer seems to be the expected one,
			// use public cross login key from config
			return ja.publicKeyCrossLogin, nil
		}

		// No cross login key configured or issuer not expected
		// Try own key
		return ja.publicKey, nil
	})
	if err != nil {
		log.Warn("JWT cookie session: error while parsing token")
		return nil, err
	}

	if !token.Valid {
		log.Warn("jwt token claims are not valid")
		return nil, errors.New("jwt token claims are not valid")
	}

	claims := token.Claims.(jwt.MapClaims)
	sub, _ := claims["sub"].(string)

	var roles []string
	projects := make([]string, 0)

	if jc.ValidateUser {
		var err error
		user, err = repository.GetUserRepository().GetUser(sub)
		if err != nil && err != sql.ErrNoRows {
			log.Errorf("Error while loading user '%v'", sub)
		}

		// Deny any logins for unknown usernames
		if user == nil {
			log.Warn("Could not find user from JWT in internal database.")
			return nil, errors.New("unknown user")
		}
	} else {
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

		// Extract roles from JWT (if present)
		if rawroles, ok := claims["roles"].([]interface{}); ok {
			for _, rr := range rawroles {
				if r, ok := rr.(string); ok {
					roles = append(roles, r)
				}
			}
		}
		user = &schema.User{
			Username:   sub,
			Name:       name,
			Roles:      roles,
			Projects:   projects,
			AuthType:   schema.AuthSession,
			AuthSource: schema.AuthViaToken,
		}

		if jc.SyncUserOnLogin {
			persistUser(user)
		}
	}

	// (Ask browser to) Delete JWT cookie
	deletedCookie := &http.Cookie{
		Name:     jc.CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	}
	http.SetCookie(rw, deletedCookie)

	return user, nil
}
