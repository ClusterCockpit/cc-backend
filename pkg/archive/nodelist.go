// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package archive provides nodelist parsing functionality for HPC cluster node specifications.
//
// # Overview
//
// The nodelist package implements parsing and querying of compact node list representations
// commonly used in HPC job schedulers and cluster management systems. It converts compressed
// node specifications (e.g., "node[01-10]") into queryable structures that can efficiently
// test node membership and expand to full node lists.
//
// # Node List Format
//
// Node lists use a compact syntax with the following rules:
//
//  1. Comma-separated terms represent alternative node patterns (OR logic)
//  2. Each term consists of a string prefix followed by optional numeric ranges
//  3. Numeric ranges are specified in square brackets with zero-padded start-end format
//  4. Multiple ranges within brackets are comma-separated
//  5. Range digits must be zero-padded and of equal length (e.g., "01-99" not "1-99")
//
// # Examples
//
//	"node01"                    // Single node
//	"node01,node02"             // Multiple individual nodes
//	"node[01-10]"               // Range: node01 through node10 (zero-padded)
//	"node[01-10,20-30]"         // Multiple ranges: node01-10 and node20-30
//	"cn-00[10-20],cn-00[50-60]" // Different prefixes with ranges
//	"login,compute[001-100]"    // Mixed individual and range terms
//
// # Usage
//
// Parse a node list specification:
//
//	nl, err := ParseNodeList("node[01-10],login")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Check if a node name matches the list:
//
//	if nl.Contains("node05") {
//	    // node05 is in the list
//	}
//
// Expand to full list of node names:
//
//	nodes := nl.PrintList()  // ["node01", "node02", ..., "node10", "login"]
//
// Count total nodes in the list:
//
//	count := nl.NodeCount()  // 11 (10 from range + 1 individual)
//
// # Integration
//
// This package is used by:
//   - clusterConfig.go: Parses SubCluster.Nodes field from cluster configuration
//   - schema.resolvers.go: GraphQL resolver for computing numberOfNodes in subclusters
//   - Job archive: Validates node assignments against configured cluster topology
//
// # Constraints
//
//   - Only zero-padded numeric ranges are supported
//   - Range start and end must have identical digit counts
//   - No whitespace allowed in node list specifications
//   - Ranges must be specified as start-end (not individual numbers)
package archive

import (
	"fmt"
	"strconv"
	"strings"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

// NodeList represents a parsed node list specification as a collection of node pattern terms.
// Each term is a sequence of expressions that must match consecutively for a node name to match.
// Terms are evaluated with OR logic - a node matches if ANY term matches completely.
//
// Internal structure:
//   - Outer slice: OR terms (comma-separated in input)
//   - Inner slice: AND expressions (must all match sequentially)
//   - Each expression implements: consume (pattern matching), limits (range info), prefix (string part)
//
// Example: "node[01-10],login" becomes:
//   - Term 1: [NLExprString("node"), NLExprIntRanges(01-10)]
//   - Term 2: [NLExprString("login")]
type NodeList [][]interface {
	consume(input string) (next string, ok bool)
	limits() []map[string]int
	prefix() string
}

// Contains tests whether the given node name matches any pattern in the NodeList.
// Returns true if the name matches at least one term completely, false otherwise.
//
// Matching logic:
//   - Evaluates each term sequentially (OR logic across terms)
//   - Within a term, all expressions must match in order (AND logic)
//   - A match is complete only if the entire input is consumed (str == "")
//
// Examples:
//   - NodeList("node[01-10]").Contains("node05") → true
//   - NodeList("node[01-10]").Contains("node11") → false
//   - NodeList("node[01-10]").Contains("node5")  → false (missing zero-padding)
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

// PrintList expands the NodeList into a full slice of individual node names.
// This performs the inverse operation of ParseNodeList, expanding all ranges
// into their constituent node names with proper zero-padding.
//
// Returns a slice of node names in the order they appear in the NodeList.
// For range terms, nodes are expanded in ascending numeric order.
//
// Example:
//   - ParseNodeList("node[01-03],login").PrintList() → ["node01", "node02", "node03", "login"]
func (nl *NodeList) PrintList() []string {
	var out []string
	for _, term := range *nl {
		prefix := term[0].prefix()
		if len(term) == 1 {
			out = append(out, prefix)
		} else {
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

// NodeCount returns the total number of individual nodes represented by the NodeList.
// This efficiently counts nodes without expanding the full list, making it suitable
// for large node ranges.
//
// Calculation:
//   - Individual node terms contribute 1
//   - Range terms contribute (end - start + 1) for each range
//
// Example:
//   - ParseNodeList("node[01-10],login").NodeCount() → 11 (10 from range + 1 individual)
func (nl *NodeList) NodeCount() int {
	out := 0
	for _, term := range *nl {
		if len(term) == 1 {
			out += 1
		} else {
			limitArr := term[1].limits()
			for _, inner := range limitArr {
				out += (inner["end"] - inner["start"]) + 1
			}
		}
	}
	return out
}

// NLExprString represents a literal string prefix in a node name pattern.
// It matches by checking if the input starts with this exact string.
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

// NLExprIntRanges represents multiple alternative integer ranges (comma-separated within brackets).
// A node name matches if it matches ANY of the contained ranges (OR logic).
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

// NLExprIntRange represents a single zero-padded integer range (e.g., "01-99").
// Fields:
//   - start, end: Numeric range boundaries (inclusive)
//   - zeroPadded: Must be true (non-padded ranges not supported)
//   - digits: Required digit count for zero-padding
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

// ParseNodeList parses a compact node list specification into a queryable NodeList structure.
//
// Input format rules:
//   - Comma-separated terms (OR logic): "node01,node02" matches either node
//   - Range syntax: "node[01-10]" expands to node01 through node10
//   - Multiple ranges: "node[01-05,10-15]" creates two ranges
//   - Zero-padding required: digits in ranges must be zero-padded and equal length
//   - Mixed formats: "login,compute[001-100]" combines individual and range terms
//
// Validation:
//   - Returns error if brackets are unclosed
//   - Returns error if ranges lack '-' separator
//   - Returns error if range digits have unequal length
//   - Returns error if range numbers fail to parse
//   - Returns error on invalid characters
//
// Examples:
//   - "node[01-10]" → NodeList with one term (10 nodes)
//   - "node01,node02" → NodeList with two terms (2 nodes)
//   - "cn[01-05,10-15]" → NodeList with ranges 01-05 and 10-15 (11 nodes total)
//   - "a[1-9]" → Error (not zero-padded)
//   - "a[01-9]" → Error (unequal digit counts)
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
					before, after, ok := strings.Cut(part, "-")
					if !ok {
						return nil, fmt.Errorf("ARCHIVE/NODELIST > no '-' found inside '[...]'")
					}

					s1, s2 := before, after
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
