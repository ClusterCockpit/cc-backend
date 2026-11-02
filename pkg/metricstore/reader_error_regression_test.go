// Copyright (C) NHR@FAU, University Erlangen-Nuremberg. MIT license.
package metricstore

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/ClusterCockpit/cc-line-protocol/v2/lineprotocol"
)

type faultAfterPrefixReader struct {
	prefix *strings.Reader
	fault  error
}

func (r *faultAfterPrefixReader) Read(p []byte) (int, error) {
	n, err := r.prefix.Read(p)
	if err == io.EOF {
		return n, r.fault
	}
	return n, err
}

func TestDecodeLinePropagatesReaderFailureAfterValidPrefix(t *testing.T) {
	const sample = "cpu_load,cluster=lp-audit,hostname=lp-test,type=node value=17 1800000000\n"
	InitMetrics(map[string]MetricConfig{"cpu_load": {Frequency: 10}})
	store := GetMemoryStore()
	previous := Keys
	t.Cleanup(func() { Keys = previous })
	Keys.Checkpoints.FileFormat = "json"
	fault := errors.New("synthetic request reader failure")
	for _, tt := range []struct {
		name, prefix string
		fault        error
		bounded      bool
	}{{"empty EOF", "", nil, false}, {"valid EOF", sample, nil, false}, {"empty reader failure", "", fault, false}, {"valid prefix before failure", sample, fault, false}, {"maximum bounded valid prefix before failure", sample, fault, true}} {
		t.Run(tt.name, func(t *testing.T) {
			if err := store.FreeAll(); err != nil {
				t.Fatal(err)
			}
			var reader io.Reader = strings.NewReader(tt.prefix)
			if tt.fault != nil {
				reader = &faultAfterPrefixReader{strings.NewReader(tt.prefix), tt.fault}
			}
			decoder := lineprotocol.NewDecoder(reader)
			if tt.bounded {
				decoder.SetMaxBufferSize(len(sample) + 1)
			}
			err := DecodeLine(decoder, store, "")
			if !errors.Is(err, tt.fault) {
				t.Errorf("DecodeLine error%v want%v; actual decoder.Err=%v", err, tt.fault, decoder.Err())
			}
			if tt.prefix != "" {
				out, qerr := FetchData(APIQueryRequest{Cluster: "lp-audit", From: 1800000000, To: 1800000010, WithData: true, Queries: []APIQuery{{Hostname: "lp-test", Metric: "cpu_load"}}})
				if qerr != nil || len(out.Results) != 1 || len(out.Results[0]) != 1 || len(out.Results[0][0].Data) != 1 || float64(out.Results[0][0].Data[0]) != 17 {
					t.Fatal("valid prefix sample was lost")
				}
			}
		})
	}
}
