// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// This file contains the API types and data fetching logic for querying metric data
// from the in-memory metric store. It provides structures for building complex queries
// with support for aggregation, scaling, padding, and statistics computation.
package metricstore

import (
	"errors"
	"fmt"
	"math"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/ClusterCockpit/cc-lib/v2/util"
)

var (
	// ErrInvalidTimeRange is returned when a query has 'from' >= 'to'
	ErrInvalidTimeRange = errors.New("[METRICSTORE]> invalid time range: 'from' must be before 'to'")
	// ErrEmptyCluster is returned when a query with ForAllNodes has no cluster specified
	ErrEmptyCluster = errors.New("[METRICSTORE]> cluster name cannot be empty")
)

// APIMetricData represents the response data for a single metric query.
//
// It contains both the time-series data points and computed statistics (avg, min, max).
// If an error occurred during data retrieval, the Error field will be set and other
// fields may be incomplete.
type APIMetricData struct {
	Error      *string           `json:"error,omitempty"`
	Data       schema.FloatArray `json:"data,omitempty"`
	From       int64             `json:"from"`
	To         int64             `json:"to"`
	Resolution int64             `json:"resolution"`
	Avg        schema.Float      `json:"avg"`
	Min        schema.Float      `json:"min"`
	Max        schema.Float      `json:"max"`
}

// APIQueryRequest represents a batch query request for metric data.
//
// It supports two modes of operation:
//  1. Explicit queries via the Queries field
//  2. Automatic query generation via ForAllNodes (queries all specified metrics for all nodes in the cluster)
//
// The request can be customized with flags to include/exclude statistics, raw data, and padding.
type APIQueryRequest struct {
	Cluster     string     `json:"cluster"`
	Queries     []APIQuery `json:"queries"`
	ForAllNodes []string   `json:"for-all-nodes"`
	From        int64      `json:"from"`
	To          int64      `json:"to"`
	WithStats   bool       `json:"with-stats"`
	WithData    bool       `json:"with-data"`
	WithPadding bool       `json:"with-padding"`
}

// APIQueryResponse represents the response to an APIQueryRequest.
//
// Results is a 2D array where each outer element corresponds to a query,
// and each inner element corresponds to a selector within that query
// (e.g., multiple CPUs or cores).
type APIQueryResponse struct {
	Queries []APIQuery        `json:"queries,omitempty"`
	Results [][]APIMetricData `json:"results"`
}

// APIQuery represents a single metric query with optional hierarchical selectors.
//
// The hierarchical selection works as follows:
//   - Hostname: The node to query
//   - Type + TypeIds: First level of hierarchy (e.g., "cpu" + ["0", "1", "2"])
//   - SubType + SubTypeIds: Second level of hierarchy (e.g., "core" + ["0", "1"])
//
// If Aggregate is true, data from multiple type/subtype IDs will be aggregated according
// to the metric's aggregation strategy. Otherwise, separate results are returned for each combination.
type APIQuery struct {
	Type        *string      `json:"type,omitempty"`
	SubType     *string      `json:"subtype,omitempty"`
	Metric      string       `json:"metric"`
	Hostname    string       `json:"host"`
	Resolution  int64        `json:"resolution"`
	TypeIds     []string     `json:"type-ids,omitempty"`
	SubTypeIds  []string     `json:"subtype-ids,omitempty"`
	ScaleFactor schema.Float `json:"scale-by,omitempty"`
	Aggregate   bool         `json:"aggreg"`
}

// AddStats computes and populates the Avg, Min, and Max fields from the Data array.
//
// NaN values in the data are ignored during computation. If all values are NaN,
// the statistics fields will be set to NaN.
//
// TODO: Optimize this, just like the stats endpoint!
func (data *APIMetricData) AddStats() {
	n := 0
	sum, min, max := 0.0, math.MaxFloat64, -math.MaxFloat64
	for _, x := range data.Data {
		if x.IsNaN() {
			continue
		}

		n += 1
		sum += float64(x)
		min = math.Min(min, float64(x))
		max = math.Max(max, float64(x))
	}

	if n > 0 {
		avg := sum / float64(n)
		data.Avg = schema.Float(avg)
		data.Min = schema.Float(min)
		data.Max = schema.Float(max)
	} else {
		data.Avg, data.Min, data.Max = schema.NaN, schema.NaN, schema.NaN
	}
}

// ScaleBy multiplies all data points and statistics by the given factor.
//
// This is commonly used for unit conversion (e.g., bytes to gigabytes).
// Scaling by 0 or 1 is a no-op for performance reasons.
func (data *APIMetricData) ScaleBy(f schema.Float) {
	if f == 0 || f == 1 {
		return
	}

	data.Avg *= f
	data.Min *= f
	data.Max *= f
	for i := 0; i < len(data.Data); i++ {
		data.Data[i] *= f
	}
}

// PadDataWithNull pads the beginning of the data array with NaN values if needed.
//
// This ensures that the data aligns with the requested 'from' timestamp, even if
// the metric store doesn't have data for the earliest time points. This is useful
// for maintaining consistent array indexing across multiple queries.
//
// Parameters:
//   - ms: MemoryStore instance to lookup metric configuration
//   - from: The requested start timestamp
//   - to: The requested end timestamp (unused but kept for API consistency)
//   - metric: The metric name to lookup frequency information
func (data *APIMetricData) PadDataWithNull(ms *MemoryStore, from, to int64, metric string) {
	minfo, ok := ms.Metrics[metric]
	if !ok {
		return
	}

	if (data.From / minfo.Frequency) > (from / minfo.Frequency) {
		padfront := int((data.From / minfo.Frequency) - (from / minfo.Frequency))
		ndata := make([]schema.Float, 0, padfront+len(data.Data))
		for range padfront {
			ndata = append(ndata, schema.NaN)
		}
		for j := 0; j < len(data.Data); j++ {
			ndata = append(ndata, data.Data[j])
		}
		data.Data = ndata
	}
}

// FetchData executes a batch metric query request and returns the results.
//
// This is the primary API for retrieving metric data from the memory store. It supports:
//   - Individual queries via req.Queries
//   - Batch queries for all nodes via req.ForAllNodes
//   - Hierarchical selector construction (cluster → host → type → subtype)
//   - Optional statistics computation (avg, min, max)
//   - Optional data scaling
//   - Optional data padding with NaN values
//
// The function constructs selectors based on the query parameters and calls MemoryStore.Read()
// for each selector. If a query specifies Aggregate=false with multiple type/subtype IDs,
// separate results are returned for each combination.
//
// Parameters:
//   - req: The query request containing queries, time range, and options
//
// Returns:
//   - APIQueryResponse containing results for each query, or error if validation fails
//
// Errors:
//   - ErrInvalidTimeRange if req.From > req.To
//   - ErrEmptyCluster if req.ForAllNodes is used without specifying a cluster
//   - Error if MemoryStore is not initialized
//   - Individual query errors are stored in APIMetricData.Error field
func FetchData(req APIQueryRequest) (*APIQueryResponse, error) {
	if req.From > req.To {
		return nil, ErrInvalidTimeRange
	}
	if req.Cluster == "" && req.ForAllNodes != nil {
		return nil, ErrEmptyCluster
	}

	req.WithData = true
	ms := GetMemoryStore()
	if ms == nil {
		return nil, fmt.Errorf("[METRICSTORE]> memorystore not initialized")
	}

	response := APIQueryResponse{
		Results: make([][]APIMetricData, 0, len(req.Queries)),
	}
	if req.ForAllNodes != nil {
		nodes := ms.ListChildren([]string{req.Cluster})
		for _, node := range nodes {
			for _, metric := range req.ForAllNodes {
				q := APIQuery{
					Metric:   metric,
					Hostname: node,
				}
				req.Queries = append(req.Queries, q)
				response.Queries = append(response.Queries, q)
			}
		}
	}

	for _, query := range req.Queries {
		sels := make([]util.Selector, 0, 1)
		if query.Aggregate || query.Type == nil {
			sel := util.Selector{{String: req.Cluster}, {String: query.Hostname}}
			if query.Type != nil {
				if len(query.TypeIds) == 1 {
					sel = append(sel, util.SelectorElement{String: *query.Type + query.TypeIds[0]})
				} else {
					ids := make([]string, len(query.TypeIds))
					for i, id := range query.TypeIds {
						ids[i] = *query.Type + id
					}
					sel = append(sel, util.SelectorElement{Group: ids})
				}

				if query.SubType != nil {
					if len(query.SubTypeIds) == 1 {
						sel = append(sel, util.SelectorElement{String: *query.SubType + query.SubTypeIds[0]})
					} else {
						ids := make([]string, len(query.SubTypeIds))
						for i, id := range query.SubTypeIds {
							ids[i] = *query.SubType + id
						}
						sel = append(sel, util.SelectorElement{Group: ids})
					}
				}
			}
			sels = append(sels, sel)
		} else {
			for _, typeID := range query.TypeIds {
				if query.SubType != nil {
					for _, subTypeID := range query.SubTypeIds {
						sels = append(sels, util.Selector{
							{String: req.Cluster},
							{String: query.Hostname},
							{String: *query.Type + typeID},
							{String: *query.SubType + subTypeID},
						})
					}
				} else {
					sels = append(sels, util.Selector{
						{String: req.Cluster},
						{String: query.Hostname},
						{String: *query.Type + typeID},
					})
				}
			}
		}

		var err error
		res := make([]APIMetricData, 0, len(sels))
		for _, sel := range sels {
			data := APIMetricData{}

			data.Data, data.From, data.To, data.Resolution, err = ms.Read(sel, query.Metric, req.From, req.To, query.Resolution)
			if err != nil {
				msg := err.Error()
				data.Error = &msg
				res = append(res, data)
				continue
			}

			if req.WithStats {
				data.AddStats()
			}
			if query.ScaleFactor != 0 {
				data.ScaleBy(query.ScaleFactor)
			}
			if req.WithPadding {
				data.PadDataWithNull(ms, req.From, req.To, query.Metric)
			}
			if !req.WithData {
				data.Data = nil
			}
			res = append(res, data)
		}
		response.Results = append(response.Results, res)
	}

	return &response, nil
}
