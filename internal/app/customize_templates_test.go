package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

type fakeTemplates struct {
	have    map[int]*entities.ContentTemplate
	log     []string
	failNew bool
}

func (f *fakeTemplates) Create(_ context.Context, _ string, in entities.CreateContentTemplateInput) (*entities.ContentTemplate, error) {
	if f.failNew {
		return nil, apperror.NewBadRequest("template name is required")
	}
	f.log = append(f.log, "create:"+in.Name)
	return &entities.ContentTemplate{}, nil
}
func (f *fakeTemplates) GetByID(_ context.Context, id int) (*entities.ContentTemplate, error) {
	if t, ok := f.have[id]; ok {
		return t, nil
	}
	return nil, apperror.NewNotFound("content template not found")
}
func (f *fakeTemplates) Update(_ context.Context, id int, in entities.UpdateContentTemplateInput) (*entities.ContentTemplate, error) {
	_, hasDesc := in.Description.Get()
	name, hasName := in.Name.Get()
	f.log = append(f.log, "update:"+name+map[bool]string{true: "+desc", false: ""}[hasDesc]+map[bool]string{true: "", false: "(noname)"}[hasName])
	return &entities.ContentTemplate{}, nil
}
func (f *fakeTemplates) Delete(_ context.Context, id int) error {
	f.log = append(f.log, "delete")
	return nil
}

func TestTemplatesSection_Apply(t *testing.T) {
	camp := "camp-1"
	other := "camp-2"
	mk := func() *fakeTemplates {
		return &fakeTemplates{have: map[int]*entities.ContentTemplate{
			1: {ID: 1, CampaignID: &camp},
			2: {ID: 2, CampaignID: &other},
			3: {ID: 3, CampaignID: &camp, IsGlobal: true},
		}}
	}
	tests := []struct {
		name          string
		section       string
		failNew       bool
		wantLog       []string
		wantRemaining bool
		wantErr       bool
	}{
		{"update keeps unsent fields absent", `{"items":{"1":{"name":"Recap"}}}`, false, []string{"update:Recap"}, false, false},
		{"create and delete", `{"items":{"1":{"delete":true},"new-a":{"new":true,"name":"NPC","content_json":"{}"}}}`, false, []string{"delete", "create:NPC"}, false, false},
		{"another campaign's template refused, nothing applied", `{"items":{"1":{"name":"x"},"2":{"name":"y"}}}`, false, nil, false, true},
		{"built-in template refused", `{"items":{"3":{"name":"y"}}}`, false, nil, false, true},
		{"a bad key is refused", `{"items":{"../1":{"name":"y"}}}`, false, nil, false, true},
		{"a failure keeps what did not apply", `{"items":{"1":{"name":"A"},"new-b":{"new":true}}}`, true, []string{"update:A"}, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := mk()
			f.failNew = tc.failNew
			rem, err := applyTemplatesSection(context.Background(), camp, f, json.RawMessage(tc.section))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error %v", err, tc.wantErr)
			}
			if len(f.log) != len(tc.wantLog) {
				t.Fatalf("log = %v, want %v", f.log, tc.wantLog)
			}
			for i := range f.log {
				if f.log[i] != tc.wantLog[i] {
					t.Fatalf("log = %v, want %v", f.log, tc.wantLog)
				}
			}
			if (rem != nil) != tc.wantRemaining {
				t.Fatalf("remaining = %s, want remaining %v", rem, tc.wantRemaining)
			}
		})
	}
}

func TestTemplatesSection_OnlyOwners(t *testing.T) {
	s := newTemplatesSection(&fakeTemplates{})
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe}
	if _, err := s.Apply(context.Background(), cc, "u", json.RawMessage(`{"items":{"1":{"name":"x"}}}`)); err == nil {
		t.Fatal("a scribe changed starting text")
	}
}
