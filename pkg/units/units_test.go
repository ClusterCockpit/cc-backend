package units

import (
	"fmt"
	"regexp"
	"testing"
)

func TestUnitsExact(t *testing.T) {
	testCases := []struct {
		in   string
		want Unit
	}{
		{"b", NewUnit("Bytes")},
		{"B", NewUnit("Bytes")},
		{"byte", NewUnit("Bytes")},
		{"bytes", NewUnit("Bytes")},
		{"BYtes", NewUnit("Bytes")},
		{"Mb", NewUnit("MBytes")},
		{"MB", NewUnit("MBytes")},
		{"Mbyte", NewUnit("MBytes")},
		{"Mbytes", NewUnit("MBytes")},
		{"MbYtes", NewUnit("MBytes")},
		{"Gb", NewUnit("GBytes")},
		{"GB", NewUnit("GBytes")},
		{"Hz", NewUnit("Hertz")},
		{"MHz", NewUnit("MHertz")},
		{"GHz", NewUnit("GHertz")},
		{"pkts", NewUnit("Packets")},
		{"packets", NewUnit("Packets")},
		{"packet", NewUnit("Packets")},
		{"flop", NewUnit("Flops")},
		{"flops", NewUnit("Flops")},
		{"floPS", NewUnit("Flops")},
		{"Mflop", NewUnit("MFlops")},
		{"Gflop", NewUnit("GFlops")},
		{"gflop", NewUnit("GFlops")},
		{"%", NewUnit("Percent")},
		{"percent", NewUnit("Percent")},
		{"degc", NewUnit("degC")},
		{"degC", NewUnit("degC")},
		{"degf", NewUnit("degF")},
		{"°f", NewUnit("degF")},
		{"events", NewUnit("events")},
		{"event", NewUnit("events")},
		{"EveNts", NewUnit("events")},
		{"reqs", NewUnit("requests")},
		{"reQuEsTs", NewUnit("requests")},
		{"Requests", NewUnit("requests")},
		{"cyc", NewUnit("cycles")},
		{"cy", NewUnit("cycles")},
		{"Cycles", NewUnit("cycles")},
		{"J", NewUnit("Joules")},
		{"Joule", NewUnit("Joules")},
		{"joule", NewUnit("Joules")},
		{"W", NewUnit("Watt")},
		{"Watts", NewUnit("Watt")},
		{"watt", NewUnit("Watt")},
		{"s", NewUnit("seconds")},
		{"sec", NewUnit("seconds")},
		{"secs", NewUnit("seconds")},
		{"RPM", NewUnit("rpm")},
		{"rPm", NewUnit("rpm")},
		{"watt/byte", NewUnit("W/B")},
		{"watts/bytes", NewUnit("W/B")},
		{"flop/byte", NewUnit("flops/Bytes")},
		{"F/B", NewUnit("flops/Bytes")},
	}
	compareUnitExact := func(in, out Unit) bool {
		if in.getMeasure() == out.getMeasure() && in.getUnitDenominator() == out.getUnitDenominator() && in.getPrefix() == out.getPrefix() {
			return true
		}
		return false
	}
	for _, c := range testCases {
		u := NewUnit(c.in)
		if (!u.Valid()) || (!compareUnitExact(u, c.want)) {
			t.Errorf("func NewUnit(%q) == %q, want %q", c.in, u.String(), c.want.String())
		} else {
			t.Logf("NewUnit(%q) == %q", c.in, u.String())
		}
	}
}

func TestUnitUnitConversion(t *testing.T) {
	testCases := []struct {
		in           string
		want         Unit
		prefixFactor float64
	}{
		{"kb", NewUnit("Bytes"), 1000},
		{"Mb", NewUnit("Bytes"), 1000000},
		{"Mb/s", NewUnit("Bytes/s"), 1000000},
		{"Flops/s", NewUnit("MFlops/s"), 1e-6},
		{"Flops/s", NewUnit("GFlops/s"), 1e-9},
		{"MHz", NewUnit("Hertz"), 1e6},
		{"kb", NewUnit("Kib"), 1000.0 / 1024},
		{"Mib", NewUnit("MBytes"), (1024 * 1024.0) / (1e6)},
		{"mb", NewUnit("MBytes"), 1.0},
	}
	compareUnitWithPrefix := func(in, out Unit, factor float64) bool {
		if in.getMeasure() == out.getMeasure() && in.getUnitDenominator() == out.getUnitDenominator() {
			if f := GetPrefixPrefixFactor(in.getPrefix(), out.getPrefix()); f(1.0) == factor {
				return true
			} else {
				fmt.Println(f(1.0))
			}
		}
		return false
	}
	for _, c := range testCases {
		u := NewUnit(c.in)
		if (!u.Valid()) || (!compareUnitWithPrefix(u, c.want, c.prefixFactor)) {
			t.Errorf("GetPrefixPrefixFactor(%q, %q) invalid, want %q with factor %g", c.in, u.String(), c.want.String(), c.prefixFactor)
		} else {
			t.Logf("GetPrefixPrefixFactor(%q, %q) = %g", c.in, c.want.String(), c.prefixFactor)
		}
	}
}

func TestUnitPrefixConversion(t *testing.T) {
	testCases := []struct {
		in           string
		want         string
		prefixFactor float64
		wantUnit     Unit
	}{
		{"KBytes", "", 1000, NewUnit("Bytes")},
		{"MBytes", "", 1e6, NewUnit("Bytes")},
		{"MBytes", "G", 1e-3, NewUnit("GBytes")},
		{"mb", "M", 1, NewUnit("MBytes")},
	}
	compareUnitPrefix := func(in Unit, out Prefix, factor float64, outUnit Unit) bool {
		if in.Valid() {
			conv, unit := GetUnitPrefixFactor(in, out)
			value := conv(1.0)
			if value == factor && unit.String() == outUnit.String() {
				return true
			}
		}
		return false
	}
	for _, c := range testCases {
		u := NewUnit(c.in)
		p := NewPrefix(c.want)
		if (!u.Valid()) || (!compareUnitPrefix(u, p, c.prefixFactor, c.wantUnit)) {
			t.Errorf("GetUnitPrefixFactor(%q, %q) invalid, want %q with factor %g", c.in, p.Prefix(), c.wantUnit.String(), c.prefixFactor)
		} else {
			t.Logf("GetUnitPrefixFactor(%q, %q) = %g", c.in, c.wantUnit.String(), c.prefixFactor)
		}
	}
}

func TestPrefixPrefixConversion(t *testing.T) {
	testCases := []struct {
		in           string
		want         string
		prefixFactor float64
	}{
		{"K", "", 1000},
		{"M", "", 1e6},
		{"M", "G", 1e-3},
		{"", "M", 1e-6},
		{"", "m", 1e3},
		{"m", "n", 1e6},
		//{"", "n", 1e9}, //does not work because of IEEE rounding problems
	}
	for _, c := range testCases {
		i := NewPrefix(c.in)
		o := NewPrefix(c.want)
		if i != InvalidPrefix && o != InvalidPrefix {
			conv := GetPrefixPrefixFactor(i, o)
			value := conv(1.0)
			if value != c.prefixFactor {
				t.Errorf("GetPrefixPrefixFactor(%q, %q) invalid, want %q with factor %g but got %g", c.in, c.want, o.Prefix(), c.prefixFactor, value)
			} else {
				t.Logf("GetPrefixPrefixFactor(%q, %q) = %g", c.in, c.want, c.prefixFactor)
			}
		}
	}
}

func TestMeasureRegex(t *testing.T) {
	for _, data := range MeasuresMap {
		_, err := regexp.Compile(data.Regex)
		if err != nil {
			t.Errorf("failed to compile regex '%s': %s", data.Regex, err.Error())
		}
		t.Logf("succussfully compiled regex '%s' for measure %s", data.Regex, data.Long)
	}
}

func TestPrefixRegex(t *testing.T) {
	for _, data := range PrefixDataMap {
		_, err := regexp.Compile(data.Regex)
		if err != nil {
			t.Errorf("failed to compile regex '%s': %s", data.Regex, err.Error())
		}
		t.Logf("succussfully compiled regex '%s' for prefix %s", data.Regex, data.Long)
	}
}
