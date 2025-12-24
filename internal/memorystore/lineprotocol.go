// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package memorystore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/nats"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/influxdata/line-protocol/v2/lineprotocol"
)

func ReceiveNats(ms *MemoryStore,
	workers int,
	ctx context.Context,
) error {
	nc := nats.GetClient()

	if nc == nil {
		cclog.Warn("NATS client not initialized")
		return nil
	}

	var wg sync.WaitGroup

	msgs := make(chan []byte, workers*2)

	for _, sc := range Keys.Subscriptions {
		clusterTag := sc.ClusterTag
		if workers > 1 {
			wg.Add(workers)

			for range workers {
				go func() {
					for m := range msgs {
						dec := lineprotocol.NewDecoderWithBytes(m)
						if err := DecodeLine(dec, ms, clusterTag); err != nil {
							cclog.Errorf("error: %s", err.Error())
						}
					}

					wg.Done()
				}()
			}

			nc.Subscribe(sc.SubscribeTo, func(subject string, data []byte) {
				msgs <- data
			})
		} else {
			nc.Subscribe(sc.SubscribeTo, func(subject string, data []byte) {
				dec := lineprotocol.NewDecoderWithBytes(data)
				if err := DecodeLine(dec, ms, clusterTag); err != nil {
					cclog.Errorf("error: %s", err.Error())
				}
			})
		}
		cclog.Infof("NATS subscription to '%s' established", sc.SubscribeTo)
	}

	close(msgs)
	wg.Wait()

	return nil
}

// Place `prefix` in front of `buf` but if possible,
// do that inplace in `buf`.
func reorder(buf, prefix []byte) []byte {
	n := len(prefix)
	m := len(buf)
	if cap(buf) < m+n {
		return append(prefix[:n:n], buf...)
	} else {
		buf = buf[:n+m]
		for i := m - 1; i >= 0; i-- {
			buf[i+n] = buf[i]
		}
		for i := range n {
			buf[i] = prefix[i]
		}
		return buf
	}
}

// Decode lines using dec and make write calls to the MemoryStore.
// If a line is missing its cluster tag, use clusterDefault as default.
func DecodeLine(dec *lineprotocol.Decoder,
	ms *MemoryStore,
	clusterDefault string,
) error {
	// Reduce allocations in loop:
	t := time.Now()
	metric, metricBuf := Metric{}, make([]byte, 0, 16)
	selector := make([]string, 0, 4)
	typeBuf, subTypeBuf := make([]byte, 0, 16), make([]byte, 0)

	// Optimize for the case where all lines in a "batch" are about the same
	// cluster and host. By using `WriteToLevel` (level = host), we do not need
	// to take the root- and cluster-level lock as often.
	var lvl *Level = nil
	prevCluster, prevHost := "", ""

	var ok bool
	for dec.Next() {
		rawmeasurement, err := dec.Measurement()
		if err != nil {
			return err
		}

		// Needs to be copied because another call to dec.* would
		// invalidate the returned slice.
		metricBuf = append(metricBuf[:0], rawmeasurement...)

		// The go compiler optimizes map[string(byteslice)] lookups:
		metric.MetricConfig, ok = ms.Metrics[string(rawmeasurement)]
		if !ok {
			continue
		}

		typeBuf, subTypeBuf := typeBuf[:0], subTypeBuf[:0]
		cluster, host := clusterDefault, ""
		for {
			key, val, err := dec.NextTag()
			if err != nil {
				return err
			}
			if key == nil {
				break
			}

			// The go compiler optimizes string([]byte{...}) == "...":
			switch string(key) {
			case "cluster":
				if string(val) == prevCluster {
					cluster = prevCluster
				} else {
					cluster = string(val)
					lvl = nil
				}
			case "hostname", "host":
				if string(val) == prevHost {
					host = prevHost
				} else {
					host = string(val)
					lvl = nil
				}
			case "type":
				if string(val) == "node" {
					break
				}

				// We cannot be sure that the "type" tag comes before the "type-id" tag:
				if len(typeBuf) == 0 {
					typeBuf = append(typeBuf, val...)
				} else {
					typeBuf = reorder(typeBuf, val)
				}
			case "type-id":
				typeBuf = append(typeBuf, val...)
			case "subtype":
				// We cannot be sure that the "subtype" tag comes before the "stype-id" tag:
				if len(subTypeBuf) == 0 {
					subTypeBuf = append(subTypeBuf, val...)
				} else {
					subTypeBuf = reorder(subTypeBuf, val)
					// subTypeBuf = reorder(typeBuf, val)
				}
			case "stype-id":
				subTypeBuf = append(subTypeBuf, val...)
			default:
			}
		}

		// If the cluster or host changed, the lvl was set to nil
		if lvl == nil {
			selector = selector[:2]
			selector[0], selector[1] = cluster, host
			lvl = ms.GetLevel(selector)
			prevCluster, prevHost = cluster, host
		}

		// subtypes:
		selector = selector[:0]
		if len(typeBuf) > 0 {
			selector = append(selector, string(typeBuf)) // <- Allocation :(
			if len(subTypeBuf) > 0 {
				selector = append(selector, string(subTypeBuf))
			}
		}

		for {
			key, val, err := dec.NextField()
			if err != nil {
				return err
			}

			if key == nil {
				break
			}

			if string(key) != "value" {
				return fmt.Errorf("host %s: unknown field: '%s' (value: %#v)", host, string(key), val)
			}

			if val.Kind() == lineprotocol.Float {
				metric.Value = schema.Float(val.FloatV())
			} else if val.Kind() == lineprotocol.Int {
				metric.Value = schema.Float(val.IntV())
			} else if val.Kind() == lineprotocol.Uint {
				metric.Value = schema.Float(val.UintV())
			} else {
				return fmt.Errorf("host %s: unsupported value type in message: %s", host, val.Kind().String())
			}
		}

		if t, err = dec.Time(lineprotocol.Second, t); err != nil {
			t = time.Now()
			if t, err = dec.Time(lineprotocol.Millisecond, t); err != nil {
				t = time.Now()
				if t, err = dec.Time(lineprotocol.Microsecond, t); err != nil {
					t = time.Now()
					if t, err = dec.Time(lineprotocol.Nanosecond, t); err != nil {
						return fmt.Errorf("host %s: timestamp : %#v with error : %#v", host, t, err.Error())
					}
				}
			}
		}

		if err != nil {
			return fmt.Errorf("host %s: timestamp : %#v with error : %#v", host, t, err.Error())
		}

		time := t.Unix()

		if Keys.Checkpoints.FileFormat != "json" {
			LineProtocolMessages <- &AvroStruct{
				MetricName: string(metricBuf),
				Cluster:    cluster,
				Node:       host,
				Selector:   append([]string{}, selector...),
				Value:      metric.Value,
				Timestamp:  time,
			}
		}

		if err := ms.WriteToLevel(lvl, selector, time, []Metric{metric}); err != nil {
			return err
		}
	}
	return nil
}
