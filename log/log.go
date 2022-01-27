// Provides a simple way of logging with different levels.
// Time/Data are not logged on purpose because systemd adds
// them for us.
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
		v = append([]interface{}{"[DEBUG]"}, v...)
		fmt.Fprintln(DebugWriter, v...)
	}
}

func Info(v ...interface{}) {
	if InfoWriter != io.Discard {
		v = append([]interface{}{"[INFO]"}, v...)
		fmt.Fprintln(InfoWriter, v...)
	}
}

func Print(v ...interface{}) {
	Info(v...)
}

func Warn(v ...interface{}) {
	if WarnWriter != io.Discard {
		v = append([]interface{}{"[WARNING]"}, v...)
		fmt.Fprintln(WarnWriter, v...)
	}
}

func Error(v ...interface{}) {
	if ErrorWriter != io.Discard {
		v = append([]interface{}{"[ERROR]"}, v...)
		fmt.Fprintln(ErrorWriter, v...)
	}
}

func Fatal(v ...interface{}) {
	if ErrorWriter != io.Discard {
		v = append([]interface{}{"[FATAL]"}, v...)
		fmt.Fprintln(ErrorWriter, v...)
	}
}

func Debugf(format string, v ...interface{}) {
	if DebugWriter != io.Discard {
		fmt.Fprintf(DebugWriter, "[DEBUG] "+format+"\n", v...)
	}
}

func Infof(format string, v ...interface{}) {
	if InfoWriter != io.Discard {
		fmt.Fprintf(InfoWriter, "[INFO] "+format+"\n", v...)
	}
}

func Printf(format string, v ...interface{}) {
	Infof(format, v...)
}

func Warnf(format string, v ...interface{}) {
	if WarnWriter != io.Discard {
		fmt.Fprintf(WarnWriter, "[WARNING] "+format+"\n", v...)
	}
}

func Errorf(format string, v ...interface{}) {
	if ErrorWriter != io.Discard {
		fmt.Fprintf(ErrorWriter, "[ERROR] "+format+"\n", v...)
	}
}

func Fatalf(format string, v ...interface{}) {
	if ErrorWriter != io.Discard {
		fmt.Fprintf(ErrorWriter, "[FATAL] "+format+"\n", v...)
	}
}
