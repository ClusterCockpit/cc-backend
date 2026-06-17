// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package secrets provides a layered resolver for sensitive configuration
// values (JWT keys, session keys, LDAP/OIDC credentials).
//
// # Resolution order
//
// Each secret is resolved in the following order of precedence:
//
//  1. Environment variable (always honored, takes precedence).
//  2. The gitignored dev overlay file (config.local.json) — only honored when
//     running in development mode (the -dev flag).
//  3. Otherwise the secret is unset; Validate reports it if it is required.
//
// # Production guard
//
// File-based secrets are a development convenience only. In production (the
// -dev flag is NOT set) the overlay file is never read, and if a
// config.local.json is found on disk Init aborts the process. This prevents a
// deployment from silently relying on committed/leaked dev secrets and forces
// production secrets to be supplied through the environment.
package secrets

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/util"
)

// Source identifies where a resolved secret value came from.
type Source int

const (
	// SourceNone indicates the secret could not be resolved.
	SourceNone Source = iota
	// SourceEnv indicates the secret was resolved from an environment variable.
	SourceEnv
	// SourceConfig indicates the secret was resolved from the dev overlay file.
	SourceConfig
)

// String returns a human-readable description of the source, used for logging.
func (s Source) String() string {
	switch s {
	case SourceEnv:
		return "environment"
	case SourceConfig:
		return "config.local.json"
	default:
		return "<unset>"
	}
}

// overlayFile mirrors the structure of the gitignored config.local.json:
//
//	{ "secrets": { "JWT_PUBLIC_KEY": "...", ... } }
type overlayFile struct {
	Secrets map[string]string `json:"secrets"`
}

var (
	devMode bool
	overlay map[string]string
)

// Init configures the resolver. When devMode is true and overlayPath exists, the
// overlay secrets are loaded as a fallback source. When devMode is false the
// overlay is ignored; if overlayPath nonetheless exists on disk Init aborts the
// process (production guard). See the package documentation for the full
// resolution order.
func Init(dev bool, overlayPath string) {
	devMode = dev
	overlay = nil

	exists := util.CheckFileExists(overlayPath)

	if !dev {
		if exists {
			cclog.Fatalf("secrets: refusing to start: %q is present but the server is not running in development mode (-dev). "+
				"File-based secrets are a development-only convenience; provide production secrets via environment variables and remove the file.", overlayPath)
		}
		return
	}

	if !exists {
		return
	}

	raw, err := os.ReadFile(overlayPath)
	if err != nil {
		cclog.Fatalf("secrets: could not read overlay file %q: %v", overlayPath, err)
	}

	// Unknown top-level fields are tolerated so the file may carry "//" comment
	// keys (JSON has no native comments).
	var of overlayFile
	if err := json.Unmarshal(raw, &of); err != nil {
		cclog.Fatalf("secrets: could not parse overlay file %q: %v", overlayPath, err)
	}

	// Drop "//" comment entries inside the secrets object.
	overlay = make(map[string]string, len(of.Secrets))
	for k, v := range of.Secrets {
		if strings.HasPrefix(k, "//") {
			continue
		}
		overlay[k] = v
	}
	cclog.Infof("secrets: loaded %d dev secret(s) from %q (env variables still take precedence)", len(overlay), overlayPath)
}

// Get resolves a single secret following the precedence order: environment
// variable first, then the dev overlay (development mode only). It returns the
// resolved value and the Source it came from; SourceNone with an empty value
// means the secret is unset.
func Get(name string) (string, Source) {
	if v, ok := os.LookupEnv(name); ok && v != "" {
		return v, SourceEnv
	}
	if devMode && overlay != nil {
		if v, ok := overlay[name]; ok && v != "" {
			return v, SourceConfig
		}
	}
	return "", SourceNone
}

// Spec describes a secret that the application may require. Feature is a short
// label (e.g. "LDAP", "OIDC") used to make validation errors actionable.
type Spec struct {
	Name     string
	Required bool
	Feature  string
}

// Validate resolves every spec, logging the source of each resolved secret at
// debug level. It collects all missing required secrets (rather than failing on
// the first) and returns a single aggregated error naming each of them. It
// returns nil when every required secret resolves to a non-empty value.
func Validate(specs []Spec) error {
	var missing []string

	for _, spec := range specs {
		_, src := Get(spec.Name)
		if src == SourceNone {
			if spec.Required {
				if spec.Feature != "" {
					missing = append(missing, fmt.Sprintf("%s (required for %s)", spec.Name, spec.Feature))
				} else {
					missing = append(missing, spec.Name)
				}
			} else {
				cclog.Debugf("secrets: optional secret %s is not set", spec.Name)
			}
			continue
		}
		cclog.Debugf("secrets: resolved %s from %s", spec.Name, src)
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required secret(s):\n  - %s\n"+
			"Set them via environment variables, or for local development add them to config.local.json under \"secrets\"",
			strings.Join(missing, "\n  - "))
	}
	return nil
}
