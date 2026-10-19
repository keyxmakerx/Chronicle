# token_maker.js and token_frames.js

The token maker (`window.Chronicle.tokenMaker`): a creature's map token made
from its picture, in one of thirty frames drawn in code, in any colour.

**Where it opens.** The page picture's "Make token" chip (`blockImage` in
`internal/plugins/entities/show.templ`) shows for Scribes and up when the page
has a picture and its type, or the page itself, declares the `token_image`
field (`showsTokenMaker`). Draw Steel's creature editor opens it from its token
slot with its own `save` callback. Any caller can:

```js
Chronicle.tokenMaker.open({
  campaignId, entityId, name,  // whose token
  art,      // same-origin URL of the picture to start from, or ''
  from,     // the element it grows out of and hands focus back to
  save,     // 'field', or function(mediaId) for a caller that keeps it itself
  onClose   // optional, called after it closes, saved or not
});
```

**Loading (ADR-063).** `boot.js` puts a stub `Chronicle.tokenMaker` on every
page and opens it for any `[data-token-maker]` control. The first open fetches
`token_maker.js` from the layout's `#chronicle-token-maker-src`, and that file
fetches the frames (`token_frames.js`, about 350 KB), its styles
(`static/css/token_maker.css`) and the picker's previews
(`static/img/token_frames.webp`) from the same element. A page pays for none of
it until someone makes a token.

**Previews are stills.** The picker never draws thirty frames live: each tile
is one 128 px cell of the previews sprite, in the engine's design order, eight
to a row. Only the frame being edited is drawn live, plus a moving frame the
pointer rests on (250 ms), which plays in its tile with this picture and colour.
After adding, removing, reordering or restyling a design, run
`node tools/token_frames_previews.mjs`; `test/js/token_maker_browser.test.mjs`
fails when the sprite no longer matches the engine.

**Saving.** Save draws the token 512 px square as a PNG and uploads it
(`POST /media/upload`, `usage_type=entity_image`): the media sanitiser keeps a
PNG's transparency, while a WebP comes back as a JPEG with dark corners. A
moving frame is saved as its still picture. With `save: 'field'` it then writes
`fields_patch: { token_image: <id> }` to the page, which the hover card shows on
the picture's corner and the Foundry module uses as the creature's token. The
page picture itself is never changed. It fires `chronicle:token-saved`
(`{ entityId, mediaId, url }`) on the control that opened it, and remembers the
frame, colour and shape in `localStorage` for the next token.

**The frames.** `token_frames.js` draws one token into a canvas it is handed
(`ChronicleTokenFrames.render(ctx, w, h, o)`) and touches nothing else. It is
built from the signed demo's parts; each design names its own main colour, so
the colour control can turn any of them to the creature's colour. Materials are
worked out once at a master size and cached, so the first drawing of each takes
a moment.

**Motion.** Moving frames play at up to 30 frames a second (the map tokens and
the hovered tile at 15), drawing at most half the time, and rest with
`MotionRest`. Calm fades the maker in and holds frames still; Off and a device
asking for less motion show it at once, still.

**Closing.** Close, Cancel or Escape. Unsaved work asks in place first
(`Chronicle.askSlip`); a second Escape goes back to the work. The page behind is
`inert` while it is open, and focus returns to `from` (or `onClose` places it).
