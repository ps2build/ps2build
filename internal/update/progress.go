package update

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type progressReader struct {
	io.Reader
	label       string
	total       int64 // <= 0 means unknown (no Content-Length from the server)
	read        int64
	lastRender  time.Time
	lastLineLen int // rendered length of the previous line, to pad over on shrink
}

func newProgressReader(r io.Reader, label string, total int64) *progressReader {
	return &progressReader{Reader: r, label: label, total: total}
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.Reader.Read(buf)
	if n > 0 {
		p.read += int64(n)
		if time.Since(p.lastRender) >= 100*time.Millisecond {
			p.render()
			p.lastRender = time.Now()
		}
	}
	if err != nil {
		p.render() // final state (e.g. 100%) before handing the error/EOF upstream
		fmt.Fprintln(os.Stderr)
	}
	return n, err
}

func (p *progressReader) render() {
	const width = 30
	var line string
	if p.total > 0 {
		frac := float64(p.read) / float64(p.total)
		if frac > 1 {
			frac = 1
		}
		filled := int(frac * width)
		bar := strings.Repeat("=", filled) + strings.Repeat(" ", width-filled)
		line = fmt.Sprintf("  %-10s [%s] %3.0f%%  %s/%s", p.label, bar, frac*100, humanBytes(p.read), humanBytes(p.total))
	} else {
		line = fmt.Sprintf("  %-10s %s", p.label, humanBytes(p.read))
	}
	out, newLen := padOverPrevious(line, p.lastLineLen)
	fmt.Fprint(os.Stderr, "\r"+out)
	p.lastLineLen = newLen
}

func padOverPrevious(line string, prevLen int) (out string, newLen int) {
	out = line
	if pad := prevLen - len(line); pad > 0 {
		out += strings.Repeat(" ", pad)
	}
	return out, len(line)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}
