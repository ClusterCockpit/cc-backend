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
	"time"
)

// Provides a simple way of logging with different levels.
// Time/Date are not logged because systemd adds
// them for us (Default, can be changed by flag '--logdate true').
//
// Uses these prefixes: https://www.freedesktop.org/software/systemd/man/sd-daemon.html

var logDateTime bool
var logLevel string

var (
	DebugWriter io.Writer = os.Stderr
	NoteWriter  io.Writer = os.Stderr
	InfoWriter  io.Writer = os.Stderr
	WarnWriter  io.Writer = os.Stderr
	ErrWriter   io.Writer = os.Stderr
	CritWriter  io.Writer = os.Stderr
)

var (
	DebugPrefix string = "<7>[DEBUG]    "
	InfoPrefix  string = "<6>[INFO]     "
	NotePrefix  string = "<5>[NOTICE]   "
	WarnPrefix  string = "<4>[WARNING]  "
	ErrPrefix   string = "<3>[ERROR]    "
	CritPrefix  string = "<2>[CRITICAL] "
)

var (
	// No Time/Date
	DebugLog *log.Logger = log.New(DebugWriter, DebugPrefix, 0)
	InfoLog  *log.Logger = log.New(InfoWriter,  InfoPrefix,  0)
	NoteLog  *log.Logger = log.New(NoteWriter,  NotePrefix,  log.Lshortfile)
	WarnLog  *log.Logger = log.New(WarnWriter,  WarnPrefix,  log.Lshortfile)
	ErrLog   *log.Logger = log.New(ErrWriter,   ErrPrefix,   log.Llongfile)
	CritLog  *log.Logger = log.New(CritWriter,  CritPrefix,  log.Llongfile)
	// Log Time/Date
	DebugTimeLog *log.Logger = log.New(DebugWriter, DebugPrefix, log.LstdFlags)
	InfoTimeLog  *log.Logger = log.New(InfoWriter,  InfoPrefix,  log.LstdFlags)
	NoteTimeLog  *log.Logger = log.New(NoteWriter,  NotePrefix,  log.LstdFlags|log.Lshortfile)
	WarnTimeLog  *log.Logger = log.New(WarnWriter,  WarnPrefix,  log.LstdFlags|log.Lshortfile)
	ErrTimeLog   *log.Logger = log.New(ErrWriter,   ErrPrefix,   log.LstdFlags|log.Llongfile)
	CritTimeLog  *log.Logger = log.New(CritWriter,  CritPrefix,  log.LstdFlags|log.Llongfile)
)

/* CONFIG */

func SetLogLevel(lvl string) {
	// fmt.Printf("pkg/log: Set LOGLEVEL -> %s\n", lvl)
	switch lvl {
		case "crit":
			ErrWriter = io.Discard
			fallthrough
		case "err", "fatal":
			WarnWriter = io.Discard
			fallthrough
		case "warn":
			InfoWriter = io.Discard
			fallthrough
		case "notice":
			NoteWriter = io.Discard
			fallthrough
		case "info":
			DebugWriter = io.Discard
			break
		case "debug":
			// Nothing to do...
			break
		default:
			fmt.Printf("pkg/log: Flag 'loglevel' has invalid value %#v\npkg/log: Will use default loglevel 'debug'\n", lvl)
			SetLogLevel("debug")
	}
}

func SetLogDateTime(logdate bool) {
	//fmt.Printf("pkg/log: Set DATEBOOL -> %v\n", logdate)
	logDateTime = logdate
}

/* PRINT */

// Private helper
func printStr(v ...interface{}) string {
	return fmt.Sprint(v...)
}

func Print(v ...interface{}) {
	Info(v...)
}

func Debug(v ...interface{}) {
	if DebugWriter != io.Discard {
		out := printStr(v...)
		if logDateTime {
			DebugTimeLog.Output(2, out)
		} else {
			DebugLog.Output(2, out)
		}
	}
}

func Info(v ...interface{}) {
	if InfoWriter != io.Discard {
		out := printStr(v...)
		if logDateTime {
			InfoTimeLog.Output(2, out)
		} else {
			InfoLog.Output(2, out)
		}
	}
}

func Note(v ...interface{}) {
	if NoteWriter != io.Discard {
		out := printStr(v...)
		if logDateTime {
		  NoteTimeLog.Output(2, out)
		} else {
			NoteLog.Output(2, out)
		}
	}
}

func Warn(v ...interface{}) {
	if WarnWriter != io.Discard {
		out := printStr(v...)
		if logDateTime {
			WarnTimeLog.Output(2, out)
		} else {
			WarnLog.Output(2, out)
		}
	}
}

func Error(v ...interface{}) {
	if ErrWriter != io.Discard {
		out := printStr(v...)
		if logDateTime {
			ErrTimeLog.Output(2, out)
		} else {
			ErrLog.Output(2, out)
		}
	}
}

// Writes panic stacktrace, keeps application alive
func Panic(v ...interface{}) {
	Error(v...)
	panic("Panic triggered ...")
}

// Writes error log, stops application
func Fatal(v ...interface{}) {
	Error(v...)
	os.Exit(1)
}

func Crit(v ...interface{}) {
	if CritWriter != io.Discard {
		out := printStr(v...)
		if logDateTime {
			CritTimeLog.Output(2, out)
		} else {
			CritLog.Output(2, out)
		}
	}
}

/* PRINT FORMAT*/

// Private helper
func printfStr(format string, v ...interface{}) string {
	return fmt.Sprintf(format, v...)
}

func Printf(format string, v ...interface{}) {
	Infof(format, v...)
}

func Debugf(format string, v ...interface{}) {
	if DebugWriter != io.Discard {
		out := printfStr(format, v...)
		if logDateTime {
			DebugTimeLog.Output(2, out)
		} else {
			DebugLog.Output(2, out)
		}
	}
}

func Infof(format string, v ...interface{}) {
	if InfoWriter != io.Discard {
		out := printfStr(format, v...)
		if logDateTime {
			InfoTimeLog.Output(2, out)
		} else {
			InfoLog.Output(2, out)
		}
	}
}

func Notef(format string, v ...interface{}) {
	if NoteWriter != io.Discard {
		out := printfStr(format, v...)
		if logDateTime {
			NoteTimeLog.Output(2, out)
		} else {
			NoteLog.Output(2, out)
		}
	}
}

func Warnf(format string, v ...interface{}) {
	if WarnWriter != io.Discard {
		out := printfStr(format, v...)
		if logDateTime {
			WarnTimeLog.Output(2, out)
		} else {
			WarnLog.Output(2, out)
		}
	}
}

func Errorf(format string, v ...interface{}) {
	if ErrWriter != io.Discard {
		out := printfStr(format, v...)
		if logDateTime {
			ErrTimeLog.Output(2, out)
		} else {
			ErrLog.Output(2, out)
		}
	}
}

// Writes panic stacktrace, keeps application alive
func Panicf(format string, v ...interface{}) {
	Errorf(format, v...)
	panic("Panic triggered ...")
}

// Writes error log, stops application
func Fatalf(format string, v ...interface{}) {
	Errorf(format, v...)
	os.Exit(1)
}

func Critf(format string, v ...interface{}) {
	if CritWriter != io.Discard {
		out := printfStr(format, v...)
		if logDateTime {
			CritTimeLog.Output(2, out)
		} else {
			CritLog.Output(2, out)
		}
	}
}

/* SPECIAL */

func Finfof(w io.Writer, format string, v ...interface{}) {
	if w != io.Discard {
		if logDateTime {
			currentTime := time.Now()
			fmt.Fprintf(InfoWriter, currentTime.String()+InfoPrefix+format+"\n", v...)
		} else {
			fmt.Fprintf(InfoWriter, InfoPrefix+format+"\n", v...)
		}
	}
}
