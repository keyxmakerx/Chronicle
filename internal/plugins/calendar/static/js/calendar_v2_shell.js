// calendar_v2_shell.js — V2 shell-wide JS (Wave 1 PR 5 §F + §J).
// Owns the `?` keyboard shortcuts modal + the day-detail popover.
// Lives alongside event_grid.js (which owns drag + drawer + visibility
// editor) so calendar-level shell concerns stay separated from event-
// level interactions.

(function () {
    'use strict';

    function init() {
        var root = document.querySelector('[data-cal-v2-root]');
        // Per-root guard: bind once per shell node. The shell arrives via
        // hx-boost nav + re-renders on view/date nav; under
        // allowScriptTags=false this <script> never re-runs, so init() is also
        // called on htmx:afterSettle/htmx:load below (C-CAL-V2-MONTH-GRID-ALIGN
        // #3 — the day popover/shortcuts weren't wiring after boosted nav).
        if (!root || root.__calV2ShellInited) return;
        root.__calV2ShellInited = true;
        wireShortcuts(root);
        wireDayPopover(root);
        wireEventPeek(root);
        wireSidebarPin(root);
    }

    // --- Event quick-peek (C-CAL-CLOSEOUT PR A §1) --------------
    //
    // HOVER an event chip → a compact preview card (title / time / category /
    // tier). CLICK still opens the full drawer (event_grid.js owns that; the
    // peek is pointer-events:none so it never swallows the click). Listeners are
    // bound to `root` (not document) so they die with the shell node on a
    // boosted-nav swap — no document-listener leak. Touch / no-hover devices
    // skip the peek entirely and fall straight through to the drawer.
    function wireEventPeek(root) {
        var peek = document.getElementById('cal-v2-event-peek');
        if (!peek) return;
        if (window.matchMedia && window.matchMedia('(hover: none)').matches) return;
        var titleEl = peek.querySelector('[data-peek-title]');
        var timeEl = peek.querySelector('[data-peek-time]');
        var catEl = peek.querySelector('[data-peek-category]');
        var tierEl = peek.querySelector('[data-peek-tier]');

        function eventsData() {
            try { return JSON.parse(root.dataset.calV2Events || '[]'); } catch (e) { return []; }
        }
        function titleCase(s) { return s ? s.charAt(0).toUpperCase() + s.slice(1) : ''; }
        function pad(n) { return n < 10 ? '0' + n : '' + n; }
        function fmtTime(ev) {
            if (ev.all_day) return 'All day';
            if (ev.start_hour == null) return '';
            return pad(ev.start_hour) + ':' + pad(ev.start_minute == null ? 0 : ev.start_minute);
        }
        function setChip(el, val) {
            if (!el) return;
            if (val) { el.textContent = titleCase(val); el.classList.remove('hidden'); }
            else { el.classList.add('hidden'); }
        }
        function hide() { peek.classList.add('hidden'); }

        function show(chip) {
            var id = chip.dataset.eventId;
            if (!id) return;
            var ev = eventsData().filter(function (e) { return e.id === id; })[0];
            if (!ev) return;
            if (titleEl) titleEl.textContent = ev.name || 'Event';
            var t = fmtTime(ev);
            if (timeEl) { timeEl.textContent = t; timeEl.classList.toggle('hidden', !t); }
            setChip(catEl, ev.category);
            setChip(tierEl, ev.tier);
            // Position below the chip, flipping above / clamping to the viewport.
            peek.classList.remove('hidden');
            var rect = chip.getBoundingClientRect();
            var pr = peek.getBoundingClientRect();
            var left = Math.min(rect.left, window.innerWidth - pr.width - 8);
            var top = rect.bottom + 6;
            if (top + pr.height > window.innerHeight - 8) top = rect.top - pr.height - 6;
            peek.style.left = Math.max(8, left) + 'px';
            peek.style.top = Math.max(8, top) + 'px';
        }

        root.addEventListener('mouseover', function (e) {
            var chip = e.target.closest && e.target.closest('[data-event-card][data-event-id]');
            if (chip) show(chip);
        });
        root.addEventListener('mouseout', function (e) {
            var chip = e.target.closest && e.target.closest('[data-event-card][data-event-id]');
            if (!chip) return;
            // Ignore moves that stay within the same chip.
            if (e.relatedTarget && chip.contains(e.relatedTarget)) return;
            hide();
        });
        // A click opens the drawer (event_grid.js) — drop the peek so it never
        // lingers over the opening drawer.
        root.addEventListener('click', hide);
    }

    // --- Sidebar pin toggle (Wave 1.7A §G) ---
    //
    // Toggle hides/shows the sidebar without reloading; persists the
    // preference via POST /calendar/v2/sidebar-pin. Server state +
    // client display kept in lock-step so a page refresh preserves
    // the operator's choice. On failure, the visual state reverts +
    // toast surfaces the error.
    function wireSidebarPin(root) {
        var btn = document.querySelector('[data-cal-v2-sidebar-pin]');
        if (!btn) return;
        var sidebar = document.querySelector('[data-cal-v2-sidebar]');
        var campaignID = root.dataset.calV2CampaignId;
        var csrfToken = root.dataset.calV2CsrfToken;
        btn.addEventListener('click', function () {
            var nextPinned = false; // toggle from pinned → unpinned
            // Optimistic UI: hide sidebar immediately.
            if (sidebar) sidebar.style.display = 'none';
            window.Chronicle.apiFetch('/campaigns/' + campaignID + '/calendar/v2/sidebar-pin', {
                method: 'POST',
                body: { pinned: nextPinned },
                headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (resp) {
                if (!resp.ok) throw new Error('Toggle failed');
            }).catch(function (err) {
                // Revert visual; toast error.
                if (sidebar) sidebar.style.display = '';
                window.Chronicle.notify((err && err.message) || 'Pin toggle failed', 'error');
            });
        });
    }

    // --- Shortcuts modal ----------------------------------------

    function wireShortcuts(root) {
        var modal = document.getElementById('cal-v2-shortcuts-modal');
        if (!modal) return;
        var backdrop = modal.querySelector('[data-shortcuts-backdrop]');
        var close = modal.querySelector('[data-shortcuts-close]');
        function open() { modal.classList.remove('hidden'); }
        function dismiss() { modal.classList.add('hidden'); }

        if (backdrop) backdrop.addEventListener('click', dismiss);
        if (close) close.addEventListener('click', dismiss);

        document.addEventListener('keydown', function (e) {
            // Respect focus context: ignore shortcuts when typing in
            // form inputs (text / textarea / contenteditable). The
            // visibility editor + drawer + search bar all use these,
            // so a single check covers them all.
            var t = e.target;
            if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) {
                if (e.key !== 'Escape') return;
            }
            if (e.key === 'Escape') {
                if (!modal.classList.contains('hidden')) {
                    dismiss();
                    return;
                }
                return; // let other handlers (drawer / popover) handle Esc
            }
            if (e.key === '?' && !e.shiftKey === false) {
                // '?' is Shift+/ on US layouts; the e.key value is
                // already '?' regardless. Skip if any modifier other
                // than Shift is pressed.
                if (e.ctrlKey || e.metaKey || e.altKey) return;
                e.preventDefault();
                open();
                return;
            }
            // Navigation shortcuts: t / j / k / m / w / d / l / n / /
            // Map to existing UI affordances.
            var calendarID = root.dataset.calV2CalendarId;
            var campaignID = root.dataset.calV2CampaignId;
            if (!calendarID) return;
            var path = '/campaigns/' + campaignID + '/calendar/v2/' + calendarID;
            switch (e.key) {
                case 't':
                    // Today nav — let the header's Today link handle it
                    // by following the existing href.
                    var todayLink = document.querySelector('a[aria-label="Today"], a[href*="month"]');
                    if (todayLink) todayLink.click();
                    e.preventDefault();
                    break;
                case 'j':
                case 'ArrowLeft':
                    // 'Previous year' covers the Timeline (Ledger) view, which
                    // steps by year (C-CAL-TIMELINE-V2-W1).
                    clickByLabel('Previous month', 'Previous week', 'Previous day', 'Previous year');
                    if (e.key === 'j') e.preventDefault();
                    break;
                case 'k':
                case 'ArrowRight':
                    clickByLabel('Next month', 'Next week', 'Next day', 'Next year');
                    if (e.key === 'k') e.preventDefault();
                    break;
                case 'm':
                    window.location.href = path + '/month';
                    break;
                case 'w':
                    window.location.href = path + '/week';
                    break;
                case 'd':
                    window.location.href = path + '/day';
                    break;
                case 'l':
                    // 'l' → Timeline (Ledger) view. Route segment is 'ledger'
                    // (the design name) to avoid the timeline-plugin slug
                    // collision (C-CAL-TIMELINE-V2-W1).
                    window.location.href = path + '/ledger';
                    break;
                case 'n':
                    var firstCellAdd = document.querySelector('[data-cell-add-event]');
                    if (firstCellAdd) firstCellAdd.click();
                    e.preventDefault();
                    break;
                case '/':
                    var searchInput = document.querySelector('[data-cal-v2-search]');
                    if (searchInput) {
                        searchInput.focus();
                        e.preventDefault();
                    }
                    break;
            }
        });
    }

    function clickByLabel() {
        for (var i = 0; i < arguments.length; i++) {
            var el = document.querySelector('[aria-label="' + arguments[i] + '"]');
            if (el) { el.click(); return; }
        }
    }

    // --- Day-detail popover ------------------------------------

    // wireDayPopover wires the day MINI-VIEW (cordinator#33 item 4): a plain
    // click on a date cell (ALL roles) — or the "+N more" overflow — opens the
    // pinned day card with that day's events + worldstate peek + (Scribe) an
    // "Add event" button. This is now the FIRST tier on a date click, replacing
    // empty-cell→create-drawer; "Add event" is the create path. A drag ACROSS
    // cells is the Scribe drag-create (event_grid.js) and must still win, so the
    // card opens only when the press goes DOWN and UP on the SAME trigger (no
    // cross-cell drag) — the same single-vs-multi test event_grid uses. Every
    // listener binds to `root` (or the popover node), never document, so it dies
    // with the shell on a boosted-nav swap (the E4/E5 leak class).
    function wireDayPopover(root) {
        var popover = document.getElementById('cal-v2-day-popover');
        if (!popover) return;
        var title = popover.querySelector('[data-day-popover-title]');
        var list = popover.querySelector('[data-day-popover-list]');

        function dismiss() { popover.classList.add('hidden'); }

        // triggerFor returns an open-descriptor for an element, or null: the
        // "+N more" overflow button, OR a date-cell click on empty space (not a
        // chip / button / link). `key` is the element identity used to require
        // the press to go down AND up on the SAME trigger.
        function triggerFor(target) {
            if (!target || !target.closest) return null;
            var ov = target.closest('[data-cell-overflow-toggle]');
            if (ov) {
                var od = parseInt(ov.dataset.cellOverflowDay, 10);
                return isNaN(od) ? null : { key: ov, anchor: ov, day: od };
            }
            var cell = target.closest('[data-day-cell]');
            if (cell && !target.closest('[data-event-card], a, button')) {
                var cd = parseInt(cell.dataset.cellDay, 10);
                return isNaN(cd) ? null : { key: cell, anchor: cell, day: cd };
            }
            return null;
        }

        // Open on a same-trigger press (pointerup, not click) so it survives the
        // drag-create's pointerdown preventDefault on touch. A cross-cell drag
        // releases on a different trigger → skipped (event_grid handles the
        // multiselect-days create). Outside-press dismissal lives on the same
        // pointerdown.
        var downTrig = null;
        root.addEventListener('pointerdown', function (e) {
            if (e.button != null && e.button !== 0) { downTrig = null; return; }
            downTrig = triggerFor(e.target);
            if (!downTrig && !popover.classList.contains('hidden') && !popover.contains(e.target)) {
                dismiss();
            }
        });
        root.addEventListener('pointerup', function (e) {
            var down = downTrig; downTrig = null;
            if (!down) return;
            var up = triggerFor(e.target);
            if (!up || up.key !== down.key) return; // dragged off the trigger → not a click
            openPopover(down.anchor, down.day);
        });

        // Outside-CLICK closer, root-bound (never document) — mirrors the
        // quick-edit card. The trailing click after an opening pointerup lands on
        // the trigger, which triggerFor() recognizes, so the card stays open.
        root.addEventListener('click', function (e) {
            if (popover.classList.contains('hidden')) return;
            if (popover.contains(e.target)) return;
            if (triggerFor(e.target)) return; // a (re)open trigger, not a dismiss
            dismiss();
        });

        // Element-scoped wiring (close ×, Add event, Escape) — guarded per node
        // so a persistent popover isn't double-bound across re-inits. These read
        // the current day from popover.dataset (set on open) + a live root lookup
        // rather than a captured closure, so they never act on a stale shell.
        if (popover.dataset.dvWired !== '1') {
            popover.dataset.dvWired = '1';
            var close = popover.querySelector('[data-day-popover-close]');
            if (close) close.addEventListener('click', dismiss);
            var add = popover.querySelector('[data-day-popover-add]');
            if (add) add.addEventListener('click', function () {
                var d = parseInt(popover.dataset.dvDay, 10);
                var r = document.querySelector('[data-cal-v2-root]');
                if (isNaN(d) || !r) return;
                dismiss();
                if (window.calV2OpenCreateDrawer) {
                    window.calV2OpenCreateDrawer({
                        year: parseInt(r.dataset.calV2Year, 10),
                        month: parseInt(r.dataset.calV2Month, 10),
                        day: d,
                    });
                }
            });
            popover.addEventListener('keydown', function (e) {
                if (e.key === 'Escape') { e.stopPropagation(); dismiss(); }
            });
        }

        function openPopover(anchor, day) {
            popover.dataset.dvDay = String(day);
            if (title) title.textContent = 'Day ' + day;
            if (list) renderPopoverList(list, day);
            renderWorldStatePeek(day);
            // Reveal first (so it has dimensions), then position fixed + clamp to
            // the viewport — left clamped, flipped above if it'd overflow the
            // bottom — mirroring the quick-edit card's clamp.
            popover.classList.remove('hidden');
            var rect = anchor.getBoundingClientRect();
            var pr = popover.getBoundingClientRect();
            var left = Math.min(rect.left, window.innerWidth - pr.width - 8);
            if (left < 8) left = 8;
            var top = rect.bottom + 8;
            if (top + pr.height > window.innerHeight - 8) top = rect.top - pr.height - 8;
            if (top < 8) top = 8;
            popover.style.left = left + 'px';
            popover.style.top = top + 'px';
            if (typeof popover.focus === 'function') popover.focus();
        }

        // WorldState peek (2b-2): fetch the clicked day's seed (#401 GET,
        // dm_only filtered by role) and show its moon phase(s) + weather +
        // celestial events read-only above the events list. Reuses the same
        // BuildWorldStateSeed the 2a band renders — no new endpoint.
        function wsEsc(s) {
            return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
                return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
            });
        }
        function wsTitleCase(s) { return s ? s.charAt(0).toUpperCase() + s.slice(1) : ''; }

        function renderWorldStatePeek(day) {
            var box = popover.querySelector('[data-day-popover-worldstate]');
            if (!box) return;
            box.classList.add('hidden');
            box.innerHTML = '';
            var cid = root.dataset.calV2CampaignId;
            var calId = root.dataset.calV2CalendarId;
            var year = root.dataset.calV2Year;
            var month = root.dataset.calV2Month;
            if (!cid || !year || !month) return;
            var url = '/campaigns/' + cid + '/calendar/world-state?year=' + year +
                '&month=' + month + '&day=' + day + (calId ? '&calendarId=' + calId : '');
            window.Chronicle.apiFetch(url, { method: 'GET' })
                .then(function (r) { return r.ok ? r.json() : null; })
                .then(function (ws) { if (ws) fillWorldStatePeek(box, ws); })
                .catch(function () {});
        }

        function fillWorldStatePeek(box, ws) {
            var rows = [];
            if (ws.weather && ws.weather.type) {
                rows.push('<div><span class="text-fg-secondary">Weather:</span> ' + wsEsc(wsTitleCase(ws.weather.type)) + '</div>');
            }
            (ws.moons || []).forEach(function (m) {
                if (m && m.name) {
                    rows.push('<div><span class="text-fg-secondary">' + wsEsc(m.name) + ':</span> ' + wsEsc(m.namedPhase || '') + '</div>');
                }
            });
            (ws.events || []).forEach(function (ev) {
                if (ev && ev.name) rows.push('<div>✦ ' + wsEsc(ev.name) + '</div>');
            });
            if (!rows.length) return;
            box.innerHTML = rows.join('');
            box.classList.remove('hidden');
        }

        function renderPopoverList(listEl, day) {
            listEl.innerHTML = '';
            var events = [];
            try {
                events = JSON.parse(root.dataset.calV2Events || '[]');
            } catch (e) { return; }
            var filtered = events.filter(function (ev) {
                return ev.day === day && !isMultiDay(ev);
            });
            if (filtered.length === 0) {
                var empty = document.createElement('div');
                empty.className = 'text-xs text-fg-secondary italic';
                empty.textContent = 'No events';
                listEl.appendChild(empty);
                return;
            }
            filtered.forEach(function (ev) {
                var row = document.createElement('button');
                row.type = 'button';
                row.className = 'w-full text-left text-xs px-2 py-1 hover:bg-surface-2 rounded transition-colors duration-micro';
                row.dataset.eventId = ev.id;
                row.textContent = ev.name + (ev.visibility !== 'everyone' ? '  🔒' : '');
                row.addEventListener('click', function () {
                    dismiss();
                    var card = document.querySelector('[data-event-card][data-event-id="' + ev.id + '"]');
                    if (card) card.click();
                    else if (window.calV2OpenDrawerByID) window.calV2OpenDrawerByID(ev.id);
                });
                listEl.appendChild(row);
            });
        }

        function isMultiDay(ev) {
            if (!ev) return false;
            if (ev.end_year == null && ev.end_month == null && ev.end_day == null) return false;
            if (ev.end_year != null && ev.end_year !== ev.year) return true;
            if (ev.end_month != null && ev.end_month !== ev.month) return true;
            if (ev.end_day != null && ev.end_day !== ev.day) return true;
            return false;
        }
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
    try {
        document.addEventListener('htmx:afterSettle', init);
        document.addEventListener('htmx:load', init);
    } catch (e) {}
})();
