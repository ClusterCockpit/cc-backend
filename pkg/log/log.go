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
// Time/Date are not logged because systemd adds
// them for us (Default, can be changed by flag '--logdate true').
//
// Uses these prefixes: https://www.freedesktop.org/software/systemd/man/sd-daemon.html

var (
	DebugWriter io.Writer = os.Stderr
	InfoWriter  io.Writer = os.Stderr
	WarnWriter  io.Writer = os.Stderr
	ErrWriter   io.Writer = os.Stderr
	CritWriter  io.Writer = os.Stderr
)

var (
	DebugPrefix string = "<7>[DEBUG]    "
	InfoPrefix  string = "<6>[INFO]     "
	WarnPrefix  string = "<4>[WARNING]  "
	ErrPrefix   string = "<3>[ERROR]    "
	CritPrefix  string = "<2>[CRITICAL] "
)

var (
	DebugLog *log.Logger
	InfoLog  *log.Logger
	WarnLog  *log.Logger
	ErrLog   *log.Logger
	CritLog  *log.Logger
)

/* CONFIG */

func Init(lvl string, logdate bool) {
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
	case "info":
		DebugWriter = io.Discard
	case "debug":
		// Nothing to do...
		break
	default:
		fmt.Printf("pkg/log: Flag 'loglevel' has invalid value %#v\npkg/log: Will use default loglevel 'debug'\n", lvl)
		//SetLogLevel("debug")
	}

	if !logdate {
		DebugLog = log.New(DebugWriter, DebugPrefix, 0)
		InfoLog = log.New(InfoWriter, InfoPrefix, log.Lshortfile)
		WarnLog = log.New(WarnWriter, WarnPrefix, log.Lshortfile)
		ErrLog = log.New(ErrWriter, ErrPrefix, log.Llongfile)
		CritLog = log.New(CritWriter, CritPrefix, log.Llongfile)
	} else {
		DebugLog = log.New(DebugWriter, DebugPrefix, log.LstdFlags)
		InfoLog = log.New(InfoWriter, InfoPrefix, log.LstdFlags|log.Lshortfile)
		WarnLog = log.New(WarnWriter, WarnPrefix, log.LstdFlags|log.Lshortfile)
		ErrLog = log.New(ErrWriter, ErrPrefix, log.LstdFlags|log.Llongfile)
		CritLog = log.New(CritWriter, CritPrefix, log.LstdFlags|log.Llongfile)
	}
}

/* PRINT */

// Private helper
func printStr(v ...interface{}) string {
	return fmt.Sprint(v...)
}

// Uses Info() -> If errorpath required at some point:
// Will need own writer with 'Output(2, out)' to correctly render path
func Print(v ...interface{}) {
	Info(v...)
}

func Debug(v ...interface{}) {
	DebugLog.Output(2, printStr(v...))
}

func Info(v ...interface{}) {
	InfoLog.Output(2, printStr(v...))
}

func Warn(v ...interface{}) {
	WarnLog.Output(2, printStr(v...))
}

func Error(v ...interface{}) {
	ErrLog.Output(2, printStr(v...))
}

// Writes panic stacktrace, but keeps application alive
func Panic(v ...interface{}) {
	ErrLog.Output(2, printStr(v...))
	panic("Panic triggered ...")
}

func Crit(v ...interface{}) {
	CritLog.Output(2, printStr(v...))
}

// Writes critical log, stops application
func Fatal(v ...interface{}) {
	CritLog.Output(2, printStr(v...))
	os.Exit(1)
}

/* PRINT FORMAT*/

// Private helper
func printfStr(format string, v ...interface{}) string {
	return fmt.Sprintf(format, v...)
}

// Uses Infof() -> If errorpath required at some point:
// Will need own writer with 'Output(2, out)' to correctly render path
func Printf(format string, v ...interface{}) {
	Infof(format, v...)
}

func Debugf(format string, v ...interface{}) {
	DebugLog.Output(2, printfStr(format, v...))
}

func Infof(format string, v ...interface{}) {
	InfoLog.Output(2, printfStr(format, v...))
}

func Warnf(format string, v ...interface{}) {
	WarnLog.Output(2, printfStr(format, v...))
}

func Errorf(format string, v ...interface{}) {
	ErrLog.Output(2, printfStr(format, v...))
}

// Writes panic stacktrace, but keeps application alive
func Panicf(format string, v ...interface{}) {
	ErrLog.Output(2, printfStr(format, v...))
	panic("Panic triggered ...")
}

func Critf(format string, v ...interface{}) {
	CritLog.Output(2, printfStr(format, v...))
}

// Writes crit log, stops application
func Fatalf(format string, v ...interface{}) {
	CritLog.Output(2, printfStr(format, v...))
	os.Exit(1)
}

/* SPECIAL */

// func Finfof(w io.Writer, format string, v ...interface{}) {
// 	if w != io.Discard {
// 		if logDateTime {
// 			currentTime := time.Now()
// 			fmt.Fprintf(InfoWriter, currentTime.String()+InfoPrefix+format+"\n", v...)
// 		} else {
// 			fmt.Fprintf(InfoWriter, InfoPrefix+format+"\n", v...)
// 		}
// 	}
// }
