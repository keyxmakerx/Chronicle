// Package auth handles user authentication, session management, and password
// security for Chronicle. It provides registration, login, logout, and session
// validation via PASETO v4 tokens stored in Redis.
//
// This is a CORE plugin -- always enabled, cannot be disabled.
package auth

import (
	"time"
)

// User represents a registered Chronicle user. This is the domain model used
// throughout the application. Database scanning and JSON marshaling use this
// struct directly.
type User struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	DisplayName  string     `json:"display_name"`
	PasswordHash string     `json:"-"` // Never expose in JSON responses.
	AvatarPath   *string    `json:"avatar_path,omitempty"`
	IsAdmin      bool       `json:"is_admin"`
	TOTPSecret   *string    `json:"-"` // Never expose.
	TOTPEnabled  bool       `json:"totp_enabled"`
	CreatedAt    time.Time  `json:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

// --- Request DTOs (bound from HTTP requests) ---

// RegisterRequest holds the data submitted by the registration form.
type RegisterRequest struct {
	Email       string `json:"email" form:"email" validate:"required,email,max=255"`
	DisplayName string `json:"display_name" form:"display_name" validate:"required,min=2,max=100"`
	Password    string `json:"password" form:"password" validate:"required,min=8,max=128"`
	Confirm     string `json:"confirm" form:"confirm" validate:"required,eqfield=Password"`
}

// LoginRequest holds the data submitted by the login form.
type LoginRequest struct {
	Email    string `json:"email" form:"email" validate:"required,email"`
	Password string `json:"password" form:"password" validate:"required"`
}

// --- Service Input DTOs (passed from handler to service) ---

// RegisterInput is the validated input for creating a new user.
type RegisterInput struct {
	Email       string
	DisplayName string
	Password    string
}

// LoginInput is the validated input for authenticating a user.
type LoginInput struct {
	Email    string
	Password string
}

// --- Session ---

// Session represents an authenticated user session stored in Redis.
// The session ID is the key, and this struct is the value (JSON-encoded).
type Session struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}
