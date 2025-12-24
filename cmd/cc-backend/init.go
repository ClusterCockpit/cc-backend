// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package main provides the entry point for the ClusterCockpit backend server.
// This file contains bootstrap logic for initializing the environment,
// creating default configuration files, and setting up the database.
package main

import (
	"encoding/json"
	"os"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/util"
)

const envString = `
# Base64 encoded Ed25519 keys (DO NOT USE THESE TWO IN PRODUCTION!)
# You can generate your own keypair using the gen-keypair tool
JWT_PUBLIC_KEY="kzfYrYy+TzpanWZHJ5qSdMj5uKUWgq74BWhQG6copP0="
JWT_PRIVATE_KEY="dtPC/6dWJFKZK7KZ78CvWuynylOmjBFyMsUWArwmodOTN9itjL5POlqdZkcnmpJ0yPm4pRaCrvgFaFAbpyik/Q=="

# Some random bytes used as secret for cookie-based sessions (DO NOT USE THIS ONE IN PRODUCTION)
SESSION_KEY="67d829bf61dc5f87a73fd814e2c9f629"
`

const configString = `
{
  "main": {
    "addr": "127.0.0.1:8080",
    "short-running-jobs-duration": 300,
    "resampling": {
      "minimumPoints": 600,
      "trigger": 180,
      "resolutions": [
        240,
        60
      ]
    },
    "apiAllowedIPs": [
      "*"
    ],
    "emission-constant": 317
  },
  "cron": {
    "commit-job-worker": "2m",
    "duration-worker": "5m",
    "footprint-worker": "10m"
  },
  "archive": {
    "kind": "file",
    "path": "./var/job-archive"
  },
  "auth": {
    "jwts": {
      "max-age": "2000h"
    }
  },
 "clusters": [
     {
         "name": "name",
         "metricDataRepository": {
             "kind": "cc-metric-store",
             "url": "http://localhost:8082",
             "token": ""
         },
         "filterRanges": {
             "numNodes": {
                 "from": 1,
                 "to": 64
             },
             "duration": {
                 "from": 0,
                 "to": 86400
             },
             "startTime": {
                 "from": "2023-01-01T00:00:00Z",
                 "to": null
             }
         }
     }
 ]
}
`

func initEnv() {
	if util.CheckFileExists("var") {
		cclog.Exit("Directory ./var already exists. Cautiously exiting application initialization.")
	}

	if err := os.WriteFile("config.json", []byte(configString), 0o666); err != nil {
		cclog.Abortf("Could not write default ./config.json with permissions '0o666'. Application initialization failed, exited.\nError: %s\n", err.Error())
	}

	if err := os.WriteFile(".env", []byte(envString), 0o666); err != nil {
		cclog.Abortf("Could not write default ./.env file with permissions '0o666'. Application initialization failed, exited.\nError: %s\n", err.Error())
	}

	if err := os.Mkdir("var", 0o777); err != nil {
		cclog.Abortf("Could not create default ./var folder with permissions '0o777'. Application initialization failed, exited.\nError: %s\n", err.Error())
	}

	err := repository.MigrateDB("./var/job.db")
	if err != nil {
		cclog.Abortf("Could not initialize default SQLite database as './var/job.db'. Application initialization failed, exited.\nError: %s\n", err.Error())
	}
	if err := os.Mkdir("var/job-archive", 0o777); err != nil {
		cclog.Abortf("Could not create default ./var/job-archive folder with permissions '0o777'. Application initialization failed, exited.\nError: %s\n", err.Error())
	}
	archiveCfg := "{\"kind\": \"file\",\"path\": \"./var/job-archive\"}"
	if err := archive.Init(json.RawMessage(archiveCfg), config.Keys.DisableArchive); err != nil {
		cclog.Abortf("Could not initialize job-archive, exited.\nError: %s\n", err.Error())
	}
}
