// Copyright (C) 2023 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package util

func Contains[T comparable](items []T, item T) bool {
	for _, v := range items {
		if v == item {
			return true
		}
	}
	return false
}
