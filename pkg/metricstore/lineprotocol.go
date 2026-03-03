// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// This file implements ingestion of InfluxDB line-protocol metric data received
// over NATS. Each line encodes one metric sample with the following structure:
//
//	<measurement>[,cluster=<c>][,hostname=<h>][,type=<t>][,type-id=<id>][,subtype=<s>][,stype-id=<id>] value=<v> [<timestamp>]
//
// The measurement name identifies the metric (e.g. "cpu_load"). Tags provide
// routing information (cluster, host) and optional sub-device selectors (type,
// subtype). Only one field is expected per line: "value".
//
// After decoding, each sample is:
//  1. Written to the in-memory store via ms.WriteToLevel.
//  2. If the checkpoint format is "wal", also forwarded to the WAL staging
//     goroutine via the WALMessages channel for durable write-ahead logging.
package metricstore

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/nats"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/ClusterCockpit/cc-line-protocol/v2/lineprotocol"
)

// ReceiveNats subscribes to all configured NATS subjects and feeds incoming
// line-protocol messages into the MemoryStore.
//
// When workers > 1 a pool of goroutines drains a shared channel so that
// multiple messages can be decoded in parallel. With workers == 1 the NATS
// callback decodes inline (no channel overhead, lower latency).
//
// The function blocks until ctx is cancelled and all worker goroutines have
// finished. It returns nil when the NATS client is not configured; callers
// should treat that as a no-op rather than an error.
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

	for _, sc := range *Keys.Subscriptions {
		clusterTag := sc.ClusterTag
		if workers > 1 {
			wg.Add(workers)

			for range workers {
				go func() {
					defer wg.Done()
					for m := range msgs {
						dec := lineprotocol.NewDecoderWithBytes(m)
						if err := DecodeLine(dec, ms, clusterTag); err != nil {
							cclog.Errorf("error: %s", err.Error())
						}
					}
				}()
			}

			nc.Subscribe(sc.SubscribeTo, func(subject string, data []byte) {
				select {
				case msgs <- data:
				case <-ctx.Done():
				}
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

	go func() {
		<-ctx.Done()
		close(msgs)
	}()

	wg.Wait()

	return nil
}

// reorder prepends prefix to buf in-place when buf has enough spare capacity,
// avoiding an allocation. Falls back to a regular append otherwise.
//
// It is used to assemble the "type<type-id>" and "subtype<stype-id>" selector
// strings when the type tag arrives before the type-id tag in the line, so the
// two byte slices need to be concatenated in tag-declaration order regardless
// of wire order.
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

// decodeState holds the per-call scratch buffers used by DecodeLine.
// Instances are recycled via decodeStatePool to avoid repeated allocations
// during high-throughput ingestion.
type decodeState struct {
	// metricBuf holds a copy of the current measurement name (line-protocol
	// measurement field). Copied because dec.Measurement() returns a slice
	// that is invalidated by the next decoder call.
	metricBuf []byte

	// selector is the sub-device path passed to WriteToLevel and WALMessage
	// (e.g. ["socket0"] or ["socket0", "memctrl1"]). Reused across lines.
	selector []string

	// typeBuf accumulates the concatenated "type"+"type-id" tag value for the
	// current line. Reset at the start of each line's tag-decode loop.
	typeBuf []byte

	// subTypeBuf accumulates the concatenated "subtype"+"stype-id" tag value.
	// Reset at the start of each line's tag-decode loop.
	subTypeBuf []byte

	// prevTypeBytes / prevTypeStr cache the last seen typeBuf content and its
	// string conversion. Because consecutive lines in a batch typically address
	// the same sub-device, the cache hit rate is very high and avoids
	// repeated []byte→string allocations.
	prevTypeBytes []byte
	prevTypeStr   string

	// prevSubTypeBytes / prevSubTypeStr are the same cache for the subtype.
	prevSubTypeBytes []byte
	prevSubTypeStr   string
}

// decodeStatePool recycles decodeState values across DecodeLine calls to
// reduce GC pressure during sustained metric ingestion.
var decodeStatePool = sync.Pool{
	New: func() any {
		return &decodeState{
			metricBuf:  make([]byte, 0, 16),
			selector:   make([]string, 0, 4),
			typeBuf:    make([]byte, 0, 16),
			subTypeBuf: make([]byte, 0, 16),
		}
	},
}

// DecodeLine reads all lines from dec (InfluxDB line-protocol) and writes each
// decoded metric sample into ms.
//
// clusterDefault is used as the cluster name for lines that do not carry a
// "cluster" tag. Callers typically supply the ClusterTag value from the NATS
// subscription configuration.
//
// Performance notes:
//   - A decodeState is obtained from decodeStatePool to reuse scratch buffers.
//   - The Level pointer (host-level node in the metric tree) is cached across
//     consecutive lines that share the same cluster+host pair to avoid
//     repeated lock acquisitions on the root and cluster levels.
//   - []byte→string conversions for type/subtype selectors are cached via
//     prevType*/prevSubType* fields because batches typically repeat the same
//     sub-device identifiers.
//   - Timestamp parsing tries Second precision first; if that fails it retries
//     Millisecond, Microsecond, and Nanosecond in turn. A missing timestamp
//     falls back to time.Now().
//
// When the checkpoint format is "wal" each successfully decoded sample is also
// sent to WALMessages so the WAL staging goroutine can persist it durably
// before the next binary snapshot.
func DecodeLine(dec *lineprotocol.Decoder,
	ms *MemoryStore,
	clusterDefault string,
) error {
	// Reduce allocations in loop:
	t := time.Now()
	metric := Metric{}
	st := decodeStatePool.Get().(*decodeState)
	defer decodeStatePool.Put(st)

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
		st.metricBuf = append(st.metricBuf[:0], rawmeasurement...)

		// The go compiler optimizes map[string(byteslice)] lookups:
		metric.MetricConfig, ok = ms.Metrics[string(rawmeasurement)]
		if !ok {
			continue
		}

		st.typeBuf, st.subTypeBuf = st.typeBuf[:0], st.subTypeBuf[:0]
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
				if len(st.typeBuf) == 0 {
					st.typeBuf = append(st.typeBuf, val...)
				} else {
					st.typeBuf = reorder(st.typeBuf, val)
				}
			case "type-id":
				st.typeBuf = append(st.typeBuf, val...)
			case "subtype":
				// We cannot be sure that the "subtype" tag comes before the "stype-id" tag:
				if len(st.subTypeBuf) == 0 {
					st.subTypeBuf = append(st.subTypeBuf, val...)
				} else {
					st.subTypeBuf = reorder(st.subTypeBuf, val)
				}
			case "stype-id":
				st.subTypeBuf = append(st.subTypeBuf, val...)
			default:
			}
		}

		// If the cluster or host changed, the lvl was set to nil
		if lvl == nil {
			st.selector = st.selector[:2]
			st.selector[0], st.selector[1] = cluster, host
			lvl = ms.GetLevel(st.selector)
			prevCluster, prevHost = cluster, host
		}

		// subtypes: cache []byte→string conversions; messages in a batch typically
		// share the same type/subtype so the hit rate is very high.
		st.selector = st.selector[:0]
		if len(st.typeBuf) > 0 {
			if !bytes.Equal(st.typeBuf, st.prevTypeBytes) {
				st.prevTypeBytes = append(st.prevTypeBytes[:0], st.typeBuf...)
				st.prevTypeStr = string(st.typeBuf)
			}
			st.selector = append(st.selector, st.prevTypeStr)
			if len(st.subTypeBuf) > 0 {
				if !bytes.Equal(st.subTypeBuf, st.prevSubTypeBytes) {
					st.prevSubTypeBytes = append(st.prevSubTypeBytes[:0], st.subTypeBuf...)
					st.prevSubTypeStr = string(st.subTypeBuf)
				}
				st.selector = append(st.selector, st.prevSubTypeStr)
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

		if Keys.Checkpoints.FileFormat == "wal" {
			WALMessages <- &WALMessage{
				MetricName: string(st.metricBuf),
				Cluster:    cluster,
				Node:       host,
				Selector:   append([]string{}, st.selector...),
				Value:      metric.Value,
				Timestamp:  time,
			}
		}

		if err := ms.WriteToLevel(lvl, st.selector, time, []Metric{metric}); err != nil {
			return err
		}
	}
	return nil
}
