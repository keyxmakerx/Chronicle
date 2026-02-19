// Package config handles loading application configuration from environment
// variables. All config is centralized here so no other package reads env
// vars directly. Sensible defaults are provided for development.
package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Config holds all application configuration. Populated from environment
// variables at startup. Passed to other packages via dependency injection.
type Config struct {
	// Env is the runtime environment: "development" or "production".
	Env string

	// Port is the HTTP listen port (default: 8080).
	Port int

	// BaseURL is the public-facing URL used for links and redirects.
	BaseURL string

	// LogLevel controls log verbosity: "debug", "info", "warn", "error".
	LogLevel string

	// Database holds MariaDB connection settings.
	Database DatabaseConfig

	// Redis holds Redis connection settings.
	Redis RedisConfig

	// Auth holds authentication-related settings.
	Auth AuthConfig

	// Upload holds file upload settings.
	Upload UploadConfig
}

// DatabaseConfig holds MariaDB connection parameters. Individual fields
// (Host, User, Password, Name) are read from separate env vars so
// container orchestrators like Cosmos Cloud can manage each independently.
// If DATABASE_URL is set, it takes precedence over the individual fields.
type DatabaseConfig struct {
	// Host is the MariaDB address in host:port format (default: "localhost:3306").
	// If no port is specified, 3306 is appended automatically.
	Host string

	// User is the MariaDB username (default: "chronicle").
	User string

	// Password is the MariaDB password (default: "chronicle").
	Password string

	// Name is the database name (default: "chronicle").
	Name string

	// dsnOverride is set when DATABASE_URL is provided, bypassing individual fields.
	dsnOverride string

	// MaxOpenConns is the maximum number of open connections in the pool.
	MaxOpenConns int

	// MaxIdleConns is the maximum number of idle connections in the pool.
	MaxIdleConns int

	// ConnMaxLifetime is how long a connection can be reused.
	ConnMaxLifetime time.Duration
}

// DSN returns the go-sql-driver/mysql connection string. If DATABASE_URL was
// set, it is returned as-is. Otherwise the DSN is built from the individual
// Host/User/Password/Name fields using the driver's Config.FormatDSN()
// to safely handle special characters in passwords.
func (d DatabaseConfig) DSN() string {
	if d.dsnOverride != "" {
		return d.dsnOverride
	}
	cfg := mysql.NewConfig()
	cfg.User = d.User
	cfg.Passwd = d.Password
	cfg.Net = "tcp"
	cfg.Addr = ensurePort(d.Host, "3306")
	cfg.DBName = d.Name
	cfg.ParseTime = true
	return cfg.FormatDSN()
}

// ensurePort appends the default port if the host string doesn't include one.
// Allows users to set DB_HOST=mydb (gets :3306) or DB_HOST=mydb:3307 (as-is).
func ensurePort(host, defaultPort string) string {
	_, _, err := net.SplitHostPort(host)
	if err != nil {
		return net.JoinHostPort(host, defaultPort)
	}
	return host
}

// RedisConfig holds Redis connection parameters.
type RedisConfig struct {
	// URL is the Redis connection URL (e.g., "redis://localhost:6379").
	URL string
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	// SecretKey is the PASETO signing key (must be 32+ bytes, base64-encoded).
	SecretKey string

	// SessionTTL is how long sessions last before expiring.
	SessionTTL time.Duration
}

// UploadConfig holds file upload settings.
type UploadConfig struct {
	// MaxSize is the maximum upload file size in bytes.
	MaxSize int64

	// MediaPath is the root directory for media file storage.
	MediaPath string
}

// Load reads configuration from environment variables with sensible defaults.
// Returns an error if required variables are missing.
func Load() (*Config, error) {
	cfg := &Config{
		Env:      getEnv("ENV", "development"),
		Port:     getEnvInt("PORT", 8080),
		BaseURL:  getEnv("BASE_URL", "http://localhost:8080"),
		LogLevel: getEnv("LOG_LEVEL", "debug"),

		Database: DatabaseConfig{
			Host:            getEnv("DB_HOST", "localhost:3306"),
			User:            getEnv("DB_USER", "chronicle"),
			Password:        getEnv("DB_PASSWORD", "chronicle"),
			Name:            getEnv("DB_NAME", "chronicle"),
			dsnOverride:     getEnv("DATABASE_URL", ""),
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		},

		Redis: RedisConfig{
			URL: getEnv("REDIS_URL", "redis://localhost:6379"),
		},

		Auth: AuthConfig{
			SecretKey:  getEnv("SECRET_KEY", ""),
			SessionTTL: getEnvDuration("SESSION_TTL", 720*time.Hour),
		},

		Upload: UploadConfig{
			MaxSize:   getEnvInt64("MAX_UPLOAD_SIZE", 10*1024*1024), // 10MB
			MediaPath: getEnv("MEDIA_PATH", "./media"),
		},
	}

	// Validate required fields in production. Case-insensitive check catches
	// common variants like "Production", "prod", etc.
	envLower := strings.ToLower(cfg.Env)
	if envLower == "production" || envLower == "prod" {
		if cfg.Auth.SecretKey == "" {
			return nil, fmt.Errorf("SECRET_KEY is required in production")
		}
		if len(cfg.Auth.SecretKey) < 32 {
			return nil, fmt.Errorf("SECRET_KEY must be at least 32 characters in production")
		}
	}

	// Provide a dev-only default secret so local dev works without .env.
	if cfg.Auth.SecretKey == "" {
		cfg.Auth.SecretKey = "dev-secret-key-do-not-use-in-production!!"
	}

	return cfg, nil
}

// IsDevelopment returns true if running in development mode.
func (c *Config) IsDevelopment() bool {
	env := strings.ToLower(c.Env)
	return env == "development" || env == "dev"
}

// --- Helper functions for reading environment variables ---

// getEnv reads a string env var or returns the default.
func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

// getEnvInt reads an integer env var or returns the default.
func getEnvInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

// getEnvInt64 reads an int64 env var or returns the default.
func getEnvInt64(key string, defaultVal int64) int64 {
	if val, ok := os.LookupEnv(key); ok {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return i
		}
	}
	return defaultVal
}

// getEnvDuration reads a duration env var (e.g., "720h") or returns the default.
func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
