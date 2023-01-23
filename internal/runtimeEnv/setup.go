// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package runtimeEnv

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
)

// Very simple and limited .env file reader.
// All variable definitions found are directly
// added to the processes environment.
func LoadEnv(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}

	defer f.Close()
	s := bufio.NewScanner(bufio.NewReader(f))
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") || len(line) == 0 {
			continue
		}

		if strings.Contains(line, "#") {
			return errors.New("'#' are only supported at the start of a line")
		}

		line = strings.TrimPrefix(line, "export ")
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("RUNTIME/SETUP > unsupported line: %#v", line)
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if strings.HasPrefix(val, "\"") {
			if !strings.HasSuffix(val, "\"") {
				return fmt.Errorf("RUNTIME/SETUP > unsupported line: %#v", line)
			}

			runes := []rune(val[1 : len(val)-1])
			sb := strings.Builder{}
			for i := 0; i < len(runes); i++ {
				if runes[i] == '\\' {
					i++
					switch runes[i] {
					case 'n':
						sb.WriteRune('\n')
					case 'r':
						sb.WriteRune('\r')
					case 't':
						sb.WriteRune('\t')
					case '"':
						sb.WriteRune('"')
					default:
						return fmt.Errorf("RUNTIME/SETUP > unsupported escape sequence in quoted string: backslash %#v", runes[i])
					}
					continue
				}
				sb.WriteRune(runes[i])
			}

			val = sb.String()
		}

		os.Setenv(key, val)
	}

	return s.Err()
}

// Changes the processes user and group to that
// specified in the config.json. The go runtime
// takes care of all threads (and not only the calling one)
// executing the underlying systemcall.
func DropPrivileges(username string, group string) error {
	if group != "" {
		g, err := user.LookupGroup(group)
		if err != nil {
			return err
		}

		gid, _ := strconv.Atoi(g.Gid)
		if err := syscall.Setgid(gid); err != nil {
			return err
		}
	}

	if username != "" {
		u, err := user.Lookup(username)
		if err != nil {
			return err
		}

		uid, _ := strconv.Atoi(u.Uid)
		if err := syscall.Setuid(uid); err != nil {
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
