// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package memorystore

const configSchema = `{
    "type": "object",
    "description": "Configuration specific to built-in metric-store.",
    "properties": {
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
                },
                "restore": {
                    "description": "When cc-backend starts up, look for checkpointed files that are less than X hours old and load metrics from these selected checkpoint files.",
                    "type": "string"
                }
            }
        },
        "archive": {
            "description": "Configuration for archiving the already checkpointed files.",
            "type": "object",
            "properties": {
                "interval": {
                    "description": "Interval at which the checkpointed files should be archived.",
                    "type": "string"
                },
                "directory": {
                    "description": "Specify the parent directy in which the archived files should be placed.",
                    "type": "string"
                }
            }
        },
        "retention-in-memory": {
            "description": "Keep the metrics within memory for given time interval. Retention for X hours, then the metrics would be freed.",
            "type": "string"
        },
        "nats": {
            "description": "Configuration for accepting published data through NATS.",
            "type": "object",
            "properties": {
                "address": {
                    "description": "Address of the NATS server.",
                    "type": "string"
                },
                "username": {
                    "description": "Optional: If configured with username/password method.",
                    "type": "string"
                },
                "password": {
                    "description": "Optional: If configured with username/password method.",
                    "type": "string"
                },
                "creds-file-path": {
                    "description": "Optional: If configured with Credential File method. Path to your NATS cred file.",
                    "type": "string"
                },
                "subscriptions": {
                    "description": "Array of various subscriptions. Allows to subscibe to different subjects and publishers.",
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
        }
    }
}`
