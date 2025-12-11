// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package archive

var configSchema = `
	{
      "type": "object",
      "properties": {
        "kind": {
          "description": "Backend type for job-archive",
          "type": "string",
          "enum": ["file", "s3", "sqlite"]
        },
        "path": {
          "description": "Path to job archive for file backend",
          "type": "string"
        },
        "dbPath": {
          "description": "Path to SQLite database file for sqlite backend",
          "type": "string"
        },
        "endpoint": {
          "description": "S3 endpoint URL (for S3-compatible services like MinIO)",
          "type": "string"
        },
        "accessKey": {
          "description": "S3 access key ID",
          "type": "string"
        },
        "secretKey": {
          "description": "S3 secret access key",
          "type": "string"
        },
        "bucket": {
          "description": "S3 bucket name for job archive",
          "type": "string"
        },
        "region": {
          "description": "AWS region for S3 bucket",
          "type": "string"
        },
        "usePathStyle": {
          "description": "Use path-style S3 URLs (required for MinIO and some S3-compatible services)",
          "type": "boolean"
        },
        "compression": {
          "description": "Setup automatic compression for jobs older than number of days",
          "type": "integer"
        },
        "retention": {
          "description": "Configuration keys for retention",
          "type": "object",
          "properties": {
            "policy": {
              "description": "Retention policy",
              "type": "string",
              "enum": ["none", "delete", "move"]
            },
            "includeDB": {
              "description": "Also remove jobs from database",
              "type": "boolean"
            },
            "age": {
              "description": "Act on jobs with startTime older than age (in days)",
              "type": "integer"
            },
            "location": {
              "description": "The target directory for retention. Only applicable for retention move.",
              "type": "string"
            }
          },
          "required": ["policy"]
        }
      },
      "required": ["kind"]}`
