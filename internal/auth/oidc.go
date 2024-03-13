// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package auth

import (
	"context"
	"log"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/mux"
	"golang.org/x/oauth2"
)

type OIDC struct {
	client       *oauth2.Config
	provider     *oidc.Provider
	state        string
	codeVerifier string
}

func (oa *OIDC) Init(r *mux.Router) error {
	oa.client = &oauth2.Config{
		ClientID:     "YOUR_CLIENT_ID",
		ClientSecret: "YOUR_CLIENT_SECRET",
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://provider.com/o/oauth2/auth",
			TokenURL: "https://provider.com/o/oauth2/token",
		},
	}

	provider, err := oidc.NewProvider(context.Background(), "https://provider")
	if err != nil {
		log.Fatal(err)
	}

	oa.provider = provider

	r.HandleFunc("/oidc-login", oa.OAuth2Login)
	r.HandleFunc("/oidc-callback", oa.OAuth2Callback)

	return nil
}

func (oa *OIDC) OAuth2Callback(rw http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	state := r.Form.Get("state")
	if state != oa.state {
		http.Error(rw, "State invalid", http.StatusBadRequest)
		return
	}
	code := r.Form.Get("code")
	if code == "" {
		http.Error(rw, "Code not found", http.StatusBadRequest)
		return
	}
	token, err := oa.client.Exchange(context.Background(), code, oauth2.VerifierOption(oa.codeVerifier))
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (oa *OIDC) OAuth2Login(rw http.ResponseWriter, r *http.Request) {
	// use PKCE to protect against CSRF attacks
	oa.codeVerifier = oauth2.GenerateVerifier()

	// Redirect user to consent page to ask for permission
	url := oa.client.AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(oa.codeVerifier))
	http.Redirect(rw, r, url, http.StatusFound)
}
