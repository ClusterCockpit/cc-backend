package authv2

import (
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/golang-jwt/jwt/v4"
)

type JWTAuthenticator struct {
	auth       *Authentication
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey

	maxAge time.Duration
}

var _ Authenticator = (*JWTAuthenticator)(nil)

func (ja *JWTAuthenticator) Init(auth *Authentication, rawConfig json.RawMessage) error {
	ja.auth = auth

	pubKey, privKey := os.Getenv("JWT_PUBLIC_KEY"), os.Getenv("JWT_PRIVATE_KEY")
	if pubKey == "" || privKey == "" {
		log.Warn("environment variables 'JWT_PUBLIC_KEY' or 'JWT_PRIVATE_KEY' not set (token based authentication will not work)")
	} else {
		bytes, err := base64.StdEncoding.DecodeString(pubKey)
		if err != nil {
			return err
		}
		ja.publicKey = ed25519.PublicKey(bytes)
		bytes, err = base64.StdEncoding.DecodeString(privKey)
		if err != nil {
			return err
		}
		ja.privateKey = ed25519.PrivateKey(bytes)
	}

	return nil
}

func (ja *JWTAuthenticator) CanLogin(user *User, rw http.ResponseWriter, r *http.Request) bool {
	return user.AuthSource == AuthViaToken || r.Header.Get("Authorization") != ""
}

func (ja *JWTAuthenticator) Login(_ *User, password string, rw http.ResponseWriter, r *http.Request) (*User, error) {
	rawtoken := r.Header.Get("X-Auth-Token")
	if rawtoken == "" {
		rawtoken = r.Header.Get("Authorization")
		rawtoken = strings.TrimPrefix("Bearer ", rawtoken)
	}

	token, err := jwt.Parse(rawtoken, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodEdDSA {
			return nil, errors.New("only Ed25519/EdDSA supported")
		}
		return ja.publicKey, nil
	})
	if err != nil {
		return nil, err
	}

	if err := token.Claims.Valid(); err != nil {
		return nil, err
	}

	claims := token.Claims.(jwt.MapClaims)
	sub, _ := claims["sub"].(string)
	exp, _ := claims["exp"].(float64)
	var roles []string
	if rawroles, ok := claims["roles"].([]interface{}); ok {
		for _, rr := range rawroles {
			if r, ok := rr.(string); ok {
				roles = append(roles, r)
			}
		}
	}

	user, err := ja.auth.GetUser(sub)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if err != nil && err == sql.ErrNoRows {
		user = &User{
			Username:   user.Username,
			Roles:      roles,
			AuthSource: AuthViaToken,
		}
		if err := ja.auth.AddUser(user); err != nil {
			return nil, err
		}
	}

	user.Expiration = time.Unix(int64(exp), 0)
	return user, nil
}

func (ja *JWTAuthenticator) Auth(rw http.ResponseWriter, r *http.Request) (*User, error) {
	rawtoken := r.Header.Get("X-Auth-Token")
	if rawtoken == "" {
		rawtoken = r.Header.Get("Authorization")
		rawtoken = strings.TrimPrefix("Bearer ", rawtoken)
	}

	// Because a user can also log in via a token, the
	// session cookie must be checked here as well:
	if rawtoken == "" {
		user, err := ja.auth.AuthViaSession(rw, r)
		if err != nil {
			return nil, err
		}

		user.AuthSource = AuthViaToken
		return user, nil
	}

	token, err := jwt.Parse(rawtoken, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodEdDSA {
			return nil, errors.New("only Ed25519/EdDSA supported")
		}
		return ja.publicKey, nil
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

	return &User{
		Username:   sub,
		Roles:      roles,
		AuthSource: AuthViaToken,
	}, nil
}

// Generate a new JWT that can be used for authentication
func (ja *JWTAuthenticator) ProvideJWT(user *User) (string, error) {
	if ja.privateKey == nil {
		return "", errors.New("environment variable 'JWT_PRIVATE_KEY' not set")
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   user.Username,
		"roles": user.Roles,
		"iat":   now.Unix(),
	}
	if ja.maxAge != 0 {
		claims["exp"] = now.Add(ja.maxAge).Unix()
	}

	return jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(ja.privateKey)
}
