# cc-units - A unit system for ClusterCockpit

When working with metrics, the problem comes up that they may use different unit name but have the same unit in fact. There are a lot of real world examples like 'kB' and 'Kbyte'. In [cc-metric-collector](https://github.com/ClusterCockpit/cc-metric-collector), the collectors read data from different sources which may use different units or the programmer specifies a unit for a metric by hand. The cc-units system is not comparable with the SI unit system. If you are looking for a package for the SI units, see [here](https://pkg.go.dev/github.com/gurre/si).

In order to enable unit comparison and conversion, the ccUnits package provides some helpers:
```go
NewUnit(unit string) Unit // create a new unit from some string like 'GHz', 'Mbyte' or 'kevents/s'
func GetUnitUnitFactor(in Unit, out Unit) (func(value float64) float64, error) // Get conversion function between two units
func GetPrefixFactor(in Prefix, out Prefix) func(value float64) float64 // Get conversion function between two prefixes
func GetUnitPrefixFactor(in Unit, out Prefix) (func(value float64) float64, Unit) // Get conversion function for prefix changes and the new unit for further use

type Unit interface {
	Valid() bool
	String() string
	Short() string
	AddUnitDenominator(div Measure)
}
```

In order to get the "normalized" string unit back or test for validity, you can use:
```go
u := NewUnit("MB")
fmt.Println(u.Valid())                   // true
fmt.Printf("Long string %q", u.String()) // MegaBytes
fmt.Printf("Short string %q", u.Short()) // MBytes
v := NewUnit("foo")
fmt.Println(v.Valid())                   // false
```

If you have two units or other components and need the conversion function:
```go
// Get conversion functions for 'kB' to 'MBytes'
u1 := NewUnit("kB")
u2 := NewUnit("MBytes")
convFunc, err := GetUnitUnitFactor(u1, u2) // Returns an error if the units have different measures
if err == nil {
    v2 := convFunc(v1)
	fmt.Printf("%f %s\n", v2, u2.Short())
}
// Get conversion function for 'kB' -> 'G' prefix.
// Returns the function and the new unit 'GBytes'
p1 := NewPrefix("G")
convFunc, u_p1 := GetUnitPrefixFactor(u1, p1)
// or
// convFunc, u_p1 := GetUnitPrefixStringFactor(u1, "G")
if convFunc != nil {
	v2 := convFunc(v1)
	fmt.Printf("%f %s\n", v2, u_p1.Short())
}
// Get conversion function for two prefixes: 'G' -> 'T'
p2 := NewPrefix("T")
convFunc = GetPrefixPrefixFactor(p1, p2)
if convFunc != nil {
	v2 := convFunc(v1)
	fmt.Printf("%f %s -> %f %s\n", v1, p1.Prefix(), v2, p2.Prefix())
}


```

(In the ClusterCockpit ecosystem the separation between values and units if useful since they are commonly not stored as a single entity but the value is a field in the CCMetric while unit is a tag or a meta information).

If you have a metric and want the derivation to a bandwidth or events per second, you can use the original unit:

```go
in_unit, err := metric.GetMeta("unit")
if err == nil {
    value, ok := metric.GetField("value")
    if ok {
        out_unit = NewUnit(in_unit)
        out_unit.AddUnitDenominator("seconds")
		seconds := timeDiff.Seconds()
        y, err := lp.New(metric.Name()+"_bw",
                         metric.Tags(),
                         metric.Meta(),
                         map[string]interface{"value": value/seconds},
                         metric.Time())
        if err == nil {
            y.AddMeta("unit", out_unit.Short())
        }
    }
}
```

## Special unit detection

Some used measures like Bytes and Flops are non-dividable. Consequently there prefixes like Milli, Micro and Nano are not useful. This is quite handy since a unit `mb` for `MBytes` is not uncommon but would by default be parsed as "MilliBytes".

Special parsing rules for the following measures: iff `prefix==Milli`, use `prefix==Mega`
  - `Bytes`
  - `Flops`
  - `Packets`
  - `Events`
  - `Cycles`
  - `Requests`

This means the prefixes `Micro` (like `ubytes`) and `Nano` like (`nflops/sec`) are not allowed and return an invalid unit. But you can specify `mflops` and `mb`.

Prefixes for `%` or `percent` are ignored.

## Supported prefixes

```go
const (
	Base  Prefix = 1
	Exa          = 1e18
	Peta         = 1e15
	Tera         = 1e12
	Giga         = 1e9
	Mega         = 1e6
	Kilo         = 1e3
	Milli        = 1e-3
	Micro        = 1e-6
	Nano         = 1e-9
	Kibi         = 1024
	Mebi         = 1024 * 1024
	Gibi         = 1024 * 1024 * 1024
	Tebi         = 1024 * 1024 * 1024 * 1024
)
```

The prefixes are detected using a regular expression `^([kKmMgGtTpP]?[i]?)(.*)` that splits the prefix from the measure. You probably don't need to deal with the prefixes in the code.

## Supported measures

```go
const (
	None Measure = iota
	Bytes
	Flops
	Percentage
	TemperatureC
	TemperatureF
	Rotation
	Hertz
	Time
	Watt
	Joule
	Cycles
	Requests
	Packets
	Events
)
```

There a regular expression for each of the measures like `^([bB][yY]?[tT]?[eE]?[sS]?)` for the `Bytes` measure. 


## New units

If the selected units are not suitable for your metric, feel free to send a PR.

### New prefix

For a new prefix, add it to the big `const` in `ccUnitPrefix.go` and adjust the prefix-unit-splitting regular expression. Afterwards, you have to add cases to the three functions `String()`, `Prefix()` and `NewPrefix()`. `NewPrefix()` contains the parser (`k` or `K` -> `Kilo`). The other one are used for output. `String()` outputs a longer version of the prefix (`Kilo`), while `Prefix()` returns only the short notation (`K`).

### New measure

Adding new prefixes is probably rare but adding a new measure is a more common task. At first, add it to the big `const` in `ccUnitMeasure.go`. Moreover, create a regular expression matching the measure (and pre-compile it like the others). Add the expression matching to `NewMeasure()`. The `String()` and `Short()` functions return descriptive strings for the measure in long form (like `Hertz`) and short form (like `Hz`).

If there are special conversation rules between measures and you want to convert one measure to another, like temperatures in Celsius to Fahrenheit, a special case in `GetUnitPrefixFactor()` is required.

### Special parsing rules

The two parsers for prefix and measure are called under the hood by `NewUnit()` and there might some special rules apply. Like in the above section about 'special unit detection', special rules for your new measure might be required. Currently there are two special cases:

- Measures that are non-dividable like Flops, Bytes, Events, ... cannot use `Milli`, `Micro` and `Nano`. The prefix `m` is forced to `M` for these measures
- If the prefix is `p`/`P` (`Peta`) or `e`/`E` (`Exa`) and the measure is not detectable, it retries detection with the prefix. So first round it tries, for example, prefix `p` and measure `ackets` which fails, so it retries the detection with measure `packets` and `<empty>` prefix (resolves to `Base` prefix).

## Limitations

The `ccUnits` package is a simple implemtation of a unit system and comes with some limitations:

- The unit denominator (like `s` in `Mbyte/s`) can only have the `Base` prefix, you cannot specify `Byte/ms` for "Bytes per milli second".
