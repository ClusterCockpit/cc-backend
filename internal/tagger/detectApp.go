// Copyright (C) 2023 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package tagger

const tagType = "app"

type appInfo struct {
	tag     string
	strings []string
}
type AppTagger struct {
	apps []appInfo
}

func (t *AppTagger) Register() error {

	return nil
}
