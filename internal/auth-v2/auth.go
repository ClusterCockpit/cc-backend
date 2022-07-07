package authv2

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	sq "github.com/Masterminds/squirrel"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
)

const (
	RoleAdmin string = "admin"
	RoleApi   string = "api"
	RoleUser  string = "user"
)

const (
	AuthViaLocalPassword int8 = 0
	AuthViaLDAP          int8 = 1
	AuthViaToken         int8 = 2
)

type User struct {
	Username   string   `json:"username"`
	Password   string   `json:"-"`
	Name       string   `json:"name"`
	Roles      []string `json:"roles"`
	AuthSource int8     `json:"via"`
	Email      string   `json:"email"`
	Expiration time.Time
}

func (u *User) HasRole(role string) bool {
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

type Authenticator interface {
	Init(auth *Authentication, config json.RawMessage) error
	CanLogin(user *User, rw http.ResponseWriter, r *http.Request) bool
	Login(user *User, password string, rw http.ResponseWriter, r *http.Request) (*User, error)
	Auth(rw http.ResponseWriter, r *http.Request) (*User, error)
}

type ContextKey string

const ContextUserKey ContextKey = "user"

type Authentication struct {
	db             *sqlx.DB
	sessionStore   *sessions.CookieStore
	authenticators []Authenticator
}

func Init(db *sqlx.DB) (*Authentication, error) {
	auth := &Authentication{}
	auth.db = db
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS user (
		username varchar(255) PRIMARY KEY NOT NULL,
		password varchar(255) DEFAULT NULL,
		ldap     tinyint      NOT NULL DEFAULT 0, /* col called "ldap" for historic reasons, fills the "AuthSource" */
		name     varchar(255) DEFAULT NULL,
		roles    varchar(255) NOT NULL DEFAULT "[]",
		email    varchar(255) DEFAULT NULL);`)
	if err != nil {
		return nil, err
	}

	return auth, nil
}

func (auth *Authentication) GetUser(username string) (*User, error) {
	user := &User{Username: username}
	var hashedPassword, name, rawRoles, email sql.NullString
	if err := sq.Select("password", "ldap", "name", "roles", "email").From("user").
		Where("user.username = ?", username).RunWith(auth.db).
		QueryRow().Scan(&hashedPassword, &user.AuthSource, &name, &rawRoles, &email); err != nil {
		return nil, err
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

func (auth *Authentication) AddUser(user *User) error {
	rolesJson, _ := json.Marshal(user.Roles)
	cols := []string{"username", "password", "roles"}
	vals := []interface{}{user.Username, user.Password, string(rolesJson)}
	if user.Name != "" {
		cols = append(cols, "name")
		vals = append(vals, user.Name)
	}
	if user.Email != "" {
		cols = append(cols, "email")
		vals = append(vals, user.Email)
	}

	if _, err := sq.Insert("user").Columns(cols...).Values(vals...).RunWith(auth.db).Exec(); err != nil {
		return err
	}

	log.Infof("new user %#v created (roles: %s)", user.Username, rolesJson)
	return nil
}

func (auth *Authentication) AuthViaSession(rw http.ResponseWriter, r *http.Request) (*User, error) {
	session, err := auth.sessionStore.Get(r, "session")
	if err != nil {
		return nil, err
	}

	if session.IsNew {
		return nil, nil
	}

	username, _ := session.Values["username"].(string)
	roles, _ := session.Values["roles"].([]string)
	return &User{
		Username: username,
		Roles:    roles,
	}, nil
}
