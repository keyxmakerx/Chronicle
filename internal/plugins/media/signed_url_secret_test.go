// Tests for LoadOrInitSigningSecret — the persistence layer added
// in C-UPDATER-MANIFEST-403 (cordinator Issue #17). Pins the
// invariant violated by the previous behavior: an auto-generated
// signing secret MUST survive process restarts so that
// foundry_vtt's TokenSigner (which shares this secret as its HMAC
// key) does not silently invalidate every previously-minted
// Foundry manifest token.
package media

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOrInitSigningSecret_EnvWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".signing-secret")

	secret, source, err := LoadOrInitSigningSecret("operator-provided", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if secret != "operator-provided" {
		t.Errorf("secret = %q, want %q", secret, "operator-provided")
	}
	if source != SecretFromEnv {
		t.Errorf("source = %q, want %q", source, SecretFromEnv)
	}
	// Env-managed path must NOT touch the persisted file.
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("env-managed path should not create persisted file; stat err = %v", err)
	}
}

func TestLoadOrInitSigningSecret_GeneratesAndPersistsOnFirstBoot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", ".signing-secret") // forces MkdirAll

	secret, source, err := LoadOrInitSigningSecret("", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != SecretGeneratedAndPersisted {
		t.Errorf("source = %q, want %q", source, SecretGeneratedAndPersisted)
	}
	if len(secret) != 64 {
		t.Errorf("generated secret length = %d, want 64 (hex of 32 bytes)", len(secret))
	}
	// File must exist with the same content.
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted file: %v", err)
	}
	if strings.TrimSpace(string(persisted)) != secret {
		t.Errorf("persisted content does not match returned secret")
	}
	// File permissions: owner read/write only.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 0600", info.Mode().Perm())
	}
}

// This is the load-bearing test for Issue #17: the secret returned
// from the first call MUST equal the secret returned from a second
// call on the same path. If this fails, every Chronicle restart
// silently invalidates every outstanding Foundry manifest token.
func TestLoadOrInitSigningSecret_RestartReusesPersistedSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".signing-secret")

	first, firstSource, err := LoadOrInitSigningSecret("", path)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if firstSource != SecretGeneratedAndPersisted {
		t.Fatalf("first call source = %q, want %q", firstSource, SecretGeneratedAndPersisted)
	}

	second, secondSource, err := LoadOrInitSigningSecret("", path)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if secondSource != SecretFromFile {
		t.Errorf("second call source = %q, want %q", secondSource, SecretFromFile)
	}
	if second != first {
		t.Errorf("second call returned a DIFFERENT secret — restart would invalidate tokens.\n"+
			"  first:  %s\n  second: %s\n"+
			"This is the exact regression cordinator Issue #17 was filed for.",
			first, second)
	}
}

func TestLoadOrInitSigningSecret_EmptyPathDisablesPersistence(t *testing.T) {
	secret, source, err := LoadOrInitSigningSecret("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != SecretGeneratedInMemory {
		t.Errorf("source = %q, want %q (empty path means no persistence)", source, SecretGeneratedInMemory)
	}
	if len(secret) != 64 {
		t.Errorf("generated secret length = %d, want 64", len(secret))
	}
}

func TestLoadOrInitSigningSecret_UnwritableDirFallsBackToInMemory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses unix mode bits — this branch needs a non-root uid to exercise")
	}
	dir := t.TempDir()
	// Make the dir read-only so MkdirAll on a subdir + write both fail.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	path := filepath.Join(dir, "nested", ".signing-secret")
	secret, source, err := LoadOrInitSigningSecret("", path)
	if err == nil {
		t.Error("expected a non-nil error reporting the persistence failure")
	}
	if source != SecretGeneratedInMemory {
		t.Errorf("source = %q, want %q (write should have failed)", source, SecretGeneratedInMemory)
	}
	if len(secret) != 64 {
		t.Errorf("secret should still be usable even if persistence failed; got length %d", len(secret))
	}
}

func TestLoadOrInitSigningSecret_EmptyFileTreatedAsAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".signing-secret")
	// A zero-byte file (e.g. left over from a failed write) should
	// be treated as missing: regenerate + overwrite.
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatalf("write empty file: %v", err)
	}

	secret, source, err := LoadOrInitSigningSecret("", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != SecretGeneratedAndPersisted {
		t.Errorf("source = %q, want %q (empty file should trigger regen+persist)",
			source, SecretGeneratedAndPersisted)
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted file: %v", err)
	}
	if strings.TrimSpace(string(persisted)) != secret {
		t.Errorf("persisted content does not match returned secret after empty-file regen")
	}
}
