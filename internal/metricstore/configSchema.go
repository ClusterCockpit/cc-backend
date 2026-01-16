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
      "description": "Configuration for checkpointing the metrics within metric-store",
      "type": "object",
      "properties": {
        "file-format": {
          "description": "Specify the type of checkpoint file. There are 2 variants: 'avro' and 'json'. If nothing is specified, 'avro' is default.",
          "type": "string"
        },
        "interval": {
          "description": "Interval at which the metrics should be checkpointed.",
          "type": "string"
        },
        "directory": {
          "description": "Specify the parent directy in which the checkpointed files should be placed.",
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
      "description": "Array of various subscriptions. Allows to subscibe to different subjects and publishers.",
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "subscribe-to": {
            "description": "Channel name",
            "type": "string"
          },
          "cluster-tag": {
            "description": "Optional: Allow lines without a cluster tag, use this as default",
            "type": "string"
          }
        }
      }
    }
  },
  "required": ["checkpoints", "retention-in-memory", "memory-cap"]
}`
