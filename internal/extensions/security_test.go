package extensions

import (
	"testing"
)

func TestValidateZipEntry(t *testing.T) {
	tests := []struct {
		name    string
		entry   string
		wantErr bool
	}{
		{name: "valid json", entry: "manifest.json"},
		{name: "valid nested", entry: "data/entities.json"},
		{name: "valid css", entry: "themes/dark.css"},
		{name: "valid svg", entry: "icons/marker.svg"},
		{name: "valid png", entry: "images/preview.png"},
		{name: "valid webp", entry: "images/photo.webp"},
		{name: "valid jpg", entry: "images/photo.jpg"},
		{name: "valid jpeg", entry: "images/photo.jpeg"},
		{name: "valid txt", entry: "README.txt"},
		{name: "valid md", entry: "README.md"},
		{name: "directory entry", entry: "data/"},

		// Rejections.
		{name: "empty", entry: "", wantErr: true},
		{name: "absolute path", entry: "/etc/passwd", wantErr: true},
		{name: "dotdot traversal", entry: "../secret.json", wantErr: true},
		{name: "nested traversal", entry: "data/../../secret.json", wantErr: true},
		{name: "leading slash", entry: "/manifest.json", wantErr: true},
		{name: "dotfile", entry: ".env", wantErr: true},
		{name: "dotfile nested", entry: "data/.gitignore", wantErr: true},
		{name: "valid js", entry: "widgets/my-widget.js"},
		{name: "exe not allowed", entry: "run.exe", wantErr: true},
		{name: "html not allowed", entry: "page.html", wantErr: true},
		{name: "php not allowed", entry: "backdoor.php", wantErr: true},
		{name: "sh not allowed", entry: "setup.sh", wantErr: true},
		{name: "go not allowed", entry: "main.go", wantErr: true},
		{name: "no extension", entry: "Makefile", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateZipEntry(tt.entry)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateCSS(t *testing.T) {
	tests := []struct {
		name    string
		css     string
		wantErr bool
	}{
		{
			name: "valid CSS",
			css:  `.entity-card { color: red; background: #fff; }`,
		},
		{
			name: "valid data URI",
			css:  `.icon { background: url(data:image/png;base64,abc123); }`,
		},
		{
			name: "valid quoted data URI",
			css:  `.icon { background: url("data:image/svg+xml,..."); }`,
		},
		{
			name:    "import blocked",
			css:     `@import url("https://evil.com/style.css");`,
			wantErr: true,
		},
		{
			name:    "external url blocked",
			css:     `.bg { background: url(https://evil.com/image.png); }`,
			wantErr: true,
		},
		{
			name:    "external url quoted blocked",
			css:     `.bg { background: url("https://evil.com/image.png"); }`,
			wantErr: true,
		},
		{
			name:    "expression blocked",
			css:     `.hack { width: expression(document.body.clientWidth); }`,
			wantErr: true,
		},
		{
			name:    "behavior blocked",
			css:     `.hack { behavior: url(xss.htc); }`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCSS([]byte(tt.css))
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateSVG(t *testing.T) {
	tests := []struct {
		name    string
		svg     string
		wantErr bool
	}{
		{
			name: "valid SVG",
			svg:  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><circle cx="50" cy="50" r="40"/></svg>`,
		},
		{
			name:    "script tag",
			svg:     `<svg><script>alert(1)</script></svg>`,
			wantErr: true,
		},
		{
			name:    "script tag uppercase",
			svg:     `<svg><SCRIPT>alert(1)</SCRIPT></svg>`,
			wantErr: true,
		},
		{
			name:    "javascript URI",
			svg:     `<svg><a href="javascript:alert(1)">click</a></svg>`,
			wantErr: true,
		},
		{
			name:    "onclick handler",
			svg:     `<svg><rect onclick="alert(1)"/></svg>`,
			wantErr: true,
		},
		{
			name:    "onload handler",
			svg:     `<svg onload="alert(1)"><rect/></svg>`,
			wantErr: true,
		},
		{
			name:    "onerror handler",
			svg:     `<svg><image onerror="alert(1)"/></svg>`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSVG([]byte(tt.svg))
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGenerateUUID(t *testing.T) {
	uuid := generateUUID()

	// Check format: 8-4-4-4-12 hex chars.
	if len(uuid) != 36 {
		t.Errorf("expected UUID length 36, got %d: %s", len(uuid), uuid)
	}

	// Check version 4 indicator.
	if uuid[14] != '4' {
		t.Errorf("expected version 4 at position 14, got %c", uuid[14])
	}

	// Check uniqueness.
	uuid2 := generateUUID()
	if uuid == uuid2 {
		t.Error("two generated UUIDs should not be identical")
	}
}
