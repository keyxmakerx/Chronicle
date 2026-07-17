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
        wireSkyStrip(root);
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

    // --- Sky strip (C-CAL-SKY-STRIP) -----------------------------
    //
    // Collapse toggle (localStorage-persisted per campaign) + the Calendaria
    // sync chip. The chip's data comes from a client-side fetch of the
    // EXISTING member-accessible GET /campaigns/:id/foundry-presence
    // (foundry_vtt/handler.go:81-97) — not a new endpoint (Step-0 finding,
    // PR body: no trustworthy calendar-specific last-sync source exists;
    // this presence signal is the cleanest real one available). The calendar
    // plugin does not import foundry_vtt (T-B2 plugin isolation) — a plain
    // HTTP fetch to the other plugin's own endpoint is the cross-plugin seam,
    // same as any other widget-to-API call.

    var SKY_STRIP_STALE_AFTER_MS = 15 * 60 * 1000; // a live Foundry session pings well inside this

    // computeSyncChipState is a PURE function (no DOM/network/Date.now() of
    // its own — the caller supplies "now") mapping a presence signal +
    // (optional) a last-confirmed-Foundry-date comparison to the chip's
    // display state. fmConfirmedDate/chronicleCurrentDate are always '' from
    // wireSkyStrip below — no source persists a Foundry-confirmed date today
    // (see the PR body) — so 'drift' is reachable by this function and its
    // tests, but dormant in production until an FM-lane dispatch adds that
    // signal. Exposed on window.__calSkyStripSync for node --test
    // (test/js/calendar_v2_sky_strip.test.mjs), the same reuse-seam
    // convention as window.__calMoonSim / window.__calSkyFxMeta.
    function computeSyncChipState(neverSeen, lastSeenMs, nowMs, staleAfterMs, fmConfirmedDate, chronicleCurrentDate) {
        if (neverSeen || lastSeenMs == null) {
            return { status: 'never_synced', agoMs: null, date: '' };
        }
        var agoMs = Math.max(0, nowMs - lastSeenMs);
        if (fmConfirmedDate && chronicleCurrentDate && fmConfirmedDate !== chronicleCurrentDate) {
            return { status: 'drift', agoMs: agoMs, date: fmConfirmedDate };
        }
        return { status: agoMs > staleAfterMs ? 'stale' : 'in_sync', agoMs: agoMs, date: chronicleCurrentDate || '' };
    }

    function agoLabel(ms) {
        if (ms == null) return '';
        var mins = Math.round(ms / 60000);
        if (mins < 1) return 'just now';
        if (mins < 60) return mins + ' min ago';
        var hours = Math.round(mins / 60);
        if (hours < 24) return hours + (hours === 1 ? ' hr ago' : ' hrs ago');
        var days = Math.round(hours / 24);
        return days + (days === 1 ? ' day ago' : ' days ago');
    }

    // skyChipCopy splits into `word` (always visible — the mobile-collapsed
    // form, dispatch item 4) + `detail` (the "· Nm ago · date" context, sm+
    // only). word + detail concatenated is the full desktop copy — no
    // separate "full text" to keep in sync.
    function skyChipCopy(state) {
        switch (state.status) {
            case 'never_synced':
                return { word: 'No module', detail: ' connected', cls: 'neutral' };
            case 'drift':
                return { word: 'Drift', detail: ' · ' + agoLabel(state.agoMs) + (state.date ? ' · ' + state.date : ''), cls: 'warn' };
            case 'stale':
                return { word: 'Stale', detail: ' · last seen ' + agoLabel(state.agoMs), cls: 'warn' };
            default:
                return { word: 'In sync', detail: ' · ' + agoLabel(state.agoMs), cls: 'ok' };
        }
    }

    var SKY_CHIP_CLASS = {
        ok: { chip: 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400 border-green-300/50 dark:border-green-700/50', dot: 'bg-green-500' },
        warn: { chip: 'bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 border-amber-300/50 dark:border-amber-700/50', dot: 'bg-amber-500' },
        neutral: { chip: 'bg-surface-alt text-fg-secondary border-edge', dot: 'bg-fg-muted' },
    };

    function paintSkyChip(chip, state) {
        var copy = skyChipCopy(state);
        var cls = SKY_CHIP_CLASS[copy.cls] || SKY_CHIP_CLASS.neutral;
        chip.className = 'flex-none inline-flex items-center gap-1.5 text-[11px] font-semibold px-2.5 py-1.5 rounded-full border ' + cls.chip;
        var dot = chip.querySelector('[data-cal-sky-sync-dot]');
        if (dot) dot.className = 'w-1.5 h-1.5 rounded-full flex-none ' + cls.dot;
        var word = chip.querySelector('[data-cal-sky-sync-word]');
        if (word) word.textContent = copy.word;
        var detail = chip.querySelector('[data-cal-sky-sync-detail]');
        if (detail) detail.textContent = copy.detail;
        chip.title = copy.word + copy.detail;
    }

    function wireSkyStrip(root) {
        var strip = document.querySelector('[data-cal-sky-strip]');
        if (!strip || strip.__calSkyStripInited) return;
        strip.__calSkyStripInited = true;

        var campaignId = strip.dataset.campaignId || root.dataset.calV2CampaignId || '';
        var storageKey = 'chronicle-cal-sky-strip-open-' + campaignId;
        var toggle = strip.querySelector('[data-cal-sky-strip-toggle]');
        var pane = strip.querySelector('[data-cal-sky-strip-pane]');
        var chevron = strip.querySelector('[data-cal-sky-chevron]');

        function setOpen(open) {
            if (pane) pane.classList.toggle('hidden', !open);
            if (chevron) chevron.style.transform = open ? 'rotate(180deg)' : '';
            if (toggle) {
                toggle.setAttribute('aria-expanded', String(open));
                toggle.setAttribute('aria-label', open ? 'Collapse sky panel' : 'Expand sky panel');
            }
            try { localStorage.setItem(storageKey, open ? '1' : '0'); } catch (e) { /* ignore */ }
        }

        var storedOpen = false;
        try { storedOpen = localStorage.getItem(storageKey) === '1'; } catch (e) { /* ignore */ }
        if (storedOpen) setOpen(true);
        if (toggle) toggle.addEventListener('click', function () {
            setOpen(toggle.getAttribute('aria-expanded') !== 'true');
        });

        // Sync-now: copy-help only (dispatch: no module-triggerable push
        // exists — do not build a new push pipeline).
        var syncNowBtn = strip.querySelector('[data-cal-sky-sync-now]');
        var syncHelp = strip.querySelector('[data-cal-sky-sync-help]');
        if (syncNowBtn && syncHelp) {
            syncNowBtn.addEventListener('click', function () {
                var shown = syncNowBtn.getAttribute('aria-expanded') === 'true';
                syncHelp.classList.toggle('hidden', shown);
                syncNowBtn.setAttribute('aria-expanded', String(!shown));
            });
        }

        var chip = strip.querySelector('[data-cal-sky-sync-chip]');
        if (chip && campaignId && window.Chronicle && window.Chronicle.apiFetch) {
            window.Chronicle.apiFetch('/campaigns/' + campaignId + '/foundry-presence')
                .then(function (r) { if (!r.ok) throw new Error('presence ' + r.status); return r.json(); })
                .then(function (presence) {
                    var lastSeenMs = presence.last_seen ? new Date(presence.last_seen).getTime() : null;
                    var state = computeSyncChipState(!!presence.never_seen, lastSeenMs, Date.now(), SKY_STRIP_STALE_AFTER_MS, '', '');
                    paintSkyChip(chip, state);
                })
                .catch(function () {
                    paintSkyChip(chip, { status: 'never_synced', agoMs: null, date: '' });
                });
        } else if (chip) {
            paintSkyChip(chip, { status: 'never_synced', agoMs: null, date: '' });
        }
    }

    window.__calSkyStripSync = { computeSyncChipState: computeSyncChipState, agoLabel: agoLabel };

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
                // Time prefix + (real-time calendars only) the viewer-zone hint
                // (C-CAL-UX-PAIR §Fix 2) — same rtAnchorZone/rtEventHint pure math
                // wireEventTimeHints uses below, called directly here since the
                // popover list is plain JS-built (no [data-rt-hint] marker node).
                var prefix = '';
                if (!ev.all_day && ev.start_hour != null) {
                    var p2 = function (n) { return n < 10 ? '0' + n : '' + n; };
                    prefix = p2(ev.start_hour) + ':' + p2(ev.start_minute == null ? 0 : ev.start_minute) + ' ';
                    var zone = rtAnchorZone();
                    var hint = zone ? rtEventHint(ev, zone) : '';
                    if (hint) prefix += '· your time ' + hint + ' ';
                }
                row.textContent = prefix + ev.name + (ev.visibility !== 'everyone' ? '  🔒' : '');
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

    // --- Real-time wall-clock (C-REAL-CALENDAR-P3) --------------
    //
    // For UsesRealTime() calendars the server marks each clock element with
    // [data-cal-rt-clock] + data-rt-zone (the IANA anchor zone) and renders a
    // no-JS fallback time inside [data-rt-anchor]. Here we re-render the live
    // time in that zone via Intl at MINUTE granularity (no per-second work), and
    // — when the viewer's browser zone differs from the anchor — a second "Your
    // time" line computed client-side from the SAME instant (the server never
    // sees the browser zone). prefers-reduced-motion → paint once, no live tick
    // (static). Scans the whole document (header clock + dashboard cards) with a
    // per-node guard so it is safe on load AND htmx settle without double-wiring,
    // and works on pages with no [data-cal-v2-root] (the dashboard).
    var rtClockTimers = [];

    function wireRealTimeClocks() {
        // Reap timers whose clock node was swapped out by an htmx nav, so a long
        // session never accumulates no-op minute intervals on detached nodes.
        rtClockTimers = rtClockTimers.filter(function (t) {
            if (!document.contains(t.node)) {
                clearTimeout(t.timeoutId);
                clearInterval(t.intervalId);
                return false;
            }
            return true;
        });

        var clocks = document.querySelectorAll('[data-cal-rt-clock]');
        if (!clocks.length) return;

        var reduce = false;
        try {
            reduce = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
        } catch (e) {}
        var browserZone = '';
        try {
            browserZone = Intl.DateTimeFormat().resolvedOptions().timeZone || '';
        } catch (e) {}

        function fmtTime(zone, date) {
            try {
                return new Intl.DateTimeFormat('en-US', {
                    timeZone: zone, hour: '2-digit', minute: '2-digit', hour12: false
                }).format(date);
            } catch (e) { return ''; }
        }
        function shortZone(zone, date) {
            try {
                var parts = new Intl.DateTimeFormat('en-US', {
                    timeZone: zone, timeZoneName: 'short'
                }).formatToParts(date);
                for (var i = 0; i < parts.length; i++) {
                    if (parts[i].type === 'timeZoneName') return parts[i].value;
                }
            } catch (e) {}
            return '';
        }
        function paint(clock) {
            var zone = clock.getAttribute('data-rt-zone');
            if (!zone) return;
            var now = new Date();
            var anchor = clock.querySelector('[data-rt-anchor]');
            if (anchor) {
                var t = fmtTime(zone, now);
                if (t) {
                    var z = shortZone(zone, now);
                    anchor.textContent = z ? t + ' ' + z : t;
                }
            }
            var local = clock.querySelector('[data-rt-local]');
            if (local) {
                // Dual line only when the viewer's zone differs from the anchor.
                if (browserZone && browserZone !== zone) {
                    var lt = fmtTime(browserZone, now);
                    var lz = shortZone(browserZone, now);
                    local.textContent = 'Your time: ' + lt + (lz ? ' ' + lz : '');
                    local.classList.remove('hidden');
                } else {
                    local.classList.add('hidden');
                }
            }
        }

        for (var i = 0; i < clocks.length; i++) {
            var clock = clocks[i];
            if (clock.__rtClockInited) continue;
            clock.__rtClockInited = true;
            paint(clock); // correct at load (also the static value under reduced-motion)
            if (reduce) continue; // reduced-motion → no live tick
            (function (c) {
                // Align the first tick to the next minute boundary, then tick each
                // minute. Registry entry lets the reaper clear it once c detaches.
                var rec = { node: c, timeoutId: 0, intervalId: 0 };
                var msToMinute = (60 - new Date().getSeconds()) * 1000;
                rec.timeoutId = setTimeout(function () {
                    paint(c);
                    rec.intervalId = setInterval(function () { paint(c); }, 60000);
                }, msToMinute);
                rtClockTimers.push(rec);
            })(clock);
        }
    }

    // --- Event-time viewer-zone hints (C-CAL-UX-PAIR §Fix 2) -----
    //
    // On a UsesRealTime() calendar, EVENT times (not "now") render in the
    // anchor zone for every viewer — the server has no browser-zone signal.
    // This paints a small "your time HH:MM" hint next to each rendered event
    // time, reusing the SAME anchor zone the P3 live clock above already
    // carries in the DOM ([data-cal-rt-clock][data-rt-zone] — wireRealTimeClocks
    // is the reference dual-line pattern this generalizes from "now" to "this
    // event's start time"). Hidden entirely when the viewer's browser zone
    // matches the anchor, or when the page has no real-time calendar at all
    // (no [data-cal-rt-clock] node) — a zero-change no-op everywhere else.
    // Pure math is exposed on window.__calRtHint for node --test
    // (test/js/calendar_v2_rt_hint.test.mjs) and for event_grid.js's drawer
    // WHEN-section hint, the same reuse-seam convention as
    // window.__calSkyStripSync / window.__calDayRange.

    // rtAnchorZone reads the page's real-time anchor zone from the live-clock
    // node — '' when the calendar isn't real-time, the suppression signal
    // every caller below keys off.
    function rtAnchorZone() {
        if (typeof document === 'undefined') return '';
        var clock = document.querySelector('[data-cal-rt-clock]');
        return (clock && clock.getAttribute('data-rt-zone')) || '';
    }

    function rtBrowserZone() {
        try {
            return Intl.DateTimeFormat().resolvedOptions().timeZone || '';
        } catch (e) { return ''; }
    }

    // rtZonedWallTimeToUTC converts a WALL-CLOCK date+time as displayed in
    // `zone` to the UTC instant it represents: guess the instant assuming
    // UTC, read back what that instant reads as in `zone` via Intl, then
    // correct the guess by the delta. Exact outside the DST-transition hour
    // itself (the ambiguous/skipped local time), an acceptable approximation
    // for a display-only hint.
    function rtZonedWallTimeToUTC(year, month, day, hour, minute, zone) {
        var guess = Date.UTC(year, month - 1, day, hour, minute);
        try {
            var parts = new Intl.DateTimeFormat('en-US', {
                timeZone: zone, hourCycle: 'h23',
                year: 'numeric', month: '2-digit', day: '2-digit',
                hour: '2-digit', minute: '2-digit',
            }).formatToParts(new Date(guess));
            var m = {};
            for (var i = 0; i < parts.length; i++) m[parts[i].type] = parts[i].value;
            var readHour = parseInt(m.hour, 10);
            if (readHour === 24) readHour = 0; // some locales format midnight as 24:00
            var asIfUTC = Date.UTC(
                parseInt(m.year, 10), parseInt(m.month, 10) - 1, parseInt(m.day, 10),
                readHour, parseInt(m.minute, 10));
            return new Date(guess - (asIfUTC - guess));
        } catch (e) {
            return new Date(guess);
        }
    }

    // rtFormatInZone formats a UTC instant as 24h "HH:MM" in the given zone.
    function rtFormatInZone(date, zone) {
        try {
            return new Intl.DateTimeFormat('en-US', {
                timeZone: zone, hour: '2-digit', minute: '2-digit', hour12: false,
            }).format(date);
        } catch (e) { return ''; }
    }

    // rtEventHint returns '' (suppressed) or "HH:MM" — the viewer-browser-zone
    // wall time for a timed event. Suppressed for: no anchor zone (non-RT
    // calendar), all-day events, events with no start time, an unreadable
    // browser zone, or a browser zone equal to the anchor (nothing to add).
    function rtEventHint(ev, anchorZone) {
        if (!anchorZone || !ev || ev.all_day || ev.start_hour == null) return '';
        var browserZone = rtBrowserZone();
        if (!browserZone || browserZone === anchorZone) return '';
        var utc = rtZonedWallTimeToUTC(
            ev.year, ev.month, ev.day, ev.start_hour,
            ev.start_minute == null ? 0 : ev.start_minute, anchorZone);
        return rtFormatInZone(utc, browserZone);
    }

    if (typeof window !== 'undefined') {
        window.__calRtHint = {
            anchorZone: rtAnchorZone,
            browserZone: rtBrowserZone,
            zonedWallTimeToUTC: rtZonedWallTimeToUTC,
            formatInZone: rtFormatInZone,
            eventHint: rtEventHint,
        };
    }

    // wireEventTimeHints paints the hint onto every rendered event time it can
    // reach: month pips get a tooltip + aria-label augmentation (the pip line
    // is one truncated 11px row — no room for a visible suffix); every
    // [data-rt-hint] marker (week/day event cards, the mobile agenda subtitle)
    // gets a visible "· your time HH:MM" suffix instead — those surfaces have
    // room, and the agenda card in particular is touch-first, where a
    // hover-only tooltip would never be reachable. Per-node __rtHinted guards
    // make repeat calls (every htmx settle/load, since view-slot swaps don't
    // re-run init()'s once-per-root guard) safe and idempotent.
    function wireEventTimeHints() {
        var root = document.querySelector('[data-cal-v2-root]');
        var zone = rtAnchorZone();
        if (!root || !zone) return;

        var events = {};
        try {
            (JSON.parse(root.dataset.calV2Events || '[]')).forEach(function (e) { events[e.id] = e; });
        } catch (e) { /* ignore */ }

        function hintFor(id) {
            var ev = id && events[id];
            return ev ? rtEventHint(ev, zone) : '';
        }

        document.querySelectorAll('[data-day-cell] [data-event-card="compact"][data-event-id]').forEach(function (el) {
            if (el.__rtHinted) return;
            var hint = hintFor(el.getAttribute('data-event-id'));
            if (!hint) return;
            el.__rtHinted = true;
            el.title = 'Your time: ' + hint;
            var label = el.getAttribute('aria-label') || '';
            el.setAttribute('aria-label', label + (label ? ', ' : '') + 'your time ' + hint);
        });

        document.querySelectorAll('[data-rt-hint]').forEach(function (el) {
            if (el.__rtHinted) return;
            var card = el.closest('[data-event-id]');
            var hint = card ? hintFor(card.getAttribute('data-event-id')) : '';
            if (!hint) return;
            el.__rtHinted = true;
            el.textContent = '· your time ' + hint;
            el.classList.remove('hidden');
        });
    }

    // boot wires the root-gated shell PLUS the document-wide real-time clocks
    // + event-time hints. The clocks/hints run even on pages without a
    // [data-cal-v2-root] shell (the campaign dashboard) — wireEventTimeHints
    // itself no-ops without a root — and, unlike init(), they re-scan on
    // EVERY boot (their own per-node guards make that safe), which is what
    // lets a Month→Week/Day view-slot swap (same root, so init()'s
    // __calV2ShellInited guard skips it) still pick up newly-swapped-in event
    // nodes.
    function boot() {
        init();
        wireRealTimeClocks();
        wireEventTimeHints();
    }

    if (typeof document !== 'undefined') {
        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', boot);
        } else {
            boot();
        }
        try {
            document.addEventListener('htmx:afterSettle', boot);
            document.addEventListener('htmx:load', boot);
        } catch (e) {}
    }
})();
