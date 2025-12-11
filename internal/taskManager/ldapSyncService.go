// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package taskManager

import (
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/auth"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/go-co-op/gocron/v2"
)

func RegisterLdapSyncService(ds string) {
	interval, err := parseDuration(ds)
	if err != nil {
		cclog.Warnf("Could not parse duration for sync interval: %v",
			ds)
		return
	}

	auth := auth.GetAuthInstance()

	cclog.Info("Register LDAP sync service")
	s.NewJob(gocron.DurationJob(interval),
		gocron.NewTask(
			func() {
				t := time.Now()
				cclog.Infof("ldap sync started at %s", t.Format(time.RFC3339))
				if err := auth.LdapAuth.Sync(); err != nil {
					cclog.Errorf("ldap sync failed: %s", err.Error())
				}
				cclog.Print("ldap sync done")
			}))
}
