package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	gosmtp "net/smtp"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// MailService is the interface other plugins use to send email.
// This is the cross-plugin contract -- campaigns uses this for transfer emails.
type MailService interface {
	SendMail(ctx context.Context, to []string, subject, body string) error
	IsConfigured(ctx context.Context) bool
}

// SMTPService extends MailService with admin settings management.
type SMTPService interface {
	MailService

	// GetSettings returns the SMTP configuration (password redacted).
	GetSettings(ctx context.Context) (*SMTPSettings, error)

	// UpdateSettings saves new SMTP settings. Empty password keeps existing.
	UpdateSettings(ctx context.Context, req UpdateSMTPRequest) error

	// TestConnection verifies SMTP connectivity with current settings.
	TestConnection(ctx context.Context) error
}

// smtpService implements SMTPService.
type smtpService struct {
	repo   SMTPRepository
	secret string // Application secret key for password encryption.
}

// NewSMTPService creates a new SMTP service.
func NewSMTPService(repo SMTPRepository, secretKey string) SMTPService {
	return &smtpService{
		repo:   repo,
		secret: secretKey,
	}
}

// --- MailService (cross-plugin interface) ---

// IsConfigured returns true if SMTP is enabled and has a host configured.
func (s *smtpService) IsConfigured(ctx context.Context) bool {
	row, err := s.repo.Get(ctx)
	if err != nil {
		return false
	}
	return row.Enabled && row.Host != ""
}

// SendMail sends an email using the stored SMTP settings. Decrypts the
// password at send time -- never caches plaintext credentials.
func (s *smtpService) SendMail(ctx context.Context, to []string, subject, body string) error {
	row, err := s.repo.Get(ctx)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading smtp settings: %w", err))
	}
	if !row.Enabled || row.Host == "" {
		return apperror.NewBadRequest("SMTP is not configured")
	}

	// Decrypt password at send time.
	var password string
	if len(row.PasswordEncrypted) > 0 {
		plaintext, err := decrypt(row.PasswordEncrypted, s.secret)
		if err != nil {
			return apperror.NewInternal(fmt.Errorf("decrypting smtp password: %w", err))
		}
		password = string(plaintext)
	}

	from := mail.Address{Name: row.FromName, Address: row.FromAddress}

	// Build RFC 2822 message.
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from.String()))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(to, ", ")))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z)))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	addr := fmt.Sprintf("%s:%d", row.Host, row.Port)

	// Send based on encryption mode.
	switch row.Encryption {
	case "ssl":
		return s.sendSSL(addr, row.Host, row.Username, password, from.Address, to, msg.String())
	case "none":
		return s.sendPlain(addr, row.Host, row.Username, password, from.Address, to, msg.String())
	default: // "starttls"
		return s.sendStartTLS(addr, row.Host, row.Username, password, from.Address, to, msg.String())
	}
}

// sendStartTLS sends email using STARTTLS (port 587 typical).
func (s *smtpService) sendStartTLS(addr, host, username, password, from string, to []string, msg string) error {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", addr, err)
	}
	defer conn.Close()

	client, err := gosmtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("creating smtp client: %w", err)
	}
	defer client.Close()

	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("starting TLS: %w", err)
	}

	if username != "" {
		auth := gosmtp.PlainAuth("", username, password, host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authenticating: %w", err)
		}
	}

	return s.sendMessage(client, from, to, msg)
}

// sendSSL sends email using implicit SSL/TLS (port 465 typical).
func (s *smtpService) sendSSL(addr, host, username, password, from string, to []string, msg string) error {
	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("connecting to %s (SSL): %w", addr, err)
	}
	defer conn.Close()

	client, err := gosmtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("creating smtp client: %w", err)
	}
	defer client.Close()

	if username != "" {
		auth := gosmtp.PlainAuth("", username, password, host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authenticating: %w", err)
		}
	}

	return s.sendMessage(client, from, to, msg)
}

// sendPlain sends email without encryption.
func (s *smtpService) sendPlain(addr, host, username, password, from string, to []string, msg string) error {
	var auth gosmtp.Auth
	if username != "" {
		auth = gosmtp.PlainAuth("", username, password, host)
	}
	if err := gosmtp.SendMail(addr, auth, from, to, []byte(msg)); err != nil {
		return fmt.Errorf("sending mail: %w", err)
	}
	return nil
}

// sendMessage handles MAIL FROM, RCPT TO, DATA for an existing SMTP client.
func (s *smtpService) sendMessage(client *gosmtp.Client, from string, to []string, msg string) error {
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("RCPT TO %s: %w", recipient, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("writing message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing data: %w", err)
	}
	return client.Quit()
}

// --- SMTPService (admin management) ---

// GetSettings returns SMTP settings with the password redacted.
func (s *smtpService) GetSettings(ctx context.Context) (*SMTPSettings, error) {
	row, err := s.repo.Get(ctx)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("loading smtp settings: %w", err))
	}
	return row.toSettings(), nil
}

// UpdateSettings saves SMTP settings. If the password field is empty,
// the existing encrypted password is preserved.
func (s *smtpService) UpdateSettings(ctx context.Context, req UpdateSMTPRequest) error {
	// Load current settings to preserve password if not changed.
	current, err := s.repo.Get(ctx)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading current smtp settings: %w", err))
	}

	row := &smtpRow{
		Host:        strings.TrimSpace(req.Host),
		Port:        req.Port,
		Username:    strings.TrimSpace(req.Username),
		FromAddress: strings.TrimSpace(req.FromAddress),
		FromName:    strings.TrimSpace(req.FromName),
		Encryption:  req.Encryption,
		Enabled:     req.Enabled,
	}

	if row.Port <= 0 {
		row.Port = 587
	}
	if row.FromName == "" {
		row.FromName = "Chronicle"
	}
	if row.Encryption == "" {
		row.Encryption = "starttls"
	}

	// Handle password: empty = keep existing, non-empty = encrypt + store.
	if req.Password != "" {
		encrypted, err := encrypt([]byte(req.Password), s.secret)
		if err != nil {
			return apperror.NewInternal(fmt.Errorf("encrypting smtp password: %w", err))
		}
		row.PasswordEncrypted = encrypted
	} else {
		// Preserve existing encrypted password.
		row.PasswordEncrypted = current.PasswordEncrypted
	}

	if err := s.repo.Upsert(ctx, row); err != nil {
		return apperror.NewInternal(fmt.Errorf("saving smtp settings: %w", err))
	}

	slog.Info("smtp settings updated",
		slog.String("host", row.Host),
		slog.Int("port", row.Port),
		slog.Bool("enabled", row.Enabled),
	)
	return nil
}

// TestConnection verifies SMTP connectivity by establishing a connection
// and performing the EHLO handshake.
func (s *smtpService) TestConnection(ctx context.Context) error {
	row, err := s.repo.Get(ctx)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading smtp settings: %w", err))
	}
	if row.Host == "" {
		return apperror.NewBadRequest("SMTP host is not configured")
	}

	addr := fmt.Sprintf("%s:%d", row.Host, row.Port)

	// Decrypt password for authentication test.
	var password string
	if len(row.PasswordEncrypted) > 0 {
		plaintext, err := decrypt(row.PasswordEncrypted, s.secret)
		if err != nil {
			return apperror.NewInternal(fmt.Errorf("decrypting smtp password: %w", err))
		}
		password = string(plaintext)
	}

	switch row.Encryption {
	case "ssl":
		return s.testSSL(addr, row.Host, row.Username, password)
	default: // "starttls" or "none"
		return s.testStartTLS(addr, row.Host, row.Username, password, row.Encryption == "starttls")
	}
}

// testStartTLS tests connectivity with optional STARTTLS.
func (s *smtpService) testStartTLS(addr, host, username, password string, useTLS bool) error {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return apperror.NewBadRequest(fmt.Sprintf("could not connect to %s: %v", addr, err))
	}
	defer conn.Close()

	client, err := gosmtp.NewClient(conn, host)
	if err != nil {
		return apperror.NewBadRequest(fmt.Sprintf("SMTP handshake failed: %v", err))
	}
	defer client.Close()

	if useTLS {
		tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
		if err := client.StartTLS(tlsConfig); err != nil {
			return apperror.NewBadRequest(fmt.Sprintf("STARTTLS failed: %v", err))
		}
	}

	if username != "" {
		auth := gosmtp.PlainAuth("", username, password, host)
		if err := client.Auth(auth); err != nil {
			return apperror.NewBadRequest(fmt.Sprintf("authentication failed: %v", err))
		}
	}

	return client.Quit()
}

// testSSL tests connectivity with implicit SSL/TLS.
func (s *smtpService) testSSL(addr, host, username, password string) error {
	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, tlsConfig)
	if err != nil {
		return apperror.NewBadRequest(fmt.Sprintf("could not connect to %s (SSL): %v", addr, err))
	}
	defer conn.Close()

	client, err := gosmtp.NewClient(conn, host)
	if err != nil {
		return apperror.NewBadRequest(fmt.Sprintf("SMTP handshake failed: %v", err))
	}
	defer client.Close()

	if username != "" {
		auth := gosmtp.PlainAuth("", username, password, host)
		if err := client.Auth(auth); err != nil {
			return apperror.NewBadRequest(fmt.Sprintf("authentication failed: %v", err))
		}
	}

	return client.Quit()
}
