// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"fmt"
	"time"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

type HeathCheckResponse struct {
	Status schema.MonitoringState
	Error  error
	list   List
}

type List struct {
	StaleNodeMetricList       []string
	StaleHardwareMetricList   map[string][]string
	MissingNodeMetricList     []string
	MissingHardwareMetricList map[string][]string
}

// MaxMissingDataPoints is a threshold that allows a node to be healthy with certain number of data points missing.
// Suppose a node does not receive last 5 data points, then healthCheck endpoint will still say a
// node is healthy. Anything more than 5 missing points in metrics of the node will deem the node unhealthy.
const MaxMissingDataPoints int64 = 5

func (b *buffer) healthCheck() bool {
	// Check if the buffer is empty
	if b.data == nil {
		return true
	}

	bufferEnd := b.start + b.frequency*int64(len(b.data))
	t := time.Now().Unix()

	// Check if the buffer is too old
	if t-bufferEnd > MaxMissingDataPoints*b.frequency {
		return true
	}

	return false
}

func (l *Level) healthCheck(m *MemoryStore) (List, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	list := List{
		StaleNodeMetricList:       make([]string, 0),
		StaleHardwareMetricList:   make(map[string][]string, 0),
		MissingNodeMetricList:     make([]string, 0),
		MissingHardwareMetricList: make(map[string][]string, 0),
	}

	for metricName, mc := range m.Metrics {
		if b := l.metrics[mc.offset]; b != nil {
			if b.healthCheck() {
				list.StaleNodeMetricList = append(list.StaleNodeMetricList, metricName)
			}
		} else {
			list.MissingNodeMetricList = append(list.MissingNodeMetricList, metricName)
		}
	}

	for hardwareMetricName, lvl := range l.children {
		l, err := lvl.healthCheck(m)
		if err != nil {
			return List{}, err
		}

		if len(l.StaleNodeMetricList) != 0 {
			list.StaleHardwareMetricList[hardwareMetricName] = l.StaleNodeMetricList
		}
		if len(l.MissingNodeMetricList) != 0 {
			list.MissingHardwareMetricList[hardwareMetricName] = l.MissingNodeMetricList
		}
	}

	return list, nil
}

func (m *MemoryStore) HealthCheck(selector []string, subcluster string) (*HeathCheckResponse, error) {
	response := HeathCheckResponse{
		Status: schema.MonitoringStateFull,
	}

	lvl := m.root.findLevel(selector)
	if lvl == nil {
		response.Status = schema.MonitoringStateFailed
		response.Error = fmt.Errorf("[METRICSTORE]> error while HealthCheck, host not found: %#v", selector)
		return &response, nil
	}

	var err error

	response.list, err = lvl.healthCheck(m)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Response: %#v\n", response)

	if len(response.list.StaleNodeMetricList) != 0 ||
		len(response.list.StaleHardwareMetricList) != 0 {
		response.Status = schema.MonitoringStatePartial
		return &response, nil
	}

	if len(response.list.MissingHardwareMetricList) != 0 ||
		len(response.list.MissingNodeMetricList) != 0 {
		response.Status = schema.MonitoringStateFailed
		return &response, nil
	}

	return &response, nil
}
