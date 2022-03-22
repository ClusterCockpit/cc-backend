package config

import (
	"testing"
)

func TestNodeList(t *testing.T) {
	nl, err := ParseNodeList("hallo,wel123t,emmy[01-99],fritz[005-500],woody[100-200]")
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Printf("terms\n")
	// for i, term := range nl.terms {
	// 	fmt.Printf("term %d: %#v\n", i, term)
	// }

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
