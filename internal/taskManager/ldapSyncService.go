// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package taskManager

import (
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/go-co-op/gocron/v2"
)

func RegisterLdapSyncService(ds string) {
	interval, err := parseDuration(ds)
	if err != nil {
		log.Warnf("Could not parse duration for sync interval: %v",
			ds)
		return
	}

	auth := auth.GetAuthInstance()

	log.Info("Register LDAP sync service")
	s.NewJob(gocron.DurationJob(interval),
		gocron.NewTask(
			func() {
				t := time.Now()
				log.Printf("ldap sync started at %s", t.Format(time.RFC3339))
				if err := auth.LdapAuth.Sync(); err != nil {
					log.Errorf("ldap sync failed: %s", err.Error())
				}
				log.Print("ldap sync done")
			}))
}
