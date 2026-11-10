package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Level is the severity of a log message.
type Level int

// Severity levels used by the tool.
const (
	LevelError Level = iota
	LevelWarn
	LevelInfo
	LevelDebug
)

// ParseLevel converts a level name into a Level. Unknown names fall back to
// LevelInfo so a typo never silences the tool completely.
func ParseLevel(name string) Level {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "error":
		return LevelError
	case "warn", "warning":
		return LevelWarn
	case "info":
		return LevelInfo
	case "debug", "trace":
		return LevelDebug
	default:
		return LevelInfo
	}
}

// Logger prints progress messages. Informational output goes to stdout while
// warnings and errors go to stderr, so stdout stays usable for piping.
type Logger struct {
	out io.Writer
	err io.Writer
	min Level
}

// NewLogger builds a logger writing to the given streams. Messages below min
// are discarded.
func NewLogger(out, err io.Writer, min Level) *Logger {
	return &Logger{out: out, err: err, min: min}
}

// DefaultLogger returns a logger writing to the process streams.
func DefaultLogger(min Level) *Logger {
	return NewLogger(os.Stdout, os.Stderr, min)
}

func (l *Logger) logf(level Level, name string, format string, args ...any) {
	if l == nil || level > l.min {
		return
	}
	msg := fmt.Sprintf(format, args...)
	w := l.out
	if level <= LevelWarn {
		w = l.err
	}
	fmt.Fprintf(w, "%s: %s\n", name, msg)
}

// Errorf logs an error message.
func (l *Logger) Errorf(format string, args ...any) {
	l.logf(LevelError, "error", format, args...)
}

// Warnf logs a warning message.
func (l *Logger) Warnf(format string, args ...any) {
	l.logf(LevelWarn, "warning", format, args...)
}

// Infof logs an informational message.
func (l *Logger) Infof(format string, args ...any) {
	l.logf(LevelInfo, "mirror-clone", format, args...)
}

// Debugf logs a message that is only interesting while troubleshooting.
func (l *Logger) Debugf(format string, args ...any) {
	l.logf(LevelDebug, "debug", format, args...)
}
