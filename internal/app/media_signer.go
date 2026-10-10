package app

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// mediaSignerFor binds signed media-URL generators to whoever is making the
// request, so a link is only good for that viewer (ADR-058). It is shared by
// page rendering and JSON handlers so both mint links the same way. A nil
// signer (signing off) returns nil funcs and callers fall back to unsigned
// /media/ paths.
func mediaSignerFor(signer *media.URLSigner, c echo.Context) (layouts.MediaURLFunc, layouts.MediaThumbFunc) {
	if signer == nil {
		return nil, nil
	}
	viewer := media.ViewerAnonymous
	if userID := auth.GetUserID(c); userID != "" {
		viewer = media.ViewerSession(userID)
	}
	return func(fileID string) string {
			return signer.Sign(fileID, viewer, media.SignedURLTTL)
		}, func(fileID, size string) string {
			return signer.SignThumb(fileID, size, viewer, media.SignedURLTTL)
		}
}
