// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package archiver

import (
	"slices"
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

func TestArchiveScopes(t *testing.T) {
	withFS := &schema.SubCluster{Topology: schema.Topology{
		Filesystems: []*schema.Filesystem{{ID: "/home", Type: "nfs"}},
	}}
	withFSAndNet := &schema.SubCluster{Topology: schema.Topology{
		Filesystems: []*schema.Filesystem{{ID: "/home", Type: "nfs"}},
		Networks:    []*schema.Network{{ID: "ib0", Type: "infiniband"}},
	}}
	noDevices := &schema.SubCluster{}

	tests := []struct {
		name       string
		job        schema.Job
		subCluster *schema.SubCluster
		want       []schema.MetricScope
	}{
		{
			name: "large job keeps filesystem", job: schema.Job{NumNodes: 64}, subCluster: withFS,
			want: []schema.MetricScope{schema.MetricScopeNode, schema.MetricScopeFilesystem},
		},
		{
			name: "small GPU job", job: schema.Job{NumNodes: 4, NumAcc: 4}, subCluster: noDevices,
			want: []schema.MetricScope{schema.MetricScopeNode, schema.MetricScopeCore, schema.MetricScopeAccelerator},
		},
		{
			name: "small job with filesystems and networks", job: schema.Job{NumNodes: 4}, subCluster: withFSAndNet,
			want: []schema.MetricScope{
				schema.MetricScopeNode, schema.MetricScopeCore,
				schema.MetricScopeFilesystem, schema.MetricScopeNetwork,
			},
		},
		{
			name: "subcluster without devices", job: schema.Job{NumNodes: 4}, subCluster: noDevices,
			want: []schema.MetricScope{schema.MetricScopeNode, schema.MetricScopeCore},
		},
		{
			name: "subcluster lookup failed", job: schema.Job{NumNodes: 4}, subCluster: nil,
			want: []schema.MetricScope{schema.MetricScopeNode, schema.MetricScopeCore},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := archiveScopes(&tt.job, tt.subCluster); !slices.Equal(got, tt.want) {
				t.Errorf("archiveScopes = %v, want %v", got, tt.want)
			}
		})
	}
}
