package config

import "testing"

// TestLoad_ProductionSecretKey pins which SECRET_KEY values production
// refuses: empty, short, and the placeholders published in this repository.
func TestLoad_ProductionSecretKey(t *testing.T) {
	cases := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{"empty", "", true},
		{"too short", "short-secret", true},
		{".env.example placeholder", "change-me-generate-a-real-secret-key", true},
		{"placeholder with stray spaces", " change-me-generate-a-real-secret-key ", true},
		{"development fallback", devSecretKey, true},
		{"a real secret", "q3J8vX0bN2mL5pR7tY9wZ1cE4gH6kA8sD0fU2iO4", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ENV", "production")
			t.Setenv("DB_PASSWORD", "not-the-default")
			t.Setenv("SECRET_KEY", tc.secret)
			_, err := Load()
			if (err != nil) != tc.wantErr {
				t.Fatalf("SECRET_KEY %q: err = %v, want error %v", tc.secret, err, tc.wantErr)
			}
		})
	}
}

// TestLoad_DevelopmentKeepsFallback pins that development still starts with
// no SECRET_KEY set.
func TestLoad_DevelopmentKeepsFallback(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("SECRET_KEY", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("development load: %v", err)
	}
	if cfg.Auth.SecretKey != devSecretKey {
		t.Fatalf("development secret = %q, want the fallback", cfg.Auth.SecretKey)
	}
}
