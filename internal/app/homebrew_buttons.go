package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

// buttonPressedEvent is the Homebrew event a drawer button the Director
// named fires. The name lives in the rule, so writing a rule for "Dose"
// is what puts a Dose button in the drawer.
const buttonPressedEvent = "button.pressed"

// buttonNameChars bounds a button's name to what reads as a button label.
const buttonNameChars = 40

func init() {
	addWorkshopSource("drawer buttons", func(reg *homebrew.Registry, _ workshopDeps) error {
		return reg.RegisterEvent(homebrew.EventDef{
			Name: buttonPressedEvent, Label: "A drawer button is pressed",
			Fields:   []homebrew.Field{{Name: "button", Label: "Button", Type: homebrew.FieldText}},
			PressArg: "button", Check: checkButtonPressed, Match: buttonMatches,
		})
	})
}

// buttonArgs is a button.pressed rule's test, and the payload a press fires
// with: the button's name.
type buttonArgs struct {
	Button string `json:"button"`
}

// checkButtonPressed keeps a button's name one short, single-spaced line,
// spaced as the drawer compares names. A rule with no name would be a
// button no one can see, so the name is required.
func checkButtonPressed(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
	var a buttonArgs
	if err := strictJSON(raw, &a); err != nil {
		return nil, err
	}
	var err error
	if a.Button, err = calendarText("the button's name", strings.Join(strings.Fields(a.Button), " "), buttonNameChars, true); err != nil {
		return nil, err
	}
	return json.Marshal(a)
}

// buttonMatches meets a rule whose button is the one pressed, without regard
// to case or spacing, as the drawer folds such names into one button.
func buttonMatches(args, payload json.RawMessage) (bool, error) {
	var a, p buttonArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return false, fmt.Errorf("button.pressed args: %w", err)
	}
	if len(payload) == 0 {
		return false, nil
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return false, fmt.Errorf("button.pressed payload: %w", err)
	}
	return a.Button != "" && foldName(a.Button) == foldName(p.Button), nil
}
