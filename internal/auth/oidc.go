// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	"golang.org/x/oauth2"
)

type OpenIDConfig struct {
	Provider          string `json:"provider"`
	SyncUserOnLogin   bool   `json:"sync-user-on-login"`
	UpdateUserOnLogin bool   `json:"update-user-on-login"`
}

type OIDC struct {
	client         *oauth2.Config
	provider       *oidc.Provider
	authentication *Authentication
	clientID       string
}

func randString(nByte int) (string, error) {
	b := make([]byte, nByte)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func setCallbackCookie(w http.ResponseWriter, r *http.Request, name, value string) {
	c := &http.Cookie{
		Name:     name,
		Value:    value,
		MaxAge:   int(time.Hour.Seconds()),
		Secure:   r.TLS != nil,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, c)
}

// NewOIDC creates a new OIDC authenticator with the configured provider
func NewOIDC(a *Authentication) *OIDC {
	// Use context with timeout for provider initialization
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider, err := oidc.NewProvider(ctx, Keys.OpenIDConfig.Provider)
	if err != nil {
		cclog.Fatal(err)
	}
	clientID := os.Getenv("OID_CLIENT_ID")
	if clientID == "" {
		cclog.Warn("environment variable 'OID_CLIENT_ID' not set (Open ID connect auth will not work)")
	}
	clientSecret := os.Getenv("OID_CLIENT_SECRET")
	if clientSecret == "" {
		cclog.Warn("environment variable 'OID_CLIENT_SECRET' not set (Open ID connect auth will not work)")
	}

	client := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile"},
	}

	oa := &OIDC{provider: provider, client: client, clientID: clientID, authentication: a}

	return oa
}

func (oa *OIDC) RegisterEndpoints(r chi.Router) {
	r.HandleFunc("/oidc-login", oa.OAuth2Login)
	r.HandleFunc("/oidc-callback", oa.OAuth2Callback)
}

func (oa *OIDC) OAuth2Callback(rw http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("state")
	if err != nil {
		http.Error(rw, "state cookie not found", http.StatusBadRequest)
		return
	}
	state := c.Value

	c, err = r.Cookie("verifier")
	if err != nil {
		http.Error(rw, "verifier cookie not found", http.StatusBadRequest)
		return
	}
	codeVerifier := c.Value

	_ = r.ParseForm()
	if r.Form.Get("state") != state {
		http.Error(rw, "State invalid", http.StatusBadRequest)
		return
	}
	code := r.Form.Get("code")
	if code == "" {
		http.Error(rw, "Code not found", http.StatusBadRequest)
		return
	}
	// Exchange authorization code for token with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	token, err := oa.client.Exchange(ctx, code, oauth2.VerifierOption(codeVerifier))
	if err != nil {
		cclog.Errorf("token exchange failed: %s", err.Error())
		http.Error(rw, "Authentication failed during token exchange", http.StatusInternalServerError)
		return
	}

	// Get user info from OIDC provider with same timeout
	userInfo, err := oa.provider.UserInfo(ctx, oauth2.StaticTokenSource(token))
	if err != nil {
		cclog.Errorf("failed to get userinfo: %s", err.Error())
		http.Error(rw, "Failed to retrieve user information", http.StatusInternalServerError)
		return
	}

	// Verify ID token and nonce to prevent replay attacks
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(rw, "ID token not found in response", http.StatusInternalServerError)
		return
	}

	nonceCookie, err := r.Cookie("nonce")
	if err != nil {
		http.Error(rw, "nonce cookie not found", http.StatusBadRequest)
		return
	}

	verifier := oa.provider.Verifier(&oidc.Config{ClientID: oa.clientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		cclog.Errorf("ID token verification failed: %s", err.Error())
		http.Error(rw, "ID token verification failed", http.StatusInternalServerError)
		return
	}

	if idToken.Nonce != nonceCookie.Value {
		http.Error(rw, "Nonce mismatch", http.StatusBadRequest)
		return
	}

	projects := make([]string, 0)

	// Extract custom claims from userinfo
	var claims struct {
		Username string `json:"preferred_username"`
		Name     string `json:"name"`
		// Keycloak realm-level roles
		RealmAccess struct {
			Roles []string `json:"roles"`
		} `json:"realm_access"`
		// Keycloak client-level roles
		ResourceAccess struct {
			Client struct {
				Roles []string `json:"roles"`
			} `json:"clustercockpit"`
		} `json:"resource_access"`
	}
	if err := userInfo.Claims(&claims); err != nil {
		cclog.Errorf("failed to extract claims: %s", err.Error())
		http.Error(rw, "Failed to extract user claims", http.StatusInternalServerError)
		return
	}

	if claims.Username == "" {
		http.Error(rw, "Username claim missing from OIDC provider", http.StatusBadRequest)
		return
	}

	// Merge roles from both client-level and realm-level access
	oidcRoles := append(claims.ResourceAccess.Client.Roles, claims.RealmAccess.Roles...)

	roleSet := make(map[string]bool)
	for _, r := range oidcRoles {
		switch r {
		case "user":
			roleSet[schema.GetRoleString(schema.RoleUser)] = true
		case "admin":
			roleSet[schema.GetRoleString(schema.RoleAdmin)] = true
		case "manager":
			roleSet[schema.GetRoleString(schema.RoleManager)] = true
		case "support":
			roleSet[schema.GetRoleString(schema.RoleSupport)] = true
		}
	}

	var roles []string
	for role := range roleSet {
		roles = append(roles, role)
	}

	if len(roles) == 0 {
		roles = append(roles, schema.GetRoleString(schema.RoleUser))
	}

	user := &schema.User{
		Username:   claims.Username,
		Name:       claims.Name,
		Roles:      roles,
		Projects:   projects,
		AuthSource: schema.AuthViaOIDC,
	}

	if Keys.OpenIDConfig.SyncUserOnLogin || Keys.OpenIDConfig.UpdateUserOnLogin {
		handleOIDCUser(user)
	}

	if err := oa.authentication.SaveSession(rw, r, user); err != nil {
		cclog.Errorf("session save failed for user %q: %s", user.Username, err.Error())
		http.Error(rw, "Failed to create session", http.StatusInternalServerError)
		return
	}
	cclog.Infof("login successful: user: %#v (roles: %v, projects: %v)", user.Username, user.Roles, user.Projects)
	userCtx := context.WithValue(r.Context(), repository.ContextUserKey, user)
	http.RedirectHandler("/", http.StatusTemporaryRedirect).ServeHTTP(rw, r.WithContext(userCtx))
}

func (oa *OIDC) OAuth2Login(rw http.ResponseWriter, r *http.Request) {
	state, err := randString(16)
	if err != nil {
		http.Error(rw, "Internal error", http.StatusInternalServerError)
		return
	}
	setCallbackCookie(rw, r, "state", state)

	// use PKCE to protect against CSRF attacks
	codeVerifier := oauth2.GenerateVerifier()
	setCallbackCookie(rw, r, "verifier", codeVerifier)

	// Generate nonce for ID token replay protection
	nonce, err := randString(16)
	if err != nil {
		http.Error(rw, "Internal error", http.StatusInternalServerError)
		return
	}
	setCallbackCookie(rw, r, "nonce", nonce)

	// Build redirect URL from the incoming request
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	oa.client.RedirectURL = fmt.Sprintf("%s://%s/oidc-callback", scheme, r.Host)

	// Redirect user to consent page to ask for permission
	url := oa.client.AuthCodeURL(state, oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(codeVerifier),
		oidc.Nonce(nonce))
	http.Redirect(rw, r, url, http.StatusFound)
}
