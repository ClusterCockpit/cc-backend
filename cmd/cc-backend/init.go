// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"os"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/internal/util"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
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
    "addr": "127.0.0.1:8080",
    "archive": {
        "kind": "file",
        "path": "./var/job-archive"
    },
    "jwts": {
        "max-age": "2000h"
    },
    "apiAllowedIPs": [
      "*"
    ],
    "enable-resampling": {
      "trigger": 30,
      "resolutions": [
        600,
        300,
        120,
        60
      ]
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
		log.Exit("Directory ./var already exists. Cautiously exiting application initialization.")
	}

	if err := os.WriteFile("config.json", []byte(configString), 0o666); err != nil {
		log.Abortf("Could not write default ./config.json with permissions '0o666'. Application initialization failed, exited.\nError: %s\n", err.Error())
	}

	if err := os.WriteFile(".env", []byte(envString), 0o666); err != nil {
		log.Abortf("Could not write default ./.env file with permissions '0o666'. Application initialization failed, exited.\nError: %s\n", err.Error())
	}

	if err := os.Mkdir("var", 0o777); err != nil {
		log.Abortf("Could not create default ./var folder with permissions '0o777'. Application initialization failed, exited.\nError: %s\n", err.Error())
	}

	err := repository.MigrateDB("sqlite3", "./var/job.db")
	if err != nil {
		log.Abortf("Could not initialize default sqlite3 database as './var/job.db'. Application initialization failed, exited.\nError: %s\n", err.Error())
	}
}
