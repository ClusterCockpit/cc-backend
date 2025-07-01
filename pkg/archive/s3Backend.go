// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

type S3ArchiveConfig struct {
	Path string `json:"filePath"`
}

type S3Archive struct {
	path string
}
