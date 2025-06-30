// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

import (
	"testing"
)

func TestNodeList(t *testing.T) {
	nl, err := ParseNodeList("hallo,wel123t,emmy[01-99],fritz[005-500],woody[100-200]")
	if err != nil {
		t.Fatal(err)
	}

	if nl.Contains("hello") || nl.Contains("woody") {
		t.Fail()
	}

	if nl.Contains("fritz1") || nl.Contains("fritz9") || nl.Contains("fritz004") || nl.Contains("woody201") {
		t.Fail()
	}

	if !nl.Contains("hallo") || !nl.Contains("wel123t") {
		t.Fail()
	}

	if !nl.Contains("emmy01") || !nl.Contains("emmy42") || !nl.Contains("emmy99") {
		t.Fail()
	}

	if !nl.Contains("woody100") || !nl.Contains("woody199") {
		t.Fail()
	}
}

func TestNodeListCommasInBrackets(t *testing.T) {
	nl, err := ParseNodeList("a[1000-2000,2010-2090,3000-5000]")
	if err != nil {
		t.Fatal(err)
	}

	if nl.Contains("hello") || nl.Contains("woody") {
		t.Fatal("1")
	}

	if nl.Contains("a0") || nl.Contains("a0000") || nl.Contains("a5001") || nl.Contains("a2005") {
		t.Fatal("2")
	}

	if !nl.Contains("a1001") || !nl.Contains("a2000") {
		t.Fatal("3")
	}

	if !nl.Contains("a2042") || !nl.Contains("a4321") || !nl.Contains("a3000") {
		t.Fatal("4")
	}
}

func TestNodeListCommasOutsideBrackets(t *testing.T) {
	nl, err := ParseNodeList("cn-0010,cn0011,cn-00[13-18,22-24]")
	if err != nil {
		t.Fatal(err)
	}
	if !nl.Contains("cn-0010") || !nl.Contains("cn0011") {
		t.Fatal("1")
	}
	if !nl.Contains("cn-0013") ||
		!nl.Contains("cn-0015") ||
		!nl.Contains("cn-0022") ||
		!nl.Contains("cn-0018") {
		t.Fatal("2")
	}
}
