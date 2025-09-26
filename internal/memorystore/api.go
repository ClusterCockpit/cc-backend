// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package memorystore

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/ClusterCockpit/cc-lib/util"

	"github.com/influxdata/line-protocol/v2/lineprotocol"
)

// @title                      cc-metric-store REST API
// @version                    1.0.0
// @description                API for cc-metric-store

// @contact.name               ClusterCockpit Project
// @contact.url                https://clustercockpit.org
// @contact.email              support@clustercockpit.org

// @license.name               MIT License
// @license.url                https://opensource.org/licenses/MIT

// @host                       localhost:8082
// @basePath                   /api/

// @securityDefinitions.apikey ApiKeyAuth
// @in                         header
// @name                       X-Auth-Token

// ErrorResponse model
type ErrorResponse struct {
	// Statustext of Errorcode
	Status string `json:"status"`
	Error  string `json:"error"` // Error Message
}

type ApiMetricData struct {
	Error      *string           `json:"error,omitempty"`
	Data       schema.FloatArray `json:"data,omitempty"`
	From       int64             `json:"from"`
	To         int64             `json:"to"`
	Resolution int64             `json:"resolution"`
	Avg        schema.Float      `json:"avg"`
	Min        schema.Float      `json:"min"`
	Max        schema.Float      `json:"max"`
}

func handleError(err error, statusCode int, rw http.ResponseWriter) {
	// log.Warnf("REST ERROR : %s", err.Error())
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(statusCode)
	json.NewEncoder(rw).Encode(ErrorResponse{
		Status: http.StatusText(statusCode),
		Error:  err.Error(),
	})
}

// TODO: Optimize this, just like the stats endpoint!
func (data *ApiMetricData) AddStats() {
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

func (data *ApiMetricData) ScaleBy(f schema.Float) {
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

func (data *ApiMetricData) PadDataWithNull(ms *MemoryStore, from, to int64, metric string) {
	minfo, ok := ms.Metrics[metric]
	if !ok {
		return
	}

	if (data.From / minfo.Frequency) > (from / minfo.Frequency) {
		padfront := int((data.From / minfo.Frequency) - (from / minfo.Frequency))
		ndata := make([]schema.Float, 0, padfront+len(data.Data))
		for i := 0; i < padfront; i++ {
			ndata = append(ndata, schema.NaN)
		}
		for j := 0; j < len(data.Data); j++ {
			ndata = append(ndata, data.Data[j])
		}
		data.Data = ndata
	}
}

// handleFree godoc
// @summary
// @tags free
// @description This endpoint allows the users to free the Buffers from the
// metric store. This endpoint offers the users to remove then systematically
// and also allows then to prune the data under node, if they do not want to
// remove the whole node.
// @produce     json
// @param       to        query    string        false  "up to timestamp"
// @success     200            {string} string  "ok"
// @failure     400            {object} api.ErrorResponse       "Bad Request"
// @failure     401            {object} api.ErrorResponse       "Unauthorized"
// @failure     403            {object} api.ErrorResponse       "Forbidden"
// @failure     500            {object} api.ErrorResponse       "Internal Server Error"
// @security    ApiKeyAuth
// @router      /free/ [post]
func HandleFree(rw http.ResponseWriter, r *http.Request) {
	rawTo := r.URL.Query().Get("to")
	if rawTo == "" {
		handleError(errors.New("'to' is a required query parameter"), http.StatusBadRequest, rw)
		return
	}

	to, err := strconv.ParseInt(rawTo, 10, 64)
	if err != nil {
		handleError(err, http.StatusInternalServerError, rw)
		return
	}

	// // TODO: lastCheckpoint might be modified by different go-routines.
	// // Load it using the sync/atomic package?
	// freeUpTo := lastCheckpoint.Unix()
	// if to < freeUpTo {
	// 	freeUpTo = to
	// }

	bodyDec := json.NewDecoder(r.Body)
	var selectors [][]string
	err = bodyDec.Decode(&selectors)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	ms := GetMemoryStore()
	n := 0
	for _, sel := range selectors {
		bn, err := ms.Free(sel, to)
		if err != nil {
			handleError(err, http.StatusInternalServerError, rw)
			return
		}

		n += bn
	}

	rw.WriteHeader(http.StatusOK)
	fmt.Fprintf(rw, "buffers freed: %d\n", n)
}

// handleWrite godoc
// @summary Receive metrics in InfluxDB line-protocol
// @tags write
// @description Write data to the in-memory store in the InfluxDB line-protocol using [this format](https://github.com/ClusterCockpit/cc-specifications/blob/master/metrics/lineprotocol_alternative.md)

// @accept      plain
// @produce     json
// @param       cluster        query string false "If the lines in the body do not have a cluster tag, use this value instead."
// @success     200            {string} string  "ok"
// @failure     400            {object} api.ErrorResponse       "Bad Request"
// @failure     401            {object} api.ErrorResponse       "Unauthorized"
// @failure     403            {object} api.ErrorResponse       "Forbidden"
// @failure     500            {object} api.ErrorResponse       "Internal Server Error"
// @security    ApiKeyAuth
// @router      /write/ [post]
func HandleWrite(rw http.ResponseWriter, r *http.Request) {
	bytes, err := io.ReadAll(r.Body)
	rw.Header().Add("Content-Type", "application/json")
	if err != nil {
		handleError(err, http.StatusInternalServerError, rw)
		return
	}

	ms := GetMemoryStore()
	dec := lineprotocol.NewDecoderWithBytes(bytes)
	if err := decodeLine(dec, ms, r.URL.Query().Get("cluster")); err != nil {
		log.Printf("/api/write error: %s", err.Error())
		handleError(err, http.StatusBadRequest, rw)
		return
	}
	rw.WriteHeader(http.StatusOK)
}

type ApiQueryRequest struct {
	Cluster     string     `json:"cluster"`
	Queries     []ApiQuery `json:"queries"`
	ForAllNodes []string   `json:"for-all-nodes"`
	From        int64      `json:"from"`
	To          int64      `json:"to"`
	WithStats   bool       `json:"with-stats"`
	WithData    bool       `json:"with-data"`
	WithPadding bool       `json:"with-padding"`
}

type ApiQueryResponse struct {
	Queries []ApiQuery        `json:"queries,omitempty"`
	Results [][]ApiMetricData `json:"results"`
}

type ApiQuery struct {
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

func FetchData(req ApiQueryRequest) (*ApiQueryResponse, error) {

	req.WithData = true
	req.WithData = true
	req.WithData = true

	ms := GetMemoryStore()

	response := ApiQueryResponse{
		Results: make([][]ApiMetricData, 0, len(req.Queries)),
	}
	if req.ForAllNodes != nil {
		nodes := ms.ListChildren([]string{req.Cluster})
		for _, node := range nodes {
			for _, metric := range req.ForAllNodes {
				q := ApiQuery{
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
			for _, typeId := range query.TypeIds {
				if query.SubType != nil {
					for _, subTypeId := range query.SubTypeIds {
						sels = append(sels, util.Selector{
							{String: req.Cluster},
							{String: query.Hostname},
							{String: *query.Type + typeId},
							{String: *query.SubType + subTypeId},
						})
					}
				} else {
					sels = append(sels, util.Selector{
						{String: req.Cluster},
						{String: query.Hostname},
						{String: *query.Type + typeId},
					})
				}
			}
		}

		// log.Printf("query: %#v\n", query)
		// log.Printf("sels: %#v\n", sels)
		var err error
		res := make([]ApiMetricData, 0, len(sels))
		for _, sel := range sels {
			data := ApiMetricData{}

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

// handleDebug godoc
// @summary Debug endpoint
// @tags debug
// @description This endpoint allows the users to print the content of
// nodes/clusters/metrics to review the state of the data.
// @produce     json
// @param       selector        query    string            false "Selector"
// @success     200            {string} string  "Debug dump"
// @failure     400            {object} api.ErrorResponse       "Bad Request"
// @failure     401            {object} api.ErrorResponse       "Unauthorized"
// @failure     403            {object} api.ErrorResponse       "Forbidden"
// @failure     500            {object} api.ErrorResponse       "Internal Server Error"
// @security    ApiKeyAuth
// @router      /debug/ [post]
func HandleDebug(rw http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("selector")
	rw.Header().Add("Content-Type", "application/json")
	selector := []string{}
	if len(raw) != 0 {
		selector = strings.Split(raw, ":")
	}

	ms := GetMemoryStore()
	if err := ms.DebugDump(bufio.NewWriter(rw), selector); err != nil {
		handleError(err, http.StatusBadRequest, rw)
		return
	}
}

// handleHealthCheck godoc
// @summary HealthCheck endpoint
// @tags healthcheck
// @description This endpoint allows the users to check if a node is healthy
// @produce     json
// @param       selector        query    string            false "Selector"
// @success     200            {string} string  "Debug dump"
// @failure     400            {object} api.ErrorResponse       "Bad Request"
// @failure     401            {object} api.ErrorResponse       "Unauthorized"
// @failure     403            {object} api.ErrorResponse       "Forbidden"
// @failure     500            {object} api.ErrorResponse       "Internal Server Error"
// @security    ApiKeyAuth
// @router      /healthcheck/ [get]
func HandleHealthCheck(rw http.ResponseWriter, r *http.Request) {
	rawCluster := r.URL.Query().Get("cluster")
	rawNode := r.URL.Query().Get("node")

	if rawCluster == "" || rawNode == "" {
		handleError(errors.New("'cluster' and 'node' are required query parameter"), http.StatusBadRequest, rw)
		return
	}

	rw.Header().Add("Content-Type", "application/json")

	selector := []string{rawCluster, rawNode}

	ms := GetMemoryStore()
	if err := ms.HealthCheck(bufio.NewWriter(rw), selector); err != nil {
		handleError(err, http.StatusBadRequest, rw)
		return
	}
}
