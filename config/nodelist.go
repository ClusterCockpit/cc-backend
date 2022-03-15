package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ClusterCockpit/cc-backend/log"
)

type NLExprString string

func (nle NLExprString) consume(input string) (next string, ok bool) {
	str := string(nle)
	if strings.HasPrefix(input, str) {
		return strings.TrimPrefix(input, str), true
	}
	return "", false
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

type NodeList [][]interface {
	consume(input string) (next string, ok bool)
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

func ParseNodeList(raw string) (NodeList, error) {
	nl := NodeList{}

	isLetter := func(r byte) bool { return ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') }
	isDigit := func(r byte) bool { return '0' <= r && r <= '9' }

	for _, rawterm := range strings.Split(raw, ",") {
		exprs := []interface {
			consume(input string) (next string, ok bool)
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

				minus := strings.Index(rawterm[i:i+end], "-")
				if minus == -1 {
					return nil, fmt.Errorf("node list: no '-' found inside '[...]'")
				}

				s1, s2 := rawterm[i+1:i+minus], rawterm[i+minus+1:i+end]
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

				exprs = append(exprs, NLExprIntRange{
					start:      x1,
					end:        x2,
					digits:     len(s1),
					zeroPadded: true,
				})
				i += end
			} else {
				return nil, fmt.Errorf("node list: invalid character: %#v", rune(c))
			}
		}
		nl = append(nl, exprs)
	}

	return nl, nil
}
