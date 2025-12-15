// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package taskmanager provides a background task scheduler for the cc-backend.
// It manages various periodic tasks such as job archiving (retention),
// database compression, LDAP synchronization, and statistic updates.
//
// The package uses the gocron library to schedule tasks. Configuration
// for the tasks is provided via JSON configs passed to the Start function.
package taskmanager
