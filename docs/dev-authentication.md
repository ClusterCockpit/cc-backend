# Overview

The implementation of authentication is not easy to understand by just looking
at the code. The authentication is implemented in `internal/auth/`. In `auth.go`
an interface is defined that any authentication provider must fulfill. It also
acts as a dispatcher to delegate the calls to the available authentication
providers.

The most important routine are:
* `CanLogin()` Check if the authentication method is supported for login attempt
* `Login()` Handle POST request to login user and start a new session
* `Auth()`  Authenticate user and put User Object in context of the request

The http router calls auth in the following cases:
* `r.Handle("/login", authentication.Login( ... )).Methods(http.MethodPost)`:
  The POST request on the `/login` route will call the Login callback.
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

For non API routes a JWT token can be used to initiate an authenticated user
session. This can either happen by calling the login route with a token
provided in a header or query URL or via the `Auth()` method on first access
to a secured URL via a special cookie containing the JWT token.
For API routes the access is authenticated on every request using the JWT token
and no session is initiated.

# Login

The Login function (located in `auth.go`):
* Extracts the user name and gets the user from the user database table. In case the
  user is not found the user object is set to nil.
* Iterates over all authenticators and:
  - Calls the `CanLogin` function which checks if the authentication method is
    supported for this user and the user object is valid.
  - Calls the `Login` function to authenticate the user. On success a valid user
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
	return nil, fmt.Errorf("AUTH/LOCAL > Authentication failed")
}
```

## LDAP authenticator

This authenticator is applied if 
```
return user != nil && user.AuthSource == AuthViaLDAP
```

Gets the LDAP connection and tries a bind with the provided credentials:
```
if err := l.Bind(userDn, r.FormValue("password")); err != nil {
	log.Errorf("AUTH/LOCAL > Authentication for user %s failed: %v", user.Username, err)
	return nil, fmt.Errorf("AUTH/LDAP > Authentication failed")
}
```

## JWT authenticator

Login via JWT token will create a session without password.
For login the `X-Auth-Token` header is not supported.
This authenticator is applied if either user is not nil and auth source is
`AuthViaToken` or the Authorization header is present or the URL query key
login-token is present:
```
return (user != nil && user.AuthSource == AuthViaToken) ||
        r.Header.Get("Authorization") != "" ||
        r.URL.Query().Get("login-token") != ""
```

The Login function:
* Parses the token
* Check if the signing method is EdDSA or HS256 or HS512
* Check if claims are valid and extracts the claims
* The following claims have to be present:
   - `sub`: The subject, in this case this is the username
   - `exp`: Expiration in Unix epoch time
   - `roles`: String array with roles of user
* In case user is not yet set, which is usually the case:
   - Try to fetch user from database
   - In case user is not yet present add user to user database table with `AuthViaToken` AuthSource.
* Return valid user object

# Auth

The Auth function (located in `auth.go`):
* Returns a new http handler function that is defined right away
* This handler iterates over all authenticators
* Calls `Auth()` on every authenticator
* If err is not nil and the user object is valid it puts the user object in the
  request context and starts the onSuccess http handler
* Otherwise it calls the onFailure handler

## Local

Calls the `AuthViaSession()` function in `auth.go`. This will extract username,
projects and roles from the session and initialize a user object with those
values.

## LDAP

Calls the `AuthViaSession()` function in `auth.go`. This will extract username,
projects and roles from the session and initialize a user object with those
values.

# JWT

Check for JWT token:
* Is token passed in the `X-Auth-Token` or `Authorization` header
* If no token is found in a header it tries to read the token from a configured
cookie.

Finally it calls AuthViaSession in `auth.go` if a valid session exists. This is
true if a JWT token was previously used to initiate a session. In this case the
user object initialized with the session is returned right away.

In case a token was found extract and parse the token:
* Check if signing method is Ed25519/EdDSA 
* In case publicKeyCrossLogin is configured:
   - Check if `iss` issuer claim matched trusted issuer from configuration
   - Return public cross login key
   - Otherwise return standard public key
* Check if claims are valid
* Depending on the option `ForceJWTValidationViaDatabase ` the roles are
  extracted from JWT token or taken from user object fetched from database
* In case the token was extracted from cookie create a new session and ask the
  browser to delete the JWT cookie
* Return valid user object

