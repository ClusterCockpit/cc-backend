// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

var configSchema = `
	{
  "type": "object",
  "properties": {
    "addr": {
      "description": "Address where the http (or https) server will listen on (for example: 'localhost:80').",
      "type": "string"
    },
    "apiAllowedIPs": {
      "description": "Addresses from which secured API endpoints can be reached",
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "user": {
      "description": "Drop root permissions once .env was read and the port was taken. Only applicable if using privileged port.",
      "type": "string"
    },
    "group": {
      "description": "Drop root permissions once .env was read and the port was taken. Only applicable if using privileged port.",
      "type": "string"
    },
    "disable-authentication": {
      "description": "Disable authentication (for everything: API, Web-UI, ...).",
      "type": "boolean"
    },
    "embed-static-files": {
      "description": "If all files in web/frontend/public should be served from within the binary itself (they are embedded) or not.",
      "type": "boolean"
    },
    "static-files": {
      "description": "Folder where static assets can be found, if embed-static-files is false.",
      "type": "string"
    },
    "db": {
      "description": "Path to SQLite database file (e.g., './var/job.db')",
      "type": "string"
    },
    "disable-archive": {
      "description": "Keep all metric data in the metric data repositories, do not write to the job-archive.",
      "type": "boolean"
    },
    "enable-job-taggers": {
      "description": "Turn on automatic application and jobclass taggers",
      "type": "boolean"
    },
    "validate": {
      "description": "Validate all input json documents against json schema.",
      "type": "boolean"
    },
    "session-max-age": {
      "description": "Specifies for how long a session shall be valid  as a string parsable by time.ParseDuration(). If 0 or empty, the session/token does not expire!",
      "type": "string"
    },
    "https-cert-file": {
      "description": "Filepath to SSL certificate. If also https-key-file is set use HTTPS using those certificates.",
      "type": "string"
    },
    "https-key-file": {
      "description": "Filepath to SSL key file. If also https-cert-file is set use HTTPS using those certificates.",
      "type": "string"
    },
    "redirect-http-to": {
      "description": "If not the empty string and addr does not end in :80, redirect every request incoming at port 80 to that url.",
      "type": "string"
    },
    "stop-jobs-exceeding-walltime": {
      "description": "If not zero, automatically mark jobs as stopped running X seconds longer than their walltime. Only applies if walltime is set for job.",
      "type": "integer"
    },
    "short-running-jobs-duration": {
      "description": "Do not show running jobs shorter than X seconds.",
      "type": "integer"
    },
    "emission-constant": {
      "description": ".",
      "type": "integer"
    },
    "cron-frequency": {
      "description": "Frequency of cron job workers.",
      "type": "object",
      "properties": {
        "duration-worker": {
          "description": "Duration Update Worker [Defaults to '5m']",
          "type": "string"
        },
        "footprint-worker": {
          "description": "Metric-Footprint Update Worker [Defaults to '10m']",
          "type": "string"
        }
      }
    },
    "enable-resampling": {
      "description": "Enable dynamic zoom in frontend metric plots.",
      "type": "object",
      "properties": {
        "minimumPoints": {
          "description": "Minimum points to trigger resampling of time-series data.",
          "type": "integer"
        },
        "trigger": {
          "description": "Trigger next zoom level at less than this many visible datapoints.",
          "type": "integer"
        },
        "resolutions": {
          "description": "Array of resampling target resolutions, in seconds.",
          "type": "array",
          "items": {
            "type": "integer"
          }
        }
      },
      "required": ["trigger", "resolutions"]
    }
	},
  "required": ["apiAllowedIPs"]
	}`

var clustersSchema = `
  {
    "type": "array",
    "items": {
      "type": "object",
      "properties": {
        "name": {
          "description": "The name of the cluster.",
          "type": "string"
        },
        "metricDataRepository": {
          "description": "Type of the metric data repository for this cluster",
          "type": "object",
          "properties": {
            "kind": {
              "type": "string",
                "enum": ["influxdb", "prometheus", "cc-metric-store", "cc-metric-store-internal", "test"]
            },
            "url": {
              "type": "string"
            },
            "token": {
              "type": "string"
            }
          },
          "required": ["kind"]
        },
        "filterRanges": {
          "description": "This option controls the slider ranges for the UI controls of numNodes, duration, and startTime.",
          "type": "object",
          "properties": {
            "numNodes": {
              "description": "UI slider range for number of nodes",
              "type": "object",
              "properties": {
                "from": {
                  "type": "integer"
                },
                "to": {
                  "type": "integer"
                }
              },
              "required": ["from", "to"]
            },
            "duration": {
              "description": "UI slider range for duration",
              "type": "object",
              "properties": {
                "from": {
                  "type": "integer"
                },
                "to": {
                  "type": "integer"
                }
              },
              "required": ["from", "to"]
            },
            "startTime": {
              "description": "UI slider range for start time",
              "type": "object",
              "properties": {
                "from": {
                  "type": "string",
                  "format": "date-time"
                },
                "to": {
                  "type": "null"
                }
              },
              "required": ["from", "to"]
            }
          },
          "required": ["numNodes", "duration", "startTime"]
        }
      },
      "required": ["name", "filterRanges"],
      "minItems": 1
    }
  }`
