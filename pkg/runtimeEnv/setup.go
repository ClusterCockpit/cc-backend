// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package runtimeEnv

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
)

// Changes the processes user and group to that
// specified in the config.json. The go runtime
// takes care of all threads (and not only the calling one)
// executing the underlying systemcall.
func DropPrivileges(username string, group string) error {
	if group != "" {
		g, err := user.LookupGroup(group)
		if err != nil {
			log.Warn("Error while looking up group")
			return err
		}

		gid, _ := strconv.Atoi(g.Gid)
		if err := syscall.Setgid(gid); err != nil {
			log.Warn("Error while setting gid")
			return err
		}
	}

	if username != "" {
		u, err := user.Lookup(username)
		if err != nil {
			log.Warn("Error while looking up user")
			return err
		}

		uid, _ := strconv.Atoi(u.Uid)
		if err := syscall.Setuid(uid); err != nil {
			log.Warn("Error while setting uid")
			return err
		}
	}

	return nil
}

// If started via systemd, inform systemd that we are running:
// https://www.freedesktop.org/software/systemd/man/sd_notify.html
func SystemdNotifiy(ready bool, status string) {
	if os.Getenv("NOTIFY_SOCKET") == "" {
		// Not started using systemd
		return
	}

	args := []string{fmt.Sprintf("--pid=%d", os.Getpid())}
	if ready {
		args = append(args, "--ready")
	}

	if status != "" {
		args = append(args, fmt.Sprintf("--status=%s", status))
	}

	cmd := exec.Command("systemd-notify", args...)
	cmd.Run() // errors ignored on purpose, there is not much to do anyways.
}
