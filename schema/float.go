package schema

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

func (f Float) IsNaN() bool {
	return math.IsNaN(float64(f))
}

// NaN will be serialized to `null`.
func (f Float) MarshalJSON() ([]byte, error) {
	if f.IsNaN() {
		return []byte("null"), nil
	}

	return []byte(strconv.FormatFloat(float64(f), 'f', 2, 64)), nil
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
		w.Write([]byte(`null`))
	} else {
		w.Write([]byte(strconv.FormatFloat(float64(f), 'f', 2, 64)))
	}
}
