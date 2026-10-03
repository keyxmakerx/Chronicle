package sitelook

import (
	"bytes"
	"image"
	_ "image/jpeg" // Registers the JPEG decoder for image.DecodeConfig.
	_ "image/png"  // Registers the PNG decoder for image.DecodeConfig.
	"net/http"

	_ "golang.org/x/image/webp" // Registers the WebP decoder for image.DecodeConfig.

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Upload kinds.
const (
	KindLogo    = "logo"
	KindPicture = "picture"
)

// allowedMIME are the image types accepted for a site upload. Sniffed from the
// bytes, never taken from the request, and SVG is not among them.
var allowedMIME = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true}

// CheckImage validates an upload before it is stored: size, real content
// type, and shape. A logo must be square-ish (width within 0.8-1.25 of the
// height); a sign-in picture must be wider than tall. It returns the sniffed
// MIME type.
func CheckImage(kind string, data []byte) (mime string, err error) {
	var limit int64
	switch kind {
	case KindLogo:
		limit = LogoMaxBytes
	case KindPicture:
		limit = PictureMaxBytes
	default:
		return "", apperror.NewBadRequest("unknown picture kind")
	}
	if len(data) == 0 {
		return "", apperror.NewBadRequest("no file provided")
	}
	if int64(len(data)) > limit {
		return "", apperror.NewBadRequest("that picture is too large")
	}
	mime = http.DetectContentType(data)
	if !allowedMIME[mime] {
		return "", apperror.NewBadRequest("use a PNG, JPEG or WebP picture")
	}
	cfg, _, derr := image.DecodeConfig(bytes.NewReader(data))
	if derr != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return "", apperror.NewBadRequest("that file is not a readable picture")
	}
	ratio := float64(cfg.Width) / float64(cfg.Height)
	switch kind {
	case KindLogo:
		if ratio < 0.8 || ratio > 1.25 {
			return "", apperror.NewBadRequest("the logo must be square or close to it")
		}
	case KindPicture:
		if ratio <= 1 {
			return "", apperror.NewBadRequest("the sign-in picture must be wider than it is tall")
		}
	}
	return mime, nil
}
