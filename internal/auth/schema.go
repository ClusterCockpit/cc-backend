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
        "cookie-name": {
          "description": "Cookie that should be checked for a JWT token.",
          "type": "string"
        },
        "validate-user": {
          "description": "Deny login for users not in database (but defined in JWT). Overwrite roles in JWT with database roles.",
          "type": "boolean"
        },
        "trusted-issuer": {
          "description": "Issuer that should be accepted when validating external JWTs ",
          "type": "string"
        },
        "sync-user-on-login": {
          "description": "Add non-existent user to DB at login attempt with values provided in JWT.",
          "type": "boolean"
        },
        "update-user-on-login": {
          "description": "Should an existent user attributes in the DB be updated at login attempt with values provided in JWT.",
          "type": "boolean"
        },
        "public-key": {
          "description": "Base64 encoded Ed25519 public key used to validate JWTs. Overridden by the JWT_PUBLIC_KEY environment variable when set.",
          "type": "string"
        },
        "private-key": {
          "description": "Base64 encoded Ed25519 private key used to sign JWTs. Overridden by the JWT_PRIVATE_KEY environment variable when set.",
          "type": "string"
        },
        "cross-login-public-key": {
          "description": "Base64 encoded Ed25519 public key for accepting externally generated JWTs. Overridden by the CROSS_LOGIN_JWT_PUBLIC_KEY environment variable when set.",
          "type": "string"
        },
        "cross-login-hs512-key": {
          "description": "Base64 encoded HMAC (HS256/HS512) key for accepting externally generated session login tokens. Overridden by the CROSS_LOGIN_JWT_HS512_KEY environment variable when set.",
          "type": "string"
        }
      },
      "required": ["max-age"]
    },
    "oidc": {
      "type": "object",
      "properties": {
        "provider": {
          "description": "OpenID Connect provider URL.",
          "type": "string"
        },
        "sync-user-on-login": {
          "description": "Add non-existent user to DB at login attempt with values provided.",
          "type": "boolean"
        },
        "update-user-on-login": {
          "description": "Should an existent user attributes in the DB be updated at login attempt with values provided.",
          "type": "boolean"
        },
        "client-id": {
          "description": "OAuth2 client ID for the OIDC provider. Overridden by the OID_CLIENT_ID environment variable when set.",
          "type": "string"
        },
        "client-secret": {
          "description": "OAuth2 client secret for the OIDC provider. Overridden by the OID_CLIENT_SECRET environment variable when set.",
          "type": "string"
        }
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
        "user-base": {
          "description": "Base DN of user tree root.",
          "type": "string"
        },
        "search-dn": {
          "description": "DN for authenticating LDAP admin account with general read rights.",
          "type": "string"
        },
        "user-bind": {
          "description": "Expression used to authenticate users via LDAP bind. Must contain uid={username}.",
          "type": "string"
        },
        "user-filter": {
          "description": "Filter to extract users for syncing.",
          "type": "string"
        },
        "username-attr": {
          "description": "Attribute with full username. Default: gecos",
          "type": "string"
        },
        "sync-interval": {
          "description": "Interval used for syncing local user table with LDAP directory. Parsed using time.ParseDuration.",
          "type": "string"
        },
        "sync-del-old-users": {
          "description": "Delete obsolete users in database.",
          "type": "boolean"
        },
        "uid-attr": {
          "description": "LDAP attribute used as login username. Default: uid",
          "type": "string"
        },
        "sync-user-on-login": {
          "description": "Add non-existent user to DB at login attempt if user exists in Ldap directory",
          "type": "boolean"
        },
        "update-user-on-login": {
          "description": "Should an existent user attributes in the DB be updated at login attempt with values from LDAP.",
          "type": "boolean"
        },
        "sync-password": {
          "description": "Password for the LDAP admin account used for syncing. Overridden by the LDAP_ADMIN_PASSWORD environment variable when set.",
          "type": "string"
        }
      },
      "required": ["url", "user-base", "search-dn", "user-bind", "user-filter"]
    },
  "required": ["jwts"]
	}`
