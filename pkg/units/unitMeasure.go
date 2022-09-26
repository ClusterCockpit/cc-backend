package units

import "regexp"

type Measure int

const (
	InvalidMeasure Measure = iota
	Bytes
	Flops
	Percentage
	TemperatureC
	TemperatureF
	Rotation
	Frequency
	Time
	Watt
	Joule
	Cycles
	Requests
	Packets
	Events
)

type MeasureData struct {
	Long  string
	Short string
	Regex string
}

// Different names and regex used for input and output
var InvalidMeasureLong string = "Invalid"
var InvalidMeasureShort string = "inval"
var MeasuresMap map[Measure]MeasureData = map[Measure]MeasureData{
	Bytes: {
		Long:  "byte",
		Short: "B",
		Regex: "^([bB][yY]?[tT]?[eE]?[sS]?)",
	},
	Flops: {
		Long:  "Flops",
		Short: "Flops",
		Regex: "^([fF][lL]?[oO]?[pP]?[sS]?)",
	},
	Percentage: {
		Long:  "Percent",
		Short: "%",
		Regex: "^(%|[pP]ercent)",
	},
	TemperatureC: {
		Long:  "DegreeC",
		Short: "degC",
		Regex: "^(deg[Cc]|°[cC])",
	},
	TemperatureF: {
		Long:  "DegreeF",
		Short: "degF",
		Regex: "^(deg[fF]|°[fF])",
	},
	Rotation: {
		Long:  "RPM",
		Short: "RPM",
		Regex: "^([rR][pP][mM])",
	},
	Frequency: {
		Long:  "Hertz",
		Short: "Hz",
		Regex: "^([hH][eE]?[rR]?[tT]?[zZ])",
	},
	Time: {
		Long:  "Seconds",
		Short: "s",
		Regex: "^([sS][eE]?[cC]?[oO]?[nN]?[dD]?[sS]?)",
	},
	Cycles: {
		Long:  "Cycles",
		Short: "cyc",
		Regex: "^([cC][yY][cC]?[lL]?[eE]?[sS]?)",
	},
	Watt: {
		Long:  "Watts",
		Short: "W",
		Regex: "^([wW][aA]?[tT]?[tT]?[sS]?)",
	},
	Joule: {
		Long:  "Joules",
		Short: "J",
		Regex: "^([jJ][oO]?[uU]?[lL]?[eE]?[sS]?)",
	},
	Requests: {
		Long:  "Requests",
		Short: "requests",
		Regex: "^([rR][eE][qQ][uU]?[eE]?[sS]?[tT]?[sS]?)",
	},
	Packets: {
		Long:  "Packets",
		Short: "packets",
		Regex: "^([pP][aA]?[cC]?[kK][eE]?[tT][sS]?)",
	},
	Events: {
		Long:  "Events",
		Short: "events",
		Regex: "^([eE][vV]?[eE]?[nN][tT][sS]?)",
	},
}

// String returns the long string for the measure like 'Percent' or 'Seconds'
func (m *Measure) String() string {
	if data, ok := MeasuresMap[*m]; ok {
		return data.Long
	}
	return InvalidMeasureLong
}

// Short returns the short string for the measure like 'B' (Bytes), 's' (Time) or 'W' (Watt). Is is recommened to use Short() over String().
func (m *Measure) Short() string {
	if data, ok := MeasuresMap[*m]; ok {
		return data.Short
	}
	return InvalidMeasureShort
}

// NewMeasure creates a new measure out of a string representing a measure like 'Bytes', 'Flops' and 'precent'.
// It uses regular expressions for matching.
func NewMeasure(unit string) Measure {
	for m, data := range MeasuresMap {
		regex := regexp.MustCompile(data.Regex)
		match := regex.FindStringSubmatch(unit)
		if match != nil {
			return m
		}
	}
	return InvalidMeasure
}
