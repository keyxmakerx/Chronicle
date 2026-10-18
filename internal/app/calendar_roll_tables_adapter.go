package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolltables"
)

// calendarRollTablesAdapter lets a calendar event roll draw from one of the
// campaign's rolling tables, read through the rolltables service.
type calendarRollTablesAdapter struct {
	svc rolltables.Service
}

func (a calendarRollTablesAdapter) RollTable(ctx context.Context, campaignID, tableID string) (*calendar.RollTable, error) {
	doc, err := a.svc.Get(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	for _, t := range doc.Tables {
		if t.ID != tableID {
			continue
		}
		out := &calendar.RollTable{Name: t.Name, Entries: make([]calendar.RollTableEntry, 0, len(t.Entries))}
		for _, e := range t.Entries {
			out.Entries = append(out.Entries, calendar.RollTableEntry{Name: e.Name, Brief: e.Brief, Weight: e.Weight})
		}
		return out, nil
	}
	return nil, nil
}
