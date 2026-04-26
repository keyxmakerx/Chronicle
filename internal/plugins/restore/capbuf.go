package restore

// capBuf is a write-capped byte buffer used to capture restore.sh
// stdout/stderr without unbounded memory growth. Writes past the cap
// are silently dropped; the child never sees a short-write error.
//
// Duplicated rather than imported from the backup plugin because the
// two plugins ship as independent PRs; once both land we can extract a
// shared helper. The duplication is ~25 lines and trivially testable.
type capBuf struct {
	max int
	buf []byte
}

func newCapBuf(max int) *capBuf { return &capBuf{max: max} }

func (b *capBuf) Write(p []byte) (int, error) {
	if len(b.buf) >= b.max {
		return len(p), nil
	}
	room := b.max - len(b.buf)
	if room >= len(p) {
		b.buf = append(b.buf, p...)
		return len(p), nil
	}
	b.buf = append(b.buf, p[:room]...)
	return len(p), nil
}

func (b *capBuf) String() string { return string(b.buf) }
