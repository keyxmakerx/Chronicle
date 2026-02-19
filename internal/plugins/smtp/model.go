// Package smtp provides outbound email functionality for Chronicle.
// SMTP settings are stored in the database and managed by site admins.
// The encrypted password is NEVER returned to the UI -- only a boolean
// indicating whether a password is configured.
package smtp

import "time"

// SMTPSettings holds the SMTP configuration. This is what the service layer
// and handlers work with. The password is intentionally omitted -- use
// HasPassword to show whether one is set.
type SMTPSettings struct {
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	Username    string    `json:"username"`
	HasPassword bool      `json:"has_password"` // True if encrypted password exists.
	FromAddress string    `json:"from_address"`
	FromName    string    `json:"from_name"`
	Encryption  string    `json:"encryption"` // "starttls", "ssl", or "none".
	Enabled     bool      `json:"enabled"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// smtpRow is the raw database row including encrypted password bytes.
// Internal only -- never exposed outside the repository.
type smtpRow struct {
	Host              string
	Port              int
	Username          string
	PasswordEncrypted []byte // AES-256-GCM encrypted, nil if not set.
	FromAddress       string
	FromName          string
	Encryption        string
	Enabled           bool
	UpdatedAt         time.Time
}

// toSettings converts a database row to the safe SMTPSettings struct.
func (r *smtpRow) toSettings() *SMTPSettings {
	return &SMTPSettings{
		Host:        r.Host,
		Port:        r.Port,
		Username:    r.Username,
		HasPassword: len(r.PasswordEncrypted) > 0,
		FromAddress: r.FromAddress,
		FromName:    r.FromName,
		Encryption:  r.Encryption,
		Enabled:     r.Enabled,
		UpdatedAt:   r.UpdatedAt,
	}
}

// UpdateSMTPRequest holds form data for updating SMTP settings.
// Password is optional -- empty means "keep existing".
type UpdateSMTPRequest struct {
	Host        string `json:"host" form:"host"`
	Port        int    `json:"port" form:"port"`
	Username    string `json:"username" form:"username"`
	Password    string `json:"password" form:"password"` // Empty = keep existing.
	FromAddress string `json:"from_address" form:"from_address"`
	FromName    string `json:"from_name" form:"from_name"`
	Encryption  string `json:"encryption" form:"encryption"`
	Enabled     bool   `json:"enabled" form:"enabled"`
}

// Mail represents an email message to be sent.
type Mail struct {
	To      []string
	Subject string
	Body    string
}
