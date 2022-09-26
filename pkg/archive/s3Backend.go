// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

type S3ArchiveConfig struct {
	Path string `json:"filePath"`
}

type S3Archive struct {
	path string
}
