# shop_inventory.js

## Purpose

Inventory editor for a shop entity: lists the items it sells with price,
quantity and in-stock toggle, lets Scribes add existing entities or create a
new item page on the spot, and shows a recent-transactions panel.

## Mount

`data-widget="shop_inventory"` (underscore, unlike most widget names), loaded
by a `<script defer>` tag in `layouts/base.templ`. Mounted by the shop
inventory block in `plugins/entities/show.templ`, next to the shop-room mount.
The block type is registered in `plugins/entities/block_registry_core.go`.

| Attribute | Meaning |
|---|---|
| `data-relations-endpoint` | `/campaigns/:id/entities/:eid/relations` |
| `data-entity-search-endpoint` | `/campaigns/:id/entities/search` |
| `data-quick-create-endpoint` | `/campaigns/:id/entities/quick-create` |
| `data-campaign-url` | `/campaigns/:id`, base for armory calls |
| `data-editable` | Set when the viewer is Scribe or above |
| `data-csrf-token` | Read into a variable, unused; `apiFetch` attaches the header |

## Data it calls

- `GET <relations>`; keeps only `relationType === 'sells'`. Field names
  (`relationType`) match `widgets/relations/model.go`.
- `POST <relations>` with `{targetEntityId, relationType: 'sells',
  reverseRelationType: 'sold by', metadata: {price, quantity, in_stock}}`.
- `PUT <relations>/:rid/metadata`, `DELETE <relations>/:rid`.
- `GET <search>?q=`, `POST <quick-create>`, and the entity-type list at the
  quick-create URL with `/quick-create` replaced by `/types`.
- `GET <campaign-url>/armory/transactions?shop=<eid>&per_page=20`; the entity
  id is parsed from the relations URL.

All exist in `routes_snapshot.txt`.

## Events

None emitted or listened to.

## Gotchas

- Item data lives in the relation's `metadata` (price, quantity, in_stock); a
  custom item without a linked page stores its name there too.
- A failed inventory load is swallowed, leaving the widget on its loading
  state.
- Shop-room and transaction-log widgets are separate mounts on the same page.
