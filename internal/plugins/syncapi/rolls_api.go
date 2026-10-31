package syncapi

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// RollDie is one die of a reported roll.
type RollDie struct {
	Sides int `json:"sides"`
	Value int `json:"value"`
}

// RollReport is a roll the Foundry module reports from Foundry chat.
type RollReport struct {
	OriginID      string    `json:"originId"`
	FoundryUserID string    `json:"foundryUserId"`
	RollerName    string    `json:"rollerName"`
	Label         string    `json:"label"`
	Formula       string    `json:"formula"`
	Dice          []RollDie `json:"dice"`
	Total         int       `json:"total"`
	Mode          string    `json:"mode"`
}

// RollsProvider records rolls into the campaign's roll log, so this plugin
// need not import the rolls plugin; the adapter in internal/app passes them
// on. memberID is the Chronicle member the roller is linked to, or "".
type RollsProvider interface {
	Report(ctx context.Context, campaignID, memberID string, in RollReport) (id string, duplicate bool, err error)
}

// SetRolls wires the roll log. Until it is set the route answers 404, which
// the module reads as "this Chronicle has no shared rolls".
func (h *APIHandler) SetRolls(p RollsProvider, players FoundryPlayerRepository) {
	h.rolls, h.rollPlayers = p, players
}

// maxRollReportBytes bounds one report: a hundred dice and a few short texts.
const maxRollReportBytes = 16 << 10

// ReportRoll handles POST /api/v1/campaigns/:id/rolls: a roll made in Foundry,
// which Foundry decided. Only a stored Bearer key may report one, so a
// signed-in browser on the same route can never pass a number off as
// Foundry's. A replay of the same chat message answers 200 with duplicate.
func (h *APIHandler) ReportRoll(c echo.Context) error {
	if h.rolls == nil {
		return apperror.NewNotFound("this Chronicle has no shared rolls")
	}
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewUnauthorized("authentication required")
	}
	if key.ID == synthKeySessionID {
		return apperror.NewForbidden("rolls are reported by a connected app's key")
	}
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxRollReportBytes)
	var in RollReport
	if err := c.Bind(&in); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	ctx := c.Request().Context()
	campaignID := c.Param("id")
	member := ""
	if in.FoundryUserID != "" && h.rollPlayers != nil {
		// The link is the GM's own report of who is who; a Foundry user
		// nobody linked rolls as a name only.
		if players, err := h.rollPlayers.List(ctx, campaignID); err == nil {
			for _, p := range players {
				if p.FoundryUserID == in.FoundryUserID {
					member = p.MemberUserID
					break
				}
			}
		}
	}
	id, dup, err := h.rolls.Report(ctx, campaignID, member, in)
	if err != nil {
		return err
	}
	status := http.StatusCreated
	if dup {
		status = http.StatusOK
	}
	return c.JSON(status, map[string]any{"id": id, "duplicate": dup})
}
