package media

import (
	"context"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// PageAccess is what one viewer may do with one page.
type PageAccess struct {
	// CanView is false for a page that does not exist in the campaign, is in
	// the Trash, or that the viewer may not see: the three look the same.
	CanView bool
	// CanEdit is the page's own edit rule (a Scribe on an ordinary page, or a
	// player the owner gave edit on a page with custom permissions).
	CanEdit bool
}

// PageAccessChecker answers what a viewer may do with a page. Implemented over
// the entities service in app/routes.go, so access to a page file is exactly
// access to its page and media never reads entity tables or repeats the page
// visibility rule. Nil is a valid (unwired) value: every call then fails
// closed.
type PageAccessChecker interface {
	PageAccess(ctx context.Context, campaignID, entityID string, role int, userID string) (PageAccess, error)
}

// PageFileViewer is who is asking: their user and their promoted campaign role
// (a co-DM counts as the owner), the same role every other visibility check
// uses.
type PageFileViewer struct {
	UserID string
	Role   int
}

// SeesGMOnly is true for the GM tier, the people who may see and mark GM-only
// files.
func (v PageFileViewer) SeesGMOnly() bool {
	return v.Role >= int(campaigns.RoleScribe)
}

// PageFileListing is a page's files as one viewer may see them.
type PageFileListing struct {
	Files []PageFile
	// CanAttach is true when the viewer may add and remove files.
	CanAttach bool
	// CanMarkGMOnly is true when the viewer may mark a file GM-only.
	CanMarkGMOnly bool
}

// PageFileUpload is one file being attached.
type PageFileUpload struct {
	CampaignID string
	EntityID   string
	Viewer     PageFileViewer
	Name       string
	Bytes      []byte
	GMOnly     bool
}

// PageFileService attaches files to pages and decides who may open them. Access
// is always decided by the page the file is bound to, never by where the file's
// id appears in text.
type PageFileService interface {
	List(ctx context.Context, campaignID, entityID string, v PageFileViewer) (*PageFileListing, error)
	Attach(ctx context.Context, in PageFileUpload) (*PageFile, error)
	// Open returns the file's binding and stored record for a download. Every
	// refusal is the same not-found, so a hidden file looks like a missing one.
	Open(ctx context.Context, campaignID, entityID, fileID string, v PageFileViewer) (*PageFile, *MediaFile, error)
	Remove(ctx context.Context, campaignID, entityID, fileID string, v PageFileViewer) (*PageFile, error)
	SetGMOnly(ctx context.Context, campaignID, entityID, fileID string, v PageFileViewer, gmOnly bool) (*PageFile, error)
}

// maxPageFiles bounds one page's list, so a page cannot grow a listing that
// loads slowly for every reader.
const maxPageFiles = 50

type pageFileService struct {
	repo   PageFileRepository
	media  MediaService
	access PageAccessChecker
}

// NewPageFileService wires the page file service. access may be nil only in
// tests; every method then refuses.
func NewPageFileService(repo PageFileRepository, media MediaService, access PageAccessChecker) PageFileService {
	return &pageFileService{repo: repo, media: media, access: access}
}

func notFoundFile() error { return apperror.NewNotFound("file not found") }

// pageAccess is the one door to the page rule. A missing checker or an error
// reads as "cannot see": the caller answers not-found.
func (s *pageFileService) pageAccess(ctx context.Context, campaignID, entityID string, v PageFileViewer) (PageAccess, error) {
	// A role below Player is not a campaign member: the routes never let one
	// through, and this keeps the rule closed if a caller ever does.
	if s.access == nil || campaignID == "" || entityID == "" || v.UserID == "" || v.Role < int(campaigns.RolePlayer) {
		return PageAccess{}, nil
	}
	return s.access.PageAccess(ctx, campaignID, entityID, v.Role, v.UserID)
}

func (s *pageFileService) List(ctx context.Context, campaignID, entityID string, v PageFileViewer) (*PageFileListing, error) {
	acc, err := s.pageAccess(ctx, campaignID, entityID, v)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if !acc.CanView {
		return nil, apperror.NewNotFound("page not found")
	}
	files, err := s.repo.ListByEntity(ctx, campaignID, entityID, v.SeesGMOnly())
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return &PageFileListing{Files: files, CanAttach: acc.CanEdit, CanMarkGMOnly: acc.CanEdit && v.SeesGMOnly()}, nil
}

func (s *pageFileService) Attach(ctx context.Context, in PageFileUpload) (*PageFile, error) {
	acc, err := s.pageAccess(ctx, in.CampaignID, in.EntityID, in.Viewer)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if !acc.CanView {
		return nil, apperror.NewNotFound("page not found")
	}
	if !acc.CanEdit {
		return nil, apperror.NewForbidden("you can't add files to this page")
	}
	if in.GMOnly && !in.Viewer.SeesGMOnly() {
		return nil, apperror.NewForbidden("only the GM can mark a file GM only")
	}

	name := CleanPageFileName(in.Name)
	mime, ok := PageFileMimeForName(name)
	if !ok {
		return nil, apperror.NewBadRequest("that kind of file can't be attached. Allowed: PDF, pictures, text, Word, Excel, OpenDocument, MP3, OGG and ZIP")
	}

	n, err := s.repo.CountByEntity(ctx, in.EntityID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if n >= maxPageFiles {
		return nil, apperror.NewBadRequest("this page has the most files it can hold")
	}

	// The media service applies the same size limits, quota, disk check and
	// picture clean-up every other upload gets.
	stored, err := s.media.Upload(ctx, UploadInput{
		CampaignID:   in.CampaignID,
		UploadedBy:   in.Viewer.UserID,
		OriginalName: name,
		MimeType:     mime,
		FileSize:     int64(len(in.Bytes)),
		UsageType:    UsagePageFile,
		FileBytes:    in.Bytes,
	})
	if err != nil {
		return nil, err
	}

	f := PageFile{
		ID: stored.ID, EntityID: in.EntityID, CampaignID: in.CampaignID,
		Name: name, MimeType: stored.MimeType, Size: stored.FileSize,
		GMOnly: in.GMOnly, UploadedBy: in.Viewer.UserID, CreatedAt: stored.CreatedAt,
	}
	if err := s.repo.Bind(ctx, f); err != nil {
		// An unbound page file would sit in the quota with no way to reach it.
		if delErr := s.media.Delete(ctx, stored.ID); delErr != nil {
			slog.Warn("page file: could not remove a file that failed to bind",
				slog.String("file_id", stored.ID), slog.Any("error", delErr))
		}
		return nil, apperror.NewInternal(err)
	}
	return &f, nil
}

// find loads a file's binding and applies the read rule: it must belong to this
// campaign and page, the viewer must see the page, and a GM-only file needs the
// GM tier. Anything else is the same not-found.
func (s *pageFileService) find(ctx context.Context, campaignID, entityID, fileID string, v PageFileViewer) (*PageFile, PageAccess, error) {
	f, err := s.repo.Find(ctx, fileID)
	if err != nil {
		if apperror.SafeCode(err) == 404 {
			return nil, PageAccess{}, notFoundFile()
		}
		return nil, PageAccess{}, apperror.NewInternal(err)
	}
	if f.CampaignID != campaignID || f.EntityID != entityID {
		return nil, PageAccess{}, notFoundFile()
	}
	acc, err := s.pageAccess(ctx, campaignID, entityID, v)
	if err != nil {
		slog.Error("page file: page access check failed; denying",
			slog.String("file_id", fileID), slog.Any("error", err))
		return nil, PageAccess{}, notFoundFile()
	}
	if !acc.CanView || (f.GMOnly && !v.SeesGMOnly()) {
		return nil, PageAccess{}, notFoundFile()
	}
	return f, acc, nil
}

func (s *pageFileService) Open(ctx context.Context, campaignID, entityID, fileID string, v PageFileViewer) (*PageFile, *MediaFile, error) {
	f, _, err := s.find(ctx, campaignID, entityID, fileID, v)
	if err != nil {
		return nil, nil, err
	}
	stored, err := s.media.GetByID(ctx, f.ID)
	if err != nil || !stored.IsPageFile() {
		return nil, nil, notFoundFile()
	}
	return f, stored, nil
}

func (s *pageFileService) Remove(ctx context.Context, campaignID, entityID, fileID string, v PageFileViewer) (*PageFile, error) {
	f, acc, err := s.find(ctx, campaignID, entityID, fileID, v)
	if err != nil {
		return nil, err
	}
	if !acc.CanEdit {
		return nil, apperror.NewForbidden("you can't remove files from this page")
	}
	if err := s.media.Delete(ctx, f.ID); err != nil {
		return nil, err
	}
	return f, nil
}

func (s *pageFileService) SetGMOnly(ctx context.Context, campaignID, entityID, fileID string, v PageFileViewer, gmOnly bool) (*PageFile, error) {
	f, acc, err := s.find(ctx, campaignID, entityID, fileID, v)
	if err != nil {
		return nil, err
	}
	if !acc.CanEdit || !v.SeesGMOnly() {
		return nil, apperror.NewForbidden("only the GM can change who sees a file")
	}
	if err := s.repo.SetGMOnly(ctx, f.ID, gmOnly); err != nil {
		return nil, apperror.NewInternal(err)
	}
	f.GMOnly = gmOnly
	return f, nil
}
