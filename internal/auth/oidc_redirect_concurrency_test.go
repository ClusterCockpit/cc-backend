// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/go-chi/chi/v5"
	"github.com/go-jose/go-jose/v4"
	"golang.org/x/oauth2"
)

type auditOIDCFlow struct {
	code, redirect, nonce, verifier, username string
	cookies                                   []*http.Cookie
	baseURL, forwardedProto                   string
}

type auditRedirectProvider struct {
	server *httptest.Server
	mu     sync.Mutex
	flows  map[string]auditOIDCFlow
}

func newAuditRedirectOIDC(t *testing.T) (*OIDC, http.Handler, *auditRedirectProvider) {
	t.Helper()
	oldKeys := Keys
	t.Cleanup(func() { Keys = oldKeys })
	Keys = AuthConfig{OpenIDConfig: &OpenIDConfig{RoleMapping: map[string]string{"support": "support"}}}
	t.Setenv("OID_CLIENT_ID", "audit-redirect-client")
	t.Setenv("OID_CLIENT_SECRET", "audit-synthetic-secret")
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithHeader("kid", "audit-redirect-key"))
	if err != nil {
		t.Fatal(err)
	}
	p := &auditRedirectProvider{flows: make(map[string]auditOIDCFlow)}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer": p.server.URL, "authorization_endpoint": p.server.URL + "/authorize",
				"token_endpoint": p.server.URL + "/token", "userinfo_endpoint": p.server.URL + "/userinfo",
				"jwks_uri": p.server.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
				Key: &key.PublicKey, KeyID: "audit-redirect-key", Algorithm: "RS256", Use: "sig",
			}}})
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			p.mu.Lock()
			flow, ok := p.flows[r.Form.Get("code")]
			p.mu.Unlock()
			if !ok || r.Form.Get("redirect_uri") != flow.redirect || r.Form.Get("code_verifier") != flow.verifier {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant", "error_description": "code redirect_uri or PKCE mismatch"})
				return
			}
			claims, _ := json.Marshal(map[string]any{
				"iss": p.server.URL, "aud": "audit-redirect-client", "sub": flow.username,
				"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": flow.nonce,
				"preferred_username": flow.username, "realm_access": map[string]any{"roles": []string{"support"}},
				"projects": []string{"audit-oidc-project"},
			})
			signed, err := signer.Sign(claims)
			if err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			rawToken, err := signed.CompactSerialize()
			if err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "audit-access-" + flow.code, "token_type": "Bearer", "id_token": rawToken})
		case "/userinfo":
			code := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer audit-access-")
			p.mu.Lock()
			flow, ok := p.flows[code]
			p.mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"sub": flow.username, "preferred_username": flow.username})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.server.Close)
	Keys.OpenIDConfig.Provider = p.server.URL
	a := newSecurityAuthentication()
	oa := NewOIDC(a)
	router := chi.NewRouter()
	oa.RegisterEndpoints(router)
	return oa, a.sessionManager.LoadAndSave(router), p
}

func startAuditOIDCFlow(t *testing.T, h http.Handler, p *auditRedirectProvider, code, baseURL, forwardedProto, wantRedirect string) auditOIDCFlow {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, baseURL+"/oidc-login", nil)
	r.Header.Set("X-Forwarded-Proto", forwardedProto)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusFound {
		t.Fatalf("login status %d", w.Code)
	}
	location, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if location.Query().Get("redirect_uri") != wantRedirect {
		t.Fatalf("authorization redirect_uri=%q want=%q", location.Query().Get("redirect_uri"), wantRedirect)
	}
	flow := auditOIDCFlow{
		code: code, redirect: wantRedirect, baseURL: baseURL, forwardedProto: forwardedProto,
		username: "audit-user-" + code, cookies: w.Result().Cookies(), nonce: location.Query().Get("nonce"),
	}
	for _, cookie := range flow.cookies {
		if cookie.Name == "verifier" {
			flow.verifier = cookie.Value
		}
	}
	if flow.nonce == "" || flow.verifier == "" || location.Query().Get("code_challenge_method") != "S256" {
		t.Fatal("login did not issue complete nonce/PKCE cookies")
	}
	p.mu.Lock()
	p.flows[code] = flow
	p.mu.Unlock()
	return flow
}

func finishAuditOIDCFlow(t *testing.T, oa *OIDC, h http.Handler, flow auditOIDCFlow) {
	t.Helper()
	state := ""
	for _, cookie := range flow.cookies {
		if cookie.Name == "state" {
			state = cookie.Value
		}
	}
	r := httptest.NewRequest(http.MethodGet, flow.baseURL+"/oidc-callback?"+url.Values{"state": {state}, "code": {flow.code}}.Encode(), nil)
	r.Header.Set("X-Forwarded-Proto", flow.forwardedProto)
	for _, cookie := range flow.cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusTemporaryRedirect {
		t.Errorf("flow %s: callback=%d, want 307; body=%s", flow.code, w.Code, w.Body.String())
		return
	}
	sessionRequest := httptest.NewRequest(http.MethodGet, flow.baseURL+"/", nil)
	for _, cookie := range w.Result().Cookies() {
		sessionRequest.AddCookie(cookie)
	}
	var user *schema.User
	var err error
	oa.authentication.LoadSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { user, err = oa.authentication.AuthViaSession(w, r) })).ServeHTTP(httptest.NewRecorder(), sessionRequest)
	if err != nil || user == nil {
		t.Fatalf("flow %s: no valid session: %v", flow.code, err)
	}
	if user.Username != flow.username || !slices.Equal(user.Roles, []string{schema.GetRoleString(schema.RoleSupport)}) || !slices.Equal(user.Projects, []string{"audit-oidc-project"}) {
		t.Errorf("flow %s: unknown-user Token role/project fallback changed: %#v", flow.code, user)
	}
}

// The provider binds authorization codes to their issued redirect URI, as a
// real OIDC provider does. Each flow retains its own cookies, PKCE and nonce.
func TestOIDCInterleavedLoginCallbacksUseOwnRedirect(t *testing.T) {
	oa, h, p := newAuditRedirectOIDC(t)
	flows := []auditOIDCFlow{
		startAuditOIDCFlow(t, h, p, "plain", "http://audit-a.example", "", "http://audit-a.example/oidc-callback"),
		startAuditOIDCFlow(t, h, p, "tls", "https://audit-b.example", "", "https://audit-b.example/oidc-callback"),
		startAuditOIDCFlow(t, h, p, "proxy", "http://audit-c.example", "https", "https://audit-c.example/oidc-callback"),
	}
	for _, flow := range flows {
		finishAuditOIDCFlow(t, oa, h, flow)
	}
}

func TestOIDCSingleOriginCallbackControl(t *testing.T) {
	oa, h, p := newAuditRedirectOIDC(t)
	flow := startAuditOIDCFlow(t, h, p, "control", "https://audit-control.example", "", "https://audit-control.example/oidc-callback")
	finishAuditOIDCFlow(t, oa, h, flow)
}

func TestOIDCConcurrentLoginRedirectsAreRequestScoped(t *testing.T) {
	oa := &OIDC{client: &oauth2.Config{ClientID: "audit-redirect-client", Endpoint: oauth2.Endpoint{AuthURL: "http://audit-provider.invalid/authorize"}}}
	router := chi.NewRouter()
	oa.RegisterEndpoints(router)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := range 32 {
		wg.Go(func() {
			<-start
			for range 8 {
				base := fmt.Sprintf("https://audit-origin-%d.example", worker%2)
				r := httptest.NewRequest(http.MethodGet, base+"/oidc-login", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				location, err := url.Parse(w.Header().Get("Location"))
				if err != nil || w.Code != http.StatusFound || location.Query().Get("redirect_uri") != base+"/oidc-callback" {
					t.Errorf("request for %s returned status %d redirect %q error %v", base, w.Code, w.Header().Get("Location"), err)
				}
			}
		})
	}
	close(start)
	wg.Wait()
}
