package armory

import (
	"context"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/changesource"
)

// recordingFields keeps the change source each write ran under, so a test can
// see the label money history will read.
type recordingFields struct {
	*fakeFields
	sources []changesource.Source
}

func (r *recordingFields) UpdateEntityFields(ctx context.Context, id string, p map[string]any) error {
	src, _ := changesource.From(ctx)
	r.sources = append(r.sources, src)
	return r.fakeFields.UpdateEntityFields(ctx, id, p)
}

func newPayFx() (*giveFx, *recordingFields) {
	g := newGiveFx()
	rf := &recordingFields{fakeFields: g.fields}
	g.dir.ents["c5"] = &EntityRef{ID: "c5", Name: "Ash", IsCharacter: true, OwnerUserID: "u1", MoneyKey: "gp",
		Purse: map[string]string{"cp": "cp", "sp": "sp", "gp": "gp"}}
	g.dir.ents["c6"] = &EntityRef{ID: "c6", Name: "Vex", IsCharacter: true, MoneyKey: "wealth"}
	g.fields.data["c2"] = map[string]any{"gp": 10.0}
	g.fields.data["c5"] = map[string]any{"cp": 1.0, "sp": 0.0, "gp": 3.0}
	g.svc = NewStashService(StashDeps{
		Repo: g.repo, Directory: g.dir, Visibility: g.vis, Actor: fakeActor{g.fx.dir},
		Fields: rf, Relations: g.rels, Events: g.events, Handouts: g.hand, Notifier: g.notify,
	})
	return g, rf
}

func TestPay_SingleNumberSheet(t *testing.T) {
	g, rf := newPayFx()
	out, err := g.svc.Pay(context.Background(), "camp", gm, PayInput{CharacterID: "c2", Amount: 4050, Reason: "as a quest reward"})
	if err != nil {
		t.Fatal(err)
	}
	if got := g.fields.data["c2"]["gp"]; got != 50.5 {
		t.Fatalf("gp = %v, want 50.5", got)
	}
	if out.CharacterName != "Mira" || out.Paid != "40.50 gp" {
		t.Fatalf("outcome %+v", out)
	}
	if len(rf.sources) != 1 || rf.sources[0].Label != "as a quest reward" || rf.sources[0].UserID != "gm" || rf.sources[0].Kind != changesource.KindWeb {
		t.Fatalf("source %+v", rf.sources)
	}
	if len(g.notify.messages) != 1 || g.notify.messages[0] != "Your GM gave you 40.50 gp" || g.notify.users[0][0] != "u2" {
		t.Fatalf("notification %v %v", g.notify.users, g.notify.messages)
	}
}

func TestPay_PurseSplitsIntoCoins(t *testing.T) {
	g, _ := newPayFx()
	out, err := g.svc.Pay(context.Background(), "camp", gm, PayInput{CharacterID: "c5", Amount: 1234})
	if err != nil {
		t.Fatal(err)
	}
	d := g.fields.data["c5"]
	if d["gp"] != 15.0 || d["sp"] != 3.0 || d["cp"] != 5.0 {
		t.Fatalf("purse %v", d)
	}
	if out.Paid != "12 gp 3 sp 4 cp" {
		t.Fatalf("paid %q", out.Paid)
	}
}

func TestPay_Rejections(t *testing.T) {
	tests := []struct {
		name string
		camp string
		who  Actor
		in   PayInput
		want int
	}{
		{"player", "camp", Actor{"u2", rPlayer}, PayInput{CharacterID: "c2", Amount: 100}, http.StatusForbidden},
		{"scribe", "camp", Actor{"sc", rScribe}, PayInput{CharacterID: "c2", Amount: 100}, http.StatusForbidden},
		{"zero", "camp", gm, PayInput{CharacterID: "c2", Amount: 0}, http.StatusBadRequest},
		{"negative takes nothing", "camp", gm, PayInput{CharacterID: "c2", Amount: -500}, http.StatusBadRequest},
		{"too much", "camp", gm, PayInput{CharacterID: "c2", Amount: maxPayCents + 1}, http.StatusBadRequest},
		{"not a character", "camp", gm, PayInput{CharacterID: "i1", Amount: 100}, http.StatusNotFound},
		{"other campaign", "other", gm, PayInput{CharacterID: "c2", Amount: 100}, http.StatusNotFound},
		{"wealth is a level", "camp", gm, PayInput{CharacterID: "c6", Amount: 100}, http.StatusBadRequest},
		{"no money field", "camp", gm, PayInput{CharacterID: "c4", Amount: 100}, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, _ := newPayFx()
			before := g.fields.data["c2"]["gp"]
			_, err := g.svc.Pay(context.Background(), tt.camp, tt.who, tt.in)
			if got := code(err); got != tt.want {
				t.Fatalf("code %d (%v), want %d", got, err, tt.want)
			}
			if g.fields.data["c2"]["gp"] != before {
				t.Fatalf("a refused payment wrote the sheet")
			}
		})
	}
}
