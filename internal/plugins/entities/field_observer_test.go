package entities

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

type observedWrite struct {
	entity   *Entity
	old, new map[string]any
}

type fakeFieldObserver struct {
	calls []observedWrite
	err   error
}

func (f *fakeFieldObserver) FieldsChanged(_ context.Context, e *Entity, o, n map[string]any) error {
	f.calls = append(f.calls, observedWrite{e, o, n})
	return f.err
}

func TestFieldChangeObserver(t *testing.T) {
	stored := func() *Entity {
		return &Entity{ID: "e1", CampaignID: "c1", Name: "Aria", Slug: "aria",
			FieldsData: map[string]any{"wealth": 2, "hp": 9}}
	}
	tests := []struct {
		name      string
		write     func(svc EntityService) error
		wantCalls int
		wantOld   map[string]any
		wantNew   map[string]any
		observer  error
	}{
		{
			name: "UpdateFields reports pre-write fields",
			write: func(svc EntityService) error {
				return svc.UpdateFields(context.Background(), "e1", map[string]any{"wealth": 3, "hp": 9})
			},
			wantCalls: 1,
			wantOld:   map[string]any{"wealth": 2, "hp": 9},
			wantNew:   map[string]any{"wealth": 3, "hp": 9},
		},
		{
			name: "MergeFields goes through UpdateFields",
			write: func(svc EntityService) error {
				return svc.MergeFields(context.Background(), "e1", map[string]any{"wealth": 5})
			},
			wantCalls: 1,
			wantOld:   map[string]any{"wealth": 2, "hp": 9},
			wantNew:   map[string]any{"wealth": 5, "hp": 9},
		},
		{
			name: "Update with fields reports pre-write fields",
			write: func(svc EntityService) error {
				_, err := svc.Update(context.Background(), "e1", UpdateEntityInput{FieldsData: map[string]any{"wealth": 4}})
				return err
			},
			wantCalls: 1,
			wantOld:   map[string]any{"wealth": 2, "hp": 9},
			wantNew:   map[string]any{"wealth": 4},
		},
		{
			name: "Update without fields is silent",
			write: func(svc EntityService) error {
				_, err := svc.Update(context.Background(), "e1", UpdateEntityInput{Name: patch.Of("Aria II")})
				return err
			},
			wantCalls: 0,
		},
		{
			name: "observer error never fails the write",
			write: func(svc EntityService) error {
				return svc.UpdateFields(context.Background(), "e1", map[string]any{"wealth": 3})
			},
			wantCalls: 1,
			wantOld:   map[string]any{"wealth": 2, "hp": 9},
			wantNew:   map[string]any{"wealth": 3},
			observer:  errors.New("boom"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockEntityRepo{
				findByIDFn: func(context.Context, string) (*Entity, error) { return stored(), nil },
			}
			svc := NewEntityService(repo, &mockEntityTypeRepo{}, &mockPermissionRepo{})
			obs := &fakeFieldObserver{err: tt.observer}
			svc.SetFieldChangeObserver(obs)

			if err := tt.write(svc); err != nil {
				t.Fatalf("write failed: %v", err)
			}
			if len(obs.calls) != tt.wantCalls {
				t.Fatalf("observer calls = %d, want %d", len(obs.calls), tt.wantCalls)
			}
			if tt.wantCalls == 0 {
				return
			}
			got := obs.calls[0]
			if got.entity == nil || got.entity.ID != "e1" || got.entity.CampaignID != "c1" {
				t.Errorf("observer entity = %+v", got.entity)
			}
			if !reflect.DeepEqual(got.old, tt.wantOld) {
				t.Errorf("old = %v, want %v", got.old, tt.wantOld)
			}
			if !reflect.DeepEqual(got.new, tt.wantNew) {
				t.Errorf("new = %v, want %v", got.new, tt.wantNew)
			}
		})
	}
}

func TestFieldChangeObserver_NoneWiredIsHarmless(t *testing.T) {
	repo := &mockEntityRepo{
		findByIDFn: func(context.Context, string) (*Entity, error) {
			return &Entity{ID: "e1", CampaignID: "c1", FieldsData: map[string]any{"a": 1}}, nil
		},
	}
	svc := NewEntityService(repo, &mockEntityTypeRepo{}, &mockPermissionRepo{})
	if err := svc.UpdateFields(context.Background(), "e1", map[string]any{"a": 2}); err != nil {
		t.Fatal(err)
	}
}
