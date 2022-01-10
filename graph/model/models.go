package model

type Cluster struct {
	Name         string          `json:"name"`
	MetricConfig []*MetricConfig `json:"metricConfig"`
	FilterRanges *FilterRanges   `json:"filterRanges"`
	Partitions   []*Partition    `json:"partitions"`

	// NOT part of the API:
	MetricDataRepository *MetricDataRepository `json:"metricDataRepository"`
}

type MetricDataRepository struct {
	Kind  string `json:"kind"`
	Url   string `json:"url"`
	Token string `json:"token"`
}
