package media

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// MemberChecker verifies campaign membership and role without importing the
// full campaigns service. Implemented via an adapter in app/routes.go.
type MemberChecker interface {
	IsCampaignMember(campaignID, userID string) bool

	// MemberRole returns the caller's role in campaignID on campaigns.Role's
	// scale (RoleNone=0, RolePlayer=1, RoleScribe=2, RoleOwner=3), or
	// RoleNone if not a member. Gates campaign-scoped media writes at the
	// same Scribe+ threshold other campaign media write routes use.
	MemberRole(campaignID, userID string) int

	// IsUserDmGranted reports whether the campaign owner granted the user
	// dm_only/co-DM visibility (mirrors campaigns.CampaignService.
	// IsUserDmGranted). ADR-058 requires the promoted role
	// (VisibilityRole()'s formula: DM-granted -> RoleOwner, else raw
	// MemberRole) wherever a campaign context exists, so a co-DM sees what
	// the DM sees. The unscoped /media/:id route has no
	// *campaigns.CampaignContext, so checkMediaAccess recomposes that
	// formula from MemberRole + this call.
	IsUserDmGranted(campaignID, userID string) bool
}

// EntityVisibilityFilter narrows a set of entity IDs to the ones a given
// viewer (role + userID) may see. Implemented by an adapter over the
// entities plugin's canonical EntityService.FilterViewableEntityIDs — media
// forwards to this seam rather than keeping its own copy of the visibility
// predicate.
//
// ADR-058: a file at least one entity references is readable when at least
// one of those entities is visible to the viewer. Nil is a valid (if
// unwired) value — checkMediaAccess fails closed when it is nil.
type EntityVisibilityFilter interface {
	FilterViewableEntityIDs(ctx context.Context, campaignID string, entityIDs []string, role int, userID string) (map[string]bool, error)
}

// SecurityEventLogger records security events for the admin security dashboard.
// Implemented by the admin security service; wired after both are initialized.
type SecurityEventLogger interface {
	LogEvent(ctx context.Context, eventType, userID, actorID, ip, userAgent string, details map[string]any) error
}

// Handler handles HTTP requests for media operations.
type Handler struct {
	service          MediaService
	signer           *URLSigner
	memberChecker    MemberChecker
	securityLogger   SecurityEventLogger
	entityVisibility EntityVisibilityFilter
	// cache is optional (nil in tests and any deploy without Redis wired).
	// A miss or error falls through to computing the decision fresh — the
	// cache can only make ADR-058's access check slower, never laxer.
	cache *redis.Client
}

// NewHandler creates a new media handler.
func NewHandler(service MediaService) *Handler {
	return &Handler{service: service}
}

// SetURLSigner sets the HMAC URL signer for signed URL generation and
// verification. Called during wiring in app/routes.go.
func (h *Handler) SetURLSigner(signer *URLSigner) {
	h.signer = signer
}

// SetMemberChecker sets the campaign membership checker for access control
// on private campaign media. Called during wiring in app/routes.go.
func (h *Handler) SetMemberChecker(checker MemberChecker) {
	h.memberChecker = checker
}

// SetEntityVisibilityFilter wires the entity-visibility seam (ADR-058
// decision 1). Called during wiring in app/routes.go, reusing the same
// entityVisibilityFilterAdapter sessions/npcs/armory already construct
// there — media does not get its own adapter type either.
func (h *Handler) SetEntityVisibilityFilter(f EntityVisibilityFilter) {
	h.entityVisibility = f
}

// SetCache wires the Redis client used to cache the ADR-058 entity-scoped
// access decision per (file, viewer). nil disables caching (every request
// recomputes the decision, fail-safe).
func (h *Handler) SetCache(rdb *redis.Client) {
	h.cache = rdb
}

// SetSecurityLogger wires a security event logger for recording media events
// (uploads, deletes, quota failures). Called during wiring in app/routes.go.
func (h *Handler) SetSecurityLogger(logger SecurityEventLogger) {
	h.securityLogger = logger
}

// logSecurityEvent fires a security event if a logger is wired. Fire-and-forget
// so media operations are never blocked by logging failures.
func (h *Handler) logSecurityEvent(ctx context.Context, eventType, userID, actorID, ip, userAgent string, details map[string]any) {
	if h.securityLogger != nil {
		_ = h.securityLogger.LogEvent(ctx, eventType, userID, actorID, ip, userAgent, details)
	}
}

// Upload handles multipart file uploads (POST /media/upload).
func (h *Handler) Upload(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}

	// Authorize the write before touching the file: campaign_id is
	// caller-supplied form data, so without this check any authenticated
	// user who knew a campaign UUID could write into its media space (and
	// the dedup short-circuit would hand back an existing file's signed
	// URL for that campaign on a byte match). Scribe+ matches every other
	// campaign-scoped media write route. A blank campaign_id (avatars,
	// backdrops — routed through other endpoints) stays unscoped.
	if campaignID := c.FormValue("campaign_id"); campaignID != "" {
		if h.memberChecker == nil || h.memberChecker.MemberRole(campaignID, userID) < int(campaigns.RoleScribe) {
			slog.Warn("media upload: rejected, insufficient campaign role",
				slog.String("user_id", userID),
				slog.String("campaign_id", campaignID),
			)
			return apperror.NewForbidden("insufficient permissions to upload media to this campaign")
		}
	}

	file, err := c.FormFile("file")
	if err != nil {
		slog.Warn("media upload: FormFile error",
			slog.String("user_id", userID),
			slog.Any("error", err),
		)
		return apperror.NewBadRequest("no file provided")
	}

	src, err := file.Open()
	if err != nil {
		slog.Error("media upload: file open error",
			slog.String("user_id", userID),
			slog.Any("error", err),
		)
		return apperror.NewInternal(err)
	}
	defer func() { _ = src.Close() }()

	fileBytes, err := io.ReadAll(src)
	if err != nil {
		// MaxBytesReader returns a specific error when body exceeds limit.
		if err.Error() == "http: request body too large" {
			return apperror.NewBadRequest("file too large")
		}
		slog.Error("media upload: read error",
			slog.String("user_id", userID),
			slog.Any("error", err),
		)
		return apperror.NewInternal(err)
	}

	// Detect MIME type from file content if browser didn't provide one.
	declaredMime := file.Header.Get("Content-Type")
	if declaredMime == "" || declaredMime == "application/octet-stream" {
		declaredMime = http.DetectContentType(fileBytes)
	}

	input := UploadInput{
		CampaignID:   c.FormValue("campaign_id"),
		UploadedBy:   userID,
		OriginalName: file.Filename,
		MimeType:     declaredMime,
		FileSize:     int64(len(fileBytes)),
		UsageType:    c.FormValue("usage_type"),
		FileBytes:    fileBytes,
	}

	if input.UsageType == "" {
		input.UsageType = UsageAttachment
	}

	mediaFile, err := h.service.Upload(c.Request().Context(), input)
	if err != nil {
		// Log quota-exceeded errors as security events for admin visibility.
		if apperror.SafeCode(err) == http.StatusBadRequest &&
			(strings.Contains(err.Error(), "quota") || strings.Contains(err.Error(), "limit")) {
			h.logSecurityEvent(c.Request().Context(), "media.quota_exceeded",
				userID, "", c.RealIP(), c.Request().UserAgent(),
				map[string]any{
					"campaign_id": input.CampaignID,
					"mime_type":   input.MimeType,
					"size":        input.FileSize,
					"reason":      apperror.SafeMessage(err),
				})
		}
		return err
	}

	h.logSecurityEvent(c.Request().Context(), "media.uploaded",
		userID, "", c.RealIP(), c.Request().UserAgent(),
		map[string]any{
			"file_id":     mediaFile.ID,
			"campaign_id": input.CampaignID,
			"mime_type":   mediaFile.MimeType,
			"size":        mediaFile.FileSize,
		})

	// Return signed URLs if signer is available. Bound to the uploader's own
	// session (ADR-058 decision 6) -- userID is already known non-empty,
	// checked at the top of this handler.
	var url, thumbURL string
	if h.signer != nil {
		viewer := ViewerSession(userID)
		url = h.signer.Sign(mediaFile.ID, viewer, SignedURLTTL)
		if _, ok := mediaFile.ThumbnailPaths["300"]; ok {
			thumbURL = h.signer.SignThumb(mediaFile.ID, "300", viewer, SignedURLTTL)
		}
	} else {
		url = "/media/" + mediaFile.ID
		if _, ok := mediaFile.ThumbnailPaths["300"]; ok {
			thumbURL = "/media/" + mediaFile.ID + "/thumb/300"
		}
	}

	return c.JSON(http.StatusCreated, UploadResponse{
		ID:           mediaFile.ID,
		URL:          url,
		ThumbnailURL: thumbURL,
		MimeType:     mediaFile.MimeType,
		FileSize:     mediaFile.FileSize,
		// ADR-058 decision 5: populated only when the service safely merged
		// this upload into an existing file; see UploadResponse's comment.
		Deduplicated: mediaFile.MatchedExisting,
		UsedBy:       mediaFile.UsedBy,
	})
}

// Serve serves a media file (GET /media/:id).
// Enforces HMAC-signed URL verification for campaign media and access
// control for private campaigns. Files without a campaign (avatars,
// backdrops) are served without signing.
func (h *Handler) Serve(c echo.Context) error {
	fileID := c.Param("id")
	fileID = strings.TrimSuffix(fileID, "/")

	file, err := h.service.GetByID(c.Request().Context(), fileID)
	if err != nil {
		return err
	}

	// Enforce access control.
	if err := h.checkMediaAccess(c, file, false, ""); err != nil {
		return err
	}

	filePath := h.service.FilePath(file)
	h.setSecurityHeaders(c, file)
	c.Response().Header().Set("Content-Type", file.MimeType)

	return c.File(filePath)
}

// allowedThumbSizes restricts thumbnail size parameter to known values,
// preventing the size from being used as an arbitrary map key.
var allowedThumbSizes = map[string]bool{"300": true, "800": true}

// ServeThumbnail serves a thumbnail of a media file (GET /media/:id/thumb/:size).
func (h *Handler) ServeThumbnail(c echo.Context) error {
	fileID := c.Param("id")
	size := c.Param("size")

	if !allowedThumbSizes[size] {
		return apperror.NewBadRequest("invalid thumbnail size")
	}

	file, err := h.service.GetByID(c.Request().Context(), fileID)
	if err != nil {
		return err
	}

	// Enforce access control (includes size in signature check).
	if err := h.checkMediaAccess(c, file, true, size); err != nil {
		return err
	}

	thumbPath := h.service.ThumbnailPath(file, size)
	h.setSecurityHeaders(c, file)

	return c.File(thumbPath)
}

// currentViewerIdentity resolves the presented viewer identity for verifying
// a signed media URL (ADR-058 decision 6): the session user id, or
// ViewerAnonymous when no session cookie is present (e.g. a cross-origin
// <img> request). Verify decides whether that still satisfies a link minted
// for ViewerAPIKey.
func currentViewerIdentity(c echo.Context) string {
	if userID := auth.GetUserID(c); userID != "" {
		return ViewerSession(userID)
	}
	return ViewerAnonymous
}

// checkMediaAccess enforces signed URL verification and private/public
// campaign access control. Returns nil if access is allowed, or an error to
// return to the client.
func (h *Handler) checkMediaAccess(c echo.Context, file *MediaFile, isThumb bool, thumbSize string) error {
	// A nil campaign is only public when the file was uploaded that way on
	// purpose (avatar, backdrop). Campaign-scoped usage types (attachment,
	// entity_image) can end up with a nil campaign_id later — the FK is
	// ON DELETE SET NULL, and campaign-delete's media cleanup is
	// best-effort — and must not fall through to public just because the
	// column went NULL. Deny exactly like an unknown id, so an orphaned
	// row isn't distinguishable from one that was never there.
	if file.CampaignID == nil {
		switch file.UsageType {
		case UsageAvatar:
			// #730 decision 2: a profile picture is visible to any
			// signed-in user, but not to an anonymous visitor — unlike a
			// backdrop (below), which has no owner to protect.
			if auth.GetUserID(c) == "" {
				return apperror.NewNotFound("media file not found")
			}
			return nil
		case UsageBackdrop:
			return nil
		default:
			return apperror.NewNotFound("media file not found")
		}
	}

	fileID := file.ID
	expiresStr := c.QueryParam("expires")
	sig := c.QueryParam("sig")
	presentedViewer := currentViewerIdentity(c)

	// signatureValid is hoisted to function scope so the defense-in-depth
	// block below can gate on it. A valid signed URL (HMAC-SHA256 over
	// fileID:viewer:expires, time-bounded by `expires`, scoped to the
	// minted viewer per ADR-058 decision 6) is itself proof of
	// authorization, so a request presenting one must not also be forced
	// through the cookie+membership check — that would break cross-origin
	// <img> requests (e.g. from Foundry) that can't carry session cookies.
	signatureValid := false

	// Check signed URL if signer is configured.
	if h.signer != nil {
		if expiresStr != "" && sig != "" {
			if isThumb {
				signatureValid = h.signer.VerifyThumb(fileID, thumbSize, presentedViewer, expiresStr, sig)
			} else {
				signatureValid = h.signer.Verify(fileID, presentedViewer, expiresStr, sig)
			}
		}

		if !signatureValid {
			// No valid signature. Fall back: allow if the user is an
			// authenticated member of the file's campaign.
			if !h.allowUnsignedAccess(c, file) {
				// An anonymous, unsigned request against a real private file
				// must get the same NotFound an unknown id gets (not
				// Forbidden), or the status code becomes an id-existence
				// oracle (see existence_oracle_reachability_test.go). An
				// authenticated non-member still gets Forbidden: that
				// caller already proved an identity.
				//
				// A residual timing side channel remains open (the DB lookup
				// that found a real file takes measurably longer than a
				// lookup that found none); this fix only equalizes status,
				// body and headers.
				if file.CampaignIsPublic != nil && !*file.CampaignIsPublic && auth.GetUserID(c) == "" {
					return apperror.NewNotFound("media file not found")
				}
				return apperror.NewForbidden("signed URL required")
			}
		}
	}

	// Defense-in-depth: for private campaigns, also require authenticated
	// campaign membership if no signed URL was validated above. A valid
	// signature (HMAC-SHA256 over a server-side secret) is itself proof of
	// authorization, so skipping this check when signatureValid lets
	// cross-origin <img> tags (e.g. Foundry) resolve without a cookie.
	if !signatureValid {
		userID := auth.GetUserID(c)

		// Site admins bypass every check below. This ADR narrows what a
		// campaign member (or anonymous visitor) may reach, not an admin.
		if session := auth.GetSession(c); session != nil && session.IsAdmin {
			return nil
		}

		switch {
		case file.CampaignIsPublic != nil && !*file.CampaignIsPublic:
			// ADR-058 decisions 1-3: a private campaign requires at least
			// one referencing entity to be visible to this viewer (falling
			// back to plain membership when the file has no reference).
			// Anonymous callers are rejected outright — campaign privacy
			// trumps any individual entity's visibility setting.
			if userID == "" {
				return apperror.NewNotFound("media file not found")
			}
			allowed, err := h.checkEntityScopedAccess(c.Request().Context(), file, userID)
			if err != nil {
				// Fail closed: log the real reason, return the same generic
				// 404 as every other denial so a probing client can't tell
				// "hidden" from "broken".
				slog.Error("media: entity-scoped access check failed; denying access",
					slog.String("file_id", file.ID),
					slog.String("campaign_id", *file.CampaignID),
					slog.Any("error", err),
				)
				return apperror.NewNotFound("media file not found")
			}
			if !allowed {
				return apperror.NewNotFound("media file not found")
			}

		case file.CampaignIsPublic != nil && *file.CampaignIsPublic:
			// ADR-058 decision 7: a public campaign no longer grants an
			// unsigned request to any of its files by default — it reuses
			// decision 1's checkEntityScopedAccess instead. userID may be ""
			// here (unlike the private branch); viewerVisibilityRole
			// resolves an anonymous caller to RoleNone.
			allowed, err := h.checkEntityScopedAccess(c.Request().Context(), file, userID)
			if err != nil {
				slog.Error("media: entity-scoped access check failed (public campaign); denying access",
					slog.String("file_id", file.ID),
					slog.String("campaign_id", *file.CampaignID),
					slog.Any("error", err),
				)
				return apperror.NewNotFound("media file not found")
			}
			if !allowed {
				return apperror.NewNotFound("media file not found")
			}
		}
	}

	return nil
}

// mediaAccessCacheTTL bounds how long an ADR-058 entity-scoped access
// decision is trusted before recompute. Kept short so a viewer loses access
// to a newly-hidden page's image within about a minute, matching the TTL
// entities.Handler uses for its own hover-card cache.
const mediaAccessCacheTTL = 60 * time.Second

// mediaAccessCacheAllow/Deny are the cached payloads for the ADR-058
// entity-scoped decision. Plain strings so an unrecognized value is
// trivially detected and treated as a miss rather than misread.
const (
	mediaAccessCacheAllow = "1"
	mediaAccessCacheDeny  = "0"
)

// mediaAccessCacheKey scopes the cache per (file, viewer), as ADR-058
// requires. Only written for the has-references branch of
// checkEntityScopedAccess; the no-references branch is deliberately
// uncached.
func mediaAccessCacheKey(fileID, userID string) string {
	return "media:access:" + fileID + ":" + userID
}

// checkEntityScopedAccess applies ADR-058 decisions 1-3, and (per decision 7)
// the has-references half of the public-campaign rule. The private-campaign
// caller always passes a real authenticated userID; the public-campaign
// caller (decision 7) may pass "" for anonymous, which viewerVisibilityRole
// resolves to RoleNone via the same MemberChecker calls.
//
// Fails closed throughout: an error from the reference lookup or the
// visibility filter denies access rather than being treated as "visible".
func (h *Handler) checkEntityScopedAccess(ctx context.Context, file *MediaFile, userID string) (bool, error) {
	campaignID := *file.CampaignID

	// Cache hit skips both the reference lookup and the visibility filter
	// call. A miss (including a connection error or unrecognized payload)
	// falls through to a fresh computation — the cache can only make this
	// slower, never laxer.
	cacheKey := mediaAccessCacheKey(file.ID, userID)
	if h.cache != nil {
		switch cached, cerr := h.cache.Get(ctx, cacheKey).Result(); {
		case cerr == nil && cached == mediaAccessCacheAllow:
			return true, nil
		case cerr == nil && cached == mediaAccessCacheDeny:
			return false, nil
		}
	}

	refs, err := h.service.FindReferences(ctx, campaignID, file.ID)
	if err != nil {
		return false, fmt.Errorf("finding media references for access check: %w", err)
	}

	if len(refs) == 0 {
		// Decision 3: no entity references this file — campaign membership
		// decides. Deliberately not cached: caching would add a staleness
		// window a revoked member could ride out.
		//
		// An absent checker refuses rather than admits (fail closed), same
		// as the upload gate above.
		if h.memberChecker == nil {
			return false, fmt.Errorf("media: member checker not configured")
		}
		return h.memberChecker.IsCampaignMember(campaignID, userID), nil
	}

	// Decision 1: at least one referencing entity must be visible to this
	// viewer.
	if h.entityVisibility == nil {
		// Misconfiguration, not a policy outcome — the caller logs this
		// loudly and returns the same generic 404 as a real denial.
		return false, fmt.Errorf("media: entity visibility filter not configured")
	}

	entityIDs := make([]string, 0, len(refs))
	seen := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if seen[ref.EntityID] {
			continue
		}
		seen[ref.EntityID] = true
		entityIDs = append(entityIDs, ref.EntityID)
	}

	role := h.viewerVisibilityRole(campaignID, userID)

	viewable, err := h.entityVisibility.FilterViewableEntityIDs(ctx, campaignID, entityIDs, role, userID)
	if err != nil {
		return false, fmt.Errorf("filtering viewable entities for access check: %w", err)
	}

	allowed := false
	for _, ok := range viewable {
		if ok {
			allowed = true
			break
		}
	}

	if h.cache != nil {
		val := mediaAccessCacheDeny
		if allowed {
			val = mediaAccessCacheAllow
		}
		if err := h.cache.Set(ctx, cacheKey, val, mediaAccessCacheTTL).Err(); err != nil {
			// Cache write failure never affects the decision just made —
			// only the NEXT request's cost. Log and move on.
			slog.Warn("media: failed to cache entity-scoped access decision",
				slog.String("file_id", file.ID), slog.Any("error", err))
		}
	}

	return allowed, nil
}

// viewerVisibilityRole computes the same promotion
// campaigns.CampaignContext.VisibilityRole() applies (DM-granted →
// RoleOwner, else raw member role) via MemberChecker, since the unscoped
// /media/:id route has no real *campaigns.CampaignContext to call it on.
// Delegates to promotedVisibilityRole so mediaService.canMergeWithExisting
// (decision 5) shares the same formula.
func (h *Handler) viewerVisibilityRole(campaignID, userID string) int {
	return promotedVisibilityRole(h.memberChecker, campaignID, userID)
}

// promotedVisibilityRole is the shared ADR-058 promotion formula: DM-granted
// -> RoleOwner, else the raw member role. A nil checker or DM-grant error
// resolves to the un-promoted role, never a guessed promotion — fail closed,
// never more visible than the raw role. Package-level so mediaService.
// canMergeWithExisting shares the identical formula.
func promotedVisibilityRole(checker MemberChecker, campaignID, userID string) int {
	if checker == nil {
		return int(campaigns.RoleNone)
	}
	if checker.IsUserDmGranted(campaignID, userID) {
		return int(campaigns.RoleOwner)
	}
	return checker.MemberRole(campaignID, userID)
}

// allowUnsignedAccess is the coarse fallback gate when no valid signed URL is
// present: it only decides whether checkMediaAccess proceeds to its
// fine-grained defense-in-depth switch (ADR-058 decisions 1-3, 7), which can
// still deny regardless. Allows authenticated campaign members through so
// pre-signing URLs keep working.
func (h *Handler) allowUnsignedAccess(c echo.Context, file *MediaFile) bool {
	// Public campaigns: let the request through to the decision-7 check
	// rather than answering "signed URL required" outright; that check has
	// the final word, not this function.
	if file.CampaignIsPublic != nil && *file.CampaignIsPublic {
		return true
	}

	// Authenticated user who is a campaign member.
	userID := auth.GetUserID(c)
	if userID != "" && file.CampaignID != nil {
		if h.memberChecker != nil && h.memberChecker.IsCampaignMember(*file.CampaignID, userID) {
			return true
		}
		// Site admins always have access.
		if session := auth.GetSession(c); session != nil && session.IsAdmin {
			return true
		}
	}

	return false
}

// setSecurityHeaders applies defense-in-depth headers to media responses.
func (h *Handler) setSecurityHeaders(c echo.Context, file *MediaFile) {
	resp := c.Response()

	// Force browser to respect declared Content-Type (prevents MIME sniffing).
	resp.Header().Set("X-Content-Type-Options", "nosniff")

	// Safe filename for Content-Disposition. Serve images inline with a
	// sanitized filename to prevent header injection.
	resp.Header().Set("Content-Disposition",
		fmt.Sprintf(`inline; filename="%s"`, sanitizeFilename(file.OriginalName)))

	// Prevent media URLs from being embedded as iframes.
	resp.Header().Set("X-Frame-Options", "DENY")

	// Restrictive CSP on media responses: no scripts, no styles.
	resp.Header().Set("Content-Security-Policy",
		"default-src 'none'; img-src 'self'; style-src 'none'; script-src 'none'")

	// Prevent referrer leakage of signed URLs or UUIDs.
	resp.Header().Set("Referrer-Policy", "no-referrer")

	// Cache control based on campaign privacy.
	if file.CampaignIsPublic != nil && !*file.CampaignIsPublic {
		// Private campaign media must not be cached by shared proxies.
		resp.Header().Set("Cache-Control", "private, no-store, max-age=0")
	} else {
		// Public/orphan media: cache aggressively (UUID filenames are immutable).
		resp.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
}

// sanitizeFilename strips characters that could be used for header injection,
// allowing only safe characters for Content-Disposition filenames.
func sanitizeFilename(name string) string {
	safe := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) ||
			r == '-' || r == '_' || r == '.' || r == ' ' {
			return r
		}
		return '_'
	}, name)
	if safe == "" {
		safe = "file"
	}
	return safe
}

// Info returns metadata about a media file (GET /media/:fileID/info).
// Requires authentication. Only the uploader or a site admin can view info.
func (h *Handler) Info(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}

	fileID := c.Param("fileID")
	file, err := h.service.GetByID(c.Request().Context(), fileID)
	if err != nil {
		return err
	}

	// Ownership check: only uploader or admin can see file info.
	session := auth.GetSession(c)
	if file.UploadedBy != userID && (session == nil || !session.IsAdmin) {
		return apperror.NewNotFound("media file not found")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"id":            file.ID,
		"original_name": file.OriginalName,
		"mime_type":     file.MimeType,
		"file_size":     file.FileSize,
		"usage_type":    file.UsageType,
		"created_at":    file.CreatedAt.Format(time.RFC3339),
		"thumbnails":    file.ThumbnailPaths,
	})
}

// Delete removes a media file (DELETE /media/:fileID).
// Only the uploader or a site admin can delete.
func (h *Handler) Delete(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}

	fileID := c.Param("fileID")
	file, err := h.service.GetByID(c.Request().Context(), fileID)
	if err != nil {
		return err
	}

	// Ownership check.
	session := auth.GetSession(c)
	if file.UploadedBy != userID && (session == nil || !session.IsAdmin) {
		return apperror.NewNotFound("media file not found")
	}

	if err := h.service.Delete(c.Request().Context(), fileID); err != nil {
		return err
	}

	var campaignID string
	if file.CampaignID != nil {
		campaignID = *file.CampaignID
	}
	h.logSecurityEvent(c.Request().Context(), "media.deleted",
		userID, "", c.RealIP(), c.Request().UserAgent(),
		map[string]any{
			"file_id":     fileID,
			"campaign_id": campaignID,
		})

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Campaign-scoped media browser ---

// CampaignMediaPageData holds all data for rendering the campaign media browser.
type CampaignMediaPageData struct {
	Files     []MediaFile
	Stats     *CampaignMediaStats
	Total     int
	Page      int
	PerPage   int
	CSRFToken string
}

// CampaignMediaList returns paginated campaign media as JSON. Drives the
// media-picker widget ("Choose from campaign" next to file inputs), distinct
// from CampaignMedia which renders the admin browser HTML page. Scribe+ at
// the route level; folds in signed URLs so the widget need not know the
// signing scheme.
//
// GET /campaigns/:id/media/list?page=N&perPage=M
func (h *Handler) CampaignMediaList(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.QueryParam("perPage"))
	if perPage <= 0 || perPage > 100 {
		perPage = 24
	}

	files, total, err := h.service.ListCampaignMedia(ctx, cc.Campaign.ID, page, perPage)
	if err != nil {
		return err
	}

	// Bind every URL to the caller's own session (ADR-058 decision 6).
	// userID is expected non-empty (route requires auth); a fallback to
	// ViewerAnonymous only narrows who can use the resulting link.
	viewer := ViewerAnonymous
	if userID := auth.GetUserID(c); userID != "" {
		viewer = ViewerSession(userID)
	}

	type mediaListItem struct {
		ID           string    `json:"id"`
		OriginalName string    `json:"original_name"`
		MimeType     string    `json:"mime_type"`
		FileSize     int64     `json:"file_size"`
		URL          string    `json:"url"`
		ThumbnailURL string    `json:"thumbnail_url,omitempty"`
		CreatedAt    time.Time `json:"created_at"`
	}

	out := make([]mediaListItem, 0, len(files))
	for i := range files {
		f := &files[i]
		item := mediaListItem{
			ID:           f.ID,
			OriginalName: f.OriginalName,
			MimeType:     f.MimeType,
			FileSize:     f.FileSize,
			CreatedAt:    f.CreatedAt,
		}
		// Same signed-URL logic the upload handler uses — keep the
		// picker's URLs viewer-bound and short-lived (SignedURLTTL),
		// fall back to unsigned when no signer is configured (dev / tests).
		if h.signer != nil {
			item.URL = h.signer.Sign(f.ID, viewer, SignedURLTTL)
			if thumb := pickThumbnail(f); thumb != "" {
				item.ThumbnailURL = h.signer.SignThumb(f.ID, thumb, viewer, SignedURLTTL)
			}
		} else {
			item.URL = "/media/" + f.ID
			if thumb := pickThumbnail(f); thumb != "" {
				item.ThumbnailURL = "/media/" + f.ID + "/thumb/" + thumb
			}
		}
		out = append(out, item)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"items":    out,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// pickThumbnail picks the smallest available thumbnail size key for a
// file. Picker UIs prefer small thumbs to keep the slideout snappy.
func pickThumbnail(f *MediaFile) string {
	if !f.IsImage() || len(f.ThumbnailPaths) == 0 {
		return ""
	}
	// Prefer 300px if present; fall back to anything else.
	if _, ok := f.ThumbnailPaths["300"]; ok {
		return "300"
	}
	for size := range f.ThumbnailPaths {
		return size
	}
	return ""
}

// CampaignMedia renders the campaign media browser page (GET /campaigns/:id/media).
func (h *Handler) CampaignMedia(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewNotFound("campaign not found")
	}

	ctx := c.Request().Context()

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	perPage := 24

	files, total, err := h.service.ListCampaignMedia(ctx, cc.Campaign.ID, page, perPage)
	if err != nil {
		return err
	}

	stats, err := h.service.GetCampaignStats(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}

	data := CampaignMediaPageData{
		Files:     files,
		Stats:     stats,
		Total:     total,
		Page:      page,
		PerPage:   perPage,
		CSRFToken: middleware.GetCSRFToken(c),
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, CampaignMediaFragment(cc, data))
	}
	return middleware.Render(c, http.StatusOK, CampaignMediaPage(cc, data))
}

// CampaignDeleteMedia handles deletion of a campaign media file
// (DELETE /campaigns/:id/media/:mid).
func (h *Handler) CampaignDeleteMedia(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewNotFound("campaign not found")
	}

	mediaID := c.Param("mid")
	if err := h.service.DeleteCampaignMedia(c.Request().Context(), cc.Campaign.ID, mediaID); err != nil {
		return err
	}

	h.logSecurityEvent(c.Request().Context(), "media.deleted",
		auth.GetUserID(c), "", c.RealIP(), c.Request().UserAgent(),
		map[string]any{
			"file_id":     mediaID,
			"campaign_id": cc.Campaign.ID,
		})

	// Redirect back to media page for HTMX and standard requests.
	c.Response().Header().Set("HX-Redirect", fmt.Sprintf("/campaigns/%s/media", cc.Campaign.ID))
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// CampaignMediaRefs returns an HTMX fragment showing which entities reference
// a media file (GET /campaigns/:id/media/:mid/refs).
//
// ADR-058 decision 4: the list names pages, so the gate is keyed on the
// promoted role (VisibilityRole() >= RoleScribe) rather than raw MemberRole,
// so a co-DM sees what the DM sees. The result is then filtered to entities
// this viewer may see via the same FilterViewableEntityIDs seam checkMediaAccess
// uses, since being a Scribe doesn't mean seeing every entity.
func (h *Handler) CampaignMediaRefs(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewNotFound("campaign not found")
	}

	if cc.VisibilityRole() < int(campaigns.RoleScribe) {
		return apperror.NewForbidden("insufficient permissions")
	}

	ctx := c.Request().Context()
	mediaID := c.Param("mid")
	if _, err := uuid.Parse(mediaID); err != nil {
		// FindReferences' LIKE arm assumes mediaID is a server-generated
		// UUID with no wildcard characters; mediaID here is the raw URL
		// segment, never validated against a real media record. A
		// non-UUID value (e.g. "%" or "_") would otherwise widen the
		// LIKE match to any path-form reference in the campaign instead
		// of erroring, so reject it the same as a real not-found id.
		return apperror.NewNotFound("media file not found")
	}
	refs, err := h.service.FindReferences(ctx, cc.Campaign.ID, mediaID)
	if err != nil {
		return err
	}

	refs = h.filterViewableRefs(ctx, cc, auth.GetUserID(c), refs)

	return middleware.Render(c, http.StatusOK, MediaRefsFragment(cc, mediaID, refs))
}

// filterViewableRefs narrows refs to the entities userID may see, using cc's
// promoted VisibilityRole() (co-DM parity with checkEntityScopedAccess).
// Fails closed: a nil filter or filter error empties the list rather than
// risk showing a page name the viewer cannot open.
func (h *Handler) filterViewableRefs(ctx context.Context, cc *campaigns.CampaignContext, userID string, refs []MediaRef) []MediaRef {
	if len(refs) == 0 {
		return refs
	}
	if h.entityVisibility == nil {
		slog.Error("media: entity visibility filter not configured; hiding all media references",
			slog.String("campaign_id", cc.Campaign.ID))
		return nil
	}

	entityIDs := make([]string, 0, len(refs))
	seen := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if seen[ref.EntityID] {
			continue
		}
		seen[ref.EntityID] = true
		entityIDs = append(entityIDs, ref.EntityID)
	}

	viewable, err := h.entityVisibility.FilterViewableEntityIDs(ctx, cc.Campaign.ID, entityIDs, cc.VisibilityRole(), userID)
	if err != nil {
		slog.Error("media: filtering viewable entities for media refs failed; hiding all references",
			slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
		return nil
	}

	filtered := make([]MediaRef, 0, len(refs))
	for _, ref := range refs {
		if viewable[ref.EntityID] {
			filtered = append(filtered, ref)
		}
	}
	return filtered
}
