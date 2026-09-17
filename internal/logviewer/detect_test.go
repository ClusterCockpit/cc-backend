// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package logviewer

import (
	"errors"
	"testing"
)

func testProbes(env map[string]string, hasJournalctl bool) probes {
	return probes{
		getenv: func(key string) string { return env[key] },
		lookPath: func(file string) (string, error) {
			if file == "journalctl" && hasJournalctl {
				return "/usr/bin/journalctl", nil
			}
			return "", errors.New("not found")
		},
	}
}

func TestDetectMode(t *testing.T) {
	tests := []struct {
		name          string
		env           map[string]string
		hasJournalctl bool
		want          Mode
	}{
		{"systemd unit", map[string]string{"INVOCATION_ID": "abc"}, true, ModeJournal},
		{"journal stream", map[string]string{"JOURNAL_STREAM": "8:1234"}, true, ModeJournal},
		{"notify socket", map[string]string{"NOTIFY_SOCKET": "/run/systemd/notify"}, true, ModeJournal},
		{"systemd without journalctl", map[string]string{"INVOCATION_ID": "abc"}, false, ModeMemory},
		{"journalctl without systemd", map[string]string{}, true, ModeMemory},
		{"container", map[string]string{"KUBERNETES_SERVICE_HOST": "10.0.0.1"}, false, ModeMemory},
		{"bare process", map[string]string{}, false, ModeMemory},
		{"empty env values", map[string]string{"INVOCATION_ID": ""}, true, ModeMemory},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectMode(testProbes(tt.env, tt.hasJournalctl)); got != tt.want {
				t.Errorf("detectMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInitHonoursConfiguredMode(t *testing.T) {
	tests := []struct {
		mode        Mode
		want        Mode
		wantEnabled bool
	}{
		{ModeDisabled, ModeDisabled, false},
		{ModeMemory, ModeMemory, true},
		{ModeJournal, ModeJournal, true},
		{"nonsense", ModeDisabled, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			defer resetForTest()
			resetForTest()

			Init(Options{Mode: tt.mode})

			if Active() != tt.want {
				t.Errorf("Active() = %v, want %v", Active(), tt.want)
			}
			if Enabled() != tt.wantEnabled {
				t.Errorf("Enabled() = %v, want %v", Enabled(), tt.wantEnabled)
			}
			if (Get() == nil) == tt.wantEnabled {
				t.Errorf("Get() = %v, but Enabled() = %v", Get(), tt.wantEnabled)
			}
			if Get() != nil && Get().Mode() != tt.want {
				t.Errorf("Get().Mode() = %v, want %v", Get().Mode(), tt.want)
			}
		})
	}
}

func TestInitJournalUsesDefaultUnit(t *testing.T) {
	defer resetForTest()
	resetForTest()

	Init(Options{Mode: ModeJournal})
	if src, ok := Get().(*journalSource); !ok || src.unit != defaultUnit {
		t.Errorf("expected journal source with unit %q, got %+v", defaultUnit, Get())
	}

	Init(Options{Mode: ModeJournal, SystemdUnit: "custom.service"})
	if src, ok := Get().(*journalSource); !ok || src.unit != "custom.service" {
		t.Errorf("expected journal source with unit 'custom.service', got %+v", Get())
	}
}
