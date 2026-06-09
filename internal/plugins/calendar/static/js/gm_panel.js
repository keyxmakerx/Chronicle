// gm_panel.js — the GM live-control panel wiring (C-CAL-WORLDSTATE-GM-LIVE-
// CONTROL-PANEL, Phase 4a). Only loaded for capability holders (the templ +
// the page gate on CanControlWorldState), so this never reaches a player.
//
// Source of truth = the server. Every control round-trips PUT
// /calendar/world-state (#401, capability-gated by #406); the PUT response is
// the freshly-built seed, which we hand to the engine's window.__calSetWorldState
// so the ambient band (2a) re-renders the new state without a full reload.
// Pause is the lone client-only control (atmosphere freeze is ephemeral per
// #401 — it is the GM's own view, not persisted).
(function () {
    'use strict';

    function init() {
        var panel = document.querySelector('[data-gm-panel]');
        if (!panel) return;
        // C-CAL-GM-PANEL-REWORK C (QA2 class): per-element guard so re-init on
        // boosted nav binds the freshly-swapped panel once, never the same node
        // twice (a new panel node has gmWired unset → it binds).
        if (panel.dataset.gmWired === '1') return;
        panel.dataset.gmWired = '1';
        var root = document.querySelector('[data-cal-v2-root]');
        if (!root) return;
        var campaignID = root.dataset.calV2CampaignId;
        var calendarID = root.dataset.calV2CalendarId;
        var csrfToken = root.dataset.calV2CsrfToken;

        var url = '/campaigns/' + campaignID + '/calendar/world-state' +
            (calendarID ? '?calendarId=' + calendarID : '');

        // C-CAL-SKY-COMPLETION C: as the DM advances the day/time, briefly fade
        // the card translucent so the living sky animates THROUGH it, then
        // restore. GPU-only (opacity), and skipped entirely under reduced-motion.
        var reduceMotion = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
        if (!reduceMotion) panel.style.transition = 'opacity 300ms ease';
        function flashTranslucentForTransition() {
            if (reduceMotion) return; // no fade under reduced-motion
            panel.style.opacity = '0.42';
            clearTimeout(panel.__gmFadeTimer);
            panel.__gmFadeTimer = setTimeout(function () { panel.style.opacity = ''; }, 900);
        }

        // PUT a writable world-state slice, then re-render the band from the
        // authoritative response seed (or reload as a fallback).
        function commit(body, btn) {
            if (btn) btn.disabled = true;
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

        // --- Advance verbs (signed; rollover server-side) ---
        panel.querySelectorAll('[data-gm-advance]').forEach(function (btn) {
            btn.addEventListener('click', function () {
                flashTranslucentForTransition();
                commit({
                    advance: {
                        days: parseInt(btn.dataset.gmDays || '0', 10) || 0,
                        hours: parseInt(btn.dataset.gmHours || '0', 10) || 0,
                        minutes: parseInt(btn.dataset.gmMinutes || '0', 10) || 0,
                    },
                }, btn);
            });
        });

        // --- Absolute set time ---
        var setTimeBtn = panel.querySelector('[data-gm-set-time]');
        if (setTimeBtn) {
            setTimeBtn.addEventListener('click', function () {
                flashTranslucentForTransition();
                commit({ time: { hour: num('[data-gm-time-hour]', 0), minute: num('[data-gm-time-minute]', 0) } }, setTimeBtn);
            });
        }

        // --- Absolute set date ---
        var setDateBtn = panel.querySelector('[data-gm-set-date]');
        if (setDateBtn) {
            setDateBtn.addEventListener('click', function () {
                flashTranslucentForTransition();
                commit({
                    time: {
                        year: num('[data-gm-date-year]', 0),
                        month: num('[data-gm-date-month]', 1),
                        day: num('[data-gm-date-day]', 1),
                    },
                }, setDateBtn);
            });
        }

        // --- Weather override (persists to calendar_day_weather) ---
        var setWeatherBtn = panel.querySelector('[data-gm-set-weather]');
        if (setWeatherBtn) {
            setWeatherBtn.addEventListener('click', function () {
                var sel = panel.querySelector('[data-gm-weather]');
                commit({ weather: sel ? sel.value : 'clear' }, setWeatherBtn);
            });
        }

        // --- Mood-tint preset swatches (persist to mood_tint_*) ---
        panel.querySelectorAll('[data-gm-mood]').forEach(function (sw) {
            sw.addEventListener('click', function () {
                var intensity = parseFloat(sw.dataset.gmMoodIntensity || '0.4');
                commit({ moodTint: { color: sw.dataset.gmMoodColor, intensity: isNaN(intensity) ? 0.4 : intensity } }, sw);
            });
        });
        var moodClearBtn = panel.querySelector('[data-gm-mood-clear]');
        if (moodClearBtn) {
            moodClearBtn.addEventListener('click', function () {
                // Null color + 0 intensity clears the wash (server NULLs both columns).
                commit({ moodTint: { color: null, intensity: 0 } }, moodClearBtn);
            });
        }

        // --- Trigger world-event (persists to calendar_celestial_events) ---
        var triggerBtn = panel.querySelector('[data-gm-trigger-event]');
        if (triggerBtn) {
            triggerBtn.addEventListener('click', function () {
                var typeSel = panel.querySelector('[data-gm-event-type]');
                var dmOnlyEl = panel.querySelector('[data-gm-event-dmonly]');
                var type = typeSel ? typeSel.value : '';
                var label = (typeSel && typeSel.options[typeSel.selectedIndex]) ? typeSel.options[typeSel.selectedIndex].text : type;
                commit({
                    triggerEvent: {
                        type: type,
                        name: label,
                        start_hour: 22,
                        duration_hours: 2,
                        dm_only: !!(dmOnlyEl && dmOnlyEl.checked),
                    },
                }, triggerBtn);
            });
        }

        // --- Clear active world-events (the "stuck meteor" off switch) ---
        var clearEventsBtn = panel.querySelector('[data-gm-events-clear]');
        if (clearEventsBtn) {
            clearEventsBtn.addEventListener('click', function () {
                commit({ clearEvents: true }, clearEventsBtn);
            });
        }

        // --- Pause (client-only atmosphere freeze; not persisted) ---
        var pauseBtn = panel.querySelector('[data-gm-pause]');
        if (pauseBtn) {
            var paused = false;
            pauseBtn.addEventListener('click', function () {
                paused = !paused;
                try {
                    if (window.CalParticleEngine && window.CalParticleEngine.setPaused) {
                        window.CalParticleEngine.setPaused(paused);
                    }
                } catch (e) {}
                pauseBtn.setAttribute('aria-pressed', paused ? 'true' : 'false');
                var label = pauseBtn.querySelector('[data-gm-pause-label]');
                if (label) label.textContent = paused ? 'Resume atmosphere' : 'Pause atmosphere';
            });
        }

        // --- Collapse / expand (animated max-height; the templ supplies the
        //     --dur-large/--ease-out transition). Respects reduced-motion. ---
        var toggle = panel.querySelector('[data-gm-panel-toggle]');
        var bodyEl = panel.querySelector('[data-gm-panel-body]');
        if (toggle && bodyEl) {
            var reduceMotion = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
            if (reduceMotion) bodyEl.style.transition = 'none';
            function setExpanded(expanded) {
                toggle.setAttribute('aria-expanded', expanded ? 'true' : 'false');
                toggle.setAttribute('aria-label', expanded ? 'Collapse panel' : 'Expand panel');
                var icon = toggle.querySelector('i');
                if (icon) icon.className = expanded ? 'fa-solid fa-chevron-down text-xs' : 'fa-solid fa-chevron-up text-xs';
                if (expanded) {
                    bodyEl.style.opacity = '1';
                    bodyEl.style.maxHeight = bodyEl.scrollHeight + 'px';
                    if (reduceMotion) {
                        bodyEl.style.maxHeight = 'none';
                    } else {
                        // Release to auto once expanded so nested content can grow.
                        var done = function () { bodyEl.style.maxHeight = 'none'; bodyEl.removeEventListener('transitionend', done); };
                        bodyEl.addEventListener('transitionend', done);
                    }
                } else {
                    // From auto height → a fixed px (so the transition has a start),
                    // force a reflow, then collapse to 0.
                    bodyEl.style.maxHeight = bodyEl.scrollHeight + 'px';
                    void bodyEl.offsetHeight;
                    bodyEl.style.maxHeight = '0px';
                    bodyEl.style.opacity = '0';
                }
            }
            toggle.addEventListener('click', function () {
                setExpanded(toggle.getAttribute('aria-expanded') === 'false');
            });
        }
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
    // C-CAL-GM-PANEL-REWORK C: re-init after boosted navigation (QA2 class) —
    // under htmx.config.allowScriptTags=false the swapped-in panel's <script>
    // never re-runs, so the already-loaded driver must re-bind the fresh panel.
    // The per-element gmWired guard keeps it idempotent.
    document.addEventListener('htmx:afterSettle', init);
    document.addEventListener('htmx:load', init);
})();
