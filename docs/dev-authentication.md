# Overview

The authentication is implemented in `internal/auth/`. In `auth.go`
an interface is defined that any authentication provider must fulfill. It also
acts as a dispatcher to delegate the calls to the available authentication
providers.

Two authentication types are available:
* JWT authentication for the REST API that does not create a session cookie
* Session based authentication using a session cookie

The most important routines in auth are:
* `Login()` Handle POST request to login user and start a new session
* `Auth()`  Authenticate user and put User Object in context of the request

The http router calls auth in the following cases:
* `r.Handle("/login", authentication.Login( ... )).Methods(http.MethodPost)`:
  The POST request on the `/login` route will call the Login callback.
* `r.Handle("/jwt-login", authentication.Login( ... ))`:
  Any request on the `/jwt-login` route will call the Login callback. Intended
  for use for the JWT token based authenticators.
* Any route in the secured subrouter will always call Auth(), on success it will
  call the next handler in the chain, on failure it will render the login
  template.
```
secured.Use(func(next http.Handler) http.Handler {
	return authentication.Auth(
		// On success;
		next,

		// On failure:
		func(rw http.ResponseWriter, r *http.Request, err error) {
               // Render login form
		})
})
```

A JWT token can be used to initiate an authenticated user
session. This can either happen by calling the login route with a token
provided in a header or via a special cookie containing the JWT token.
For API routes the access is authenticated on every request using the JWT token
and no session is initiated.

# Login

The Login function (located in `auth.go`):
* Extracts the user name and gets the user from the user database table. In case the
  user is not found the user object is set to nil.
* Iterates over all authenticators and:
  - Calls its `CanLogin` function which checks if the authentication method is
    supported for this user.
  - Calls its `Login` function to authenticate the user. On success a valid user
    object is returned.
  - Creates a new session object, stores the user attributes in the session and
    saves the session.
  - Starts the `onSuccess` http handler

## Local authenticator

This authenticator is applied if 
```
return user != nil && user.AuthSource == AuthViaLocalPassword
```

Compares the password provided by the login form to the password hash stored in
the user database table:
```
if e := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(r.FormValue("password"))); e != nil {
	log.Errorf("AUTH/LOCAL > Authentication for user %s failed!", user.Username)
	return nil, fmt.Errorf("Authentication failed")
}
```

## LDAP authenticator

This authenticator is applied if the user was found in the database and its
AuthSource is LDAP:
```
if user != nil {
	if user.AuthSource == schema.AuthViaLDAP {
		return user, true
	}
} 
```

If the option `SyncUserOnLogin` is set it tried to sync the user from the LDAP
directory. In case this succeeds the user is persisted to the database and can
login.

Gets the LDAP connection and tries a bind with the provided credentials:
```
if err := l.Bind(userDn, r.FormValue("password")); err != nil {
	log.Errorf("AUTH/LDAP > Authentication for user %s failed: %v", user.Username, err)
	return nil, fmt.Errorf("Authentication failed")
}
```

## JWT Session authenticator

Login via JWT token will create a session without password.
For login the `X-Auth-Token` header is not supported. This authenticator is
applied if the Authorization header or query parameter login-token is present:
```
	return user, r.Header.Get("Authorization") != "" ||
		r.URL.Query().Get("login-token") != ""
```

The Login function:
* Parses the token and checks if it is expired
* Check if the signing method is EdDSA or HS256 or HS512
* Check if claims are valid and extracts the claims
* The following claims have to be present:
   - `sub`: The subject, in this case this is the username
   - `exp`: Expiration in Unix epoch time
   - `roles`: String array with roles of user
* In case user does not exist in the database and the option `SyncUserOnLogin`
  is set add user to user database table with `AuthViaToken` AuthSource.
* Return valid user object

## JWT Cookie Session authenticator

Login via JWT cookie token will create a session without password.
It is first checked if the required configuration options are set:
* `trustedIssuer`
* `CookieName`

and optionally the environment variable `CROSS_LOGIN_JWT_PUBLIC_KEY` is set.

This authenticator is applied if the configured cookie is present:
```
	jwtCookie, err := r.Cookie(cookieName)

	if err == nil && jwtCookie.Value != "" {
		return true
	}
```

The Login function:
* Extracts and parses the token
* Checks if signing method is Ed25519/EdDSA 
* In case publicKeyCrossLogin is configured:
   - Check if `iss` issuer claim matched trusted issuer from configuration
   - Return public cross login key
   - Otherwise return standard public key
* Check if claims are valid
* Depending on the option `validateUser` the roles are
  extracted from JWT token or taken from user object fetched from database
* Ask browser to delete the JWT cookie
* In case user does not exist in the database and the option `SyncUserOnLogin`
  is set add user to user database table with `AuthViaToken` AuthSource.
* Return valid user object

# Auth

The Auth function (located in `auth.go`):
* Returns a new http handler function that is defined right away
* This handler tries two methods to authenticate a user:
   - Via a JWT API token in `AuthViaJWT()`
   - Via a valid session in `AuthViaSession()`
* If err is not nil and the user object is valid it puts the user object in the
  request context and starts the onSuccess http handler
* Otherwise it calls the onFailure handler

## AuthViaJWT

Implemented in JWTAuthenticator:
* Extract token either from header `X-Auth-Token` or `Authorization` with Bearer
  prefix
* Parse token and check if it is valid. The Parse routine will also check if the
  token is expired.
* If the option `validateUser` is set it will ensure the
  user object exists in the database and takes the roles from the database user
* Otherwise the roles are extracted from the roles claim
* Returns a valid user object with AuthType set to AuthToken

## AuthViaSession

* Extracts session
* Get values username, projects, and roles from session
* Returns a valid user object with AuthType set to AuthSession
