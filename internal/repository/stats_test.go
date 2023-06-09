// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"fmt"
	"testing"
)

func TestBuildJobStatsQuery(t *testing.T) {
	r := setup(t)
	q := r.buildStatsQuery(nil, "USER")

	sql, _, err := q.ToSql()
	noErr(t, err)

	fmt.Printf("SQL: %s\n", sql)

}
