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

var DebugWriter io.Writer = os.Stderr
var InfoWriter io.Writer = os.Stderr
var WarnWriter io.Writer = os.Stderr
var ErrorWriter io.Writer = os.Stderr

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
		v = append([]interface{}{"<7>[DEBUG]"}, v...)
		fmt.Fprintln(DebugWriter, v...)
	}
}

func Info(v ...interface{}) {
	if InfoWriter != io.Discard {
		v = append([]interface{}{"<6>[INFO]"}, v...)
		fmt.Fprintln(InfoWriter, v...)
	}
}

func Print(v ...interface{}) {
	Info(v...)
}

func Warn(v ...interface{}) {
	if WarnWriter != io.Discard {
		v = append([]interface{}{"<4>[WARNING]"}, v...)
		fmt.Fprintln(WarnWriter, v...)
	}
}

func Error(v ...interface{}) {
	if ErrorWriter != io.Discard {
		v = append([]interface{}{"<3>[ERROR]"}, v...)
		fmt.Fprintln(ErrorWriter, v...)
	}
}

func Fatal(v ...interface{}) {
	if ErrorWriter != io.Discard {
		v = append([]interface{}{"<0>[FATAL]"}, v...)
		fmt.Fprintln(ErrorWriter, v...)
	}
	os.Exit(1)
}

func Debugf(format string, v ...interface{}) {
	if DebugWriter != io.Discard {
		fmt.Fprintf(DebugWriter, "<7>[DEBUG] "+format+"\n", v...)
	}
}

func Infof(format string, v ...interface{}) {
	if InfoWriter != io.Discard {
		fmt.Fprintf(InfoWriter, "<6>[INFO] "+format+"\n", v...)
	}
}

func Printf(format string, v ...interface{}) {
	Infof(format, v...)
}

func Warnf(format string, v ...interface{}) {
	if WarnWriter != io.Discard {
		fmt.Fprintf(WarnWriter, "<4>[WARNING] "+format+"\n", v...)
	}
}

func Errorf(format string, v ...interface{}) {
	if ErrorWriter != io.Discard {
		fmt.Fprintf(ErrorWriter, "<3>[ERROR] "+format+"\n", v...)
	}
}

func Fatalf(format string, v ...interface{}) {
	if ErrorWriter != io.Discard {
		fmt.Fprintf(ErrorWriter, "<0>[FATAL] "+format+"\n", v...)
	}
	os.Exit(1)
}
