// Provides a simple way of logging with different levels.
// Time/Data are not logged on purpose because systemd adds
// them for us.
//
// Uses these prefixes: https://www.freedesktop.org/software/systemd/man/sd-daemon.html
package log

import (
	"fmt"
	"io"
	"os"
)

var (
	DebugWriter io.Writer = os.Stderr
	InfoWriter  io.Writer = os.Stderr
	WarnWriter  io.Writer = os.Stderr
	ErrorWriter io.Writer = os.Stderr
)

var (
	DebugPrefix string = "<7>[DEBUG]"
	InfoPrefix  string = "<6>[INFO]"
	WarnPrefix  string = "<4>[WARNING]"
	ErrPrefix   string = "<3>[ERROR]"
	FatalPrefix string = "<3>[FATAL]"
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
		v = append([]interface{}{DebugPrefix}, v...)
		fmt.Fprintln(DebugWriter, v...)
	}
}

func Info(v ...interface{}) {
	if InfoWriter != io.Discard {
		v = append([]interface{}{InfoPrefix}, v...)
		fmt.Fprintln(InfoWriter, v...)
	}
}

func Print(v ...interface{}) {
	Info(v...)
}

func Warn(v ...interface{}) {
	if WarnWriter != io.Discard {
		v = append([]interface{}{WarnPrefix}, v...)
		fmt.Fprintln(WarnWriter, v...)
	}
}

func Error(v ...interface{}) {
	if ErrorWriter != io.Discard {
		v = append([]interface{}{ErrPrefix}, v...)
		fmt.Fprintln(ErrorWriter, v...)
	}
}

func Fatal(v ...interface{}) {
	if ErrorWriter != io.Discard {
		v = append([]interface{}{FatalPrefix}, v...)
		fmt.Fprintln(ErrorWriter, v...)
	}
	os.Exit(1)
}

func Debugf(format string, v ...interface{}) {
	if DebugWriter != io.Discard {
		fmt.Fprintf(DebugWriter, DebugPrefix+" "+format+"\n", v...)
	}
}

func Infof(format string, v ...interface{}) {
	if InfoWriter != io.Discard {
		fmt.Fprintf(InfoWriter, InfoPrefix+" "+format+"\n", v...)
	}
}

func Printf(format string, v ...interface{}) {
	Infof(format, v...)
}

func Finfof(w io.Writer, format string, v ...interface{}) {
	if w != io.Discard {
		fmt.Fprintf(InfoWriter, InfoPrefix+" "+format+"\n", v...)
	}
}

func Warnf(format string, v ...interface{}) {
	if WarnWriter != io.Discard {
		fmt.Fprintf(WarnWriter, WarnPrefix+" "+format+"\n", v...)
	}
}

func Errorf(format string, v ...interface{}) {
	if ErrorWriter != io.Discard {
		fmt.Fprintf(ErrorWriter, ErrPrefix+" "+format+"\n", v...)
	}
}

func Fatalf(format string, v ...interface{}) {
	if ErrorWriter != io.Discard {
		fmt.Fprintf(ErrorWriter, FatalPrefix+" "+format+"\n", v...)
	}
	os.Exit(1)
}
