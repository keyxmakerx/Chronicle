// calendar_v2_shell.js — V2 shell-wide JS (Wave 1 PR 5 §F + §J).
// Owns the `?` keyboard shortcuts modal + the day-detail popover.
// Lives alongside event_grid.js (which owns drag + drawer + visibility
// editor) so calendar-level shell concerns stay separated from event-
// level interactions.

(function () {
    'use strict';

    function init() {
        var root = document.querySelector('[data-cal-v2-root]');
        if (!root) return;
        wireShortcuts(root);
        wireDayPopover(root);
        wireSidebarPin(root);
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
            // Navigation shortcuts: t / j / k / m / w / d / n / /
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
                    clickByLabel('Previous month', 'Previous week', 'Previous day');
                    if (e.key === 'j') e.preventDefault();
                    break;
                case 'k':
                case 'ArrowRight':
                    clickByLabel('Next month', 'Next week', 'Next day');
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

    function wireDayPopover(root) {
        var popover = document.getElementById('cal-v2-day-popover');
        if (!popover) return;
        var title = popover.querySelector('[data-day-popover-title]');
        var list = popover.querySelector('[data-day-popover-list]');
        var close = popover.querySelector('[data-day-popover-close]');

        function dismiss() { popover.classList.add('hidden'); }
        if (close) close.addEventListener('click', dismiss);

        document.addEventListener('click', function (e) {
            // Dismiss when clicking outside the popover.
            if (!popover.classList.contains('hidden') && !popover.contains(e.target)) {
                var trigger = e.target.closest('[data-cell-overflow-toggle]');
                if (!trigger) dismiss();
            }
        });

        document.addEventListener('keydown', function (e) {
            if (e.key === 'Escape' && !popover.classList.contains('hidden')) dismiss();
        });

        // Wire each "+N more" trigger.
        document.querySelectorAll('[data-cell-overflow-toggle]').forEach(function (btn) {
            btn.addEventListener('click', function (e) {
                e.stopPropagation();
                var day = parseInt(btn.dataset.cellOverflowDay, 10);
                if (isNaN(day)) return;
                openPopover(btn, day);
            });
        });

        function openPopover(anchor, day) {
            // Position popover below the anchor, flipping above if it
            // would extend past the viewport bottom.
            var rect = anchor.getBoundingClientRect();
            popover.style.left = rect.left + 'px';
            popover.style.top = (rect.bottom + 8) + 'px';
            popover.classList.remove('hidden');
            // Flip if needed.
            var pRect = popover.getBoundingClientRect();
            if (pRect.bottom > window.innerHeight - 8) {
                popover.style.top = (rect.top - pRect.height - 8) + 'px';
            }
            if (title) title.textContent = 'Day ' + day;
            if (list) renderPopoverList(list, day);
            renderWorldStatePeek(day);
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
})();
