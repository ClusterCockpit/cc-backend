package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/log"
	sq "github.com/Masterminds/squirrel"
	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
)

// Only Username and Roles will always be filled in when returned by `GetUser`.
// If Name and Email is needed as well, use auth.FetchUser(), which does a database
// query for all fields.
type User struct {
	Username string   `json:"username"`
	Password string   `json:"-"`
	Name     string   `json:"name"`
	Roles    []string `json:"roles"`
	ViaLdap  bool     `json:"via-ldap"`
	Email    string   `json:"email"`
}

const (
	RoleAdmin string = "admin"
	RoleApi   string = "api"
	RoleUser  string = "user"
)

func (u *User) HasRole(role string) bool {
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

type ContextKey string

const ContextUserKey ContextKey = "user"

type Authentication struct {
	db            *sqlx.DB
	sessionStore  *sessions.CookieStore
	jwtPublicKey  ed25519.PublicKey
	jwtPrivateKey ed25519.PrivateKey

	ldapConfig           *LdapConfig
	ldapSyncUserPassword string

	// If zero, tokens/sessions do not expire.
	SessionMaxAge time.Duration
	JwtMaxAge     time.Duration
}

func (auth *Authentication) Init(db *sqlx.DB, ldapConfig *LdapConfig) error {
	auth.db = db
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS user (
		username varchar(255) PRIMARY KEY,
		password varchar(255) DEFAULT NULL,
		ldap     tinyint      DEFAULT 0,
		name     varchar(255) DEFAULT NULL,
		roles    varchar(255) DEFAULT NULL,
		email    varchar(255) DEFAULT NULL);`)
	if err != nil {
		return err
	}

	sessKey := os.Getenv("SESSION_KEY")
	if sessKey == "" {
		log.Warn("environment variable 'SESSION_KEY' not set (will use non-persistent random key)")
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			return err
		}
		auth.sessionStore = sessions.NewCookieStore(bytes)
	} else {
		bytes, err := base64.StdEncoding.DecodeString(sessKey)
		if err != nil {
			return err
		}
		auth.sessionStore = sessions.NewCookieStore(bytes)
	}

	pubKey, privKey := os.Getenv("JWT_PUBLIC_KEY"), os.Getenv("JWT_PRIVATE_KEY")
	if pubKey == "" || privKey == "" {
		log.Warn("environment variables 'JWT_PUBLIC_KEY' or 'JWT_PRIVATE_KEY' not set (token based authentication will not work)")
	} else {
		bytes, err := base64.StdEncoding.DecodeString(pubKey)
		if err != nil {
			return err
		}
		auth.jwtPublicKey = ed25519.PublicKey(bytes)
		bytes, err = base64.StdEncoding.DecodeString(privKey)
		if err != nil {
			return err
		}
		auth.jwtPrivateKey = ed25519.PrivateKey(bytes)
	}

	if ldapConfig != nil {
		auth.ldapConfig = ldapConfig
		if err := auth.initLdap(); err != nil {
			return err
		}
	}

	return nil
}

// arg must be formated like this: "<username>:[admin|api|]:<password>"
func (auth *Authentication) AddUser(arg string) error {
	parts := strings.SplitN(arg, ":", 3)
	if len(parts) != 3 || len(parts[0]) == 0 {
		return errors.New("invalid argument format")
	}

	roles := strings.Split(parts[1], ",")
	return auth.CreateUser(parts[0], "", parts[2], "", roles)
}

func (auth *Authentication) CreateUser(username, name, password, email string, roles []string) error {
	for _, role := range roles {
		if role != RoleAdmin && role != RoleApi && role != RoleUser {
			return fmt.Errorf("invalid user role: %#v", role)
		}
	}

	if username == "" {
		return errors.New("username should not be empty")
	}

	if password != "" {
		bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		password = string(bytes)
	}

	rolesJson, _ := json.Marshal(roles)
	cols := []string{"username", "password", "roles"}
	vals := []interface{}{username, password, string(rolesJson)}
	if name != "" {
		cols = append(cols, "name")
		vals = append(vals, name)
	}
	if email != "" {
		cols = append(cols, "email")
		vals = append(vals, email)
	}

	if _, err := sq.Insert("user").Columns(cols...).Values(vals...).RunWith(auth.db).Exec(); err != nil {
		return err
	}

	log.Infof("new user %#v created (roles: %s)", username, roles)
	return nil
}

func (auth *Authentication) DelUser(username string) error {
	_, err := auth.db.Exec(`DELETE FROM user WHERE user.username = ?`, username)
	return err
}

func (auth *Authentication) FetchUsers(viaLdap bool) ([]*User, error) {
	q := sq.Select("username", "name", "email", "roles").From("user")
	if !viaLdap {
		q = q.Where("ldap = 0")
	} else {
		q = q.Where("ldap = 1")
	}

	rows, err := q.RunWith(auth.db).Query()
	if err != nil {
		return nil, err
	}

	users := make([]*User, 0)
	defer rows.Close()
	for rows.Next() {
		rawroles := ""
		user := &User{}
		var name, email sql.NullString
		if err := rows.Scan(&user.Username, &name, &email, &rawroles); err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(rawroles), &user.Roles); err != nil {
			return nil, err
		}

		user.Name = name.String
		user.Email = email.String
		users = append(users, user)
	}
	return users, nil
}

func (auth *Authentication) FetchUser(username string) (*User, error) {
	user := &User{Username: username}
	var hashedPassword, name, rawRoles, email sql.NullString
	if err := sq.Select("password", "ldap", "name", "roles", "email").From("user").
		Where("user.username = ?", username).RunWith(auth.db).
		QueryRow().Scan(&hashedPassword, &user.ViaLdap, &name, &rawRoles, &email); err != nil {
		return nil, fmt.Errorf("user '%s' not found (%s)", username, err.Error())
	}

	user.Password = hashedPassword.String
	user.Name = name.String
	user.Email = email.String
	if rawRoles.Valid {
		if err := json.Unmarshal([]byte(rawRoles.String), &user.Roles); err != nil {
			return nil, err
		}
	}

	return user, nil
}

// Handle a POST request that should log the user in, starting a new session.
func (auth *Authentication) Login(onsuccess http.Handler, onfailure func(rw http.ResponseWriter, r *http.Request, loginErr error)) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		username, password := r.FormValue("username"), r.FormValue("password")
		user, err := auth.FetchUser(username)
		if err == nil && user.ViaLdap && auth.ldapConfig != nil {
			err = auth.loginViaLdap(user, password)
		} else if err == nil && !user.ViaLdap && user.Password != "" {
			if e := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); e != nil {
				err = fmt.Errorf("user '%s' provided the wrong password (%s)", username, e.Error())
			}
		} else {
			err = errors.New("could not authenticate user")
		}

		if err != nil {
			log.Warnf("login of user %#v failed: %s", username, err.Error())
			onfailure(rw, r, err)
			return
		}

		session, err := auth.sessionStore.New(r, "session")
		if err != nil {
			log.Errorf("session creation failed: %s", err.Error())
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		if auth.SessionMaxAge != 0 {
			session.Options.MaxAge = int(auth.SessionMaxAge.Seconds())
		}
		session.Values["username"] = user.Username
		session.Values["roles"] = user.Roles
		if err := auth.sessionStore.Save(r, rw, session); err != nil {
			log.Errorf("session save failed: %s", err.Error())
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Infof("login successfull: user: %#v (roles: %v)", user.Username, user.Roles)
		ctx := context.WithValue(r.Context(), ContextUserKey, user)
		onsuccess.ServeHTTP(rw, r.WithContext(ctx))
	})
}

var ErrTokenInvalid error = errors.New("invalid token")

func (auth *Authentication) authViaToken(r *http.Request) (*User, error) {
	if auth.jwtPublicKey == nil {
		return nil, nil
	}

	rawtoken := r.Header.Get("X-Auth-Token")
	if rawtoken == "" {
		rawtoken = r.Header.Get("Authorization")
		prefix := "Bearer "
		if !strings.HasPrefix(rawtoken, prefix) {
			return nil, nil
		}
		rawtoken = rawtoken[len(prefix):]
	}

	token, err := jwt.Parse(rawtoken, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodEdDSA {
			return nil, errors.New("only Ed25519/EdDSA supported")
		}
		return auth.jwtPublicKey, nil
	})
	if err != nil {
		return nil, err
	}

	if err := token.Claims.Valid(); err != nil {
		return nil, err
	}

	claims := token.Claims.(jwt.MapClaims)
	sub, _ := claims["sub"].(string)

	var roles []string
	if rawroles, ok := claims["roles"].([]interface{}); ok {
		for _, rr := range rawroles {
			if r, ok := rr.(string); ok {
				roles = append(roles, r)
			}
		}
	}

	// TODO: Check if sub is still a valid user!
	return &User{
		Username: sub,
		Roles:    roles,
	}, nil
}

// Authenticate the user and put a User object in the
// context of the request. If authentication fails,
// do not continue but send client to the login screen.
func (auth *Authentication) Auth(onsuccess http.Handler, onfailure func(rw http.ResponseWriter, r *http.Request, authErr error)) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		user, err := auth.authViaToken(r)
		if err != nil {
			log.Warnf("authentication failed: %s", err.Error())
			http.Error(rw, err.Error(), http.StatusUnauthorized)
			return
		}
		if user != nil {
			// Successfull authentication using a token
			ctx := context.WithValue(r.Context(), ContextUserKey, user)
			onsuccess.ServeHTTP(rw, r.WithContext(ctx))
			return
		}

		session, err := auth.sessionStore.Get(r, "session")
		if err != nil {
			// sessionStore.Get will return a new session if no current one is attached to this request.
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		if session.IsNew {
			log.Warn("authentication failed: no session or jwt found")
			onfailure(rw, r, errors.New("no valid session or JWT provided"))
			return
		}

		username, _ := session.Values["username"].(string)
		roles, _ := session.Values["roles"].([]string)
		ctx := context.WithValue(r.Context(), ContextUserKey, &User{
			Username: username,
			Roles:    roles,
		})
		onsuccess.ServeHTTP(rw, r.WithContext(ctx))
	})
}

// Generate a new JWT that can be used for authentication
func (auth *Authentication) ProvideJWT(user *User) (string, error) {
	if auth.jwtPrivateKey == nil {
		return "", errors.New("environment variable 'JWT_PRIVATE_KEY' not set")
	}

	claims := jwt.MapClaims{
		"sub":   user.Username,
		"roles": user.Roles,
	}
	if auth.JwtMaxAge != 0 {
		claims["exp"] = time.Now().Add(auth.JwtMaxAge).Unix()
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)

	return tok.SignedString(auth.jwtPrivateKey)
}

func GetUser(ctx context.Context) *User {
	x := ctx.Value(ContextUserKey)
	if x == nil {
		return nil
	}

	return x.(*User)
}

// Clears the session cookie
func (auth *Authentication) Logout(onsuccess http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		session, err := auth.sessionStore.Get(r, "session")
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		if !session.IsNew {
			session.Options.MaxAge = -1
			if err := auth.sessionStore.Save(r, rw, session); err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		onsuccess.ServeHTTP(rw, r)
	})
}
