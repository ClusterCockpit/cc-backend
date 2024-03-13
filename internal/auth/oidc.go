// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/mux"
	"golang.org/x/oauth2"
)

type OIDC struct {
	client   *oauth2.Config
	provider *oidc.Provider
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

func (oa *OIDC) Init(r *mux.Router) error {
	provider, err := oidc.NewProvider(context.Background(), "https://provider")
	if err != nil {
		log.Fatal(err)
	}
	oa.provider = provider

	oa.client = &oauth2.Config{
		ClientID:     "YOUR_CLIENT_ID",
		ClientSecret: "YOUR_CLIENT_SECRET",
		Endpoint:     provider.Endpoint(),
		RedirectURL:  "https://" + config.Keys.Addr + "/oidc-callback",
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	r.HandleFunc("/oidc-login", oa.OAuth2Login)
	r.HandleFunc("/oidc-callback", oa.OAuth2Callback)

	return nil
}

func (oa *OIDC) OAuth2Callback(rw http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("state")
	if err != nil {
		http.Error(rw, "state not found", http.StatusBadRequest)
		return
	}

	str := strings.Split(c.Value, " ")
	state := str[0]
	codeVerifier := str[1]

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
}

func (oa *OIDC) OAuth2Login(rw http.ResponseWriter, r *http.Request) {
	state, err := randString(16)
	if err != nil {
		http.Error(rw, "Internal error", http.StatusInternalServerError)
		return
	}

	// use PKCE to protect against CSRF attacks
	codeVerifier := oauth2.GenerateVerifier()

	setCallbackCookie(rw, r, "state", strings.Join([]string{state, codeVerifier}, " "))

	// Redirect user to consent page to ask for permission
	url := oa.client.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(codeVerifier))
	http.Redirect(rw, r, url, http.StatusFound)
}
