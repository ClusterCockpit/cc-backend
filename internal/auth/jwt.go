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

	publicKey           ed25519.PublicKey
	privateKey          ed25519.PrivateKey
	publicKeyCrossLogin ed25519.PublicKey // For accepting externally generated JWTs

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

	if pubKey = os.Getenv("CROSS_LOGIN_JWT_HS512_KEY"); pubKey != "" {
		bytes, err := base64.StdEncoding.DecodeString(pubKey)
		if err != nil {
			log.Warn("Could not decode cross login JWT HS512 key")
			return err
		}
		ja.loginTokenKey = bytes
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

		// Warn if other necessary settings are not configured
		if ja.config != nil {
			if ja.config.CookieName == "" {
				log.Warn("cookieName for JWTs not configured (cross login via JWT cookie will fail)")
			}
			if !ja.config.ForceJWTValidationViaDatabase {
				log.Warn("forceJWTValidationViaDatabase not set to true: CC will accept users and roles defined in JWTs regardless of its own database!")
			}
			if ja.config.TrustedExternalIssuer == "" {
				log.Warn("trustedExternalIssuer for JWTs not configured (cross login via JWT cookie will fail)")
			}
		} else {
			log.Warn("cookieName and trustedExternalIssuer for JWTs not configured (cross login via JWT cookie will fail)")
		}
	} else {
		ja.publicKeyCrossLogin = nil
		log.Debug("environment variable 'CROSS_LOGIN_JWT_PUBLIC_KEY' not set (cross login token based authentication will not work)")
	}

	return nil
}

func (ja *JWTAuthenticator) CanLogin(
	user *User,
	rw http.ResponseWriter,
	r *http.Request) bool {

	return (user != nil && user.AuthSource == AuthViaToken) ||
		r.Header.Get("Authorization") != "" ||
		r.URL.Query().Get("login-token") != ""
}

func (ja *JWTAuthenticator) Login(
	user *User,
	rw http.ResponseWriter,
	r *http.Request) (*User, error) {

	rawtoken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if rawtoken == "" {
		rawtoken = r.URL.Query().Get("login-token")
	}

	token, err := jwt.Parse(rawtoken, func(t *jwt.Token) (interface{}, error) {
		if t.Method == jwt.SigningMethodEdDSA {
			return ja.publicKey, nil
		}
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
	var roles []string
	if rawroles, ok := claims["roles"].([]interface{}); ok {
		for _, rr := range rawroles {
			if r, ok := rr.(string); ok {
				if isValidRole(r) {
					roles = append(roles, r)
				}
			}
		}
	}
	if rawrole, ok := claims["roles"].(string); ok {
		if isValidRole(rawrole) {
			roles = append(roles, rawrole)
		}
	}

	if user == nil {
		user, err = ja.auth.GetUser(sub)
		if err != nil && err != sql.ErrNoRows {
			log.Errorf("Error while loading user '%v'", sub)
			return nil, err
		} else if user == nil {
			user = &User{
				Username:   sub,
				Roles:      roles,
				AuthSource: AuthViaToken,
			}
			if err := ja.auth.AddUser(user); err != nil {
				log.Errorf("Error while adding user '%v' to auth from token", user.Username)
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

	// If no auth header was found, check for a certain cookie containing a JWT
	cookieName := ""
	cookieFound := false
	if ja.config != nil && ja.config.CookieName != "" {
		cookieName = ja.config.CookieName
	}

	// Try to read the JWT cookie
	if rawtoken == "" && cookieName != "" {
		jwtCookie, err := r.Cookie(cookieName)

		if err == nil && jwtCookie.Value != "" {
			rawtoken = jwtCookie.Value
			cookieFound = true
		}
	}

	// Because a user can also log in via a token, the
	// session cookie must be checked here as well:
	if rawtoken == "" {
		return ja.auth.AuthViaSession(rw, r)
	}

	// Try to parse JWT
	token, err := jwt.Parse(rawtoken, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodEdDSA {
			return nil, errors.New("only Ed25519/EdDSA supported")
		}

		// Is there more than one public key?
		if ja.publicKeyCrossLogin != nil &&
			ja.config != nil &&
			ja.config.TrustedExternalIssuer != "" {

			// Determine whether to use the external public key
			unvalidatedIssuer, success := t.Claims.(jwt.MapClaims)["iss"].(string)
			if success && unvalidatedIssuer == ja.config.TrustedExternalIssuer {
				// The (unvalidated) issuer seems to be the expected one,
				// use public cross login key from config
				return ja.publicKeyCrossLogin, nil
			}
		}

		// No cross login key configured or issuer not expected
		// Try own key
		return ja.publicKey, nil
	})
	if err != nil {
		log.Warn("Error while parsing token")
		return nil, err
	}

	// Check token validity
	if err := token.Claims.Valid(); err != nil {
		log.Warn("jwt token claims are not valid")
		return nil, err
	}

	// Token is valid, extract payload
	claims := token.Claims.(jwt.MapClaims)
	sub, _ := claims["sub"].(string)

	var roles []string

	// Validate user + roles from JWT against database?
	if ja.config != nil && ja.config.ForceJWTValidationViaDatabase {
		user, err := ja.auth.GetUser(sub)

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

	if cookieFound {
		// Create a session so that we no longer need the JTW Cookie
		session, err := ja.auth.sessionStore.New(r, "session")
		if err != nil {
			log.Errorf("session creation failed: %s", err.Error())
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return nil, err
		}

		if ja.auth.SessionMaxAge != 0 {
			session.Options.MaxAge = int(ja.auth.SessionMaxAge.Seconds())
		}
		session.Values["username"] = sub
		session.Values["roles"] = roles

		if err := ja.auth.sessionStore.Save(r, rw, session); err != nil {
			log.Warnf("session save failed: %s", err.Error())
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return nil, err
		}

		// (Ask browser to) Delete JWT cookie
		deletedCookie := &http.Cookie{
			Name:     cookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
		}
		http.SetCookie(rw, deletedCookie)
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
