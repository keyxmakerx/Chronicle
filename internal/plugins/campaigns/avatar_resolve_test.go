package campaigns

import (
	"context"
	"testing"
)

func TestResolveAvatarPath(t *testing.T) {
	id := "b7c17bb1-6563-462c-8b49-5b2e8bd57108"
	empty := ""
	legacy := "/uploads/avatars/a.png"
	tests := []struct {
		name   string
		in     *string
		want   *string
		wantEq string
	}{
		{"nil stays nil", nil, nil, ""},
		{"empty stays empty", &empty, &empty, ""},
		{"media id becomes link", &id, nil, "/media/" + id + "/thumb/300"},
		{"legacy path unchanged", &legacy, nil, legacy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveAvatarPath(context.Background(), tt.in)
			if tt.in == nil || tt.in == &empty {
				if got != tt.in {
					t.Fatalf("got %v, want input back", got)
				}
				return
			}
			if got == nil || *got != tt.wantEq {
				t.Fatalf("got %v, want %q", got, tt.wantEq)
			}
		})
	}
}

func TestResolveGroupMemberAvatars(t *testing.T) {
	id := "b7c17bb1-6563-462c-8b49-5b2e8bd57108"
	ms := []GroupMemberInfo{{UserID: "u1", AvatarPath: &id}, {UserID: "u2"}}
	resolveGroupMemberAvatars(context.Background(), ms)
	if ms[0].AvatarPath == nil || *ms[0].AvatarPath != "/media/"+id+"/thumb/300" {
		t.Fatalf("member 1 not resolved: %v", ms[0].AvatarPath)
	}
	if ms[1].AvatarPath != nil {
		t.Fatalf("member 2 should stay nil")
	}
}
