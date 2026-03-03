# Image Upload Widget

## Purpose

Client-side widget for uploading images to entities. Mounts on placeholder or
existing image elements, handles file selection, MIME validation, two-step upload
(POST media → PUT entity image), and page reload on success.

## Widget Registration

```js
Chronicle.register('image-upload', { init: function(el, config) { ... } });
```

Mounts on: `data-widget="image-upload"`

## Configuration (data-* attributes)

| Attribute | Required | Description |
|-----------|----------|-------------|
| `data-endpoint` | Yes | Entity image PUT endpoint, e.g., `/campaigns/:id/entities/:eid/image` |
| `data-upload-url` | Yes | Media upload POST endpoint, e.g., `/media/upload` |
| `data-csrf-token` | Yes | CSRF token for mutations |

## Upload Flow

1. User clicks widget element → hidden `<input type="file">` triggers file picker
2. Client validates: MIME type (JPEG, PNG, WebP, GIF) and size (10 MB max)
3. POST `/media/upload` with FormData (`file` + `usage_type=entity_image`)
4. Response: `{id, url, thumbnail_url}`
5. PUT entity endpoint with `{image_path: id}` (media UUID)
6. Full page reload on success

## Template Integration

In `entities/show.templ`, `blockImage()` component (lines 227-272):
- Entity has image: shows `<img>` with hover overlay containing widget
- Entity has no image: shows placeholder div with widget
- Permission: only rendered for Scribe+ role

## Known Issues

- Full page reload after upload (could use HTMX swap instead)
- User reports "click does nothing" — needs browser-level debugging
  (possible Firefox file input policy or widget mounting issue)
