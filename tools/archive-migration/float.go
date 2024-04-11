// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"errors"
	"io"
	"math"
	"strconv"
)

// A custom float type is used so that (Un)MarshalJSON and
// (Un)MarshalGQL can be overloaded and NaN/null can be used.
// The default behaviour of putting every nullable value behind
// a pointer has a bigger overhead.
type Float float64

var NaN Float = Float(math.NaN())
var nullAsBytes []byte = []byte("null")

func (f Float) IsNaN() bool {
	return math.IsNaN(float64(f))
}

// NaN will be serialized to `null`.
func (f Float) MarshalJSON() ([]byte, error) {
	if f.IsNaN() {
		return nullAsBytes, nil
	}

	return strconv.AppendFloat(make([]byte, 0, 10), float64(f), 'f', 2, 64), nil
}

// `null` will be unserialized to NaN.
func (f *Float) UnmarshalJSON(input []byte) error {
	s := string(input)
	if s == "null" {
		*f = NaN
		return nil
	}

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*f = Float(val)
	return nil
}

// UnmarshalGQL implements the graphql.Unmarshaler interface.
func (f *Float) UnmarshalGQL(v interface{}) error {
	f64, ok := v.(float64)
	if !ok {
		return errors.New("invalid Float scalar")
	}

	*f = Float(f64)
	return nil
}

// MarshalGQL implements the graphql.Marshaler interface.
// NaN will be serialized to `null`.
func (f Float) MarshalGQL(w io.Writer) {
	if f.IsNaN() {
		w.Write(nullAsBytes)
	} else {
		w.Write(strconv.AppendFloat(make([]byte, 0, 10), float64(f), 'f', 2, 64))
	}
}

// Only used via REST-API, not via GraphQL.
// This uses a lot less allocations per series,
// but it turns out that the performance increase
// from using this is not that big.
func (s *Series) MarshalJSON() ([]byte, error) {
	buf := make([]byte, 0, 512+len(s.Data)*8)
	buf = append(buf, `{"hostname":"`...)
	buf = append(buf, s.Hostname...)
	buf = append(buf, '"')
	if s.Id != nil {
		buf = append(buf, `,"id":`...)
		buf = strconv.AppendInt(buf, int64(*s.Id), 10)
	}
	if s.Statistics != nil {
		buf = append(buf, `,"statistics":{"min":`...)
		buf = strconv.AppendFloat(buf, s.Statistics.Min, 'f', 2, 64)
		buf = append(buf, `,"avg":`...)
		buf = strconv.AppendFloat(buf, s.Statistics.Avg, 'f', 2, 64)
		buf = append(buf, `,"max":`...)
		buf = strconv.AppendFloat(buf, s.Statistics.Max, 'f', 2, 64)
		buf = append(buf, '}')
	}
	buf = append(buf, `,"data":[`...)
	for i := 0; i < len(s.Data); i++ {
		if i != 0 {
			buf = append(buf, ',')
		}

		if s.Data[i].IsNaN() {
			buf = append(buf, `null`...)
		} else {
			buf = strconv.AppendFloat(buf, float64(s.Data[i]), 'f', 2, 32)
		}
	}
	buf = append(buf, ']', '}')
	return buf, nil
}
