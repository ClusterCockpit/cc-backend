// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package secrets

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

func writeOverlay(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.local.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing overlay: %v", err)
	}
	return path
}

func TestEnvTakesPrecedence(t *testing.T) {
	cclog.Init("debug", false)
	path := writeOverlay(t, `{"secrets":{"FOO":"from-overlay"}}`)
	Init(true, path)
	t.Setenv("FOO", "from-env")

	v, src := Get("FOO")
	if v != "from-env" || src != SourceEnv {
		t.Errorf("expected env value, got %q from %s", v, src)
	}
}

func TestOverlayFallbackInDev(t *testing.T) {
	cclog.Init("debug", false)
	path := writeOverlay(t, `{"secrets":{"FOO":"from-overlay"}}`)
	Init(true, path)
	os.Unsetenv("FOO")

	v, src := Get("FOO")
	if v != "from-overlay" || src != SourceConfig {
		t.Errorf("expected overlay value, got %q from %s", v, src)
	}
}

func TestEmptyEnvFallsBackToOverlay(t *testing.T) {
	cclog.Init("debug", false)
	path := writeOverlay(t, `{"secrets":{"FOO":"from-overlay"}}`)
	Init(true, path)
	t.Setenv("FOO", "") // present but empty must not count

	v, src := Get("FOO")
	if v != "from-overlay" || src != SourceConfig {
		t.Errorf("expected overlay value for empty env, got %q from %s", v, src)
	}
}

func TestUnsetSecret(t *testing.T) {
	cclog.Init("debug", false)
	Init(true, filepath.Join(t.TempDir(), "does-not-exist.json"))
	os.Unsetenv("FOO")

	if v, src := Get("FOO"); src != SourceNone || v != "" {
		t.Errorf("expected unset, got %q from %s", v, src)
	}
}

func TestOverlayIgnoredInProd(t *testing.T) {
	// Production mode with no overlay file: env still works, overlay never used.
	cclog.Init("debug", false)
	Init(false, filepath.Join(t.TempDir(), "absent.json"))

	t.Setenv("FOO", "from-env")
	if v, src := Get("FOO"); v != "from-env" || src != SourceEnv {
		t.Errorf("expected env value in prod, got %q from %s", v, src)
	}

	os.Unsetenv("FOO")
	if _, src := Get("FOO"); src != SourceNone {
		t.Errorf("expected no overlay fallback in prod, got %s", src)
	}
}

func TestValidateAggregatesMissing(t *testing.T) {
	cclog.Init("debug", false)
	path := writeOverlay(t, `{"secrets":{"PRESENT":"x"}}`)
	Init(true, path)
	os.Unsetenv("MISSING_A")
	os.Unsetenv("MISSING_B")

	err := Validate([]Spec{
		{Name: "PRESENT", Required: true},
		{Name: "MISSING_A", Required: true, Feature: "LDAP"},
		{Name: "MISSING_B", Required: true, Feature: "OIDC"},
		{Name: "OPTIONAL", Required: false},
	})
	if err == nil {
		t.Fatal("expected aggregated error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"MISSING_A", "LDAP", "MISSING_B", "OIDC"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing expected substring %q", msg, want)
		}
	}
	if strings.Contains(msg, "PRESENT") || strings.Contains(msg, "OPTIONAL") {
		t.Errorf("error should not mention resolved/optional secrets: %q", msg)
	}
}

func TestValidateAllPresent(t *testing.T) {
	cclog.Init("debug", false)
	path := writeOverlay(t, `{"secrets":{"A":"1","B":"2"}}`)
	Init(true, path)

	if err := Validate([]Spec{{Name: "A", Required: true}, {Name: "B", Required: true}}); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

// TestProdGuardAborts verifies that a present overlay file aborts startup when
// not in dev mode. Init calls os.Exit, so it runs in a subprocess.
func TestProdGuardAborts(t *testing.T) {
	if os.Getenv("RUN_PROD_GUARD") == "1" {
		cclog.Init("debug", false)
		path := os.Getenv("OVERLAY_PATH")
		Init(false, path) // must os.Exit(1)
		return
	}

	path := writeOverlay(t, `{"secrets":{"FOO":"bar"}}`)
	cmd := exec.Command(os.Args[0], "-test.run=TestProdGuardAborts")
	cmd.Env = append(os.Environ(), "RUN_PROD_GUARD=1", "OVERLAY_PATH="+path)
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); !ok || ee.Success() {
		t.Fatalf("expected non-zero exit from prod guard, got %v", err)
	}
}
