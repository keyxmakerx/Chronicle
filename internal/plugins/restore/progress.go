package restore

import "bytes"

// progressWriter splits the script's stdout into lines and hands each whole
// line to onLine, so the status poll can say which step is running while the
// script is still going. It never fails a write: progress is a convenience,
// and a refusal here would break the script's pipe.
type progressWriter struct {
	onLine func(string)
	buf    []byte
}

func (w *progressWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(bytes.TrimRight(w.buf[:i], "\r"))
		w.buf = w.buf[i+1:]
		if w.onLine != nil {
			w.onLine(line)
		}
	}
	// A line that never ends must not grow without bound.
	if len(w.buf) > 8192 {
		w.buf = w.buf[:0]
	}
	return len(p), nil
}
