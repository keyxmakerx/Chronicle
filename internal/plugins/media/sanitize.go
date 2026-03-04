package media

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"

	// Register WebP decoder so image.Decode recognises WebP input.
	_ "golang.org/x/image/webp"
)

// sanitizeImage re-encodes an uploaded image to strip ALL metadata (EXIF, IPTC,
// XMP, ICC profiles) and destroy any polyglot payloads (files that are both a
// valid image and executable JS/PHP/etc). The decode-then-encode pipeline through
// Go's standard image package produces a clean file containing only pixel data.
//
// This acts as Content Disarm & Reconstruction (CDR) -- a modern best-practice
// for protecting against malicious file uploads.
//
// Returns the sanitized bytes and the effective MIME type (WebP is re-encoded
// as JPEG since Go lacks a WebP encoder in the standard library).
func sanitizeImage(data []byte, mimeType string) ([]byte, string, error) {
	// Check dimensions BEFORE full decode to prevent decompression bombs.
	// DecodeConfig reads only the image header, using minimal memory.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("invalid image: %w", err)
	}
	if cfg.Width > maxImageDimension || cfg.Height > maxImageDimension {
		return nil, "", fmt.Errorf("image too large: %dx%d exceeds %d limit", cfg.Width, cfg.Height, maxImageDimension)
	}

	// Full decode strips all metadata -- Go's image.Decode only extracts
	// pixel data, silently discarding EXIF, IPTC, XMP, and other metadata.
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode image: %w", err)
	}

	// Re-encode to produce a clean file with zero metadata.
	var buf bytes.Buffer
	outMime := mimeType

	switch mimeType {
	case "image/jpeg":
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92})

	case "image/png":
		err = png.Encode(&buf, img)

	case "image/webp":
		// Go's x/image/webp has a decoder but no encoder.
		// Re-encode as JPEG which is safe and widely supported.
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92})
		outMime = "image/jpeg"

	case "image/gif":
		// Re-encoding a GIF strips animation (only first frame survives).
		// This is an acceptable security tradeoff -- animation is lost but
		// embedded payloads and metadata are destroyed.
		err = gif.Encode(&buf, img, nil)

	default:
		return nil, "", fmt.Errorf("unsupported MIME type for sanitization: %s", mimeType)
	}

	if err != nil {
		return nil, "", fmt.Errorf("re-encoding image: %w", err)
	}

	return buf.Bytes(), outMime, nil
}
