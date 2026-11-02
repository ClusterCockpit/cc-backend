// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/gob"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/alexedwards/scs/v2"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
)

// runSecurityOIDCCallback exercises the complete callback against a local provider.
// All identities, keys, claims and database fixtures are synthetic.
func runSecurityOIDCCallback(t *testing.T, userInfoSubject string, dbRoles bool, resourceRoles map[string][]string, realmRoles []string) (*Authentication, *httptest.ResponseRecorder) {
	t.Helper()
	oldKeys := Keys
	t.Cleanup(func() { Keys = oldKeys })
	Keys = AuthConfig{OpenIDConfig: &OpenIDConfig{
		RoleMapping: map[string]string{"admin": "admin", "support": "support", "manager": "manager"},
	}}
	t.Setenv("OID_CLIENT_ID", "audit-client")
	t.Setenv("OID_CLIENT_SECRET", "audit-synthetic-secret")
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: privateKey},
		(&jose.SignerOptions{}).WithHeader("kid", "audit-key"))
	if err != nil {
		t.Fatal(err)
	}
	var providerServer *httptest.Server
	providerServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                                providerServer.URL,
				"authorization_endpoint":                providerServer.URL + "/authorize",
				"token_endpoint":                        providerServer.URL + "/token",
				"userinfo_endpoint":                     providerServer.URL + "/userinfo",
				"jwks_uri":                              providerServer.URL + "/keys",
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
				Key: &privateKey.PublicKey, KeyID: "audit-key", Algorithm: "RS256", Use: "sig",
			}}})
		case "/token":
			claims, _ := json.Marshal(map[string]any{
				"iss": providerServer.URL, "sub": "audit-signed-subject", "aud": "audit-client",
				"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
				"nonce": "audit-nonce", "preferred_username": "audit-new-user",
				"auth_level": 3, "groups": []string{"audit-allowed-group"},
				"resource_access": resourceRoleClaims(resourceRoles),
				"realm_access":    map[string]any{"roles": realmRoles},
			})
			signed, signErr := signer.Sign(claims)
			if signErr != nil {
				t.Error(signErr)
				return
			}
			token, serializeErr := signed.CompactSerialize()
			if serializeErr != nil {
				t.Error(serializeErr)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "audit-synthetic-access", "token_type": "Bearer", "id_token": token,
			})
		case "/userinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub": userInfoSubject, "preferred_username": "audit-new-user",
				"auth_level": 3, "groups": []string{"audit-allowed-group"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(providerServer.Close)
	Keys.OpenIDConfig.Provider = providerServer.URL
	provider, err := oidc.NewProvider(context.Background(), providerServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	a := newSecurityAuthentication()
	oa := NewOIDC(a)
	oa.provider = provider
	oa.client.RedirectURL = "http://audit.example/oidc-callback"
	req := httptest.NewRequest(http.MethodGet, "http://audit.example/oidc-callback?state=audit-state&code=audit-code", nil)
	for name, value := range map[string]string{"state": "audit-state", "nonce": "audit-nonce", "verifier": "audit-verifier"} {
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	response := httptest.NewRecorder()
	a.sessionManager.LoadAndSave(http.HandlerFunc(oa.OAuth2Callback)).ServeHTTP(response, req)
	return a, response
}

func resourceRoleClaims(roles map[string][]string) map[string]any {
	result := make(map[string]any, len(roles))
	for clientID, clientRoles := range roles {
		result[clientID] = map[string]any{"roles": clientRoles}
	}
	return result
}

func securitySessionUser(t *testing.T, a *Authentication, response *httptest.ResponseRecorder) *schema.User {
	t.Helper()
	if response.Code != http.StatusTemporaryRedirect {
		t.Fatalf("callback failed: status %d, body %s", response.Code, response.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, "http://audit.example/", nil)
	for _, cookie := range response.Result().Cookies() {
		req.AddCookie(cookie)
	}
	var user *schema.User
	var err error
	a.LoadSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { user, err = a.AuthViaSession(w, r) })).ServeHTTP(httptest.NewRecorder(), req)
	if err != nil || user == nil {
		t.Fatalf("no valid session: %v", err)
	}
	return user
}

func TestOIDCCallbackBindsUserInfoSubject(t *testing.T) {
	for _, tt := range []struct {
		name, subject string
		wantStatus    int
	}{
		{"matching", "audit-signed-subject", http.StatusTemporaryRedirect},
		{"different", "audit-different-subject", http.StatusUnauthorized},
		{"missing", "", http.StatusUnauthorized},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, response := runSecurityOIDCCallback(t, tt.subject, false, nil, nil)
			if response.Code != tt.wantStatus {
				t.Fatalf("callback status %d, want %d", response.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusTemporaryRedirect {
				user := securitySessionUser(t, a, response)
				if user.Username != "audit-new-user" {
					t.Fatalf("username %q", user.Username)
				}
			} else {
				for _, cookie := range response.Result().Cookies() {
					if cookie.Name == "session" && cookie.MaxAge >= 0 {
						t.Fatal("rejected identity created a session")
					}
				}
			}
		})
	}
}

// Use the current session middleware and in-memory store; never a real user DB.
func newSecurityAuthentication() *Authentication {
	gob.Register([]string{})
	return &Authentication{sessionManager: scs.New()}
}
