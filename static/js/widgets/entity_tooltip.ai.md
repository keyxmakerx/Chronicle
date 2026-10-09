# Entity Tooltip Widget

Previews of linked pages. Any element with `data-entity-preview="URL"` opens
the shared hover card (`static/js/hovercard.js`, see `hovercard.ai.md`) with
the page's picture, type, name, up to five fields and its opening lines, as
the page's `popup_config` allows. Behaviour and look belong to the hover
card; this file fetches the preview and turns it into card content.

Links still follow on click; on touch a long press opens the card pinned.

## API

```js
Chronicle.tooltip.attach(element, previewURL)  // make an element previewable
Chronicle.tooltip.detach(element)
Chronicle.tooltip.show(element, previewURL)    // open by hand
Chronicle.tooltip.hide()
Chronicle.tooltip.clearCache()                 // after editing a page's popup settings
```

`data-widget="entity-tooltip"` is still registered so a container can mark
that it holds previewable links; a document-wide binding does the work.

## Preview response

`name`, `type_name`, `type_icon`, `type_label`, `image_path`, `is_private`,
`attributes [{label, value}]`, `entry_excerpt`. Previews are LRU-cached
(100). A failed fetch (a deleted or hidden page) closes the card quietly.
