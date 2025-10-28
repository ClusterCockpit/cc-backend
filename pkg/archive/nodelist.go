// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package archive

import (
	"fmt"
	"strconv"
	"strings"

	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
)

type NodeList [][]interface {
	consume(input string) (next string, ok bool)
	limits() []map[string]int
	prefix() string
}

func (nl *NodeList) Contains(name string) bool {
	var ok bool
	for _, term := range *nl {
		str := name
		for _, expr := range term {
			str, ok = expr.consume(str)
			if !ok {
				break
			}
		}

		if ok && str == "" {
			return true
		}
	}

	return false
}

func (nl *NodeList) PrintList() []string {
	var out []string
	for _, term := range *nl {
		// Get String-Part first
		prefix := term[0].prefix()
		if len(term) == 1 { // If only String-Part in Term: Single Node Name -> Use as provided
			out = append(out, prefix)
		} else { // Else: Numeric start-end definition with x digits zeroPadded
			limitArr := term[1].limits()
			for _, inner := range limitArr {
				for i := inner["start"]; i < inner["end"]+1; i++ {
					if inner["zeroPadded"] == 1 {
						out = append(out, fmt.Sprintf("%s%0*d", prefix, inner["digits"], i))
					} else {
						cclog.Error("node list: only zero-padded ranges are allowed")
					}
				}
			}
		}
	}
	return out
}

func (nl *NodeList) NodeCount() int {
	out := 0
	for _, term := range *nl {
		if len(term) == 1 { // If only String-Part in Term: Single Node Name -> add one
			out += 1
		} else { // Else: Numeric start-end definition -> add difference + 1
			limitArr := term[1].limits()
			for _, inner := range limitArr {
				out += (inner["end"] - inner["start"]) + 1
			}
		}
	}
	return out
}

type NLExprString string

func (nle NLExprString) consume(input string) (next string, ok bool) {
	str := string(nle)
	if after, ok0 := strings.CutPrefix(input, str); ok0 {
		return after, true
	}
	return "", false
}

func (nle NLExprString) limits() []map[string]int {
	// Null implementation to  fullfill interface requirement
	l := make([]map[string]int, 0)
	return l
}

func (nle NLExprString) prefix() string {
	return string(nle)
}

type NLExprIntRanges []NLExprIntRange

func (nles NLExprIntRanges) consume(input string) (next string, ok bool) {
	for _, nle := range nles {
		if next, ok := nle.consume(input); ok {
			return next, ok
		}
	}
	return "", false
}

func (nles NLExprIntRanges) limits() []map[string]int {
	l := make([]map[string]int, 0)
	for _, nle := range nles {
		inner := nle.limits()
		l = append(l, inner[0])
	}
	return l
}

func (nles NLExprIntRanges) prefix() string {
	// Null implementation to  fullfill interface requirement
	var s string
	return s
}

type NLExprIntRange struct {
	start, end int64
	zeroPadded bool
	digits     int
}

func (nle NLExprIntRange) consume(input string) (next string, ok bool) {
	if !nle.zeroPadded || nle.digits < 1 {
		cclog.Error("only zero-padded ranges are allowed")
		return "", false
	}

	if len(input) < nle.digits {
		return "", false
	}

	numerals, rest := input[:nle.digits], input[nle.digits:]
	for len(numerals) > 1 && numerals[0] == '0' {
		numerals = numerals[1:]
	}

	x, err := strconv.ParseInt(numerals, 10, 32)
	if err != nil {
		return "", false
	}

	if nle.start <= x && x <= nle.end {
		return rest, true
	}

	return "", false
}

func (nle NLExprIntRange) limits() []map[string]int {
	l := make([]map[string]int, 0)
	m := make(map[string]int)
	m["start"] = int(nle.start)
	m["end"] = int(nle.end)
	m["digits"] = int(nle.digits)
	if nle.zeroPadded {
		m["zeroPadded"] = 1
	} else {
		m["zeroPadded"] = 0
	}
	l = append(l, m)
	return l
}

func (nles NLExprIntRange) prefix() string {
	// Null implementation to  fullfill interface requirement
	var s string
	return s
}

func ParseNodeList(raw string) (NodeList, error) {
	isLetter := func(r byte) bool { return ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') }
	isDigit := func(r byte) bool { return '0' <= r && r <= '9' }
	isDash := func(r byte) bool { return r == '-' }

	rawterms := []string{}
	prevterm := 0
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '[':
			for i < len(raw) && raw[i] != ']' {
				i++
			}
			if i == len(raw) {
				return nil, fmt.Errorf("ARCHIVE/NODELIST > unclosed '['")
			}
		case ',':
			rawterms = append(rawterms, raw[prevterm:i])
			prevterm = i + 1
		}
	}
	if prevterm != len(raw) {
		rawterms = append(rawterms, raw[prevterm:])
	}

	nl := NodeList{}
	for _, rawterm := range rawterms {
		exprs := []interface {
			consume(input string) (next string, ok bool)
			limits() []map[string]int
			prefix() string
		}{}

		for i := 0; i < len(rawterm); i++ {
			c := rawterm[i]
			if isLetter(c) || isDigit(c) {
				j := i
				for j < len(rawterm) &&
					(isLetter(rawterm[j]) ||
						isDigit(rawterm[j]) ||
						isDash(rawterm[j])) {
					j++
				}
				exprs = append(exprs, NLExprString(rawterm[i:j]))
				i = j - 1
			} else if c == '[' {
				end := strings.Index(rawterm[i:], "]")

				if end == -1 {
					return nil, fmt.Errorf("ARCHIVE/NODELIST > unclosed '['")
				}

				parts := strings.Split(rawterm[i+1:i+end], ",")
				nles := NLExprIntRanges{}

				for _, part := range parts {
					minus := strings.Index(part, "-")
					if minus == -1 {
						return nil, fmt.Errorf("ARCHIVE/NODELIST > no '-' found inside '[...]'")
					}

					s1, s2 := part[0:minus], part[minus+1:]
					if len(s1) != len(s2) || len(s1) == 0 {
						return nil, fmt.Errorf("ARCHIVE/NODELIST > %v and %v are not of equal length or of length zero", s1, s2)
					}

					x1, err := strconv.ParseInt(s1, 10, 32)
					if err != nil {
						return nil, fmt.Errorf("ARCHIVE/NODELIST > could not parse int: %w", err)
					}
					x2, err := strconv.ParseInt(s2, 10, 32)
					if err != nil {
						return nil, fmt.Errorf("ARCHIVE/NODELIST > could not parse int: %w", err)
					}

					nles = append(nles, NLExprIntRange{
						start:      x1,
						end:        x2,
						digits:     len(s1),
						zeroPadded: true,
					})
				}

				exprs = append(exprs, nles)
				i += end
			} else {
				return nil, fmt.Errorf("ARCHIVE/NODELIST > invalid character: %#v", rune(c))
			}
		}
		nl = append(nl, exprs)
	}

	return nl, nil
}
