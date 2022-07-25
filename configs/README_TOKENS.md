## Introduction

ClusterCockpit uses JSON Web Tokens (JWT) for authorization of its APIs.
JSON Web Token (JWT) is an open standard (RFC 7519) that defines a compact and self-contained way for securely transmitting information between parties as a JSON object.
This information can be verified and trusted because it is digitally signed.
In ClusterCockpit JWTs are signed using a public/private key pair using ECDSA.
Because tokens are signed using public/private key pairs, the signature also certifies that only the party holding the private key is the one that signed it.
Expiration of the generated tokens as well as the max. length of a browser session can be configured in the `config.json` file described [here](./README.md).

The [Ed25519](https://ed25519.cr.yp.to/) algorithm for signatures was used because it is compatible with other tools that require authentication, such as NATS.io, and because these elliptic-curve methods provide simillar security with smaller keys compared to something like RSA. They are sligthly more expensive to validate, but that effect is negligible.

## JWT Payload

You may view the payload of a JWT token at [https://jwt.io/#debugger-io](https://jwt.io/#debugger-io).
Currently ClusterCockpit sets the following claims:
* `iat`: Issued at claim. The “iat” claim is used to identify the the time at which the JWT was issued. This claim can be used to determine the age of the JWT.
* `sub`: Subject claim. Identifies the subject of the JWT, in our case this is the username.
* `roles`: An array of strings specifying the roles set for the subject.
* `exp`: Expiration date of the token (only if explicitly configured)

It is important to know that JWTs are not encrypted, only signed. This means that outsiders cannot create new JWTs or modify existing ones, but they are able to read out the username.

## Workflow

1. Create a new ECDSA Public/private keypair:
```
$ go build ./cmd/gen-keypair/
$ ./gen-keypair
```
2. Add keypair in your `.env` file. A template can be found in `./configs`.

When a user logs in via the `/login` page using a browser, a session cookie (secured using the random bytes in the `SESSION_KEY` env. variable you shoud change as well) is used for all requests after the successfull login. The JWTs make it easier to use the APIs of ClusterCockpit using scripts or other external programs. The token is specified n the `Authorization` HTTP header using the [Bearer schema](https://datatracker.ietf.org/doc/html/rfc6750) (there is an example below). Tokens can be issued to users from the configuration view in the Web-UI or the command line. In order to use the token for API endpoints such as `/api/jobs/start_job/`, the user that executes it needs to have the `api` role. Regular users can only perform read-only queries and only look at data connected to jobs they started themselves.

## cc-metric-store

The [cc-metric-store](https://github.com/ClusterCockpit/cc-metric-store) also uses JWTs for authentication. As it does not issue new tokens, it does not need to kown the private key. The public key of the keypair that is used to generate the JWTs that grant access to the `cc-metric-store` can be specified in its `config.json`. When configuring the `metricDataRepository` object in the `cluster.json` file, you can put a token issued by ClusterCockpit itself.

## Setup user and JWT token for REST API authorization

1. Create user:
```
$ ./cc-backend --add-user <username>:api:<password> --no-server
```
2. Issue token for user:
```
$ ./cc-backend --jwt <username> --no-server
```
3. Use issued token token on client side:
```
$ curl -X GET "<API ENDPOINT>" -H "accept: application/json" -H "Content-Type: application/json" -H "Authorization: Bearer <JWT TOKEN>"
```
