package update

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0B"},
		{1023, "1023B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1024 * 1024, "1.0MB"},
		{223 * 1024 * 1024, "223.0MB"},
		{1024 * 1024 * 1024, "1.0GB"},
	}
	for _, tt := range tests {
		if got := humanBytes(tt.n); got != tt.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestProgressReader_PassesThroughBytesUnchanged(t *testing.T) {
	want := strings.Repeat("the quick brown fox jumps over the lazy dog\n", 1000)
	pr := newProgressReader(strings.NewReader(want), "test", int64(len(want)))

	var got bytes.Buffer
	n, err := io.Copy(&got, pr)
	if err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	if n != int64(len(want)) {
		t.Errorf("copied %d bytes, want %d", n, len(want))
	}
	if got.String() != want {
		t.Error("progressReader altered the data passing through it")
	}
	if pr.read != int64(len(want)) {
		t.Errorf("pr.read = %d, want %d (byte counter should track every byte actually read)", pr.read, len(want))
	}
}

func TestPadOverPrevious(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		prevLen int
		want    string
	}{
		{"no previous line", "abc", 0, "abc"},
		{"same length", "abc", 3, "abc"},
		{"grew: no padding needed", "abcdef", 3, "abcdef"},
		{"shrank: pad to previous length", "ab", 5, "ab   "},
	}
	for _, tt := range tests {
		out, newLen := padOverPrevious(tt.line, tt.prevLen)
		if out != tt.want {
			t.Errorf("padOverPrevious(%q, %d) out = %q, want %q", tt.line, tt.prevLen, out, tt.want)
		}
		if newLen != len(tt.line) {
			t.Errorf("padOverPrevious(%q, %d) newLen = %d, want %d (the line's own unpadded length, not the padded output's)", tt.line, tt.prevLen, newLen, len(tt.line))
		}
	}
}

func TestProgressReader_UnknownTotalDoesNotPanic(t *testing.T) {
	// total <= 0 (no Content-Length from the server) must fall back to
	// a plain byte-count display, not divide by zero.
	pr := newProgressReader(strings.NewReader("hello"), "test", 0)
	if _, err := io.Copy(io.Discard, pr); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
}
