// Unit system for cluster monitoring metrics like bytes, flops and events
package units

import (
	"fmt"
	"math"
	"strings"

	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

type unit struct {
	prefix     Prefix
	measure    Measure
	divMeasure Measure
}

type Unit interface {
	Valid() bool
	String() string
	Short() string
	AddUnitDenominator(div Measure)
	getPrefix() Prefix
	getMeasure() Measure
	getUnitDenominator() Measure
	setPrefix(p Prefix)
}

var INVALID_UNIT = NewUnit("foobar")

// Valid checks whether a unit is a valid unit.
// A unit is valid if it has at least a prefix and a measure.
// The unit denominator is optional.
func (u *unit) Valid() bool {
	return u.prefix != InvalidPrefix && u.measure != InvalidMeasure
}

// String returns the long string for the unit like 'KiloHertz' or 'MegaBytes'
func (u *unit) String() string {
	if u.divMeasure != InvalidMeasure {
		return fmt.Sprintf("%s%s/%s", u.prefix.String(), u.measure.String(), u.divMeasure.String())
	} else {
		return fmt.Sprintf("%s%s", u.prefix.String(), u.measure.String())
	}
}

// Short returns the short string for the unit like 'kHz' or 'MByte'. Is is recommened to use Short() over String().
func (u *unit) Short() string {
	if u.divMeasure != InvalidMeasure {
		return fmt.Sprintf("%s%s/%s", u.prefix.Prefix(), u.measure.Short(), u.divMeasure.Short())
	} else {
		return fmt.Sprintf("%s%s", u.prefix.Prefix(), u.measure.Short())
	}
}

// AddUnitDenominator adds a unit denominator to an exising unit. Can be used if you want to derive e.g. data volume to bandwidths.
// The data volume is in a Byte unit like 'kByte' and by dividing it by the runtime in seconds, we get the bandwidth. We can use the
// data volume unit and add 'Second' as unit denominator
func (u *unit) AddUnitDenominator(div Measure) {
	u.divMeasure = div
}

func (u *unit) getPrefix() Prefix {
	return u.prefix
}

func (u *unit) setPrefix(p Prefix) {
	u.prefix = p
}

func (u *unit) getMeasure() Measure {
	return u.measure
}

func (u *unit) getUnitDenominator() Measure {
	return u.divMeasure
}

func ConvertValue(v *float64, from string, to string) {
	uf := NewUnit(from)
	ut := NewUnit(to)
	factor := float64(uf.getPrefix()) / float64(ut.getPrefix())
	*v = math.Ceil(*v * factor)
}

func ConvertSeries(s []float64, from string, to string) {
	uf := NewUnit(from)
	ut := NewUnit(to)
	factor := float64(uf.getPrefix()) / float64(ut.getPrefix())

	for i := 0; i < len(s); i++ {
		s[i] = math.Ceil(s[i] * factor)
	}
}

func getNormalizationFactor(v float64) (float64, int) {
	count := 0
	scale := -3

	if v > 1000.0 {
		for v > 1000.0 {
			v *= 1e-3
			count++
		}
	} else {
		for v < 1.0 {
			v *= 1e3
			count++
		}
		scale = 3
	}
	return math.Pow10(count * scale), count * scale
}

func NormalizeValue(v *float64, us string, nu *string) {
	u := NewUnit(us)
	f, e := getNormalizationFactor((*v))
	*v = math.Ceil(*v * f)
	u.setPrefix(NewPrefixFromFactor(u.getPrefix(), e))
	*nu = u.Short()
}

func NormalizeSeries(s []float64, avg float64, us string, nu *string) {
	u := NewUnit(us)
	f, e := getNormalizationFactor(avg)

	for i := 0; i < len(s); i++ {
		s[i] *= f
		s[i] = math.Ceil(s[i])
	}
	u.setPrefix(NewPrefixFromFactor(u.getPrefix(), e))
	fmt.Printf("Prefix: %e \n", u.getPrefix())
	*nu = u.Short()
}

func ConvertUnitString(us string) schema.Unit {
	var nu schema.Unit

	if us == "CPI" ||
		us == "IPC" ||
		us == "load" ||
		us == "" {
		nu.Base = us
		return nu
	}
	u := NewUnit(us)
	p := u.getPrefix()
	if p.Prefix() != "" {
		prefix := p.Prefix()
		nu.Prefix = &prefix
	}
	m := u.getMeasure()
	d := u.getUnitDenominator()
	if d.Short() != "inval" {
		nu.Base = fmt.Sprintf("%s/%s", m.Short(), d.Short())
	} else {
		nu.Base = m.Short()
	}

	return nu
}

// GetPrefixPrefixFactor creates the default conversion function between two prefixes.
// It returns a conversation function for the value.
func GetPrefixPrefixFactor(in Prefix, out Prefix) func(value interface{}) interface{} {
	var factor = 1.0
	var in_prefix = float64(in)
	var out_prefix = float64(out)
	factor = in_prefix / out_prefix
	conv := func(value interface{}) interface{} {
		switch v := value.(type) {
		case float64:
			return v * factor
		case float32:
			return float32(float64(v) * factor)
		case int:
			return int(float64(v) * factor)
		case int32:
			return int32(float64(v) * factor)
		case int64:
			return int64(float64(v) * factor)
		case uint:
			return uint(float64(v) * factor)
		case uint32:
			return uint32(float64(v) * factor)
		case uint64:
			return uint64(float64(v) * factor)
		}
		return value
	}
	return conv
}

// This is the conversion function between temperatures in Celsius to Fahrenheit
func convertTempC2TempF(value interface{}) interface{} {
	switch v := value.(type) {
	case float64:
		return (v * 1.8) + 32
	case float32:
		return (v * 1.8) + 32
	case int:
		return int((float64(v) * 1.8) + 32)
	case int32:
		return int32((float64(v) * 1.8) + 32)
	case int64:
		return int64((float64(v) * 1.8) + 32)
	case uint:
		return uint((float64(v) * 1.8) + 32)
	case uint32:
		return uint32((float64(v) * 1.8) + 32)
	case uint64:
		return uint64((float64(v) * 1.8) + 32)
	}
	return value
}

// This is the conversion function between temperatures in Fahrenheit to Celsius
func convertTempF2TempC(value interface{}) interface{} {
	switch v := value.(type) {
	case float64:
		return (v - 32) / 1.8
	case float32:
		return (v - 32) / 1.8
	case int:
		return int(((float64(v) - 32) / 1.8))
	case int32:
		return int32(((float64(v) - 32) / 1.8))
	case int64:
		return int64(((float64(v) - 32) / 1.8))
	case uint:
		return uint(((float64(v) - 32) / 1.8))
	case uint32:
		return uint32(((float64(v) - 32) / 1.8))
	case uint64:
		return uint64(((float64(v) - 32) / 1.8))
	}
	return value
}

// GetPrefixStringPrefixStringFactor is a wrapper for GetPrefixPrefixFactor with string inputs instead
// of prefixes. It also returns a conversation function for the value.
func GetPrefixStringPrefixStringFactor(in string, out string) func(value interface{}) interface{} {
	var i Prefix = NewPrefix(in)
	var o Prefix = NewPrefix(out)
	return GetPrefixPrefixFactor(i, o)
}

// GetUnitPrefixFactor gets the conversion function and resulting unit for a unit and a prefix. This is
// the most common case where you have some input unit and want to convert it to the same unit but with
// a different prefix. The returned unit represents the value after conversation.
func GetUnitPrefixFactor(in Unit, out Prefix) (func(value interface{}) interface{}, Unit) {
	outUnit := NewUnit(in.Short())
	if outUnit.Valid() {
		outUnit.setPrefix(out)
		conv := GetPrefixPrefixFactor(in.getPrefix(), out)
		return conv, outUnit
	}
	return nil, INVALID_UNIT
}

// GetUnitPrefixStringFactor gets the conversion function and resulting unit for a unit and a prefix as string.
// It is a wrapper for GetUnitPrefixFactor
func GetUnitPrefixStringFactor(in Unit, out string) (func(value interface{}) interface{}, Unit) {
	var o Prefix = NewPrefix(out)
	return GetUnitPrefixFactor(in, o)
}

// GetUnitStringPrefixStringFactor gets the conversion function and resulting unit for a unit and a prefix when both are only string representations.
// This is just a wrapper for GetUnitPrefixFactor with the given input unit and the desired output prefix.
func GetUnitStringPrefixStringFactor(in string, out string) (func(value interface{}) interface{}, Unit) {
	var i = NewUnit(in)
	return GetUnitPrefixStringFactor(i, out)
}

// GetUnitUnitFactor gets the conversion function and (maybe) error for unit to unit conversion.
// It is basically a wrapper for GetPrefixPrefixFactor with some special cases for temperature
// conversion between Fahrenheit and Celsius.
func GetUnitUnitFactor(in Unit, out Unit) (func(value interface{}) interface{}, error) {
	if in.getMeasure() == TemperatureC && out.getMeasure() == TemperatureF {
		return convertTempC2TempF, nil
	} else if in.getMeasure() == TemperatureF && out.getMeasure() == TemperatureC {
		return convertTempF2TempC, nil
	} else if in.getMeasure() != out.getMeasure() || in.getUnitDenominator() != out.getUnitDenominator() {
		return func(value interface{}) interface{} { return 1.0 }, fmt.Errorf("invalid measures in in and out Unit")
	}
	return GetPrefixPrefixFactor(in.getPrefix(), out.getPrefix()), nil
}

// NewUnit creates a new unit out of a string representing a unit like 'Mbyte/s' or 'GHz'.
// It uses regular expressions to detect the prefix, unit and (maybe) unit denominator.
func NewUnit(unitStr string) Unit {
	u := &unit{
		prefix:     InvalidPrefix,
		measure:    InvalidMeasure,
		divMeasure: InvalidMeasure,
	}
	matches := prefixUnitSplitRegex.FindStringSubmatch(unitStr)
	if len(matches) > 2 {
		pre := NewPrefix(matches[1])
		measures := strings.Split(matches[2], "/")
		m := NewMeasure(measures[0])
		// Special case for prefix 'p' or 'P' (Peta) and measures starting with 'p' or 'P'
		// like 'packets' or 'percent'. Same for 'e' or 'E' (Exa) for measures starting with
		// 'e' or 'E' like 'events'
		if m == InvalidMeasure {
			switch pre {
			case Peta, Exa:
				t := NewMeasure(matches[1] + measures[0])
				if t != InvalidMeasure {
					m = t
					pre = Base
				}
			}
		}
		div := InvalidMeasure
		if len(measures) > 1 {
			div = NewMeasure(measures[1])
		}

		switch m {
		// Special case for 'm' as prefix for Bytes and some others as thers is no unit like MilliBytes
		case Bytes, Flops, Packets, Events, Cycles, Requests:
			if pre == Milli {
				pre = Mega
			}
		// Special case for percentage. No/ignore prefix
		case Percentage:
			pre = Base
		}
		if pre != InvalidPrefix && m != InvalidMeasure {
			u.prefix = pre
			u.measure = m
			if div != InvalidMeasure {
				u.divMeasure = div
			}
		}
	}
	return u
}
