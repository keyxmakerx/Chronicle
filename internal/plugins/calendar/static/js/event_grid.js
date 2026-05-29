// event_grid.js — V2 calendar event grid widget. Wave 1 PR 4
// (C-CAL-V2-EVENT-CARD-COMPOSITE). Owns three drag patterns + the
// event detail drawer + the visibility editor chip-row builder.
//
// Inherits the data-drawer-* attribute contract from
// subresource_grid.js (PR #364) for drawer chrome. Adds calendar-
// grid-specific behaviors:
//
//   1. Click event card → drawer opens in edit mode
//   2. Click "+" affordance on empty cell → drawer opens in create mode
//   3. Click overflow "+N more" → expand cell (full reload to a
//      day-detail view; full popover deferred to PR 5 stretch)
//   4. Drag event card → reschedule via PUT on existing
//      /campaigns/:cid/calendars/:calId/events/:eid endpoint
//   5. Visibility editor: add/remove allow + deny rule chips inline;
//      effective-audience summary computed client-side
//
// Drag-to-create (cell click-and-drag) + drag-to-resize (ribbon edge)
// are wired but minimal in this PR; PR 5 can polish.

(function () {
    'use strict';

    function init() {
        var root = document.querySelector('[data-cal-v2-root]');
        if (!root) return;

        var calendarID = root.dataset.calV2CalendarId;
        var campaignID = root.dataset.calV2CampaignId;
        var csrfToken = root.dataset.calV2CsrfToken;
        var isScribe = root.dataset.calV2IsScribe === 'true';
        var isOwner = root.dataset.calV2IsOwner === 'true';

        if (!isScribe || !calendarID) return; // no edit affordances

        var events = [];
        try {
            events = JSON.parse(root.dataset.calV2Events || '[]');
        } catch (e) {
            console.error('event_grid: invalid events payload', e);
        }

        var drawer = document.getElementById('event-v2-drawer');
        if (!drawer) return;

        var editingID = null; // null = create mode; string = edit mode
        var dirty = false;

        function eventByID(id) {
            for (var i = 0; i < events.length; i++) {
                if (events[i].id === id) return events[i];
            }
            return null;
        }

        // --- Drawer: open / close / populate -------------------

        function openDrawer(idOrPrefill) {
            var prefill = {};
            if (typeof idOrPrefill === 'string') {
                editingID = idOrPrefill;
                prefill = eventByID(idOrPrefill) || {};
            } else if (idOrPrefill && typeof idOrPrefill === 'object') {
                editingID = null;
                prefill = idOrPrefill;
            } else {
                editingID = null;
            }

            dirty = false;
            populateDrawer(prefill);
            drawer.classList.remove('hidden');
            var first = drawer.querySelector('[data-field]');
            if (first && typeof first.focus === 'function') first.focus();

            drawer.querySelectorAll('[data-field]').forEach(function (el) {
                el.addEventListener('input', markDirty);
                el.addEventListener('change', markDirty);
            });

            // Initialize the visibility editor state from the event.
            initVisibilityEditor(prefill);
        }

        function markDirty() { dirty = true; }

        function closeDrawer(force) {
            if (dirty && !force) {
                if (!window.confirm('Discard unsaved changes?')) return;
            }
            drawer.classList.add('hidden');
            var confirmEl = drawer.querySelector('[data-drawer-confirm]');
            if (confirmEl) confirmEl.classList.add('hidden');
            editingID = null;
            dirty = false;
        }

        function populateDrawer(item) {
            var title = drawer.querySelector('[data-drawer-title]');
            var deleteBtn = drawer.querySelector('[data-drawer-delete]');

            if (title) title.textContent = editingID ? 'Edit event' : 'Add event';
            if (deleteBtn) {
                if (editingID && isOwner) deleteBtn.classList.remove('hidden');
                else deleteBtn.classList.add('hidden');
            }

            drawer.querySelectorAll('[data-field]').forEach(function (el) {
                var field = el.dataset.field;
                var value = item[field];
                if (el.type === 'checkbox') el.checked = Boolean(value);
                else el.value = (value === undefined || value === null) ? '' : value;
            });
        }

        function readDrawer() {
            var body = {};
            drawer.querySelectorAll('[data-field]').forEach(function (el) {
                var field = el.dataset.field;
                if (el.type === 'checkbox') body[field] = el.checked;
                else if (el.type === 'number') {
                    var n = parseInt(el.value, 10);
                    if (!isNaN(n)) body[field] = n;
                } else {
                    var v = el.value.trim();
                    if (v !== '') body[field] = v;
                }
            });
            // Pull the visibility editor state into the body.
            var vis = readVisibilityEditor();
            body.visibility = vis.mode === 'public' ? 'everyone' : 'dm_only';
            if (vis.rules.length > 0) {
                body.visibility_rules = JSON.stringify(vis.rules);
            }
            return body;
        }

        function saveDrawer() {
            var body = readDrawer();
            if (!body.name) {
                window.Chronicle.notify('Name is required', 'error');
                return;
            }
            var url, method;
            if (editingID) {
                url = '/campaigns/' + campaignID + '/calendars/' + calendarID + '/events/' + editingID;
                method = 'PUT';
            } else {
                url = '/campaigns/' + campaignID + '/calendars/' + calendarID + '/events';
                method = 'POST';
            }
            window.Chronicle.apiFetch(url, {
                method: method,
                body: body,
                headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (resp) {
                if (!resp.ok) {
                    return resp.json().catch(function () { return {}; }).then(function (b) {
                        throw new Error((b && b.message) || 'Save failed');
                    });
                }
                closeDrawer(true);
                window.location.reload();
            }).catch(function (e) {
                window.Chronicle.notify((e && e.message) || 'Save failed', 'error');
            });
        }

        function commitDelete() {
            if (!editingID) return;
            var url = '/campaigns/' + campaignID + '/calendars/' + calendarID + '/events/' + editingID;
            window.Chronicle.apiFetch(url, {
                method: 'DELETE',
                headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (resp) {
                if (!resp.ok) {
                    throw new Error('Delete failed');
                }
                closeDrawer(true);
                window.location.reload();
            }).catch(function (e) {
                window.Chronicle.notify((e && e.message) || 'Delete failed', 'error');
            });
        }

        // --- Visibility editor (chip-row builder per Q-V2-7) ---

        var visEditor = drawer.querySelector('[data-visibility-editor]');
        var visRules = [];

        function initVisibilityEditor(event) {
            visRules = [];
            if (event && event.visibility_rules) {
                try {
                    var parsed = JSON.parse(event.visibility_rules);
                    if (Array.isArray(parsed)) visRules = parsed;
                } catch (e) {
                    // Malformed rules ignored; widget renders empty chip row.
                }
            }
            if (!visEditor) return;
            // Set the radio state.
            var isPublic = !event || event.visibility === 'everyone';
            var radios = visEditor.querySelectorAll('input[type="radio"][data-visibility-mode]');
            radios.forEach(function (r) {
                r.checked = (r.dataset.visibilityMode === (isPublic ? 'public' : 'specific'));
            });
            updateSpecificPanel(isPublic);
            renderChipRow();
            updateSummary();
            wireVisibilityHandlers();
        }

        function updateSpecificPanel(isPublic) {
            var panel = visEditor && visEditor.querySelector('[data-visibility-specific-panel]');
            if (!panel) return;
            panel.style.display = isPublic ? 'none' : '';
        }

        function renderChipRow() {
            var row = visEditor && visEditor.querySelector('[data-visibility-chip-row]');
            if (!row) return;
            row.innerHTML = '';
            visRules.forEach(function (rule, i) {
                row.appendChild(buildChip(rule, i));
            });
            var hidden = visEditor.querySelector('[data-visibility-rules-json]');
            if (hidden) hidden.value = JSON.stringify(visRules);
        }

        function buildChip(rule, i) {
            var span = document.createElement('span');
            var color = rule.mode === 'allow' ? 'border-green-500/40 bg-green-500/10' : 'border-amber-500/40 bg-amber-500/10';
            var iconColor = rule.mode === 'allow' ? 'text-green-500' : 'text-amber-500';
            span.className = 'chip-add inline-flex items-center gap-1 text-xs rounded px-2 py-1 border ' + color;
            span.dataset.visibilityChipIndex = String(i);

            var icon = document.createElement('span');
            icon.className = iconColor;
            icon.setAttribute('aria-hidden', 'true');
            icon.innerHTML = rule.mode === 'allow' ? '<i class="fa-solid fa-check"></i>' : '<i class="fa-solid fa-ban"></i>';
            span.appendChild(icon);

            var label = document.createElement('span');
            label.className = 'text-fg';
            label.textContent = rule.label || (rule.kind === 'user' ? '@' + rule.target : rule.target);
            span.appendChild(label);

            var remove = document.createElement('button');
            remove.type = 'button';
            remove.className = 'text-fg-secondary hover:text-fg ml-1 transition-colors duration-micro';
            remove.setAttribute('aria-label', 'Remove rule');
            remove.innerHTML = '<i class="fa-solid fa-xmark text-[10px]" aria-hidden="true"></i>';
            remove.addEventListener('click', function () {
                visRules.splice(i, 1);
                renderChipRow();
                updateSummary();
                markDirty();
            });
            span.appendChild(remove);
            return span;
        }

        function updateSummary() {
            var summaryEl = visEditor && visEditor.querySelector('[data-visibility-summary]');
            if (!summaryEl) return;
            var checked = visEditor.querySelector('input[type="radio"][data-visibility-mode]:checked');
            var isPublic = checked && checked.dataset.visibilityMode === 'public';
            if (isPublic) {
                summaryEl.textContent = 'Everyone with campaign access can see this.';
                return;
            }
            if (visRules.length === 0) {
                summaryEl.textContent = 'Nobody yet — add an allow rule to grant access.';
                return;
            }
            var allows = visRules.filter(function (r) { return r.mode === 'allow'; });
            var denies = visRules.filter(function (r) { return r.mode === 'deny'; });
            if (denies.length === 0) {
                var labels = allows.slice(0, 3).map(chipLabel);
                var extra = allows.length > 3 ? ' and ' + (allows.length - 3) + ' more' : '';
                summaryEl.textContent = labels.join(', ') + extra + ' can see this.';
            } else if (allows.length === 0) {
                summaryEl.textContent = 'Everyone except: ' + denies.map(chipLabel).join(', ');
            } else {
                summaryEl.textContent = allows.length + ' allow rule(s), ' + denies.length + ' deny rule(s). Server resolves precedence.';
            }
        }

        function chipLabel(rule) {
            return rule.label || (rule.kind === 'user' ? '@' + rule.target : rule.target);
        }

        function readVisibilityEditor() {
            if (!visEditor) return { mode: 'public', rules: [] };
            var checked = visEditor.querySelector('input[type="radio"][data-visibility-mode]:checked');
            var mode = checked ? checked.dataset.visibilityMode : 'public';
            return { mode: mode, rules: visRules };
        }

        function wireVisibilityHandlers() {
            if (!visEditor || visEditor.dataset.visibilityWired === '1') {
                // Re-init only updates state; handlers stay bound from first open.
                return;
            }
            visEditor.dataset.visibilityWired = '1';

            visEditor.querySelectorAll('input[type="radio"][data-visibility-mode]').forEach(function (r) {
                r.addEventListener('change', function () {
                    updateSpecificPanel(r.dataset.visibilityMode === 'public');
                    updateSummary();
                    markDirty();
                });
            });

            var addButtons = visEditor.querySelectorAll('[data-visibility-add]');
            addButtons.forEach(function (b) {
                b.addEventListener('click', function () {
                    openPicker(b.dataset.visibilityAdd);
                });
            });

            var picker = visEditor.querySelector('[data-visibility-picker]');
            if (picker) {
                var confirm = picker.querySelector('[data-visibility-picker-confirm]');
                var cancel = picker.querySelector('[data-visibility-picker-cancel]');
                var input = picker.querySelector('[data-visibility-picker-input]');
                if (cancel) cancel.addEventListener('click', function () { picker.classList.add('hidden'); });
                if (confirm) {
                    confirm.addEventListener('click', function () {
                        var kindEl = picker.querySelector('[data-visibility-picker-kind]');
                        var kind = kindEl ? kindEl.value : 'user';
                        var target = input ? input.value.trim() : '';
                        if (!target) return;
                        visRules.push({
                            mode: picker.dataset.pickerMode || 'allow',
                            kind: kind,
                            target: target,
                            label: kind === 'user' ? '@' + target : target,
                        });
                        if (input) input.value = '';
                        picker.classList.add('hidden');
                        renderChipRow();
                        updateSummary();
                        markDirty();
                    });
                }
            }
        }

        function openPicker(mode) {
            var picker = visEditor && visEditor.querySelector('[data-visibility-picker]');
            if (!picker) return;
            picker.classList.remove('hidden');
            picker.dataset.pickerMode = mode;
            var input = picker.querySelector('[data-visibility-picker-input]');
            if (input) input.focus();
        }

        // --- Drawer controls ---------------------

        drawer.querySelectorAll('[data-drawer-close]').forEach(function (el) {
            el.addEventListener('click', function () { closeDrawer(false); });
        });
        var backdrop = drawer.querySelector('[data-drawer-backdrop]');
        if (backdrop) backdrop.addEventListener('click', function () { closeDrawer(false); });

        var saveBtn = drawer.querySelector('[data-drawer-save]');
        if (saveBtn) saveBtn.addEventListener('click', saveDrawer);

        var delBtn = drawer.querySelector('[data-drawer-delete]');
        var confirmEl = drawer.querySelector('[data-drawer-confirm]');
        if (delBtn && confirmEl) {
            delBtn.addEventListener('click', function () {
                confirmEl.classList.remove('hidden');
                var check = confirmEl.querySelector('[data-drawer-confirm-check]');
                var doBtn = confirmEl.querySelector('[data-drawer-confirm-do]');
                if (check) check.checked = false;
                if (doBtn) doBtn.disabled = true;
            });
            var check = confirmEl.querySelector('[data-drawer-confirm-check]');
            var doBtn = confirmEl.querySelector('[data-drawer-confirm-do]');
            if (check && doBtn) check.addEventListener('change', function () { doBtn.disabled = !check.checked; });
            if (doBtn) doBtn.addEventListener('click', commitDelete);
            var cancel = confirmEl.querySelector('[data-drawer-confirm-cancel]');
            if (cancel) cancel.addEventListener('click', function () { confirmEl.classList.add('hidden'); });
        }

        document.addEventListener('keydown', function (e) {
            if (e.key === 'Escape' && !drawer.classList.contains('hidden')) closeDrawer(false);
        });

        // --- Card click → edit; cell + → create ---

        document.querySelectorAll('[data-event-card]').forEach(function (card) {
            card.addEventListener('click', function () {
                openDrawer(card.dataset.eventId);
            });
        });

        document.querySelectorAll('[data-cell-add-event]').forEach(function (btn) {
            btn.addEventListener('click', function (e) {
                e.stopPropagation();
                var cell = btn.closest('.cell-drop-target');
                if (!cell) { openDrawer({}); return; }
                openDrawer({
                    year: parseInt(cell.dataset.cellYear, 10),
                    month: parseInt(cell.dataset.cellMonth, 10),
                    day: parseInt(cell.dataset.cellDay, 10),
                });
            });
        });

        // --- Drag-to-reschedule via existing PUT endpoint --

        var dragSrc = null;
        document.querySelectorAll('[data-event-card]').forEach(function (card) {
            card.addEventListener('dragstart', function (e) {
                dragSrc = card.dataset.eventId;
                e.dataTransfer.effectAllowed = 'move';
                e.dataTransfer.setData('text/plain', dragSrc || '');
                card.classList.add('drag-ghost');
            });
            card.addEventListener('dragend', function () {
                card.classList.remove('drag-ghost');
                dragSrc = null;
            });
        });
        document.querySelectorAll('.cell-drop-target').forEach(function (cell) {
            cell.addEventListener('dragover', function (e) {
                e.preventDefault();
                e.dataTransfer.dropEffect = 'move';
                cell.classList.add('ring-2', 'ring-accent');
            });
            cell.addEventListener('dragleave', function () {
                cell.classList.remove('ring-2', 'ring-accent');
            });
            cell.addEventListener('drop', function (e) {
                e.preventDefault();
                cell.classList.remove('ring-2', 'ring-accent');
                if (!dragSrc) return;
                var event = eventByID(dragSrc);
                if (!event) return;
                var newYear = parseInt(cell.dataset.cellYear, 10);
                var newMonth = parseInt(cell.dataset.cellMonth, 10);
                var newDay = parseInt(cell.dataset.cellDay, 10);
                if (event.year === newYear && event.month === newMonth && event.day === newDay) return;

                var body = Object.assign({}, event, {
                    year: newYear,
                    month: newMonth,
                    day: newDay,
                });
                var url = '/campaigns/' + campaignID + '/calendars/' + calendarID + '/events/' + dragSrc;
                window.Chronicle.apiFetch(url, {
                    method: 'PUT',
                    body: body,
                    headers: { 'X-CSRF-Token': csrfToken },
                }).then(function (resp) {
                    if (!resp.ok) throw new Error('Move failed');
                    cell.classList.add('drop-receive-pulse');
                    setTimeout(function () { window.location.reload(); }, 250);
                }).catch(function (e) {
                    window.Chronicle.notify((e && e.message) || 'Move failed', 'error');
                });
            });
        });
    }

    // --- Drag-to-resize on Month-view multi-day ribbons (Wave 1.7A §B) ---
    //
    // Pointer-capture pattern: pointerdown on the right-edge handle
    // captures the pointer to the handle element so pointermove/up
    // events route to it even when the cursor strays off the handle.
    // stopPropagation() prevents the ribbon body's dragstart
    // (drag-to-reschedule) from firing. snap-to-day-boundary keeps
    // the resize semantically discrete (no sub-day resize on Month
    // view per dispatch §A.2).
    //
    // Browser support: setPointerCapture is universal in Chromium +
    // modern Firefox + Safari 13+. Older Safari falls back to
    // document-level pointermove tracking (graceful degradation; the
    // affordance still works but cursor-off-handle may not track as
    // smoothly). Documented as known limitation per stop-and-flag.
    function initRibbonResize() {
        var root = document.querySelector('[data-cal-v2-root]');
        if (!root) return;
        var calendarID = root.dataset.calV2CalendarId;
        var campaignID = root.dataset.calV2CampaignId;
        var csrfToken = root.dataset.calV2CsrfToken;
        if (!calendarID) return;

        var events = [];
        try {
            events = JSON.parse(root.dataset.calV2Events || '[]');
        } catch (e) {
            console.error('event_grid resize: invalid events payload', e);
        }

        // Cache event-by-id lookup; the parent init() block defines
        // an equivalent closure but we redefine locally to keep the
        // resize handler self-contained.
        function eventByID(id) {
            for (var i = 0; i < events.length; i++) {
                if (events[i].id === id) return events[i];
            }
            return null;
        }

        // Active drag state — single resize at a time.
        var resizeState = null;

        function onPointerDown(e) {
            var handle = e.target.closest('[data-ribbon-resize="right"]');
            if (!handle) return;
            var ribbon = handle.closest('[data-event-id]');
            if (!ribbon) return;
            var eventID = ribbon.dataset.eventId;
            var ev = eventByID(eventID);
            if (!ev) return;
            // Suppress the ribbon body's drag-to-reschedule
            // (it shares the same DOM but different gesture target).
            e.stopPropagation();
            e.preventDefault();
            try {
                handle.setPointerCapture(e.pointerId);
            } catch (err) {
                // setPointerCapture unsupported (older Safari);
                // fall through to document-level tracking via the
                // pointermove handler registered globally below.
            }
            // Compute starting state.
            var ribbonRect = ribbon.getBoundingClientRect();
            var grid = ribbon.parentElement; // the .grid container
            var gridRect = grid.getBoundingClientRect();
            var cols = computeGridColumnCount(grid);
            var cellWidth = gridRect.width / cols;
            // Read existing grid-column to find the current start col + span.
            var styleParse = parseGridColumn(ribbon.style.gridColumn);
            resizeState = {
                handle: handle,
                ribbon: ribbon,
                eventID: eventID,
                event: ev,
                gridRect: gridRect,
                cellWidth: cellWidth,
                cols: cols,
                startCol: styleParse.start,
                originalSpan: styleParse.span,
                originalEndCol: styleParse.start + styleParse.span - 1,
                originalRight: ribbonRect.right,
                pointerID: e.pointerId,
                committed: false,
            };
            ribbon.classList.add('is-resizing');
        }

        function onPointerMove(e) {
            if (!resizeState) return;
            if (e.pointerId !== resizeState.pointerID) return;
            // Snap to nearest day boundary. cursorX within grid →
            // target end-column (1-indexed inclusive).
            var relX = e.clientX - resizeState.gridRect.left;
            var targetCol = Math.round(relX / resizeState.cellWidth);
            if (targetCol < resizeState.startCol) {
                targetCol = resizeState.startCol;
            }
            if (targetCol > resizeState.cols) {
                targetCol = resizeState.cols; // clamp to grid edge
            }
            var newSpan = targetCol - resizeState.startCol + 1;
            resizeState.ribbon.style.gridColumn = resizeState.startCol + ' / span ' + newSpan;
            resizeState.currentEndCol = targetCol;
        }

        function onPointerUp(e) {
            if (!resizeState) return;
            if (e.pointerId !== resizeState.pointerID) return;
            var state = resizeState;
            resizeState = null;
            state.ribbon.classList.remove('is-resizing');
            try {
                state.handle.releasePointerCapture(e.pointerId);
            } catch (err) { /* ignore */ }

            var newEndCol = state.currentEndCol || state.originalEndCol;
            if (newEndCol === state.originalEndCol) {
                // No change; nothing to commit.
                return;
            }
            // Convert end-col delta to end-day delta. The ribbon's
            // grid-column maps day-of-week-row → column; we don't
            // have access to the start day directly here, but the
            // event.end_day field gives us the existing value to
            // add the col delta to.
            var dayDelta = newEndCol - state.originalEndCol;
            var newEndDay = (state.event.end_day || state.event.day) + dayDelta;
            // Single-day conversion confirm: if resizing collapses
            // to start day, prompt before committing.
            if (newSpanCollapsesToSingleDay(state)) {
                if (!window.confirm('Convert to single-day event?')) {
                    // Revert visual.
                    state.ribbon.style.gridColumn = state.startCol + ' / span ' + state.originalSpan;
                    return;
                }
            }
            commitResize(state, newEndDay);
        }

        function newSpanCollapsesToSingleDay(state) {
            // If the new end-col equals start-col AND the original
            // event was multi-day, this is a collapse.
            return state.currentEndCol === state.startCol &&
                state.originalSpan > 1;
        }

        function commitResize(state, newEndDay) {
            var body = Object.assign({}, state.event, {
                end_year: state.event.end_year || state.event.year,
                end_month: state.event.end_month || state.event.month,
                end_day: newEndDay,
            });
            var url = '/campaigns/' + campaignID + '/calendars/' + calendarID + '/events/' + state.eventID;
            window.Chronicle.apiFetch(url, {
                method: 'PUT',
                body: body,
                headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (resp) {
                if (!resp.ok) throw new Error('Resize failed');
                setTimeout(function () { window.location.reload(); }, 200);
            }).catch(function (err) {
                // Revert ribbon to original span on failure.
                state.ribbon.style.gridColumn = state.startCol + ' / span ' + state.originalSpan;
                window.Chronicle.notify((err && err.message) || 'Resize failed', 'error');
            });
        }

        // Helpers ---------------------------------------------------

        // computeGridColumnCount inspects the CSS Grid template-
        // columns to derive the column count. Falls back to 7
        // (Gregorian default) when parsing fails.
        function computeGridColumnCount(grid) {
            var style = window.getComputedStyle(grid);
            var template = style.gridTemplateColumns || '';
            // template-columns resolves to a space-separated list of
            // tracks; counting them gives the column count.
            var tracks = template.trim().split(/\s+/).filter(Boolean);
            if (tracks.length > 0) return tracks.length;
            return 7;
        }

        // parseGridColumn parses an inline "grid-column: N / span M"
        // style value into {start, span}. Returns {start: 1, span: 1}
        // when parsing fails.
        function parseGridColumn(value) {
            if (!value) return { start: 1, span: 1 };
            var m = value.match(/^(\d+)\s*\/\s*span\s*(\d+)$/i);
            if (m) return { start: parseInt(m[1], 10), span: parseInt(m[2], 10) };
            return { start: 1, span: 1 };
        }

        // Register listeners at the document level so pointer events
        // route correctly even when the cursor strays off the handle
        // (graceful fallback when setPointerCapture is unavailable).
        document.addEventListener('pointerdown', onPointerDown);
        document.addEventListener('pointermove', onPointerMove);
        document.addEventListener('pointerup', onPointerUp);
        document.addEventListener('pointercancel', onPointerUp);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', function () {
            init();
            initRibbonResize();
        });
    } else {
        init();
        initRibbonResize();
    }
})();
