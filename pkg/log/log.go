// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package log

import (
	"fmt"
	"io"
	"log"
	"os"
)

// Provides a simple way of logging with different levels.
// Time/Data are not logged on purpose because systemd adds
// them for us.
//
// Uses these prefixes: https://www.freedesktop.org/software/systemd/man/sd-daemon.html

var (
	DebugWriter io.Writer = os.Stderr
	InfoWriter  io.Writer = os.Stderr
	WarnWriter  io.Writer = os.Stderr
	ErrWriter   io.Writer = os.Stderr
)

var (
	DebugPrefix string = "<7>[DEBUG]   "
	InfoPrefix  string = "<6>[INFO]    "
	WarnPrefix  string = "<4>[WARNING] "
	ErrPrefix   string = "<3>[ERROR]   "
)

var (
	DebugLog *log.Logger = log.New(DebugWriter, DebugPrefix, 0)
	InfoLog  *log.Logger = log.New(InfoWriter, InfoPrefix, 0)
	WarnLog  *log.Logger = log.New(WarnWriter, WarnPrefix, 0)
	ErrLog   *log.Logger = log.New(ErrWriter, ErrPrefix, 0)
)

func init() {
	if lvl, ok := os.LookupEnv("LOGLEVEL"); ok {
		switch lvl {
		case "err", "fatal":
			WarnWriter = io.Discard
			fallthrough
		case "warn":
			InfoWriter = io.Discard
			fallthrough
		case "info":
			DebugWriter = io.Discard
		case "debug":
			// Nothing to do...
		default:
			Warnf("environment variable LOGLEVEL has invalid value %#v", lvl)
		}
	}
}

func Debug(v ...interface{}) {
	if DebugWriter != io.Discard {
		DebugLog.Print(v...)
	}
}

func Info(v ...interface{}) {
	if InfoWriter != io.Discard {
		InfoLog.Print(v...)
	}
}

func Print(v ...interface{}) {
	Info(v...)
}

func Warn(v ...interface{}) {
	if WarnWriter != io.Discard {
		WarnLog.Print(v...)
	}
}

func Error(v ...interface{}) {
	if ErrWriter != io.Discard {
		ErrLog.Print(v...)
	}
}

func Fatal(v ...interface{}) {
	Error(v...)
	os.Exit(1)
}

func Debugf(format string, v ...interface{}) {
	if DebugWriter != io.Discard {
		DebugLog.Printf(format, v...)
	}
}

func Infof(format string, v ...interface{}) {
	if InfoWriter != io.Discard {
		InfoLog.Printf(format, v...)
	}
}

func Printf(format string, v ...interface{}) {
	Infof(format, v...)
}

func Finfof(w io.Writer, format string, v ...interface{}) {
	if w != io.Discard {
		fmt.Fprintf(InfoWriter, InfoPrefix+format+"\n", v...)
	}
}

func Warnf(format string, v ...interface{}) {
	if WarnWriter != io.Discard {
		WarnLog.Printf(format, v...)
	}
}

func Errorf(format string, v ...interface{}) {
	if ErrWriter != io.Discard {
		ErrLog.Printf(format, v...)
	}
}

func Fatalf(format string, v ...interface{}) {
	Errorf(format, v...)
	os.Exit(1)
}
