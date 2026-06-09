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

    // dayRange normalizes a drag from startDay→endDay into an ordered span:
    // a single cell (start === end) is a single-day add; otherwise a multi-day
    // range. Pure (exposed for tests). C-CAL-INTERACTIONS.
    function dayRange(startDay, endDay) {
        var lo = Math.min(startDay, endDay), hi = Math.max(startDay, endDay);
        return { startDay: lo, endDay: hi, multi: lo !== hi };
    }
    if (typeof window !== 'undefined') window.__calDayRange = dayRange;

    // _openDrawer is the LIVE drawer-opener, refreshed by each init() so the
    // document-level drag-create (wired once) always calls the current closure.
    var _openDrawer = null;
    var _dayDragWired = false;

    // wireDayDragCreateOnce installs the drag-to-create / multiselect on the
    // month grid ONCE (document-level + live DOM queries), so it survives
    // boosted-nav re-inits without leaking listeners (the QA2/E8 class). A
    // pointerdown on an empty Scribe cell starts a day-range selection; dragging
    // across cells highlights the span; release opens the editor pre-filled
    // (single cell → single day, multi → multi-day end date).
    function wireDayDragCreateOnce() {
        if (_dayDragWired || typeof document === 'undefined') return;
        _dayDragWired = true;
        var sel = null;
        function addableCell(target) {
            if (!target || !target.closest) return null;
            var cell = target.closest('.cell-drop-target');
            if (!cell || !cell.querySelector('[data-cell-add-event]')) return null; // empty Scribe cell
            if (target.closest('[data-event-card], a, button')) return null;        // not a card / + / overflow
            return cell;
        }
        function highlight(lo, hi) {
            document.querySelectorAll('.cell-drop-target').forEach(function (c) {
                var d = parseInt(c.dataset.cellDay, 10);
                var on = !isNaN(d) && d >= lo && d <= hi;
                c.classList.toggle('ring-2', on);
                c.classList.toggle('ring-accent', on);
            });
        }
        function clearHighlight() {
            document.querySelectorAll('.cell-drop-target.ring-accent').forEach(function (c) {
                c.classList.remove('ring-2', 'ring-accent');
            });
        }
        document.addEventListener('pointerdown', function (e) {
            if (e.button !== 0) return;
            var cell = addableCell(e.target);
            if (!cell) return;
            var day = parseInt(cell.dataset.cellDay, 10);
            if (isNaN(day)) return;
            sel = {
                year: parseInt(cell.dataset.cellYear, 10),
                month: parseInt(cell.dataset.cellMonth, 10),
                startDay: day, curDay: day,
            };
            highlight(day, day);
        });
        document.addEventListener('pointermove', function (e) {
            if (!sel) return;
            var el = document.elementFromPoint(e.clientX, e.clientY);
            var cell = el && el.closest ? el.closest('.cell-drop-target') : null;
            if (!cell) return;
            var d = parseInt(cell.dataset.cellDay, 10);
            if (isNaN(d) || d === sel.curDay) return;
            sel.curDay = d;
            var r = dayRange(sel.startDay, d);
            highlight(r.startDay, r.endDay);
        });
        document.addEventListener('pointerup', function () {
            if (!sel) return;
            var s = sel; sel = null;
            clearHighlight();
            if (!_openDrawer) return;
            var r = dayRange(s.startDay, s.curDay);
            if (r.multi) {
                _openDrawer({
                    year: s.year, month: s.month, day: r.startDay,
                    end_year: s.year, end_month: s.month, end_day: r.endDay,
                });
            } else {
                _openDrawer({ year: s.year, month: s.month, day: r.startDay });
            }
        });
    }

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

            // Attach-entity picker (2b): persists real entity_event_links.
            initEntityTies();
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
            // C-CAL-INTERACTIONS: reflect a multi-day prefill (a drag-create range
            // or an existing multi-day event) — show the end-date fields + check
            // the toggle when an end date is present and differs from the start.
            syncMultiday(hasEndDate(item));
        }

        // --- Multi-day (end-date) toggle ---------------------------------
        function multidayToggle() { return drawer.querySelector('[data-multiday-toggle]'); }
        function syncMultiday(on) {
            var t = multidayToggle(), f = drawer.querySelector('[data-multiday-fields]');
            if (t) t.checked = !!on;
            if (f) f.classList.toggle('hidden', !on);
        }
        function hasEndDate(item) {
            return !!(item && item.end_day != null &&
                (item.end_year !== item.year || item.end_month !== item.month || item.end_day !== item.day));
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
            // C-CAL-INTERACTIONS: the end-date span only persists when the
            // multi-day toggle is on — otherwise drop any stale end_* so a
            // single-day event never carries a leftover span.
            var mt = multidayToggle();
            if (!mt || !mt.checked) {
                delete body.end_year;
                delete body.end_month;
                delete body.end_day;
            }
            // Pull the visibility editor state into the body.
            var vis = readVisibilityEditor();
            body.visibility = vis.mode === 'public' ? 'everyone' : 'dm_only';
            if (vis.rules.length > 0) {
                body.visibility_rules = JSON.stringify(vis.rules);
            }
            return body;
        }

        // --- Attach-entity picker (2b): real entity_event_links ---------
        // Persists ties through the calendar plugin's tie endpoints; SEARCHES
        // entities via the entities plugin's own /entities/search (cross-plugin
        // stays at the API boundary). Roles come from the Go enum via
        // data-ties-roles so the vocabulary never drifts from the backend.

        var tiesSection = drawer.querySelector('[data-event-ties-section]');
        var TIE_ROLES = ((tiesSection && tiesSection.getAttribute('data-ties-roles')) ||
            'involved,present,affected,mentioned').split(',');

        function tiesEsc(s) {
            return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
                return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
            });
        }
        function tiesBase() {
            return '/campaigns/' + campaignID + '/calendars/' + calendarID + '/events/' + editingID + '/entities';
        }

        function initEntityTies() {
            if (!tiesSection) return;
            var list = tiesSection.querySelector('[data-ties-list]');
            var picker = tiesSection.querySelector('[data-ties-picker]');
            var hint = tiesSection.querySelector('[data-ties-hint]');
            var search = tiesSection.querySelector('[data-ties-search]');
            var results = tiesSection.querySelector('[data-ties-results]');
            if (list) list.innerHTML = '';
            if (results) { results.innerHTML = ''; results.classList.add('hidden'); }
            if (search) search.value = '';
            // A new event has no id yet — ties can only attach after it's saved.
            if (!editingID) {
                if (picker) picker.classList.add('hidden');
                if (hint) hint.classList.remove('hidden');
                return;
            }
            if (picker) picker.classList.remove('hidden');
            if (hint) hint.classList.add('hidden');
            loadTies();
            wireTiesSearch(search, results);
        }

        function loadTies() {
            window.Chronicle.apiFetch(tiesBase(), { method: 'GET' })
                .then(function (r) { return r.ok ? r.json() : []; })
                .then(function (ties) { renderTies(ties || []); })
                .catch(function () {});
        }

        function renderTies(ties) {
            var list = tiesSection.querySelector('[data-ties-list]');
            if (!list) return;
            if (!ties.length) {
                list.innerHTML = '<span class="text-xs text-fg-secondary italic">No entities attached.</span>';
                return;
            }
            list.innerHTML = '';
            ties.forEach(function (t) {
                var role = t.participation_role || 'involved';
                var opts = TIE_ROLES.map(function (r) {
                    return '<option value="' + r + '"' + (r === role ? ' selected' : '') + '>' + r + '</option>';
                }).join('');
                var chip = document.createElement('span');
                chip.className = 'inline-flex items-center gap-1 px-2 py-0.5 rounded bg-surface-2 text-xs';
                chip.innerHTML = '<span>' + tiesEsc(t.entity_name) + '</span>' +
                    '<select class="bg-transparent text-xs" data-tie-role aria-label="Participation role">' + opts + '</select>' +
                    '<button type="button" class="text-fg-secondary hover:text-danger" data-tie-remove aria-label="Remove">&times;</button>';
                chip.querySelector('[data-tie-role]').addEventListener('change', function (e) {
                    linkTie(t.entity_id, e.target.value);
                });
                chip.querySelector('[data-tie-remove]').addEventListener('click', function () {
                    unlinkTie(t.entity_id);
                });
                list.appendChild(chip);
            });
        }

        function linkTie(entityID, role) {
            window.Chronicle.apiFetch(tiesBase() + '/' + entityID, {
                method: 'PUT', body: { role: role }, headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (r) {
                if (r.ok) loadTies(); else window.Chronicle.notify('Attach failed', 'error');
            }).catch(function () { window.Chronicle.notify('Attach failed', 'error'); });
        }

        function unlinkTie(entityID) {
            window.Chronicle.apiFetch(tiesBase() + '/' + entityID, {
                method: 'DELETE', headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (r) { if (r.ok) loadTies(); }).catch(function () {});
        }

        var tiesSearchTimer = null;
        function wireTiesSearch(search, results) {
            if (!search || search.__tiesWired) return;
            search.__tiesWired = true;
            search.addEventListener('input', function () {
                var q = search.value.trim();
                if (tiesSearchTimer) clearTimeout(tiesSearchTimer);
                if (q.length < 2) { results.classList.add('hidden'); results.innerHTML = ''; return; }
                tiesSearchTimer = setTimeout(function () {
                    window.Chronicle.apiFetch('/campaigns/' + campaignID + '/entities/search?q=' + encodeURIComponent(q),
                        { method: 'GET', headers: { 'Accept': 'application/json' } })
                        .then(function (r) { return r.ok ? r.json() : { results: [] }; })
                        .then(function (data) { renderTieResults(results, (data && data.results) || []); })
                        .catch(function () {});
                }, 200);
            });
            document.addEventListener('click', function (e) {
                if (results && !results.contains(e.target) && e.target !== search) results.classList.add('hidden');
            });
        }

        function renderTieResults(results, items) {
            if (!results) return;
            if (!items.length) {
                results.innerHTML = '<div class="px-2 py-1 text-xs text-fg-secondary">No matches</div>';
                results.classList.remove('hidden');
                return;
            }
            results.innerHTML = '';
            items.slice(0, 12).forEach(function (it) {
                var id = it.id || it.entity_id;
                var name = it.name || it.entity_name || id;
                var row = document.createElement('button');
                row.type = 'button';
                row.className = 'block w-full text-left px-2 py-1 text-xs hover:bg-surface-2';
                row.textContent = name;
                row.addEventListener('click', function () {
                    linkTie(id, 'involved');
                    results.classList.add('hidden');
                    var s = tiesSection.querySelector('[data-ties-search]');
                    if (s) s.value = '';
                });
                results.appendChild(row);
            });
            results.classList.remove('hidden');
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

        // Add-enabled (empty Scribe) cells get the pointer cursor. The
        // drag-create selector (wired ONCE below) handles BOTH a single click
        // (single-day add — C-CAL-V2-MONTH-GRID-ALIGN-FIX #3) and a drag across
        // cells (a multi-day range = the operator's "multiselect days").
        document.querySelectorAll('.cell-drop-target').forEach(function (cell) {
            if (cell.querySelector('[data-cell-add-event]')) cell.classList.add('cursor-pointer');
        });
        // Multi-day end-date fields show/hide with their toggle.
        var mtoggle = drawer && drawer.querySelector('[data-multiday-toggle]');
        if (mtoggle) mtoggle.addEventListener('change', function () { syncMultiday(mtoggle.checked); });
        // Expose the live openDrawer to the document-level drag-create (wired
        // once so it never leaks listeners across boosted-nav re-inits).
        _openDrawer = openDrawer;
        wireDayDragCreateOnce();

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

    // boot() binds the grid handlers once PER ROOT NODE. The V2 shell arrives
    // via hx-boost navigation (and re-renders on view/date nav), but under
    // htmx.config.allowScriptTags=false this <script> never re-runs — so we
    // bind on htmx:afterSettle/htmx:load too, and the per-root guard keeps a
    // freshly-swapped grid from double-binding (C-CAL-V2-MONTH-GRID-ALIGN-FIX
    // #3 — the day-add / card-edit handlers weren't binding after boosted nav).
    function boot() {
        var root = document.querySelector('[data-cal-v2-root]');
        if (!root || root.__eventGridInited) return;
        root.__eventGridInited = true;
        init();
        initRibbonResize();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', boot);
    } else {
        boot();
    }
    try {
        document.addEventListener('htmx:afterSettle', boot);
        document.addEventListener('htmx:load', boot);
    } catch (e) {}
})();
