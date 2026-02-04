// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ClusterCockpit/cc-backend/pkg/metricstore"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"

	"github.com/influxdata/line-protocol/v2/lineprotocol"
)

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
func freeMetrics(rw http.ResponseWriter, r *http.Request) {
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

	bodyDec := json.NewDecoder(r.Body)
	var selectors [][]string
	err = bodyDec.Decode(&selectors)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	ms := metricstore.GetMemoryStore()
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
func writeMetrics(rw http.ResponseWriter, r *http.Request) {
	bytes, err := io.ReadAll(r.Body)
	rw.Header().Add("Content-Type", "application/json")
	if err != nil {
		handleError(err, http.StatusInternalServerError, rw)
		return
	}

	ms := metricstore.GetMemoryStore()
	dec := lineprotocol.NewDecoderWithBytes(bytes)
	if err := metricstore.DecodeLine(dec, ms, r.URL.Query().Get("cluster")); err != nil {
		cclog.Errorf("/api/write error: %s", err.Error())
		handleError(err, http.StatusBadRequest, rw)
		return
	}
	rw.WriteHeader(http.StatusOK)
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
func debugMetrics(rw http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("selector")
	rw.Header().Add("Content-Type", "application/json")
	selector := []string{}
	if len(raw) != 0 {
		selector = strings.Split(raw, ":")
	}

	ms := metricstore.GetMemoryStore()
	if err := ms.DebugDump(bufio.NewWriter(rw), selector); err != nil {
		handleError(err, http.StatusBadRequest, rw)
		return
	}
}
