package app

import (
	"context"
	"net/http"
	"path"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer/htmlconv"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/plugins/vault_import"
)

// vaultPagesAdapter is the entities service in the vault import's page terms,
// so the import creates pages through the same rules as the editor (slug,
// history, change events) without importing the entities plugin.
type vaultPagesAdapter struct{ svc entities.EntityService }

func (a vaultPagesAdapter) Kinds(ctx context.Context, campaignID string) ([]vault_import.PageKind, error) {
	types, err := a.svc.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]vault_import.PageKind, len(types))
	for i, t := range types {
		out[i] = vault_import.PageKind{ID: t.ID, Slug: t.Slug, Enabled: t.Enabled}
	}
	return out, nil
}

func (a vaultPagesAdapter) NameTaken(ctx context.Context, campaignID, name string) (bool, error) {
	_, err := a.svc.GetBySlug(ctx, campaignID, entities.Slugify(name))
	switch {
	case err == nil:
		return true, nil
	case apperror.SafeCode(err) == http.StatusNotFound:
		return false, nil
	}
	return false, err
}

// CreatePage always creates the page GM only (IsPrivate): the import has no way
// to ask for anything else.
func (a vaultPagesAdapter) CreatePage(ctx context.Context, campaignID, userID string, p vault_import.NewPage) (string, error) {
	e, err := a.svc.Create(ctx, campaignID, userID, entities.CreateEntityInput{
		Name:         p.Name,
		EntityTypeID: p.KindID,
		TypeLabel:    p.Label,
		ParentID:     p.ParentID,
		IsPrivate:    true,
		FieldsData:   map[string]any{},
	})
	if err != nil {
		return "", err
	}
	return e.ID, nil
}

func (a vaultPagesAdapter) SetBody(ctx context.Context, pageID, editorJSON, html string) error {
	return a.svc.UpdateEntry(ctx, pageID, editorJSON, html)
}

func (a vaultPagesAdapter) Rename(ctx context.Context, pageID, name string) error {
	_, err := a.svc.Update(ctx, pageID, entities.UpdateEntityInput{Name: patch.Of(name)})
	return err
}

// vaultPicturesAdapter stores a picture through the media service with the
// usage type the editor's own picture upload uses, so the type, size and quota
// checks are the ones a typed page gets.
type vaultPicturesAdapter struct{ svc media.MediaService }

var vaultPictureMime = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp",
}

func (a vaultPicturesAdapter) StorePicture(ctx context.Context, campaignID, userID, name string, data []byte) (string, error) {
	mime, ok := vaultPictureMime[strings.ToLower(path.Ext(name))]
	if !ok {
		return "", apperror.NewBadRequest("that kind of picture isn't supported")
	}
	f, err := a.svc.Upload(ctx, media.UploadInput{
		CampaignID:   campaignID,
		UploadedBy:   userID,
		OriginalName: name,
		MimeType:     mime,
		FileSize:     int64(len(data)),
		UsageType:    media.UsageAttachment,
		FileBytes:    data,
	})
	if err != nil {
		return "", err
	}
	return f.ID, nil
}

// vaultFilesAdapter attaches files through the media plugin's page file
// service, as the owner and GM only, so the file types, the 50-per-page limit
// and the page's own access rule all apply.
type vaultFilesAdapter struct{ svc media.PageFileService }

func (a vaultFilesAdapter) CanAttach(name string) bool {
	_, ok := media.PageFileMimeForName(name)
	return ok
}

func (a vaultFilesAdapter) AttachGMOnly(ctx context.Context, campaignID, pageID, userID, name string, data []byte) error {
	_, err := a.svc.Attach(ctx, media.PageFileUpload{
		CampaignID: campaignID,
		EntityID:   pageID,
		Viewer:     media.PageFileViewer{UserID: userID, Role: int(campaigns.RoleOwner)},
		Name:       name,
		Bytes:      data,
		GMOnly:     true,
	})
	return err
}

// vaultEditorJSON converts sanitised HTML to the editor's document format with
// the converter AI Import uses.
func vaultEditorJSON(html string) (string, error) { return htmlconv.Convert(html) }
