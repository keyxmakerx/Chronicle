package admin

import (
	"context"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/sitelook"
)

// SiteLookSettings is the slice of the settings service the Site look page
// needs. The admin plugin reaches site_settings only through this interface.
type SiteLookSettings interface {
	GetSiteLook(ctx context.Context) (sitelook.Settings, error)
	UpdateSiteLook(ctx context.Context, in sitelook.Settings) (sitelook.Settings, error)
}

// SiteMedia stores the site's logo and sign-in picture. The files belong to no
// campaign and are public (the sign-in page is shown to visitors), so the
// implementation stamps them in a way no campaign or avatar file can match.
type SiteMedia interface {
	// StoreSitePicture saves an already-validated image and returns its
	// stored filename.
	StoreSitePicture(ctx context.Context, userID string, data []byte, originalName, mimeType string) (string, error)
	// OwnsSitePicture reports whether filename is a site picture stored by
	// StoreSitePicture, as opposed to any other media file.
	OwnsSitePicture(ctx context.Context, filename string) (bool, error)
	// DeleteSitePicture removes filename only if it is a site picture; any
	// other file is left alone and is not an error.
	DeleteSitePicture(ctx context.Context, filename string) error
}

// SiteLookUpload is one file from the form.
type SiteLookUpload struct {
	Name string
	Data []byte
}

// SiteLookSubmission is one Save of the Site look page: the text choices, and
// for each picture either a new upload, a removal, or neither (keep what is
// saved). Picture names are never taken from the request, only from uploads
// this service stored, so a request cannot point the site at someone else's
// file.
type SiteLookSubmission struct {
	Draft         sitelook.Settings // Name, Look, Background, Welcome, LogoAsFavicon.
	Logo          *SiteLookUpload
	RemoveLogo    bool
	Picture       *SiteLookUpload
	RemovePicture bool
}

// SiteLookService owns the rules of the Site look page.
type SiteLookService interface {
	// Get returns the saved site look.
	Get(ctx context.Context) (sitelook.Settings, error)
	// Save applies a submission: validates and stores new pictures, saves the
	// look, and tidies pictures the save replaced. It returns what was stored.
	Save(ctx context.Context, userID string, sub SiteLookSubmission) (sitelook.Settings, error)
}

type siteLookService struct {
	settings SiteLookSettings
	media    SiteMedia
}

// NewSiteLookService builds the service. media may be nil, in which case
// uploads are refused but the text settings still save.
func NewSiteLookService(settings SiteLookSettings, media SiteMedia) SiteLookService {
	return &siteLookService{settings: settings, media: media}
}

func (s *siteLookService) Get(ctx context.Context) (sitelook.Settings, error) {
	return s.settings.GetSiteLook(ctx)
}

func (s *siteLookService) Save(ctx context.Context, userID string, sub SiteLookSubmission) (sitelook.Settings, error) {
	cur, err := s.settings.GetSiteLook(ctx)
	if err != nil {
		return sitelook.Settings{}, err
	}

	draft := sub.Draft
	draft.Logo, draft.Picture = cur.Logo, cur.Picture
	if sub.RemoveLogo {
		draft.Logo = ""
	}
	if sub.RemovePicture {
		draft.Picture = ""
	}

	// Files stored by this call, to be cleaned up if the save then fails.
	var stored []string
	discard := func() {
		for _, f := range stored {
			s.deletePicture(ctx, f)
		}
	}
	store := func(kind string, up *SiteLookUpload) (string, error) {
		mime, err := sitelook.CheckImage(kind, up.Data)
		if err != nil {
			return "", err
		}
		if s.media == nil {
			return "", apperror.NewBadRequest("pictures can't be stored right now")
		}
		name, err := s.media.StoreSitePicture(ctx, userID, up.Data, up.Name, mime)
		if err != nil {
			return "", err
		}
		stored = append(stored, name)
		return name, nil
	}
	if sub.Logo != nil {
		if draft.Logo, err = store(sitelook.KindLogo, sub.Logo); err != nil {
			discard()
			return sitelook.Settings{}, err
		}
	}
	if sub.Picture != nil {
		if draft.Picture, err = store(sitelook.KindPicture, sub.Picture); err != nil {
			discard()
			return sitelook.Settings{}, err
		}
	}

	// A picture that is neither the one already saved nor one just stored
	// here must be a site picture; this guards the invariant even though the
	// handler never passes request-supplied names.
	norm, err := sitelook.Validate(draft)
	if err != nil {
		discard()
		return sitelook.Settings{}, err
	}
	for _, name := range []string{norm.Logo, norm.Picture} {
		if name == "" || name == cur.Logo || name == cur.Picture || contains(stored, name) {
			continue
		}
		ok := false
		if s.media != nil {
			if ok, err = s.media.OwnsSitePicture(ctx, name); err != nil {
				discard()
				return sitelook.Settings{}, err
			}
		}
		if !ok {
			discard()
			return sitelook.Settings{}, apperror.NewBadRequest("that picture isn't one of the site's")
		}
	}

	saved, err := s.settings.UpdateSiteLook(ctx, norm)
	if err != nil {
		discard()
		return sitelook.Settings{}, err
	}

	// The save replaced these pictures; removing the files keeps storage from
	// filling with logos nobody sees. A failed delete only leaves a stray file.
	// A picture uploaded for a background the admin then didn't pick is
	// dropped by validation, so it is tidied here too.
	for _, old := range append([]string{cur.Logo, cur.Picture}, stored...) {
		if old != "" && old != saved.Logo && old != saved.Picture {
			s.deletePicture(ctx, old)
		}
	}
	return saved, nil
}

// deletePicture removes a site picture, logging rather than failing: nothing
// the admin did depends on the file being gone.
func (s *siteLookService) deletePicture(ctx context.Context, name string) {
	if s.media == nil {
		return
	}
	if err := s.media.DeleteSitePicture(ctx, name); err != nil {
		slog.Warn("removing a site picture", slog.String("file", name), slog.Any("error", err))
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
