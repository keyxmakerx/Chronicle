package notes

import (
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all note-related routes on the given Echo instance.
// Note routes are scoped to a campaign and require campaign membership.
// All members can manage their own notes and interact with shared notes.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	player := campaigns.RequireRole(campaigns.RolePlayer)

	// Full-page journal view, optionally opened at one note.
	cg.GET("/journal", h.ShowJournal, player)
	cg.GET("/journal/:noteId", h.ShowJournal, player)

	registerNoteJSONRoutes(cg, h, player)
}

// registerNoteJSONRoutes mounts the notes JSON routes on g. The same set
// serves the site (session) and an app the player allowed (notes grant), so
// the two can never drift apart.
func registerNoteJSONRoutes(cg *echo.Group, h *Handler, player echo.MiddlewareFunc) {
	// Members API for share-with-players picker.
	cg.GET("/notes/members", h.MembersAPI, player)

	// Journal views: the list, search, link labels, page references, bulk.
	cg.GET("/notes/index", h.Index, player)
	cg.GET("/notes/search", h.Search, player)
	cg.GET("/notes/labels", h.Labels, player)
	cg.GET("/notes/page-refs", h.PageRefs, player)
	cg.GET("/notes/page-names", h.PageNames, player)
	cg.POST("/notes/bulk", h.Bulk, player)

	// CRUD — own notes + shared note access.
	cg.GET("/notes", h.List, player)
	cg.POST("/notes", h.Create, player)
	cg.GET("/notes/:noteId", h.Get, player)
	cg.GET("/notes/:noteId/backlinks", h.Backlinks, player)
	cg.PUT("/notes/:noteId", h.Update, player)
	cg.DELETE("/notes/:noteId", h.Delete, player)
	cg.POST("/notes/:noteId/toggle", h.ToggleCheck, player)
	cg.POST("/notes/:noteId/send-to-journal", h.SendToJournal, player)

	// Edit locking — pessimistic lock for shared notes.
	cg.POST("/notes/:noteId/lock", h.Lock, player)
	cg.POST("/notes/:noteId/unlock", h.Unlock, player)
	cg.POST("/notes/:noteId/heartbeat", h.Heartbeat, player)
	cg.POST("/notes/:noteId/force-unlock", h.ForceUnlock, player) // owner check inside handler

	// Version history.
	cg.GET("/notes/:noteId/versions", h.ListVersions, player)
	cg.GET("/notes/:noteId/versions/:vid", h.GetVersion, player)
	cg.POST("/notes/:noteId/versions/:vid/restore", h.RestoreVersion, player)

	// Attachments (audio files, transcripts).
	cg.GET("/notes/:nid/attachments", h.ListAttachments, player)
	cg.POST("/notes/:nid/attachments", h.UploadAttachment, player)
	cg.DELETE("/notes/:nid/attachments/:aid", h.DeleteAttachment, player)
	cg.PUT("/notes/:nid/attachments/:aid/transcript", h.UpdateTranscript, player)
}

// RegisterAppGrantRoutes mounts the Allow window, the player's list of
// grants, and the notes JSON routes for an allowed app.
//
// The app routes live under /api/ because they are authenticated by a
// Bearer grant, never a cookie, so the site's cookie CSRF check does not
// apply to them (it skips /api/). Each request still runs as the player at
// their live campaign role through RequireCampaignAccess.
//
// It returns the app group so other plugins can add the few read-only routes
// the notes editor needs (page search), wired in app/routes.go.
func RegisterAppGrantRoutes(e *echo.Echo, h *Handler, gh *AppGrantHandler, grants AppGrantService, gate AppGate, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) *echo.Group {
	player := campaigns.RequireRole(campaigns.RolePlayer)

	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	cg.GET("/notes/allow-app", gh.ShowAllow, player)
	cg.POST("/notes/allow-app", gh.Allow, player)
	cg.GET("/notes/app-grants", gh.List, player)
	cg.DELETE("/notes/app-grants/:gid", gh.Revoke, player)

	// The per-IP limit runs before the token look-up, so a flood of made-up
	// tokens can't turn into a flood of database reads.
	ag := e.Group("/api/notes-app/campaigns/:id",
		middleware.RateLimit(600, time.Minute),
		RequireAppGrant(grants, gate),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	registerNoteJSONRoutes(ag, h, player)
	ag.GET("/notes/embed", h.EmbedFragment, player)

	// The frame shell: no sign-in, no campaign data; framable only by the
	// allowed origins.
	e.GET("/embed/campaigns/:id/notes/:mode", gh.ShowEmbed)
	return ag
}
