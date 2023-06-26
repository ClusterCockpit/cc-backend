// Copyright (C) 2022 Paderborn Center for Parallel Computing, Paderborn University
// This code is released under MIT License:
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
)

func main() {
	filepath := ""
	if len(os.Args) > 1 {
		filepath = os.Args[1]
	} else {
		PrintUsage()
		os.Exit(1)
	}

	pubkey, err := LoadEd255519PubkeyFromPEMFile(filepath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
		os.Exit(1)
	}

	fmt.Fprintf(os.Stdout,
		"CROSS_LOGIN_JWT_PUBLIC_KEY=%#v\n",
		base64.StdEncoding.EncodeToString(pubkey))
}

// Loads an ed25519 public key stored in a file in PEM format
func LoadEd255519PubkeyFromPEMFile(filePath string) (ed25519.PublicKey, error) {
	buffer, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(buffer)
	if block == nil {
		return nil, fmt.Errorf("no pem block found")
	}

	pubkey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	ed25519PublicKey, success := pubkey.(ed25519.PublicKey)
	if !success {
		return nil, fmt.Errorf("not an ed25519 key")
	}

	return ed25519PublicKey, nil
}

func PrintUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s <filename>\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "where <filename> contains an Ed25519 public key in PEM format\n")
	fmt.Fprintf(os.Stderr, "(starting with '-----BEGIN PUBLIC KEY-----')\n")
}
