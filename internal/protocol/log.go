package protocol

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"sync"
)

// Stash reads plugin log lines from stderr. A line is attributed to a log level
// when it starts with SOH, a single level character, then STX. Anything else is
// logged at the plugin's configured default level.
const (
	startLevelChar = byte(1)
	endLevelChar   = byte(2)
)

// Log writes level-tagged messages to stderr for Stash to pick up.
type Log struct {
	mu sync.Mutex
	w  io.Writer
}

// NewLog returns a Log writing to stderr.
func NewLog() *Log {
	return NewLogTo(os.Stderr)
}

// NewLogTo returns a Log writing to w. Stash only ever reads stderr; this exists
// for tests.
func NewLogTo(w io.Writer) *Log {
	return &Log{w: w}
}

func (l *Log) emit(level byte, msg string) {
	// A newline inside the message would make Stash treat the remainder as an
	// untagged line, so tag every line individually.
	l.mu.Lock()
	defer l.mu.Unlock()
	prefix := string([]byte{startLevelChar, level, endLevelChar})
	for _, line := range strings.Split(strings.TrimRight(msg, "\n"), "\n") {
		fmt.Fprintf(l.w, "%s%s\n", prefix, line)
	}
}

// Trace logs at trace level.
func (l *Log) Trace(format string, args ...interface{}) {
	l.emit('t', fmt.Sprintf(format, args...))
}

// Debug logs at debug level.
func (l *Log) Debug(format string, args ...interface{}) {
	l.emit('d', fmt.Sprintf(format, args...))
}

// Info logs at info level.
func (l *Log) Info(format string, args ...interface{}) {
	l.emit('i', fmt.Sprintf(format, args...))
}

// Warn logs at warning level.
func (l *Log) Warn(format string, args ...interface{}) {
	l.emit('w', fmt.Sprintf(format, args...))
}

// Error logs at error level.
func (l *Log) Error(format string, args ...interface{}) {
	l.emit('e', fmt.Sprintf(format, args...))
}

// Progress reports task completion between 0 and 1 to Stash's task queue.
// Values outside that range are clamped.
func (l *Log) Progress(p float64) {
	l.emit('p', fmt.Sprintf("%f", math.Min(math.Max(0, p), 1)))
}
