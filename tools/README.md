## Introduction

ClusterCockpit uses JSON Web Tokens (JWT) for authorization of its APIs.
JSON Web Token (JWT) is an open standard (RFC 7519) that defines a compact and self-contained way for securely transmitting information between parties as a JSON object.
This information can be verified and trusted because it is digitally signed.
In ClusterCockpit JWTs are signed using a public/private key pair using ECDSA.
Because tokens are signed using public/private key pairs, the signature also certifies that only the party holding the private key is the one that signed it.
Currently JWT tokens not yet expire.

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
* The APIs are used during a browser session. In this case on login a JWT token is issued on login, that is used by the web frontend to authorize against the GraphQL and REST APIs.
* The REST API is used outside a browser session, e.g. by scripts. In this case you have to issue a token manually. This possible from within the configuration view or on the command line. It is recommended to issue a JWT token in this case for a special user that only has the `api` role. By using different users for different purposes a fine grained access control and access revocation management is possible.

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
