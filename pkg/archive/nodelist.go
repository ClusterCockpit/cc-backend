// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
)

type NodeList [][]interface {
	consume(input string) (next string, ok bool)
	limits() []map[string]int64
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
		// log.Debugf("Term: %v", term)

		prefix := term[0].prefix()
		// log.Debugf("Prefix as String: %s", prefix)

		limitArr := term[1].limits()
		for _, inner := range limitArr {
			for i := inner["start"]; i < inner["end"]+1; i++ {
				node := fmt.Sprintf("%s%02d", prefix, i)
				out = append(out, node)
			}
			// log.Debugf("Inner Map @ %d: %#v", indx, inner)
			// log.Debugf("Start: %#v", inner["start"])
			// log.Debugf("End: %#v", inner["end"])
		}
	}
	// log.Debugf("Node List as Strings: %#v", out)
	return out
}

type NLExprString string

func (nle NLExprString) consume(input string) (next string, ok bool) {
	str := string(nle)
	if strings.HasPrefix(input, str) {
		return strings.TrimPrefix(input, str), true
	}
	return "", false
}

func (nle NLExprString) limits() []map[string]int64 {
	// Null implementation to  fullfill interface requirement
	l := make([]map[string]int64, 0)
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

func (nles NLExprIntRanges) limits() []map[string]int64 {
	l := make([]map[string]int64, 0)
	for _, nle := range nles {
		inner := nle.limits()
		// log.Debugf("limits @ nles: %#v", inner)
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
		log.Error("node list: only zero-padded ranges are allowed")
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

func (nle NLExprIntRange) limits() []map[string]int64 {
	l := make([]map[string]int64, 0)
	m := make(map[string]int64)
	m["start"] = nle.start
	m["end"] = nle.end
	l = append(l, m)
	// log.Debugf("limits @ nle: %#v", l)
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

	rawterms := []string{}
	prevterm := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] == '[' {
			for i < len(raw) && raw[i] != ']' {
				i++
			}
			if i == len(raw) {
				return nil, fmt.Errorf("node list: unclosed '['")
			}
		} else if raw[i] == ',' {
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
			limits() []map[string]int64
			prefix() string
		}{}
		for i := 0; i < len(rawterm); i++ {
			c := rawterm[i]
			if isLetter(c) || isDigit(c) {
				j := i
				for j < len(rawterm) && (isLetter(rawterm[j]) || isDigit(rawterm[j])) {
					j++
				}
				exprs = append(exprs, NLExprString(rawterm[i:j]))
				i = j - 1
			} else if c == '[' {
				end := strings.Index(rawterm[i:], "]")
				if end == -1 {
					return nil, fmt.Errorf("node list: unclosed '['")
				}

				parts := strings.Split(rawterm[i+1:i+end], ",")
				nles := NLExprIntRanges{}
				for _, part := range parts {
					minus := strings.Index(part, "-")
					if minus == -1 {
						return nil, fmt.Errorf("node list: no '-' found inside '[...]'")
					}

					s1, s2 := part[0:minus], part[minus+1:]
					if len(s1) != len(s2) || len(s1) == 0 {
						return nil, fmt.Errorf("node list: %#v and %#v are not of equal length or of length zero", s1, s2)
					}

					x1, err := strconv.ParseInt(s1, 10, 32)
					if err != nil {
						return nil, fmt.Errorf("node list: %w", err)
					}
					x2, err := strconv.ParseInt(s2, 10, 32)
					if err != nil {
						return nil, fmt.Errorf("node list: %w", err)
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
				return nil, fmt.Errorf("node list: invalid character: %#v", rune(c))
			}
		}
		nl = append(nl, exprs)
	}

	return nl, nil
}
