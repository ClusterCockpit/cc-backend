// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package logviewer

import (
	"os"
	"os/exec"
)

// probes abstracts the operating system lookups used for backend detection so
// that tests do not depend on the host they run on.
type probes struct {
	getenv   func(string) string
	lookPath func(string) (string, error)
}

func osProbes() probes {
	return probes{getenv: os.Getenv, lookPath: exec.LookPath}
}

// systemdEnv are the variables systemd sets for the processes it starts.
// INVOCATION_ID is set for every unit since systemd 232 and is therefore the
// signal that matters in practice; JOURNAL_STREAM is set when stdout/stderr are
// connected to journald and NOTIFY_SOCKET only for Type=notify units.
var systemdEnv = []string{"INVOCATION_ID", "JOURNAL_STREAM", "NOTIFY_SOCKET"}

// detectMode returns ModeJournal when this process runs as a systemd unit and
// journalctl is usable, and ModeMemory otherwise. The in-process buffer works
// in every environment, so there is no need to detect containers specifically.
//
// Running the binary from a shell on a systemd host deliberately yields
// ModeMemory: the unit's journal would contain a different process' log.
func detectMode(p probes) Mode {
	underSystemd := false
	for _, key := range systemdEnv {
		if p.getenv(key) != "" {
			underSystemd = true
			break
		}
	}

	if underSystemd {
		if _, err := p.lookPath("journalctl"); err == nil {
			return ModeJournal
		}
	}

	return ModeMemory
}
