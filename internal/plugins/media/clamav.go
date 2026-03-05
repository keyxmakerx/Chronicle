// clamav.go implements an antivirus scanner using ClamAV's clamd TCP protocol.
// Files are scanned in-memory via the INSTREAM command before being written to
// disk. If clamd is not reachable, uploads proceed without scanning (fail-open)
// but a warning is logged. Configure via CLAMAV_ADDRESS environment variable.

package media

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// VirusScanner scans file bytes for malware before storage.
type VirusScanner interface {
	// Scan checks file content for threats. Returns nil if clean, an apperror
	// if a threat is detected, or nil with a logged warning on scanner failure.
	Scan(data []byte, filename string) error
}

// ClamAVScanner connects to a clamd daemon via TCP to scan uploads.
type ClamAVScanner struct {
	// Address is the clamd TCP address (e.g. "chronicle-clamav:3310").
	Address string
	// Timeout for the scan operation (default 30s).
	Timeout time.Duration
}

// NewClamAVScanner creates a scanner configured for the given clamd address.
// Returns nil if address is empty (scanning disabled).
func NewClamAVScanner(address string) *ClamAVScanner {
	if address == "" {
		return nil
	}
	return &ClamAVScanner{
		Address: address,
		Timeout: 30 * time.Second,
	}
}

// Scan sends file bytes to clamd via the INSTREAM protocol and checks the
// response. Returns nil if clean, a BadRequest error if infected, or nil
// (with a logged warning) if clamd is unavailable (fail-open policy).
func (s *ClamAVScanner) Scan(data []byte, filename string) error {
	conn, err := net.DialTimeout("tcp", s.Address, 5*time.Second)
	if err != nil {
		slog.Warn("[clamav] Scanner unavailable, skipping scan",
			"address", s.Address, "error", err, "file", filename)
		return nil // Fail-open: allow upload when scanner is down.
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(s.Timeout)); err != nil {
		slog.Warn("[clamav] Failed to set deadline", "error", err)
		return nil
	}

	// Send INSTREAM command.
	if _, err := conn.Write([]byte("zINSTREAM\x00")); err != nil {
		slog.Warn("[clamav] Failed to send command", "error", err)
		return nil
	}

	// Send file data in chunks. ClamAV INSTREAM protocol uses:
	// [4-byte big-endian length][chunk data] ... [4-byte zero] to terminate.
	const chunkSize = 1024 * 1024 // 1MB chunks.
	reader := bytes.NewReader(data)
	buf := make([]byte, chunkSize)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			// Write chunk length (big-endian uint32).
			lenBuf := make([]byte, 4)
			binary.BigEndian.PutUint32(lenBuf, uint32(n))
			if _, err := conn.Write(lenBuf); err != nil {
				slog.Warn("[clamav] Failed to write chunk length", "error", err)
				return nil
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				slog.Warn("[clamav] Failed to write chunk data", "error", err)
				return nil
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			slog.Warn("[clamav] Failed to read file data", "error", err)
			return nil
		}
	}

	// Send terminating zero-length chunk.
	if _, err := conn.Write([]byte{0, 0, 0, 0}); err != nil {
		slog.Warn("[clamav] Failed to send terminator", "error", err)
		return nil
	}

	// Read response.
	var resp bytes.Buffer
	if _, err := io.Copy(&resp, conn); err != nil {
		slog.Warn("[clamav] Failed to read response", "error", err)
		return nil
	}

	result := resp.String()

	// ClamAV response format: "stream: OK\0" or "stream: <virus> FOUND\0"
	if len(result) > 0 && result[len(result)-1] == 0 {
		result = result[:len(result)-1]
	}

	if result == "stream: OK" {
		slog.Debug("[clamav] File clean", "file", filename)
		return nil
	}

	// File is infected.
	slog.Error("[clamav] THREAT DETECTED",
		"file", filename,
		"result", result,
	)
	return apperror.NewBadRequest(fmt.Sprintf("file rejected: malware detected (%s)", result))
}
