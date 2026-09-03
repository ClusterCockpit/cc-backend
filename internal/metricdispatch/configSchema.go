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
        "description": "Authentication token for the metric store. May be omitted and supplied through the environment instead: METRICSTORE_TOKEN_<SCOPE> takes precedence over METRICSTORE_TOKEN, which takes precedence over this value, and each also accepts a _FILE variant naming a file that holds the token. <SCOPE> is the scope uppercased with every character outside A-Z0-9 replaced by an underscore.",
        "type": "string"
      }
    },
    "required": ["scope", "url"]
  }
}`
