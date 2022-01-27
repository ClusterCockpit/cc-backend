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
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/ClusterCockpit/cc-jobarchive/templates"
	sq "github.com/Masterminds/squirrel"
	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
)

// TODO: Properly do this "roles" stuff.
// Add a roles array and `user.HasRole(...)` functions.
type User struct {
	Username string
	Password string
	Name     string
	Roles    []string
	ViaLdap  bool
	Email    string
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

var JwtPublicKey ed25519.PublicKey
var JwtPrivateKey ed25519.PrivateKey

var sessionStore *sessions.CookieStore

func Init(db *sqlx.DB, ldapConfig *LdapConfig) error {
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
		log.Println("warning: environment variable 'SESSION_KEY' not set (will use non-persistent random key)")
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			return err
		}
		sessionStore = sessions.NewCookieStore(bytes)
	} else {
		bytes, err := base64.StdEncoding.DecodeString(sessKey)
		if err != nil {
			return err
		}
		sessionStore = sessions.NewCookieStore(bytes)
	}

	pubKey, privKey := os.Getenv("JWT_PUBLIC_KEY"), os.Getenv("JWT_PRIVATE_KEY")
	if pubKey == "" || privKey == "" {
		log.Println("warning: environment variables 'JWT_PUBLIC_KEY' or 'JWT_PRIVATE_KEY' not set (token based authentication will not work)")
	} else {
		bytes, err := base64.StdEncoding.DecodeString(pubKey)
		if err != nil {
			return err
		}
		JwtPublicKey = ed25519.PublicKey(bytes)
		bytes, err = base64.StdEncoding.DecodeString(privKey)
		if err != nil {
			return err
		}
		JwtPrivateKey = ed25519.PrivateKey(bytes)
	}

	if ldapConfig != nil {
		if err := initLdap(ldapConfig); err != nil {
			return err
		}
	}

	return nil
}

// arg must be formated like this: "<username>:[admin]:<password>"
func AddUserToDB(db *sqlx.DB, arg string) error {
	parts := strings.SplitN(arg, ":", 3)
	if len(parts) != 3 || len(parts[0]) == 0 {
		return errors.New("invalid argument format")
	}

	password := ""
	if len(parts[2]) > 0 {
		bytes, err := bcrypt.GenerateFromPassword([]byte(parts[2]), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		password = string(bytes)
	}

	roles := []string{}
	for _, role := range strings.Split(parts[1], ",") {
		if len(role) == 0 {
			continue
		} else if role == RoleAdmin || role == RoleApi || role == RoleUser {
			roles = append(roles, role)
		} else {
			return fmt.Errorf("invalid user role: %#v", role)
		}
	}

	rolesJson, _ := json.Marshal(roles)
	_, err := sq.Insert("user").Columns("username", "password", "roles").Values(parts[0], password, string(rolesJson)).RunWith(db).Exec()
	if err != nil {
		return err
	}
	log.Printf("new user '%s' added (roles: %s)\n", parts[0], roles)
	return nil
}

func DelUserFromDB(db *sqlx.DB, username string) error {
	_, err := db.Exec(`DELETE FROM user WHERE user.username = ?`, username)
	return err
}

func FetchUserFromDB(db *sqlx.DB, username string) (*User, error) {
	user := &User{Username: username}
	var hashedPassword, name, rawRoles, email sql.NullString
	if err := sq.Select("password", "ldap", "name", "roles", "email").From("user").
		Where("user.username = ?", username).RunWith(db).
		QueryRow().Scan(&hashedPassword, &user.ViaLdap, &name, &rawRoles, &email); err != nil {
		return nil, fmt.Errorf("user '%s' not found (%s)", username, err.Error())
	}

	user.Password = hashedPassword.String
	user.Name = name.String
	user.Email = email.String
	if rawRoles.Valid {
		json.Unmarshal([]byte(rawRoles.String), &user.Roles)
	}

	return user, nil
}

// Handle a POST request that should log the user in,
// starting a new session.
func Login(db *sqlx.DB) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		username, password := r.FormValue("username"), r.FormValue("password")
		user, err := FetchUserFromDB(db, username)
		if err == nil && user.ViaLdap && ldapAuthEnabled {
			err = loginViaLdap(user, password)
		} else if err == nil && !user.ViaLdap && user.Password != "" {
			if e := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); e != nil {
				err = fmt.Errorf("user '%s' provided the wrong password (%s)", username, e.Error())
			}
		} else {
			err = errors.New("could not authenticate user")
		}

		if err != nil {
			log.Printf("login failed: %s\n", err.Error())
			rw.WriteHeader(http.StatusUnauthorized)
			templates.Render(rw, r, "login.html", &templates.Page{
				Title: "Login failed",
				Login: &templates.LoginPage{
					Error: "Username or password incorrect",
				},
			})
			return
		}

		session, err := sessionStore.New(r, "session")
		if err != nil {
			log.Printf("session creation failed: %s\n", err.Error())
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		session.Options.MaxAge = 30 * 24 * 60 * 60
		session.Values["username"] = user.Username
		session.Values["roles"] = user.Roles
		if err := sessionStore.Save(r, rw, session); err != nil {
			log.Printf("session save failed: %s\n", err.Error())
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("login successfull: user: %#v (roles: %v)\n", user.Username, user.Roles)
		http.Redirect(rw, r, "/", http.StatusTemporaryRedirect)
	})
}

var ErrTokenInvalid error = errors.New("invalid token")

func authViaToken(r *http.Request) (*User, error) {
	if JwtPublicKey == nil {
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
		return JwtPublicKey, nil
	})
	if err != nil {
		return nil, ErrTokenInvalid
	}

	if err := token.Claims.Valid(); err != nil {
		return nil, ErrTokenInvalid
	}

	claims := token.Claims.(jwt.MapClaims)
	sub, _ := claims["sub"].(string)
	roles, _ := claims["roles"].([]string)

	// TODO: Check if sub is still a valid user!
	return &User{
		Username: sub,
		Roles:    roles,
	}, nil
}

// Authenticate the user and put a User object in the
// context of the request. If authentication fails,
// do not continue but send client to the login screen.
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		user, err := authViaToken(r)
		if err == ErrTokenInvalid {
			log.Printf("authentication failed: invalid token\n")
			http.Error(rw, err.Error(), http.StatusUnauthorized)
			return
		}
		if user != nil {
			// Successfull authentication using a token
			ctx := context.WithValue(r.Context(), ContextUserKey, user)
			next.ServeHTTP(rw, r.WithContext(ctx))
			return
		}

		session, err := sessionStore.Get(r, "session")
		if err != nil {
			// sessionStore.Get will return a new session if no current one is attached to this request.
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		if session.IsNew {
			log.Printf("authentication failed: no session or jwt found\n")

			rw.WriteHeader(http.StatusUnauthorized)
			templates.Render(rw, r, "login.html", &templates.Page{
				Title: "Authentication failed",
				Login: &templates.LoginPage{
					Error: "No valid session or JWT provided",
				},
			})
			return
		}

		username, _ := session.Values["username"].(string)
		roles, _ := session.Values["roles"].([]string)
		ctx := context.WithValue(r.Context(), ContextUserKey, &User{
			Username: username,
			Roles:    roles,
		})
		next.ServeHTTP(rw, r.WithContext(ctx))
	})
}

// Generate a new JWT that can be used for authentication
func ProvideJWT(user *User) (string, error) {
	if JwtPrivateKey == nil {
		return "", errors.New("environment variable 'JWT_PRIVATE_KEY' not set")
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims{
		"sub":   user.Username,
		"roles": user.Roles,
	})

	return tok.SignedString(JwtPrivateKey)
}

func GetUser(ctx context.Context) *User {
	x := ctx.Value(ContextUserKey)
	if x == nil {
		return nil
	}

	return x.(*User)
}

// Clears the session cookie
func Logout(rw http.ResponseWriter, r *http.Request) {
	session, err := sessionStore.Get(r, "session")
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	if !session.IsNew {
		session.Options.MaxAge = -1
		if err := sessionStore.Save(r, rw, session); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	templates.Render(rw, r, "login.html", &templates.Page{
		Title: "Logout successful",
		Login: &templates.LoginPage{
			Info: "Logout successful",
		},
	})
}
