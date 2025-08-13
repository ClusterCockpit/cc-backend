// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package auth

var configSchema = `
	{
    "jwts": {
      "description": "For JWT token authentication.",
      "type": "object",
      "properties": {
        "max-age": {
          "description": "Configure how long a token is valid. As string parsable by time.ParseDuration()",
          "type": "string"
        },
        "cookieName": {
          "description": "Cookie that should be checked for a JWT token.",
          "type": "string"
        },
        "validateUser": {
          "description": "Deny login for users not in database (but defined in JWT). Overwrite roles in JWT with database roles.",
          "type": "boolean"
        },
        "trustedIssuer": {
          "description": "Issuer that should be accepted when validating external JWTs ",
          "type": "string"
        },
        "syncUserOnLogin": {
          "description": "Add non-existent user to DB at login attempt with values provided in JWT.",
          "type": "boolean"
        }
      },
      "required": ["max-age"]
    },
    "oidc": {
      "provider": {
        "description": "",
        "type": "string"
      },
      "syncUserOnLogin": {
        "description": "",
        "type": "boolean"
      },
      "updateUserOnLogin": {
        "description": "",
        "type": "boolean"
      },
      "required": ["provider"]
    },
    "ldap": {
      "description": "For LDAP Authentication and user synchronisation.",
      "type": "object",
      "properties": {
        "url": {
          "description": "URL of LDAP directory server.",
          "type": "string"
        },
        "user_base": {
          "description": "Base DN of user tree root.",
          "type": "string"
        },
        "search_dn": {
          "description": "DN for authenticating LDAP admin account with general read rights.",
          "type": "string"
        },
        "user_bind": {
          "description": "Expression used to authenticate users via LDAP bind. Must contain uid={username}.",
          "type": "string"
        },
        "user_filter": {
          "description": "Filter to extract users for syncing.",
          "type": "string"
        },
        "username_attr": {
          "description": "Attribute with full username. Default: gecos",
          "type": "string"
        },
        "sync_interval": {
          "description": "Interval used for syncing local user table with LDAP directory. Parsed using time.ParseDuration.",
          "type": "string"
        },
        "sync_del_old_users": {
          "description": "Delete obsolete users in database.",
          "type": "boolean"
        },
        "syncUserOnLogin": {
          "description": "Add non-existent user to DB at login attempt if user exists in Ldap directory",
          "type": "boolean"
        }
      },
      "required": ["url", "user_base", "search_dn", "user_bind", "user_filter"]
    },
  "required": ["jwts"]
	}`
