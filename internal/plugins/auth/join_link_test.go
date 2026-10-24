// join_link_test.go pins that a campaign share link survives the sign-in
// page: "Create one" carries it to the register page and back, and both pages
// say what signing in is for without naming the campaign.

package auth

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestAuthLinksCarryRedirect(t *testing.T) {
	tests := []struct {
		redirect     string
		wantRegister string
		wantLogin    string
		joining      bool
	}{
		{"", "/register", "/login", false},
		{"/join/abc123", "/register?redirect=%2Fjoin%2Fabc123", "/login?redirect=%2Fjoin%2Fabc123", true},
		{"/invites/accept?token=t&x=1", "/register?redirect=%2Finvites%2Faccept%3Ftoken%3Dt%26x%3D1", "/login?redirect=%2Finvites%2Faccept%3Ftoken%3Dt%26x%3D1", false},
		{"/campaigns/c1", "/register?redirect=%2Fcampaigns%2Fc1", "/login?redirect=%2Fcampaigns%2Fc1", false},
	}
	for _, tt := range tests {
		t.Run(tt.redirect, func(t *testing.T) {
			if got := registerHref(tt.redirect); got != tt.wantRegister {
				t.Errorf("registerHref = %q, want %q", got, tt.wantRegister)
			}
			if got := loginHref(tt.redirect); got != tt.wantLogin {
				t.Errorf("loginHref = %q, want %q", got, tt.wantLogin)
			}
			if got := joiningCampaign(tt.redirect); got != tt.joining {
				t.Errorf("joiningCampaign = %v, want %v", got, tt.joining)
			}
		})
	}
}

func TestLoginPage_FromJoinLink(t *testing.T) {
	render := func(redirect string) string {
		var buf bytes.Buffer
		if err := LoginPage("csrf", "", "", "", redirect, LoginOptions{}).Render(context.Background(), &buf); err != nil {
			t.Fatalf("render: %v", err)
		}
		return buf.String()
	}

	html := render("/join/abc123")
	if !strings.Contains(html, `href="/register?redirect=%2Fjoin%2Fabc123"`) {
		t.Errorf("Create one must keep the join link; got: %s", html)
	}
	if !strings.Contains(html, "re invited</h1>") {
		t.Errorf("a join link's sign-in page must say the visitor is invited")
	}

	plain := render("")
	if !strings.Contains(plain, `href="/register"`) || !strings.Contains(plain, "Welcome back") {
		t.Errorf("an ordinary sign-in keeps its plain link and heading")
	}
}

func TestRegisterPage_FromJoinLink(t *testing.T) {
	var buf bytes.Buffer
	if err := RegisterPage("csrf", nil, "", "/join/abc123", false, "").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, `name="redirect" value="/join/abc123"`) {
		t.Errorf("the form must carry the join link through sign-up")
	}
	if !strings.Contains(html, `href="/login?redirect=%2Fjoin%2Fabc123"`) {
		t.Errorf("Sign in must keep the join link")
	}
	if !strings.Contains(html, "join the campaign you were invited to") {
		t.Errorf("the register page must say the visitor joins after signing up")
	}
}
