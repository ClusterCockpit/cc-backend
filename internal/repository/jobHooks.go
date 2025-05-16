// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

type JobHook interface {
	jobStartCallback()
	jobStopCallback()
}

var hooks []JobHook

func RegisterJobJook(hook JobHook) {
	if hook != nil {
		hooks = append(hooks, hook)
	}
}

func CallJobStartHooks() {
	for _, hook := range hooks {
		if hook != nil {
			hook.jobStartCallback()
		}
	}
}

func CallJobStopHooks() {
	for _, hook := range hooks {
		if hook != nil {
			hook.jobStopCallback()
		}
	}
}
