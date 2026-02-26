// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

const configSchema = `{
  "type": "object",
  "description": "Configuration specific to built-in metric-store.",
  "properties": {
    "num-workers": {
      "description": "Number of concurrent workers for checkpoint and archive operations",
      "type": "integer"
    },
    "checkpoints": {
      "description": "Configuration for checkpointing the metrics buffers",
      "type": "object",
      "properties": {
        "file-format": {
          "description": "Specify the format for checkpoint files. Two variants: 'json' (human-readable, periodic) and 'wal' (binary snapshot + Write-Ahead Log, crash-safe). Default is 'json'.",
          "type": "string"
        },
        "interval": {
          "description": "Interval at which the metrics should be checkpointed.",
          "type": "string"
        },
        "directory": {
          "description": "Path in which the checkpointed files should be placed.",
          "type": "string"
        }
      },
      "required": ["interval"]
    },
    "cleanup": {
      "description": "Configuration for the cleanup process.",
      "type": "object",
      "properties": {
        "mode": {
          "description": "The operation mode (e.g., 'archive' or 'delete').",
          "type": "string",
          "enum": ["archive", "delete"] 
        },
        "interval": {
          "description": "Interval at which the cleanup runs.",
          "type": "string"
        },
        "directory": {
          "description": "Target directory for operations.",
          "type": "string"
        }
      },
      "if": {
        "properties": {
          "mode": { "const": "archive" }
        }
      },
      "then": {
        "required": ["interval", "directory"]
      }
    },
    "retention-in-memory": {
      "description": "Keep the metrics within memory for given time interval. Retention for X hours, then the metrics would be freed.",
      "type": "string"
    },
    "memory-cap": {
      "description": "Upper memory capacity limit used by metricstore in GB",
      "type": "integer"
    },
    "nats-subscriptions": {
      "description": "Array of various subscriptions. Allows to subscribe to different subjects and publishers.",
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "subscribe-to": {
            "description": "Subject name",
            "type": "string"
          },
          "cluster-tag": {
            "description": "Optional: Allow lines without a cluster tag, use this as default",
            "type": "string"
          }
        },
				"required": ["subscribe-to"]
      }
    }
  },
  "required": ["checkpoints", "retention-in-memory", "memory-cap"]
}`
