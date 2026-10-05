package smtp

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"
)

// buildMessage renders an RFC 5322 message. Bodies go out quoted-printable
// and the subject RFC 2047-encoded, so non-ASCII text (dashes, emoji) and long
// HTML lines stay 7-bit and under the 998-byte line limit that strict servers
// enforce. An empty htmlBody sends text/plain only.
func buildMessage(from mail.Address, to []string, subject, plainBody, htmlBody string, now time.Time) (string, error) {
	// Strip newlines from the subject to prevent header injection.
	safeSubject := strings.NewReplacer("\r", "", "\n", "").Replace(subject)

	var msg strings.Builder
	fmt.Fprintf(&msg, "From: %s\r\n", from.String())
	fmt.Fprintf(&msg, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(&msg, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", safeSubject))
	fmt.Fprintf(&msg, "Date: %s\r\n", now.UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&msg, "Message-ID: %s\r\n", messageID(from.Address))
	msg.WriteString("MIME-Version: 1.0\r\n")

	if htmlBody == "" {
		msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		msg.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		qp, err := encodeQP(plainBody)
		if err != nil {
			return "", err
		}
		msg.WriteString(qp)
		return msg.String(), nil
	}

	boundary := "chronicle_" + randomHex(12)
	fmt.Fprintf(&msg, "Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary)
	for _, part := range []struct{ ctype, body string }{
		{"text/plain", plainBody},
		{"text/html", htmlBody},
	} {
		qp, err := encodeQP(part.body)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&msg, "--%s\r\n", boundary)
		fmt.Fprintf(&msg, "Content-Type: %s; charset=UTF-8\r\n", part.ctype)
		msg.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		msg.WriteString(qp)
		msg.WriteString("\r\n")
	}
	fmt.Fprintf(&msg, "--%s--\r\n", boundary)
	return msg.String(), nil
}

// encodeQP quoted-printable encodes s; the writer turns bare \n into CRLF.
func encodeQP(s string) (string, error) {
	var buf bytes.Buffer
	w := quotedprintable.NewWriter(&buf)
	if _, err := w.Write([]byte(s)); err != nil {
		return "", fmt.Errorf("encoding body: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("encoding body: %w", err)
	}
	return buf.String(), nil
}

// messageID returns a unique Message-ID under the sender's domain. Some
// receiving servers (Gmail among them) refuse mail that arrives without one.
func messageID(fromAddr string) string {
	domain := "chronicle.invalid"
	if at := strings.LastIndex(fromAddr, "@"); at >= 0 && at < len(fromAddr)-1 {
		domain = fromAddr[at+1:]
	}
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), randomHex(8), domain)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
