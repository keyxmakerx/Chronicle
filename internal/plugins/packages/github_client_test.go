package packages

import "testing"

func TestParseRepo(t *testing.T) {
	tests := []struct {
		url, owner, repo string
		wantErr          bool
	}{
		{url: "https://github.com/keyxmakerx/Chronicle-Draw-Steel", owner: "keyxmakerx", repo: "Chronicle-Draw-Steel"},
		{url: "https://github.com/keyxmakerx/Chronicle-DnD-5.5e", owner: "keyxmakerx", repo: "Chronicle-DnD-5.5e"},
		{url: "https://github.com/keyxmakerx/Chronicle-DnD-5.5e.git", owner: "keyxmakerx", repo: "Chronicle-DnD-5.5e"},
		{url: "https://github.com/keyxmakerx/Chronicle-DnD-5.5e/releases", owner: "keyxmakerx", repo: "Chronicle-DnD-5.5e"},
		{url: "git@github.com:keyxmakerx/my_pack.v2.git", owner: "keyxmakerx", repo: "my_pack.v2"},
		{url: "https://github.com/keyxmakerx/..", wantErr: true},
		{url: "https://github.com/keyxmakerx/.git", wantErr: true},
		{url: "https://github.com/keyxmakerx", wantErr: true},
	}
	for _, tt := range tests {
		owner, repo, err := parseRepo(tt.url)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseRepo(%q) = %q/%q, want error", tt.url, owner, repo)
			}
			continue
		}
		if err != nil || owner != tt.owner || repo != tt.repo {
			t.Errorf("parseRepo(%q) = %q/%q, %v; want %q/%q", tt.url, owner, repo, err, tt.owner, tt.repo)
		}
	}
}
