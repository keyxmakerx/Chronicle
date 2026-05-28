// subresource_grid.js — V2 sub-resource grid widget.
// Wave 1 PR 2 (C-CAL-V2-SUBRESOURCE-CARDS-A). Owns three jobs:
//
//   1. Drag-to-reorder cards via HTML5 D&D API (no library), firing
//      bulk-set PUT against the calendar's existing endpoint on drop.
//      Optimistic UI: card moves immediately; revert on error toast.
//
//   2. Drawer open/close + dirty tracking. Click a card → edit mode;
//      click Add → create mode. Save patches the in-memory payload +
//      bulk-set PUTs the full list.
//
//   3. Delete confirm (V1.5 verb-set discipline: explicit checkbox).
//
// Reads everything from data attributes on the root element so the
// JS is resource-agnostic. PR 3 (sub-resource cards batch B) inherits
// this widget by mounting onto its own root with the same attributes.
//
// Dependencies: window.Chronicle.apiFetch (CSRF-aware fetch wrapper);
// window.Chronicle.notify (toast helper). Both established prior.

(function () {
    'use strict';

    function init() {
        var root = document.querySelector('[data-subresource-kind]');
        if (!root) return;

        var kind = root.dataset.subresourceKind;
        var putURL = root.dataset.subresourcePutUrl;
        var csrfToken = root.dataset.csrfToken;
        var isOwner = root.dataset.isOwner === 'true';

        var payload = [];
        try {
            payload = JSON.parse(root.dataset.subresourcePayload || '[]');
        } catch (e) {
            console.error('subresource_grid: invalid payload', e);
            payload = [];
        }

        // Editor state: which index is being edited (-1 = create mode).
        var editingIndex = -1;
        var dirty = false;

        // --- Drawer ---
        var drawer = document.getElementById('subresource-drawer');

        function openDrawer(index) {
            if (!isOwner || !drawer) return;
            editingIndex = index;
            dirty = false;
            populateDrawer(index);
            drawer.classList.remove('hidden');
            // Focus the first input for keyboard-driven flow.
            var first = drawer.querySelector('[data-field]');
            if (first && typeof first.focus === 'function') {
                first.focus();
            }
            // Listen for dirty state on any field edit.
            drawer.querySelectorAll('[data-field]').forEach(function (el) {
                el.addEventListener('input', markDirty, { once: false });
                el.addEventListener('change', markDirty, { once: false });
            });
        }

        function markDirty() { dirty = true; }

        function closeDrawer(force) {
            if (!drawer) return;
            if (dirty && !force) {
                if (!window.confirm('Discard unsaved changes?')) return;
            }
            drawer.classList.add('hidden');
            // Reset confirm overlay if it was up.
            var confirmEl = drawer.querySelector('[data-drawer-confirm]');
            if (confirmEl) confirmEl.classList.add('hidden');
            editingIndex = -1;
            dirty = false;
        }

        function populateDrawer(index) {
            var title = drawer.querySelector('[data-drawer-title]');
            var deleteBtn = drawer.querySelector('[data-drawer-delete]');
            var item = index >= 0 ? payload[index] : {};

            if (title) {
                title.textContent = index >= 0 ? ('Edit ' + singular(kind)) : ('Add ' + singular(kind));
            }
            if (deleteBtn) {
                if (index >= 0) deleteBtn.classList.remove('hidden');
                else deleteBtn.classList.add('hidden');
            }

            // Populate each field input from item.
            drawer.querySelectorAll('[data-field]').forEach(function (el) {
                var field = el.dataset.field;
                var value = item[field];
                if (el.type === 'checkbox') {
                    el.checked = Boolean(value);
                } else if (el.type === 'color') {
                    el.value = value || '#aabbcc';
                } else {
                    el.value = (value === undefined || value === null) ? '' : value;
                }
            });
        }

        function readDrawerInput() {
            var item = {};
            drawer.querySelectorAll('[data-field]').forEach(function (el) {
                var field = el.dataset.field;
                if (el.type === 'checkbox') {
                    item[field] = el.checked;
                } else if (el.type === 'number') {
                    var n = parseFloat(el.value);
                    if (!isNaN(n)) item[field] = n;
                } else {
                    var v = el.value.trim();
                    if (v !== '') item[field] = v;
                }
            });
            return item;
        }

        function saveDrawer() {
            var item = readDrawerInput();
            // Minimal client-side validation: name required across all
            // four resources. Server-side validation is the authority;
            // this catches the common operator typo quickly.
            if (!item.name) {
                window.Chronicle.notify('Name is required', 'error');
                return;
            }
            // Preserve sort_order: new items append; edited items keep
            // their slot.
            var next;
            if (editingIndex >= 0) {
                item.sort_order = payload[editingIndex].sort_order || (editingIndex + 1);
                next = payload.slice();
                next[editingIndex] = item;
            } else {
                item.sort_order = payload.length + 1;
                next = payload.concat([item]);
            }
            commitPayload(next, true);
        }

        // --- Delete confirm flow ---
        var confirmEl = drawer ? drawer.querySelector('[data-drawer-confirm]') : null;

        function startDelete() {
            if (editingIndex < 0 || !confirmEl) return;
            confirmEl.classList.remove('hidden');
            var check = confirmEl.querySelector('[data-drawer-confirm-check]');
            var doBtn = confirmEl.querySelector('[data-drawer-confirm-do]');
            if (check) check.checked = false;
            if (doBtn) doBtn.disabled = true;
        }

        function commitDelete() {
            if (editingIndex < 0) return;
            var next = payload.slice();
            next.splice(editingIndex, 1);
            // Re-pack sort_order.
            next.forEach(function (it, i) { it.sort_order = i + 1; });
            commitPayload(next, true);
        }

        // --- Bulk-set PUT + UI refresh ---
        function commitPayload(next, closeAfter) {
            var prev = payload.slice();
            payload = next;
            renderGrid();
            // Optimistic dnd / save / delete; revert on PUT failure.
            window.Chronicle.apiFetch(putURL, {
                method: 'PUT',
                body: next,
                headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (resp) {
                if (!resp.ok) {
                    return resp.json().catch(function () { return {}; }).then(function (b) {
                        throw new Error((b && b.message) || 'Save failed');
                    });
                }
                if (closeAfter) closeDrawer(true);
            }).catch(function (e) {
                payload = prev;
                renderGrid();
                window.Chronicle.notify((e && e.message) || 'Save failed; reverted.', 'error');
            });
        }

        // --- Grid render ---
        // Re-renders cards from payload after save/delete/dnd. Pure
        // DOM manipulation (no server re-fetch) keeps the optimistic
        // UI snappy. PR 4's event-card composite may replace this
        // with HTMX hx-swap if/when full-page consistency wins.
        function renderGrid() {
            var grid = document.querySelector('[data-grid-root]');
            // If the grid had collapsed to empty-state, full reload
            // is simpler than reconstructing the grid + add-card.
            if (!grid && payload.length > 0) {
                window.location.reload();
                return;
            }
            if (!grid) return;
            // Locate existing card elements (everything with data-card-index)
            // and the add-card (data-add-card).
            var addCard = grid.querySelector('[data-add-card]');
            var existing = grid.querySelectorAll('[data-card-index]');
            existing.forEach(function (el) { el.remove(); });
            payload.forEach(function (item, i) {
                var card = buildCard(item, i);
                if (addCard) {
                    grid.insertBefore(card, addCard);
                } else {
                    grid.appendChild(card);
                }
            });
            if (payload.length === 0 && !grid.querySelector('[data-add-card]')) {
                window.location.reload();
            }
        }

        function buildCard(item, index) {
            var div = document.createElement('div');
            div.className = 'card card-elev p-3 cursor-pointer group transition-transform duration-micro hover:-translate-y-px';
            if (kind === 'weekdays' && item.is_rest_day) {
                div.className += ' ring-1 ring-accent/40';
            }
            div.setAttribute('role', 'listitem');
            div.setAttribute('tabindex', '0');
            div.setAttribute('draggable', isOwner ? 'true' : 'false');
            div.dataset.cardIndex = String(index);
            div.dataset.cardId = String(index);

            var head = document.createElement('div');
            head.className = 'flex items-start justify-between gap-2';

            var leftSide = document.createElement('div');
            leftSide.className = 'flex items-center gap-2 min-w-0';
            if (item.color) {
                var swatch = document.createElement('span');
                swatch.className = 'inline-block w-3 h-3 rounded-full border border-edge flex-shrink-0';
                swatch.style.backgroundColor = item.color;
                swatch.setAttribute('aria-hidden', 'true');
                leftSide.appendChild(swatch);
            }
            var nameEl = document.createElement('span');
            nameEl.className = 'text-sm font-semibold text-fg truncate';
            nameEl.textContent = item.name || '(unnamed)';
            leftSide.appendChild(nameEl);
            head.appendChild(leftSide);

            if (isOwner) {
                var handle = document.createElement('span');
                handle.className = 'opacity-0 group-hover:opacity-100 text-fg-secondary text-xs cursor-grab transition-opacity duration-micro';
                handle.setAttribute('aria-label', 'Drag to reorder');
                handle.dataset.dragHandle = 'true';
                handle.innerHTML = '<i class="fa-solid fa-grip-vertical" aria-hidden="true"></i>';
                head.appendChild(handle);
            }
            div.appendChild(head);

            var sub = buildSubtitle(item);
            if (sub) {
                var subEl = document.createElement('div');
                subEl.className = 'text-xs text-fg-secondary mt-1';
                subEl.textContent = sub;
                div.appendChild(subEl);
            }
            attachCardListeners(div, index);
            return div;
        }

        function buildSubtitle(item) {
            switch (kind) {
                case 'months': {
                    var parts = [(item.days === 1 ? '1 day' : (item.days || 0) + ' days')];
                    if (item.is_intercalary) parts.push('intercalary');
                    if (item.leap_year_days) parts.push('+' + item.leap_year_days + ' leap');
                    return parts.join(' · ');
                }
                case 'weekdays':
                    return item.is_rest_day ? 'rest day' : '';
                case 'moons':
                    return 'cycle ' + (item.cycle_days || 0) + 'd';
                case 'seasons':
                    return 'month ' + item.start_month + ' · day ' + item.start_day +
                        ' → month ' + item.end_month + ' · day ' + item.end_day;
            }
            return '';
        }

        // --- HTML5 D&D wiring (mirrors calendar.templ's event dnd) ---
        var dragSrcIndex = -1;

        function attachCardListeners(card, index) {
            card.addEventListener('click', function (e) {
                // Ignore clicks on the drag handle (it's used to start
                // a drag; clicking it shouldn't open the drawer).
                if (e.target.closest('[data-drag-handle]')) return;
                openDrawer(index);
            });
            card.addEventListener('keydown', function (e) {
                if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    openDrawer(index);
                }
            });
            if (!isOwner) return;
            card.addEventListener('dragstart', function (e) {
                dragSrcIndex = index;
                e.dataTransfer.effectAllowed = 'move';
                e.dataTransfer.setData('text/plain', String(index));
                card.classList.add('opacity-30');
            });
            card.addEventListener('dragend', function () {
                card.classList.remove('opacity-30');
                dragSrcIndex = -1;
            });
            card.addEventListener('dragover', function (e) {
                e.preventDefault();
                e.dataTransfer.dropEffect = 'move';
                card.classList.add('ring-2', 'ring-accent');
            });
            card.addEventListener('dragleave', function () {
                card.classList.remove('ring-2', 'ring-accent');
            });
            card.addEventListener('drop', function (e) {
                e.preventDefault();
                card.classList.remove('ring-2', 'ring-accent');
                if (dragSrcIndex < 0 || dragSrcIndex === index) return;
                var next = payload.slice();
                var moved = next.splice(dragSrcIndex, 1)[0];
                next.splice(index, 0, moved);
                next.forEach(function (it, i) { it.sort_order = i + 1; });
                commitPayload(next, false);
            });
        }

        // --- Wire existing cards (server-rendered first pass) ---
        document.querySelectorAll('[data-card-index]').forEach(function (card) {
            var idx = parseInt(card.dataset.cardIndex, 10);
            if (!isNaN(idx)) attachCardListeners(card, idx);
        });

        // --- Add-card affordance ---
        document.querySelectorAll('[data-add-card]').forEach(function (el) {
            el.addEventListener('click', function () { openDrawer(-1); });
        });

        // --- Drawer controls ---
        if (drawer) {
            drawer.querySelectorAll('[data-drawer-close]').forEach(function (el) {
                el.addEventListener('click', function () { closeDrawer(false); });
            });
            var backdrop = drawer.querySelector('[data-drawer-backdrop]');
            if (backdrop) backdrop.addEventListener('click', function () { closeDrawer(false); });

            var saveBtn = drawer.querySelector('[data-drawer-save]');
            if (saveBtn) saveBtn.addEventListener('click', saveDrawer);

            var delBtn = drawer.querySelector('[data-drawer-delete]');
            if (delBtn) delBtn.addEventListener('click', startDelete);

            if (confirmEl) {
                var check = confirmEl.querySelector('[data-drawer-confirm-check]');
                var doBtn = confirmEl.querySelector('[data-drawer-confirm-do]');
                if (check && doBtn) {
                    check.addEventListener('change', function () { doBtn.disabled = !check.checked; });
                }
                if (doBtn) doBtn.addEventListener('click', commitDelete);
                var cancelBtn = confirmEl.querySelector('[data-drawer-confirm-cancel]');
                if (cancelBtn) cancelBtn.addEventListener('click', function () {
                    confirmEl.classList.add('hidden');
                });
            }

            // Esc closes drawer (with dirty guard).
            document.addEventListener('keydown', function (e) {
                if (e.key === 'Escape' && !drawer.classList.contains('hidden')) {
                    closeDrawer(false);
                }
            });
        }
    }

    function singular(kind) {
        switch (kind) {
            case 'months': return 'month';
            case 'weekdays': return 'weekday';
            case 'moons': return 'moon';
            case 'seasons': return 'season';
        }
        return 'item';
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
