package units

import (
	"regexp"
)

type Prefix float64

const (
	InvalidPrefix Prefix = iota
	Base                 = 1
	Yotta                = 1e24
	Zetta                = 1e21
	Exa                  = 1e18
	Peta                 = 1e15
	Tera                 = 1e12
	Giga                 = 1e9
	Mega                 = 1e6
	Kilo                 = 1e3
	Milli                = 1e-3
	Micro                = 1e-6
	Nano                 = 1e-9
	Kibi                 = 1024
	Mebi                 = 1024 * 1024
	Gibi                 = 1024 * 1024 * 1024
	Tebi                 = 1024 * 1024 * 1024 * 1024
	Pebi                 = 1024 * 1024 * 1024 * 1024 * 1024
	Exbi                 = 1024 * 1024 * 1024 * 1024 * 1024 * 1024
	Zebi                 = 1024 * 1024 * 1024 * 1024 * 1024 * 1024 * 1024
	Yobi                 = 1024 * 1024 * 1024 * 1024 * 1024 * 1024 * 1024 * 1024
)
const PrefixUnitSplitRegexStr = `^([kKmMgGtTpPeEzZyY]?[i]?)(.*)`

var prefixUnitSplitRegex = regexp.MustCompile(PrefixUnitSplitRegexStr)

type PrefixData struct {
	Long  string
	Short string
	Regex string
}

// Different names and regex used for input and output
var InvalidPrefixLong string = "Invalid"
var InvalidPrefixShort string = "inval"
var PrefixDataMap map[Prefix]PrefixData = map[Prefix]PrefixData{
	Base: {
		Long:  "",
		Short: "",
		Regex: "^$",
	},
	Kilo: {
		Long:  "Kilo",
		Short: "K",
		Regex: "^[kK]$",
	},
	Mega: {
		Long:  "Mega",
		Short: "M",
		Regex: "^[M]$",
	},
	Giga: {
		Long:  "Giga",
		Short: "G",
		Regex: "^[gG]$",
	},
	Tera: {
		Long:  "Tera",
		Short: "T",
		Regex: "^[tT]$",
	},
	Peta: {
		Long:  "Peta",
		Short: "P",
		Regex: "^[pP]$",
	},
	Exa: {
		Long:  "Exa",
		Short: "E",
		Regex: "^[eE]$",
	},
	Zetta: {
		Long:  "Zetta",
		Short: "Z",
		Regex: "^[zZ]$",
	},
	Yotta: {
		Long:  "Yotta",
		Short: "Y",
		Regex: "^[yY]$",
	},
	Milli: {
		Long:  "Milli",
		Short: "m",
		Regex: "^[m]$",
	},
	Micro: {
		Long:  "Micro",
		Short: "u",
		Regex: "^[u]$",
	},
	Nano: {
		Long:  "Nano",
		Short: "n",
		Regex: "^[n]$",
	},
	Kibi: {
		Long:  "Kibi",
		Short: "Ki",
		Regex: "^[kK][i]$",
	},
	Mebi: {
		Long:  "Mebi",
		Short: "Mi",
		Regex: "^[M][i]$",
	},
	Gibi: {
		Long:  "Gibi",
		Short: "Gi",
		Regex: "^[gG][i]$",
	},
	Tebi: {
		Long:  "Tebi",
		Short: "Ti",
		Regex: "^[tT][i]$",
	},
	Pebi: {
		Long:  "Pebi",
		Short: "Pi",
		Regex: "^[pP][i]$",
	},
	Exbi: {
		Long:  "Exbi",
		Short: "Ei",
		Regex: "^[eE][i]$",
	},
	Zebi: {
		Long:  "Zebi",
		Short: "Zi",
		Regex: "^[zZ][i]$",
	},
	Yobi: {
		Long:  "Yobi",
		Short: "Yi",
		Regex: "^[yY][i]$",
	},
}

// String returns the long string for the prefix like 'Kilo' or 'Mega'
func (p *Prefix) String() string {
	if data, ok := PrefixDataMap[*p]; ok {
		return data.Long
	}
	return InvalidMeasureLong
}

// Prefix returns the short string for the prefix like 'K', 'M' or 'G'. Is is recommened to use Prefix() over String().
func (p *Prefix) Prefix() string {
	if data, ok := PrefixDataMap[*p]; ok {
		return data.Short
	}
	return InvalidMeasureShort
}

// NewPrefix creates a new prefix out of a string representing a unit like 'k', 'K', 'M' or 'G'.
func NewPrefix(prefix string) Prefix {
	for p, data := range PrefixDataMap {
		regex := regexp.MustCompile(data.Regex)
		match := regex.FindStringSubmatch(prefix)
		if match != nil {
			return p
		}
	}
	return InvalidPrefix
}
