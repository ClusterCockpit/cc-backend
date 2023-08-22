// Copyright (C) 2023 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package tagger

import "github.com/ClusterCockpit/cc-backend/pkg/schema"

type Tagger interface {
	Register() error
	Match(job *schema.Job)
}

func Init() error {

	return nil
}
