package backup

// capBuf is a write-capped byte buffer. We pipe the script's stdout/stderr
// through one of these so a runaway backup script (e.g. mysqldump dumping
// gigabytes of warnings to stderr) cannot OOM the chronicle process.
// Once the cap is hit, additional writes are silently dropped — the
// admin UI will show what we captured plus an "(output truncated)"
// marker that the handler appends.
type capBuf struct {
	max int
	buf []byte
}

func newCapBuf(max int) *capBuf { return &capBuf{max: max} }

func (b *capBuf) Write(p []byte) (int, error) {
	// Always claim we wrote everything so the child process doesn't
	// block on a "short write" io error.
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
