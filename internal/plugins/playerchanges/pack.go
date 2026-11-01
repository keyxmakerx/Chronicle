package playerchanges

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
)

// maxUnpacked bounds what one packed month may grow to when unpacked, so a
// damaged or hostile blob cannot exhaust memory.
const maxUnpacked = 256 << 20

// pack turns a month's changes into one gzip'd JSON array.
func pack(rows []Change) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	if err := json.NewEncoder(zw).Encode(rows); err != nil {
		return nil, fmt.Errorf("packing changes: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("packing changes: %w", err)
	}
	return buf.Bytes(), nil
}

// unpack reads a packed month back.
func unpack(body []byte) ([]Change, error) {
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("unpacking changes: %w", err)
	}
	defer zr.Close()
	raw, err := io.ReadAll(io.LimitReader(zr, maxUnpacked+1))
	if err != nil {
		return nil, fmt.Errorf("unpacking changes: %w", err)
	}
	if len(raw) > maxUnpacked {
		return nil, fmt.Errorf("unpacking changes: a packed month is too large")
	}
	var rows []Change
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("unpacking changes: %w", err)
	}
	return rows, nil
}
