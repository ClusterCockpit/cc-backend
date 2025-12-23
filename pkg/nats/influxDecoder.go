// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package nats

import (
	"time"

	lp "github.com/ClusterCockpit/cc-lib/ccMessage"
	influx "github.com/influxdata/line-protocol/v2/lineprotocol"
)

// DecodeInfluxMessage decodes a single InfluxDB line protocol message from the decoder
// Returns the decoded CCMessage or an error if decoding fails
func DecodeInfluxMessage(d *influx.Decoder) (lp.CCMessage, error) {
	measurement, err := d.Measurement()
	if err != nil {
		return nil, err
	}

	tags := make(map[string]string)
	for {
		key, value, err := d.NextTag()
		if err != nil {
			return nil, err
		}
		if key == nil {
			break
		}
		tags[string(key)] = string(value)
	}

	fields := make(map[string]interface{})
	for {
		key, value, err := d.NextField()
		if err != nil {
			return nil, err
		}
		if key == nil {
			break
		}
		fields[string(key)] = value.Interface()
	}

	t, err := d.Time(influx.Nanosecond, time.Time{})
	if err != nil {
		return nil, err
	}

	return lp.NewMessage(
		string(measurement),
		tags,
		nil,
		fields,
		t,
	)
}
