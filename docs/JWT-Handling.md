## Introduction

ClusterCockpit uses JSON Web Tokens (JWT) for authorization of its APIs. JSON
Web Token (JWT) is an open standard (RFC 7519) that defines a compact and
self-contained way for securely transmitting information between parties as a
JSON object. This information can be verified and trusted because it is
digitally signed. In ClusterCockpit JWTs are signed using a public/private key
pair using ECDSA. Because tokens are signed using public/private key pairs, the
signature also certifies that only the party holding the private key is the one
that signed it. Token expiration is set to the configuration option MaxAge.

## JWT Payload

You may view the payload of a JWT token at [https://jwt.io/#debugger-io](https://jwt.io/#debugger-io).
Currently ClusterCockpit sets the following claims:
* `iat`: Issued at claim. The “iat” claim is used to identify the the time at which the JWT was issued. This claim can be used to determine the age of the JWT.
* `sub`: Subject claim. Identifies the subject of the JWT, in our case this is the username.
* `roles`: An array of strings specifying the roles set for the subject.

## Workflow

1. Create a new ECDSA Public/private keypair:
```
$ go build ./tools/gen-keypair.go
$ ./gen-keypair
```
2. Add keypair in your `.env` file. A template can be found in `./configs`.

There are two usage scenarios:
* The APIs are used during a browser session. API accesses are authorized with
  the active session.
* The REST API is used outside a browser session, e.g. by scripts. In this case
  you have to issue a token manually. This possible from within the
  configuration view or on the command line. It is recommended to issue a JWT
  token in this case for a special user that only has the `api` role. By using
  different users for different purposes a fine grained access control and
  access revocation management is possible.

The token is commonly specified in the Authorization HTTP header using the Bearer schema.

## Setup user and JWT token for REST API authorization

1. Create user:
```
$ ./cc-backend --add-user <username>:api:<Password> --no-server
```
2. Issue token for user:
```
$ ./cc-backend -jwt <username>  -no-server
```
3. Use issued token token on client side:
```
$ curl -X GET "<API ENDPOINT>" -H "accept: application/json"  -H "Content-Type: application/json"  -H "Authorization: Bearer <JWT TOKEN>"
```

## Accept externally generated JWTs provided via cookie
If there is an external service like an AuthAPI that can generate JWTs and hand
them over to ClusterCockpit via cookies, CC can be configured to accept them:

1. `.env`: CC needs a public ed25519 key to verify foreign JWT signatures.
   Public keys in PEM format can be converted with the instructions in
   [/tools/convert-pem-pubkey-for-cc](../tools/convert-pem-pubkey-for-cc/Readme.md)
   .

```
CROSS_LOGIN_JWT_PUBLIC_KEY="+51iXX8BdLFocrppRxIw52xCOf8xFSH/eNilN5IHVGc="
```

2. `config.json`: Insert a name for the cookie (set by the external service)
   containing the JWT so that CC knows where to look at. Define a trusted issuer
   (JWT claim 'iss'), otherwise it will be rejected. If you want usernames and
   user roles from JWTs ('sub' and 'roles' claim) to be validated against CC's
   internal database, you need to enable it here. Unknown users will then be
   rejected and roles set via JWT will be ignored.

```json
"jwts": {
    "cookieName": "access_cc",
    "forceJWTValidationViaDatabase": true,
    "trustedExternalIssuer": "auth.example.com"
}
```

3. Make sure your external service includes the same issuer (`iss`) in its JWTs.
   Example JWT payload:

```json
{
  "iat": 1668161471,
  "nbf": 1668161471,
  "exp": 1668161531,
  "sub": "alice",
  "roles": [
    "user"
  ],
  "jti": "a1b2c3d4-1234-5678-abcd-a1b2c3d4e5f6",
  "iss": "auth.example.com"
}
```
