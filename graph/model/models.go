package model

// Go look at `gqlgen.yml` and the schema package for other non-generated models.

type JobTag struct {
	ID      string `json:"id" db:"id"`
	TagType string `json:"tagType" db:"tag_type"`
	TagName string `json:"tagName" db:"tag_name"`
}

type Cluster struct {
	ClusterID            string          `json:"clusterID"`
	ProcessorType        string          `json:"processorType"`
	SocketsPerNode       int             `json:"socketsPerNode"`
	CoresPerSocket       int             `json:"coresPerSocket"`
	ThreadsPerCore       int             `json:"threadsPerCore"`
	FlopRateScalar       int             `json:"flopRateScalar"`
	FlopRateSimd         int             `json:"flopRateSimd"`
	MemoryBandwidth      int             `json:"memoryBandwidth"`
	MetricConfig         []*MetricConfig `json:"metricConfig"`
	FilterRanges         *FilterRanges   `json:"filterRanges"`
	MetricDataRepository *struct {
		Kind string `json:"kind"`
		Url  string `json:"url"`
	} `json:"metricDataRepository"`
}
