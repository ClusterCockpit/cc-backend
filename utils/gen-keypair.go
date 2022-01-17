package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
)

func main() {
	// rand.Reader uses /dev/urandom on Linux
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
		os.Exit(1)
	}

	fmt.Fprintf(os.Stdout, "JWT_PUBLIC_KEY=%#v\nJWT_PRIVATE_KEY=%#v\n",
		base64.StdEncoding.EncodeToString(pub),
		base64.StdEncoding.EncodeToString(priv))
}
