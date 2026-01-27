// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricdispatch

const configSchema = `{
  "type": "array",
  "description": "Array of metric store configurations with scope-based routing.",
  "items": {
    "type": "object",
    "properties": {
      "scope": {
        "description": "Scope identifier for routing metrics (e.g., cluster name, '*' for default)",
        "type": "string"
      },
      "url": {
        "description": "URL of the metric store endpoint",
        "type": "string"
      },
      "token": {
        "description": "Authentication token for the metric store",
        "type": "string"
      }
    },
    "required": ["scope", "url", "token"]
  }
}`
