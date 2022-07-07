package authv2

import (
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

type LocalAuthenticator struct {
	auth *Authentication
}

var _ Authenticator = (*LocalAuthenticator)(nil)

func (la *LocalAuthenticator) Init(auth *Authentication, rawConfig json.RawMessage) error {
	la.auth = auth
	return nil
}

func (la *LocalAuthenticator) CanLogin(user *User, rw http.ResponseWriter, r *http.Request) bool {
	return user.AuthSource == AuthViaLocalPassword
}

func (la *LocalAuthenticator) Login(user *User, password string, rw http.ResponseWriter, r *http.Request) (*User, error) {
	if e := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); e != nil {
		return nil, fmt.Errorf("user '%s' provided the wrong password (%w)", user.Username, e)
	}

	return user, nil
}

func (la *LocalAuthenticator) Auth(rw http.ResponseWriter, r *http.Request) (*User, error) {
	user, err := la.auth.AuthViaSession(rw, r)
	if err != nil {
		return nil, err
	}

	user.AuthSource = AuthViaLocalPassword
	return user, nil
}
