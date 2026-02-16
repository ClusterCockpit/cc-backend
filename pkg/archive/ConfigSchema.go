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
        "db-path": {
          "description": "Path to SQLite database file for sqlite backend",
          "type": "string"
        },
        "endpoint": {
          "description": "S3 endpoint URL (for S3-compatible services like MinIO)",
          "type": "string"
        },
        "access-key": {
          "description": "S3 access key ID",
          "type": "string"
        },
        "secret-key": {
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
        "use-path-style": {
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
              "enum": ["none", "delete", "copy", "move"]
            },
            "format": {
              "description": "Output format for copy/move policies",
              "type": "string",
              "enum": ["json", "parquet"]
            },
            "include-db": {
              "description": "Also remove jobs from database",
              "type": "boolean"
            },
            "age": {
              "description": "Act on jobs with startTime older than age (in days)",
              "type": "integer"
            },
            "target-kind": {
              "description": "Target storage kind: file or s3",
              "type": "string",
              "enum": ["file", "s3"]
            },
            "target-path": {
              "description": "Target directory path for file storage",
              "type": "string"
            },
            "target-endpoint": {
              "description": "S3 endpoint URL for target",
              "type": "string"
            },
            "target-bucket": {
              "description": "S3 bucket name for target",
              "type": "string"
            },
            "target-access-key": {
              "description": "S3 access key for target",
              "type": "string"
            },
            "target-secret-key": {
              "description": "S3 secret key for target",
              "type": "string"
            },
            "target-region": {
              "description": "S3 region for target",
              "type": "string"
            },
            "target-use-path-style": {
              "description": "Use path-style S3 URLs for target",
              "type": "boolean"
            },
            "max-file-size-mb": {
              "description": "Maximum parquet file size in MB before splitting",
              "type": "integer"
            }
          },
          "required": ["policy"]
        }
      },
      "required": ["kind"]}`
