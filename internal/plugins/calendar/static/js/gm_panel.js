// gm_panel.js — the GM world-state CONSOLE driver
// (C-CAL-WORLDSTATE-GM-OVERHAUL — complete redesign of the Phase-4 panel).
// Only loaded for capability holders (the templ + the page gate on
// CanControlWorldState), so this never reaches a player.
//
// Source of truth = the server. Every control round-trips
// PUT /calendar/world-state; the PUT response is the freshly-built seed,
// which (a) re-renders the ambient band via window.__calSetWorldState and
// (b) re-syncs the console's own state (active weather tile, active
// world-event chips, the header clock). Pause is the lone client-only
// control (atmosphere freeze is ephemeral — the GM's own view).
//
// The card is TRANSLUCENT GLASS over the living sky; on every world
// mutation it drops to near-transparent (data-gm-transition) so the DM
// watches the sky change through it, then recovers (gm_panel.css owns the
// fade; reduced-motion disables it there).
(function () {
    'use strict';

    function init() {
        var panel = document.querySelector('[data-gm-panel]');
        if (!panel) return;
        // QA2 class: per-element guard so re-init on boosted nav binds the
        // freshly-swapped panel once, never the same node twice.
        if (panel.dataset.gmWired === '1') return;
        panel.dataset.gmWired = '1';
        var root = document.querySelector('[data-cal-v2-root]');
        if (!root) return;
        var campaignID = root.dataset.calV2CampaignId;
        var calendarID = root.dataset.calV2CalendarId;
        var csrfToken = root.dataset.calV2CsrfToken;

        var url = '/campaigns/' + campaignID + '/calendar/world-state' +
            (calendarID ? '?calendarId=' + calendarID : '');

        // --- transition translucency (the sky shows through while it changes) ---
        var fadeTimer = null;
        function transitionFade() {
            panel.setAttribute('data-gm-transition', 'true');
            clearTimeout(fadeTimer);
            fadeTimer = setTimeout(function () { panel.removeAttribute('data-gm-transition'); }, 1400);
        }

        // --- console state sync from a fresh seed ---
        function syncFromSeed(seed) {
            if (!seed) return;
            // Active weather tile ring.
            var wtype = (seed.weather && seed.weather.type) || 'clear';
            panel.querySelectorAll('[data-gm-weather-tile]').forEach(function (tile) {
                tile.setAttribute('data-gm-active', tile.getAttribute('data-gm-id') === wtype ? 'true' : 'false');
            });
            // Active world-event chips (with per-event clear ×).
            var box = panel.querySelector('[data-gm-active-events]');
            if (box) {
                box.innerHTML = '';
                (seed.events || []).forEach(function (ev) {
                    var chip = document.createElement('span');
                    chip.className = 'gm-console__active-chip';
                    chip.setAttribute('data-gm-active-chip', '');
                    chip.appendChild(document.createTextNode(ev.name || ev.type));
                    var x = document.createElement('button');
                    x.type = 'button';
                    x.className = 'gm-console__active-x';
                    x.setAttribute('data-gm-event-clear', ev.type);
                    x.setAttribute('aria-label', 'End ' + (ev.name || ev.type));
                    x.textContent = '×';
                    chip.appendChild(x);
                    box.appendChild(chip);
                });
            }
            var empty = panel.querySelector('[data-gm-active-empty]');
            if (empty) empty.style.display = (seed.events && seed.events.length) ? 'none' : '';
            // Header clock from the seed's time-of-day.
            var clock = panel.querySelector('[data-gm-clock]');
            if (clock && typeof seed.timeOfDay === 'number') {
                var mins = Math.floor(seed.timeOfDay * 24 * 60);
                var h = Math.floor(mins / 60) % 24, m = mins % 60;
                clock.textContent = h + ':' + (m < 10 ? '0' + m : m);
            }
        }
        // First-paint sync (server rendered the chips; this aligns the rest,
        // e.g. the active weather ring after a boosted-nav re-init).
        try { syncFromSeed(window.__calWorldState); } catch (e) {}

        // PUT a writable world-state slice, then re-render the band + console
        // from the authoritative response seed (or reload as a fallback).
        function commit(body, btn) {
            if (btn) btn.disabled = true;
            transitionFade();
            window.Chronicle.apiFetch(url, {
                method: 'PUT', body: body, headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (resp) {
                if (!resp.ok) {
                    return resp.json().catch(function () { return {}; }).then(function (b) {
                        throw new Error((b && b.message) || 'World-state update failed');
                    });
                }
                return resp.json();
            }).then(function (seed) {
                if (seed && window.__calSetWorldState) {
                    window.__calSetWorldState(seed); // re-render the ambient band in place
                    syncFromSeed(seed);
                } else {
                    window.location.reload(); // engine not ready → page-load read
                }
            }).catch(function (e) {
                window.Chronicle.notify((e && e.message) || 'Update failed', 'error');
            }).then(function () {
                if (btn) btn.disabled = false;
            });
        }

        function num(sel, fallback) {
            var el = panel.querySelector(sel);
            if (!el) return fallback;
            var n = parseInt(el.value, 10);
            return isNaN(n) ? fallback : n;
        }

        // --- console state machine (r3, cordinator#33) -------------------
        // The console's visible state lives in THREE surfaces: data-gm-collapsed
        // (root), data-gm-sheet (root) and per-sheet [hidden] — plus aria on the
        // buttons. The r2 code had separate writers, so a missed path could
        // desync them ("collapsed, yet a sheet shows behind the pill").
        // applyState is now the ONLY writer: every path — init included —
        // reconciles ALL surfaces in one place. A collapsed console can never
        // hold an open sheet.
        var toggleBtn = panel.querySelector('[data-gm-panel-toggle]');
        var openBtn = panel.querySelector('[data-gm-panel-open]');
        function applyState(collapsed, sheet) {
            if (collapsed) sheet = '';
            panel.setAttribute('data-gm-collapsed', collapsed ? 'true' : 'false');
            panel.setAttribute('data-gm-sheet', sheet || '');
            if (toggleBtn) toggleBtn.setAttribute('aria-expanded', collapsed ? 'false' : 'true');
            panel.querySelectorAll('[data-gm-sheet-open]').forEach(function (b) {
                b.setAttribute('aria-expanded', b.getAttribute('data-gm-sheet-open') === sheet ? 'true' : 'false');
            });
            panel.querySelectorAll('[data-gm-sheet-panel]').forEach(function (p) {
                if (p.getAttribute('data-gm-sheet-panel') === sheet) p.removeAttribute('hidden');
                else p.setAttribute('hidden', '');
            });
        }
        function isCollapsed() { return panel.getAttribute('data-gm-collapsed') === 'true'; }
        function currentSheet() { return panel.getAttribute('data-gm-sheet') || ''; }
        // Normalize whatever state the markup arrived in (fresh render, boosted
        // -nav swap, or a desync from a pre-r3 session) on every (re)init.
        applyState(isCollapsed(), currentSheet());
        if (toggleBtn) toggleBtn.addEventListener('click', function () { applyState(true, ''); });
        if (openBtn) openBtn.addEventListener('click', function () { applyState(false, ''); });
        panel.querySelectorAll('[data-gm-sheet-open]').forEach(function (btn) {
            btn.addEventListener('click', function () {
                var id = btn.getAttribute('data-gm-sheet-open');
                applyState(false, currentSheet() === id ? '' : id);
            });
        });
        panel.querySelectorAll('[data-gm-sheet-close]').forEach(function (btn) {
            btn.addEventListener('click', function () { applyState(false, ''); });
        });

        // --- catalog search filters (weather + events share the pattern) ---
        function wireSearch(inputSel, tileSel) {
            var input = panel.querySelector(inputSel);
            if (!input) return;
            input.addEventListener('input', function () {
                var q = input.value.trim().toLowerCase();
                panel.querySelectorAll(tileSel).forEach(function (tile) {
                    var label = (tile.getAttribute('data-gm-label') || '').toLowerCase();
                    var hit = !q || label.indexOf(q) !== -1;
                    tile.setAttribute('data-gm-filtered', hit ? 'false' : 'true');
                });
                // Hide a category header when the filter empties its group.
                var cats = panel.querySelectorAll(inputSel === '[data-gm-weather-search]'
                    ? '[data-gm-weather-catalog] [data-gm-cat]' : '[data-gm-event-catalog] [data-gm-cat]');
                cats.forEach(function (cat) {
                    var anyVisible = false, n = cat.nextElementSibling;
                    while (n && !n.hasAttribute('data-gm-cat')) {
                        if (n.getAttribute('data-gm-filtered') !== 'true') { anyVisible = true; break; }
                        n = n.nextElementSibling;
                    }
                    cat.style.display = anyVisible ? '' : 'none';
                });
            });
        }
        wireSearch('[data-gm-weather-search]', '[data-gm-weather-tile]');
        wireSearch('[data-gm-event-search]', '[data-gm-event-tile]');

        // --- Advance verbs (signed; rollover server-side) ---
        panel.querySelectorAll('[data-gm-advance]').forEach(function (btn) {
            btn.addEventListener('click', function () {
                commit({
                    advance: {
                        days: parseInt(btn.dataset.gmDays || '0', 10) || 0,
                        hours: parseInt(btn.dataset.gmHours || '0', 10) || 0,
                        minutes: parseInt(btn.dataset.gmMinutes || '0', 10) || 0,
                    },
                }, btn);
            });
        });

        // --- Absolute set time / set date ---
        var setTimeBtn = panel.querySelector('[data-gm-set-time]');
        if (setTimeBtn) {
            setTimeBtn.addEventListener('click', function () {
                commit({ time: { hour: num('[data-gm-time-hour]', 0), minute: num('[data-gm-time-minute]', 0) } }, setTimeBtn);
            });
        }
        var setDateBtn = panel.querySelector('[data-gm-set-date]');
        if (setDateBtn) {
            setDateBtn.addEventListener('click', function () {
                commit({
                    time: {
                        year: num('[data-gm-date-year]', 0),
                        month: num('[data-gm-date-month]', 1),
                        day: num('[data-gm-date-day]', 1),
                    },
                }, setDateBtn);
            });
        }

        // --- Weather catalog: one tap sets the condition ---
        panel.querySelectorAll('[data-gm-weather-tile]').forEach(function (tile) {
            tile.addEventListener('click', function () {
                commit({ weather: tile.getAttribute('data-gm-id') || 'clear' }, tile);
            });
        });

        // --- World-event catalog: one tap triggers ---
        panel.querySelectorAll('[data-gm-event-tile]').forEach(function (tile) {
            tile.addEventListener('click', function () {
                var dmOnlyEl = panel.querySelector('[data-gm-event-dmonly]');
                commit({
                    triggerEvent: {
                        type: tile.getAttribute('data-gm-id'),
                        name: tile.getAttribute('data-gm-label') || tile.getAttribute('data-gm-id'),
                        start_hour: 22,
                        duration_hours: 2,
                        dm_only: !!(dmOnlyEl && dmOnlyEl.checked),
                    },
                }, tile);
            });
        });

        // --- Per-event clear × chips (delegated — chips re-render per seed) ---
        panel.addEventListener('click', function (ev) {
            var x = ev.target.closest && ev.target.closest('[data-gm-event-clear]');
            if (!x || !panel.contains(x)) return;
            commit({ clearEventType: x.getAttribute('data-gm-event-clear') }, x);
        });

        // --- Clear ALL active world-events ---
        var clearEventsBtn = panel.querySelector('[data-gm-events-clear]');
        if (clearEventsBtn) {
            clearEventsBtn.addEventListener('click', function () {
                commit({ clearEvents: true }, clearEventsBtn);
            });
        }

        // --- Mood: presets / custom color / intensity / clear ---
        function moodIntensity() {
            var slider = panel.querySelector('[data-gm-mood-intensity-slider]');
            var v = slider ? parseInt(slider.value, 10) : 40;
            return isNaN(v) ? 0.4 : Math.max(0.05, Math.min(0.8, v / 100));
        }
        panel.querySelectorAll('[data-gm-mood]').forEach(function (sw) {
            sw.addEventListener('click', function () {
                var intensity = parseFloat(sw.dataset.gmMoodIntensity || '0.4');
                commit({ moodTint: { color: sw.dataset.gmMoodColor, intensity: isNaN(intensity) ? 0.4 : intensity } }, sw);
            });
        });
        var customColor = panel.querySelector('[data-gm-mood-custom]');
        if (customColor) {
            customColor.addEventListener('change', function () {
                commit({ moodTint: { color: customColor.value, intensity: moodIntensity() } });
            });
        }
        var intensitySlider = panel.querySelector('[data-gm-mood-intensity-slider]');
        if (intensitySlider) {
            intensitySlider.addEventListener('change', function () {
                // Re-commit the current color (custom picker's value) at the new
                // strength only when a wash is actually active.
                var ws = window.__calWorldState;
                var active = ws && ws.moodTint && ws.moodTint.color;
                if (active) commit({ moodTint: { color: active, intensity: moodIntensity() } });
            });
        }
        var moodClearBtn = panel.querySelector('[data-gm-mood-clear]');
        if (moodClearBtn) {
            moodClearBtn.addEventListener('click', function () {
                // Null color + 0 intensity clears the wash (server NULLs both columns).
                commit({ moodTint: { color: null, intensity: 0 } }, moodClearBtn);
            });
        }

        // --- Pause (client-only atmosphere freeze; not persisted) ---
        var pauseBtn = panel.querySelector('[data-gm-pause]');
        if (pauseBtn) {
            var paused = false;
            pauseBtn.addEventListener('click', function () {
                paused = !paused;
                try {
                    // Route through the engine's time-control so the CSS layers
                    // freeze too (the bare setPaused only froze the canvases).
                    if (window.__calTimeControl && window.__calTimeControl.setPaused) {
                        window.__calTimeControl.setPaused(paused);
                    } else if (window.CalParticleEngine && window.CalParticleEngine.setPaused) {
                        window.CalParticleEngine.setPaused(paused);
                    }
                } catch (e) {}
                pauseBtn.setAttribute('aria-pressed', paused ? 'true' : 'false');
                var label = pauseBtn.querySelector('[data-gm-pause-label]');
                if (label) label.textContent = paused ? 'Resume' : 'Pause';
            });
        }

        // --- Reset sky: weather → clear, all events off, mood off — one PUT ---
        var resetBtn = panel.querySelector('[data-gm-reset]');
        if (resetBtn) {
            resetBtn.addEventListener('click', function () {
                commit({ weather: 'clear', clearEvents: true, moodTint: { color: null, intensity: 0 } }, resetBtn);
            });
        }
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
    // Re-init after boosted navigation (QA2 class) — under
    // htmx.config.allowScriptTags=false the swapped-in panel's <script> never
    // re-runs, so the already-loaded driver must re-bind the fresh panel.
    // The per-element gmWired guard keeps it idempotent.
    document.addEventListener('htmx:afterSettle', init);
    document.addEventListener('htmx:load', init);
})();
