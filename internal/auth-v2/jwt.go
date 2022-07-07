package authv2

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/golang-jwt/jwt/v4"
)

type JWTAuthenticator struct {
	auth       *Authentication
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
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

func (ja *JWTAuthenticator) Login(user *User, password string, rw http.ResponseWriter, r *http.Request) error {
	return nil
}

func (ja *JWTAuthenticator) Auth(rw http.ResponseWriter, r *http.Request) (*User, error) {
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

	// TODO: Check if sub is still a valid user!
	return &User{
		Username:   sub,
		Roles:      roles,
		AuthSource: AuthViaToken,
	}, nil
}
