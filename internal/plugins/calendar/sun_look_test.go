package calendar

import (
	"context"
	"testing"
)

func TestNormalizeSunLook(t *testing.T) {
	str := func(s string) *string { return &s }
	tests := []struct {
		name      string
		in        SunLook
		wantKind  string
		wantColor *string
		wantErr   bool
	}{
		{name: "empty is the yellow sun", in: SunLook{}, wantKind: SunKindYellow},
		{name: "kind is case-folded", in: SunLook{Kind: " Blue "}, wantKind: SunKindBlue},
		{name: "short colour expands", in: SunLook{Kind: "red", Color: str("F60")}, wantKind: SunKindRed, wantColor: str("#ff6600")},
		{name: "blank colour clears", in: SunLook{Kind: "red", Color: str(" ")}, wantKind: SunKindRed},
		{name: "unknown kind", in: SunLook{Kind: "green"}, wantErr: true},
		{name: "colour that isn't one", in: SunLook{Kind: "red", Color: str("red;x:y")}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSunLook(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Kind != tt.wantKind || (got.Color == nil) != (tt.wantColor == nil) || (got.Color != nil && *got.Color != *tt.wantColor) {
				t.Errorf("got %+v, want kind %q colour %v", got, tt.wantKind, tt.wantColor)
			}
		})
	}
}

func TestMoonStrength(t *testing.T) {
	str := func(s string) *string { return &s }
	tests := []struct {
		name   string
		tint   *string
		want   float64
		wantOK bool
	}{
		{name: "unset", tint: nil},
		{name: "a number", tint: str("0.75"), want: 0.75, wantOK: true},
		{name: "a hex colour the old generator stored", tint: str("#aabbcc")},
		{name: "out of range", tint: str("1.5")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := MoonStrength(tt.tint)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("MoonStrength = %v, %v; want %v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
	for _, v := range []float64{-0.1, 1.01} {
		if _, err := MoonStrengthTint(v); err == nil {
			t.Errorf("MoonStrengthTint(%v) passed, want a refusal", v)
		}
	}
	if s, err := MoonStrengthTint(0.333); err != nil || s != "0.33" {
		t.Errorf("MoonStrengthTint(0.333) = %q, %v", s, err)
	}
}

func TestSaveSunLookAndMoonStrength(t *testing.T) {
	repo := &fakeCalendarRepo{}
	var saved *SunLook
	var tint *string
	repo.saveSunLookFn = func(_ context.Context, _ string, l SunLook) error { saved = &l; return nil }
	repo.setMoonTintFn = func(_ context.Context, _ string, _ int, t *string) error { tint = t; return nil }
	repo.getByIDFn = func(_ context.Context, id string) (*Calendar, error) {
		return &Calendar{ID: id, CampaignID: testCampaignA}, nil
	}
	svc := newTestCalendarService(repo, nil, nil, nil)

	if err := svc.SaveSunLook(context.Background(), "cal-1", testCampaignA, SunLook{Kind: "Orange"}); err != nil || saved == nil || saved.Kind != SunKindOrange {
		t.Fatalf("SaveSunLook: err %v saved %+v", err, saved)
	}
	if err := svc.SaveSunLook(context.Background(), "cal-1", testCampaignA, SunLook{Kind: "purple"}); err == nil {
		t.Error("an unknown kind was saved")
	}
	if err := svc.SaveSunLook(context.Background(), "cal-1", "other-campaign", SunLook{Kind: "red"}); err == nil {
		t.Error("a calendar from another campaign was changed")
	}
	v := 0.9
	if err := svc.SetMoonStrength(context.Background(), 3, "cal-1", testCampaignA, &v); err != nil || tint == nil || *tint != "0.9" {
		t.Fatalf("SetMoonStrength: err %v tint %v", err, tint)
	}
	if err := svc.SetMoonStrength(context.Background(), 3, "cal-1", testCampaignA, nil); err != nil || tint != nil {
		t.Fatalf("SetMoonStrength(nil): err %v tint %v, want the default", err, tint)
	}
	bad := 2.0
	if err := svc.SetMoonStrength(context.Background(), 3, "cal-1", testCampaignA, &bad); err == nil {
		t.Error("a strength above 1 was saved")
	}
}
