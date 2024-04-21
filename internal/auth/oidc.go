// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/mux"
	"golang.org/x/oauth2"
)

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
	}
	http.SetCookie(w, c)
}

func NewOIDC(a *Authentication) *OIDC {
	provider, err := oidc.NewProvider(context.Background(), config.Keys.OpenIDConfig.Provider)
	if err != nil {
		log.Fatal(err)
	}
	clientID := os.Getenv("OID_CLIENT_ID")
	if clientID == "" {
		log.Warn("environment variable 'OID_CLIENT_ID' not set (Open ID connect auth will not work)")
	}
	clientSecret := os.Getenv("OID_CLIENT_SECRET")
	if clientSecret == "" {
		log.Warn("environment variable 'OID_CLIENT_SECRET' not set (Open ID connect auth will not work)")
	}

	client := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  "oidc-callback",
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	oa := &OIDC{provider: provider, client: client, clientID: clientID, authentication: a}

	return oa
}

func (oa *OIDC) RegisterEndpoints(r *mux.Router) {
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
	token, err := oa.client.Exchange(context.Background(), code, oauth2.VerifierOption(codeVerifier))
	if err != nil {
		http.Error(rw, "Failed to exchange token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	userInfo, err := oa.provider.UserInfo(context.Background(), oauth2.StaticTokenSource(token))
	if err != nil {
		http.Error(rw, "Failed to get userinfo: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// // Extract the ID Token from OAuth2 token.
	// rawIDToken, ok := token.Extra("id_token").(string)
	// if !ok {
	// 	http.Error(rw, "Cannot access idToken", http.StatusInternalServerError)
	// }
	//
	// verifier := oa.provider.Verifier(&oidc.Config{ClientID: oa.clientID})
	// // Parse and verify ID Token payload.
	// idToken, err := verifier.Verify(context.Background(), rawIDToken)
	// if err != nil {
	// 	http.Error(rw, "Failed to extract idToken: "+err.Error(), http.StatusInternalServerError)
	// }

	projects := make([]string, 0)

	// Extract custom claims
	var claims struct {
		Username string `json:"preferred_username"`
		Name     string `json:"name"`
		Profile  struct {
			Client struct {
				Roles []string `json:"roles"`
			} `json:"clustercockpit"`
		} `json:"resource_access"`
	}
	if err := userInfo.Claims(&claims); err != nil {
		http.Error(rw, "Failed to extract Claims: "+err.Error(), http.StatusInternalServerError)
	}

	var roles []string
	for _, r := range claims.Profile.Client.Roles {
		switch r {
		case "user":
			roles = append(roles, schema.GetRoleString(schema.RoleUser))
		case "admin":
			roles = append(roles, schema.GetRoleString(schema.RoleAdmin))
		}
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

	if config.Keys.OpenIDConfig.SyncUserOnLogin {
		persistUser(user)
	}

	oa.authentication.SaveSession(rw, r, user)
	log.Infof("login successfull: user: %#v (roles: %v, projects: %v)", user.Username, user.Roles, user.Projects)
	ctx := context.WithValue(r.Context(), repository.ContextUserKey, user)
	http.RedirectHandler("/", http.StatusTemporaryRedirect).ServeHTTP(rw, r.WithContext(ctx))
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

	// Redirect user to consent page to ask for permission
	url := oa.client.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(codeVerifier))
	http.Redirect(rw, r, url, http.StatusFound)
}
