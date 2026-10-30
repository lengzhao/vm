package compiler

import "bytes"

const maxDiagnosticBytes = 64 << 10

// diagnosticBuffer drains the tool's output even after the retained prefix is
// full, so truncation cannot block a compiler waiting to write stderr.
type diagnosticBuffer struct {
	buffer    bytes.Buffer
	truncated bool
}

func (b *diagnosticBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := maxDiagnosticBytes - b.buffer.Len()
	if n > remaining {
		b.truncated = true
		p = p[:remaining]
	}
	_, _ = b.buffer.Write(p)
	return n, nil
}

func (b *diagnosticBuffer) String() string {
	if b.truncated {
		return b.buffer.String() + "\n[diagnostics truncated]"
	}
	return b.buffer.String()
}
