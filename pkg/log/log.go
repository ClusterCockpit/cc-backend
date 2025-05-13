// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
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
	DebugLog *log.Logger = log.New(DebugWriter, DebugPrefix, log.LstdFlags)
	InfoLog  *log.Logger = log.New(InfoWriter, InfoPrefix, log.LstdFlags|log.Lshortfile)
	WarnLog  *log.Logger = log.New(WarnWriter, WarnPrefix, log.LstdFlags|log.Lshortfile)
	ErrLog   *log.Logger = log.New(ErrWriter, ErrPrefix, log.LstdFlags|log.Llongfile)
	CritLog  *log.Logger = log.New(CritWriter, CritPrefix, log.LstdFlags|log.Llongfile)
)

var loglevel string = "info"

/* CONFIG */

func Init(lvl string, logdate bool) {
	// Discard I/O for all writers below selected loglevel; <CRITICAL> is always written.
	switch lvl {
	case "crit":
		ErrWriter = io.Discard
		fallthrough
	case "err":
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
		fmt.Printf("pkg/log: Flag 'loglevel' has invalid value %#v\npkg/log: Will use default loglevel '%s'\n", lvl, loglevel)
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

	loglevel = lvl
}

/* HELPER */

func Loglevel() string {
	return loglevel
}

/* PRIVATE HELPER */

// Return unformatted string
func printStr(v ...interface{}) string {
	return fmt.Sprint(v...)
}

// Return formatted string
func printfStr(format string, v ...interface{}) string {
	return fmt.Sprintf(format, v...)
}

/* PRINT */

// Prints to STDOUT without string formatting; application continues.
// Used for special cases not requiring log information like date or location.
func Print(v ...interface{}) {
	fmt.Fprintln(os.Stdout, v...)
}

// Prints to STDOUT without string formatting; application exits with error code 0.
// Used for exiting succesfully with message after expected outcome, e.g. successful single-call application runs.
func Exit(v ...interface{}) {
	fmt.Fprintln(os.Stdout, v...)
	os.Exit(0)
}

// Prints to STDOUT without string formatting; application exits with error code 1.
// Used for terminating with message after to be expected errors, e.g. wrong arguments or during init().
func Abort(v ...interface{}) {
	fmt.Fprintln(os.Stdout, v...)
	os.Exit(1)
}

// Prints to DEBUG writer without string formatting; application continues.
// Used for logging additional information, primarily for development.
func Debug(v ...interface{}) {
	DebugLog.Output(2, printStr(v...))
}

// Prints to INFO writer without string formatting; application continues.
// Used for logging additional information, e.g. notable returns or common fail-cases.
func Info(v ...interface{}) {
	InfoLog.Output(2, printStr(v...))
}

// Prints to WARNING writer without string formatting; application continues.
// Used for logging important information, e.g. uncommon edge-cases or administration related information.
func Warn(v ...interface{}) {
	WarnLog.Output(2, printStr(v...))
}

// Prints to ERROR writer without string formatting; application continues.
// Used for logging errors, but code still can return default(s) or nil.
func Error(v ...interface{}) {
	ErrLog.Output(2, printStr(v...))
}

// Prints to CRITICAL writer without string formatting; application exits with error code 1.
// Used for terminating on unexpected errors with date and code location.
func Fatal(v ...interface{}) {
	CritLog.Output(2, printStr(v...))
	os.Exit(1)
}

// Prints to PANIC function without string formatting; application exits with panic.
// Used for terminating on unexpected errors with stacktrace.
func Panic(v ...interface{}) {
	panic(printStr(v...))
}

/* PRINT FORMAT*/

// Prints to STDOUT with string formatting; application continues.
// Used for special cases not requiring log information like date or location.
func Printf(format string, v ...interface{}) {
	fmt.Fprintf(os.Stdout, format, v...)
}

// Prints to STDOUT with string formatting; application exits with error code 0.
// Used for exiting succesfully with message after expected outcome, e.g. successful single-call application runs.
func Exitf(format string, v ...interface{}) {
	fmt.Fprintf(os.Stdout, format, v...)
	os.Exit(0)
}

// Prints to STDOUT with string formatting; application exits with error code 1.
// Used for terminating with message after to be expected errors, e.g. wrong arguments or during init().
func Abortf(format string, v ...interface{}) {
	fmt.Fprintf(os.Stdout, format, v...)
	os.Exit(1)
}

// Prints to DEBUG writer with string formatting; application continues.
// Used for logging additional information, primarily for development.
func Debugf(format string, v ...interface{}) {
	DebugLog.Output(2, printfStr(format, v...))
}

// Prints to INFO writer with string formatting; application continues.
// Used for logging additional information, e.g. notable returns or common fail-cases.
func Infof(format string, v ...interface{}) {
	InfoLog.Output(2, printfStr(format, v...))
}

// Prints to WARNING writer with string formatting; application continues.
// Used for logging important information, e.g. uncommon edge-cases or administration related information.
func Warnf(format string, v ...interface{}) {
	WarnLog.Output(2, printfStr(format, v...))
}

// Prints to ERROR writer with string formatting; application continues.
// Used for logging errors, but code still can return default(s) or nil.
func Errorf(format string, v ...interface{}) {
	ErrLog.Output(2, printfStr(format, v...))
}

// Prints to CRITICAL writer with string formatting; application exits with error code 1.
// Used for terminating on unexpected errors with date and code location.
func Fatalf(format string, v ...interface{}) {
	CritLog.Output(2, printfStr(format, v...))
	os.Exit(1)
}

// Prints to PANIC function with string formatting; application exits with panic.
// Used for terminating on unexpected errors with stacktrace.
func Panicf(format string, v ...interface{}) {
	panic(printfStr(format, v...))
}
