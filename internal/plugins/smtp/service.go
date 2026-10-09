package smtp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	gosmtp "net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// MailService is the interface other plugins use to send email.
// This is the cross-plugin contract -- campaigns uses this for transfer emails.
type MailService interface {
	SendMail(ctx context.Context, to []string, subject, body string) error
	SendHTMLMail(ctx context.Context, to []string, subject, plainBody, htmlBody string) error
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

	// SendTestEmail sends a test email to the given address using current settings.
	SendTestEmail(ctx context.Context, to string) error
}

// dialTimeout bounds the TCP (and implicit TLS) connect; sessionTimeout bounds
// the whole SMTP conversation after it. Vars so tests can shorten them.
var (
	dialTimeout    = 10 * time.Second
	sessionTimeout = 30 * time.Second
)

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

// SendMail sends a plain-text email using the stored SMTP settings.
func (s *smtpService) SendMail(ctx context.Context, to []string, subject, body string) error {
	return s.send(ctx, to, subject, body, "")
}

// SendHTMLMail sends a multipart/alternative email with both plain text and HTML
// variants. Email clients that support HTML will render the rich version;
// text-only clients fall back to the plain text.
func (s *smtpService) SendHTMLMail(ctx context.Context, to []string, subject, plainBody, htmlBody string) error {
	return s.send(ctx, to, subject, plainBody, htmlBody)
}

// send loads the settings, decrypts the password at send time (plaintext
// credentials are never cached) and delivers one message.
func (s *smtpService) send(ctx context.Context, to []string, subject, plainBody, htmlBody string) error {
	row, err := s.repo.Get(ctx)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading smtp settings: %w", err))
	}
	if !row.Enabled || row.Host == "" {
		return apperror.NewBadRequest("SMTP is not configured")
	}
	password, err := s.password(row)
	if err != nil {
		return err
	}

	from := mail.Address{Name: row.FromName, Address: row.FromAddress}
	msg, err := buildMessage(from, to, subject, plainBody, htmlBody, time.Now())
	if err != nil {
		return apperror.NewInternal(err)
	}

	client, err := dial(row)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := authenticate(client, row, password); err != nil {
		return err
	}
	return sendMessage(client, from.Address, to, msg)
}

// password decrypts the stored SMTP password; empty when none is set.
func (s *smtpService) password(row *smtpRow) (string, error) {
	if len(row.PasswordEncrypted) == 0 {
		return "", nil
	}
	plaintext, err := decrypt(row.PasswordEncrypted, s.secret)
	if err != nil {
		return "", apperror.NewInternal(fmt.Errorf("decrypting smtp password: %w", err))
	}
	return string(plaintext), nil
}

// dial connects and completes the handshake for the configured encryption.
// "none" still upgrades when the server offers STARTTLS, as net/smtp.SendMail
// does. The whole session runs under one deadline so a port and encryption
// that don't match (STARTTLS against an implicit-TLS port waits for a
// greeting that never comes) fail with an error instead of hanging.
func dial(row *smtpRow) (*gosmtp.Client, error) {
	addr := net.JoinHostPort(row.Host, strconv.Itoa(row.Port))
	dialer := &net.Dialer{Timeout: dialTimeout}
	tlsConfig := &tls.Config{ServerName: row.Host, MinVersion: tls.VersionTLS12}

	var conn net.Conn
	var err error
	if row.Encryption == "ssl" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", addr, err)
	}
	if err := conn.SetDeadline(time.Now().Add(sessionTimeout)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("connecting to %s: %w", addr, err)
	}

	client, err := gosmtp.NewClient(conn, row.Host)
	if err != nil {
		conn.Close()
		if isTimeout(err) {
			return nil, fmt.Errorf("no SMTP greeting from %s (check the port matches the encryption: 465 is SSL/TLS, 587 is STARTTLS): %w", addr, err)
		}
		return nil, fmt.Errorf("SMTP handshake with %s: %w", addr, err)
	}

	switch row.Encryption {
	case "ssl":
	case "none":
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfig); err != nil {
				client.Close()
				return nil, fmt.Errorf("starting TLS: %w", err)
			}
		}
	default: // "starttls"
		if err := client.StartTLS(tlsConfig); err != nil {
			client.Close()
			return nil, fmt.Errorf("starting TLS: %w", err)
		}
	}
	return client, nil
}

// authenticate logs in when a username is set. net/smtp refuses to send a
// password over an unencrypted connection, which only "none" can reach.
func authenticate(client *gosmtp.Client, row *smtpRow, password string) error {
	if row.Username == "" {
		return nil
	}
	if err := client.Auth(gosmtp.PlainAuth("", row.Username, password, row.Host)); err != nil {
		return fmt.Errorf("authenticating: %w", err)
	}
	return nil
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// sendMessage handles MAIL FROM, RCPT TO, DATA for an existing SMTP client.
func sendMessage(client *gosmtp.Client, from string, to []string, msg string) error {
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

	// Validate SMTP host to prevent SSRF attacks (connecting to internal
	// infrastructure via the SMTP test/send functionality).
	if row.Host != "" {
		if err := validateSMTPHost(row.Host); err != nil {
			return err
		}
	}

	// Restrict SMTP port to standard mail ports.
	// An empty port box falls back to 587; any other unlisted port is refused
	// out loud rather than silently replaced.
	if row.Port == 0 {
		row.Port = 587
	}
	validPorts := map[int]bool{25: true, 465: true, 587: true, 2525: true}
	if !validPorts[row.Port] {
		return apperror.NewBadRequest(fmt.Sprintf("port %d is not allowed; use 587 (STARTTLS), 465 (SSL/TLS), 25 or 2525", row.Port))
	}
	if row.FromName == "" {
		row.FromName = "Chronicle"
	}
	if row.Encryption == "" {
		row.Encryption = "starttls"
	}

	// Validate encryption mode against allowed values.
	validEncryptions := map[string]bool{"starttls": true, "ssl": true, "none": true}
	if !validEncryptions[row.Encryption] {
		return apperror.NewBadRequest("invalid encryption mode; must be starttls, ssl, or none")
	}

	// Validate from_address to prevent SMTP header injection via newlines.
	if row.FromAddress != "" {
		if _, err := mail.ParseAddress(row.FromAddress); err != nil {
			return apperror.NewBadRequest("invalid from address")
		}
	}
	// Reject newlines in from_name to prevent header injection.
	if strings.ContainsAny(row.FromName, "\r\n") {
		return apperror.NewBadRequest("from name contains invalid characters")
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

// TestConnection verifies SMTP connectivity: connect, encryption and login,
// the same path a real send takes.
func (s *smtpService) TestConnection(ctx context.Context) error {
	row, err := s.repo.Get(ctx)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading smtp settings: %w", err))
	}
	if row.Host == "" {
		return apperror.NewBadRequest("SMTP host is not configured")
	}
	password, err := s.password(row)
	if err != nil {
		return err
	}

	client, err := dial(row)
	if err != nil {
		slog.Warn("smtp test: connect failed", slog.String("host", row.Host), slog.Int("port", row.Port), slog.Any("error", err))
		return apperror.NewBadRequest(explainSendError(err))
	}
	defer client.Close()
	if err := authenticate(client, row, password); err != nil {
		slog.Warn("smtp test: authentication failed", slog.String("username", row.Username), slog.Any("error", err))
		return apperror.NewBadRequest(explainSendError(err))
	}
	slog.Info("smtp test: connection successful", slog.String("host", row.Host), slog.Int("port", row.Port))
	return client.Quit()
}

// SendTestEmail sends a test email to the given address using current SMTP settings.
// This verifies the full send pipeline (DNS, connect, TLS, auth, delivery).
func (s *smtpService) SendTestEmail(ctx context.Context, to string) error {
	if to == "" {
		return apperror.NewBadRequest("recipient email address is required")
	}
	if _, err := mail.ParseAddress(to); err != nil {
		return apperror.NewBadRequest("invalid recipient email address")
	}

	subject := "Chronicle SMTP Test"
	body := fmt.Sprintf("This is a test email from Chronicle.\n\nIf you are reading this, your SMTP configuration is working correctly.\n\nSent at: %s",
		time.Now().UTC().Format(time.RFC1123Z))

	if err := s.SendMail(ctx, []string{to}, subject, body); err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) {
			return err
		}
		slog.Warn("smtp test email failed", slog.Any("error", err))
		return apperror.NewBadRequest(explainSendError(err))
	}

	slog.Info("test email sent successfully", slog.String("to", to))
	return nil
}

// explainSendError turns a transport error into an admin-facing message that
// keeps the server's own words and adds the likely fix.
func explainSendError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "no SMTP greeting"):
		return msg + "."
	case strings.Contains(msg, "unencrypted connection"):
		return "The server does not offer encryption, so Chronicle won't send your password to it. Pick STARTTLS or SSL/TLS."
	case strings.Contains(msg, "authenticating"):
		return "Authentication failed: " + msg + ". Verify your username and password. Some providers require an app-specific password."
	case strings.Contains(msg, "TLS"):
		return "TLS error: " + msg + ". Try SSL/TLS with port 465, or STARTTLS with port 587."
	case strings.Contains(msg, "connecting to"):
		return "Connection failed: " + msg + ". Verify the host and port and that the server is reachable from Chronicle."
	default:
		return "Failed to send: " + msg
	}
}

// validateSMTPHost rejects SMTP hosts that resolve to private/reserved IP
// addresses to prevent SSRF attacks. An admin configuring SMTP to point at
// internal infrastructure (Redis, DB, metadata endpoints) could probe or
// attack internal services.
func validateSMTPHost(host string) error {
	// Reject obvious internal hostnames.
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".local") ||
		lower == "host.docker.internal" || lower == "kubernetes.default" {
		return apperror.NewBadRequest("SMTP host cannot be a local/internal address")
	}

	// Resolve the hostname and check all resulting IPs.
	ips, err := net.LookupHost(host)
	if err != nil {
		// DNS resolution failed — might be a typo. Allow the user to save
		// but they'll get a connection error when testing/sending.
		return nil
	}

	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return apperror.NewBadRequest(fmt.Sprintf(
				"SMTP host %q resolves to private/reserved IP %s; use a public mail server",
				host, ipStr,
			))
		}
		// Block AWS/cloud metadata endpoint (169.254.169.254).
		if ipStr == "169.254.169.254" {
			return apperror.NewBadRequest("SMTP host resolves to a cloud metadata endpoint")
		}
	}

	return nil
}
