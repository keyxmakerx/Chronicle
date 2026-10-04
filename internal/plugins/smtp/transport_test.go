package smtp

import (
	"bufio"
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// stubRepo serves one fixed settings row.
type stubRepo struct{ row *smtpRow }

func (r *stubRepo) Get(context.Context) (*smtpRow, error) { return r.row, nil }
func (r *stubRepo) Upsert(_ context.Context, row *smtpRow) error {
	r.row = row
	return nil
}

func listen(t *testing.T) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln, ln.Addr().(*net.TCPAddr).Port
}

// A server that never greets stands in for STARTTLS pointed at an
// implicit-TLS port: the send must fail, not hang.
func TestSend_SilentServerTimesOut(t *testing.T) {
	old := sessionTimeout
	sessionTimeout = 300 * time.Millisecond
	t.Cleanup(func() { sessionTimeout = old })

	ln, port := listen(t)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close()
		}
	}()

	svc := NewSMTPService(&stubRepo{row: &smtpRow{
		Host: "127.0.0.1", Port: port, Encryption: "starttls", Enabled: true,
		FromAddress: "chronicle@example.com",
	}}, "secret")

	done := make(chan error, 1)
	go func() { done <- svc.SendMail(context.Background(), []string{"a@example.com"}, "s", "b") }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "no SMTP greeting") {
			t.Fatalf("err = %v, want a no-greeting error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("send hung")
	}
}

// A minimal plain SMTP server receives one message end to end.
func TestSend_DeliversToPlainServer(t *testing.T) {
	ln, port := listen(t)
	got := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		w := func(s string) { c.Write([]byte(s + "\r\n")) }
		w("220 test ESMTP")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					w("250 queued")
					continue
				}
				data.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"):
				w("250 test")
			case cmd == "DATA":
				inData = true
				w("354 go")
			case cmd == "QUIT":
				w("221 bye")
				got <- data.String()
				return
			default:
				w("250 ok")
			}
		}
	}()

	svc := NewSMTPService(&stubRepo{row: &smtpRow{
		Host: "127.0.0.1", Port: port, Encryption: "none", Enabled: true,
		FromAddress: "chronicle@example.com", FromName: "Chronicle",
	}}, "secret")
	if err := svc.SendHTMLMail(context.Background(), []string{"a@example.com"}, "Invite — 🎲", "plain", "<p>html</p>"); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-got:
		if !strings.Contains(msg, "Subject: =?utf-8?q?") || !strings.Contains(msg, "Message-ID: <") {
			t.Fatalf("unexpected message:\n%s", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server never got the message")
	}
}

func TestUpdateSettings_Port(t *testing.T) {
	tests := []struct {
		port     int
		wantPort int
		wantErr  bool
	}{
		{0, 587, false},
		{465, 465, false},
		{2525, 2525, false},
		{1025, 0, true},
	}
	for _, tc := range tests {
		t.Run(strconv.Itoa(tc.port), func(t *testing.T) {
			repo := &stubRepo{row: &smtpRow{}}
			err := NewSMTPService(repo, "secret").UpdateSettings(context.Background(), UpdateSMTPRequest{Port: tc.port})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && repo.row.Port != tc.wantPort {
				t.Fatalf("saved port %d, want %d", repo.row.Port, tc.wantPort)
			}
		})
	}
}
