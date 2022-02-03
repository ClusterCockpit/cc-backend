package main

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

func loadEnv(file string) error {
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
			return fmt.Errorf("unsupported line: %#v", line)
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if strings.HasPrefix(val, "\"") {
			if !strings.HasSuffix(val, "\"") {
				return fmt.Errorf("unsupported line: %#v", line)
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
						return fmt.Errorf("unsupprorted escape sequence in quoted string: backslash %#v", runes[i])
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

func dropPrivileges() error {
	if programConfig.Group != "" {
		g, err := user.LookupGroup(programConfig.Group)
		if err != nil {
			return err
		}

		gid, _ := strconv.Atoi(g.Gid)
		if err := syscall.Setgid(gid); err != nil {
			return err
		}
	}

	if programConfig.User != "" {
		u, err := user.Lookup(programConfig.User)
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
func systemdNotifiy(ready bool, status string) {
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
