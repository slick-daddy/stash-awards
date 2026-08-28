package protocol

import (
	"bytes"
	"strings"
	"testing"
)

// captureStderr runs fn against a Log writing to a buffer and returns the lines.
func captureStderr(t *testing.T, fn func(l *Log)) []string {
	t.Helper()
	var buf bytes.Buffer
	fn(NewLogTo(&buf))
	out := strings.TrimSuffix(buf.String(), "\n")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func TestLogEncodesLevelPrefix(t *testing.T) {
	lines := captureStderr(t, func(l *Log) {
		l.Info("hello %s", "world")
		l.Error("bad")
	})

	want := []string{"\x01i\x02hello world", "\x01e\x02bad"}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines %q, want %d", len(lines), lines, len(want))
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestLogTagsEveryLineOfMultilineMessage(t *testing.T) {
	// An untagged continuation line would be logged at the plugin's default
	// error level, so each line carries its own prefix.
	lines := captureStderr(t, func(l *Log) {
		l.Warn("first\nsecond\n")
	})

	if len(lines) != 2 {
		t.Fatalf("got %d lines %q, want 2", len(lines), lines)
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, "\x01w\x02") {
			t.Errorf("line %d = %q, missing warning prefix", i, line)
		}
	}
}

func TestProgressClamps(t *testing.T) {
	lines := captureStderr(t, func(l *Log) {
		l.Progress(-1)
		l.Progress(0.25)
		l.Progress(5)
	})

	want := []string{"\x01p\x020.000000", "\x01p\x020.250000", "\x01p\x021.000000"}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}
