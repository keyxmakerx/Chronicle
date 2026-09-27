/*
 * sky_events.js — rare sky events, and the camera hook that turns the sky
 * toward the day's notable event (window.SkyEvents).
 *
 * DISCLOSED DEVIATION from the design contract's sky-events.js: the mockup's
 * SKYEV computes rich, generator-authored events (comets with their own
 * curves, meteor showers with a radiant and a rate, aurorae, custom owner
 * shapes) driven by fields (compass direction, altitude, colour, duration
 * curves) that simply do not exist on the real data model. The real
 * Event.Payload (MoonNightPayload, internal/plugins/calendar/model.go) is
 * exactly `{type, moons:[ids]}` — five types (blood, magic, conjunction,
 * eclipse, harvest), each naming which moons it concerns and nothing else.
 * So this ports the part of sky-events.js that IS load-bearing for real
 * data — placing an event relative to a moon's own position, and the `cam`
 * orientation hook — and defines the five real types' visuals directly
 * against that minimal payload, rather than inventing compass/altitude/
 * duration fields the server never sends. Comet/meteor-shower/aurora/custom
 * shapes are dropped entirely: there is no server data to drive them, and
 * inventing fake ones would be exactly the kind of un-disclosed simplifying
 * the task warns against. A follow-up issue is the right place to give
 * sky events their own richer payload if the operator wants those someday.
 *
 * One interpretive call, stated plainly: MoonNightPayload does not say
 * whether an "eclipse" is solar or lunar. Every eclipse here is rendered as
 * a lunar eclipse of the referenced moon(s) (the world's shadow crossing the
 * moon) since that is the only eclipse MOONR/the moon sprite can show without
 * a sun-crossing geometry the payload doesn't carry either. If the operator
 * wants solar eclipses distinguished, that needs a payload field, tracked as
 * a follow-up.
 */
(function () {
  'use strict';

  var clamp = window.SkyWorld.clamp, mod = window.SkyWorld.mod, D2R = window.SkyWorld.D2R;

  // camFor(dayEvents, moonStates): the azimuth (degrees from south, growing
  // west — SkyWorld's own convention) the sky should turn toward, given the
  // day's moon-night events. Mirrors the contract's V.face: turn toward the
  // referenced moons' spread, clamped to +/-100 degrees so the view never
  // spins past what a 270-degree panorama can show; span too wide (events on
  // opposite horizons) and it gives up and faces south (0), same as the
  // contract. moonStates: the state builder's per-moon array, each
  // {mo, m:{az radians, alt radians}, up, ...} — see sky_pane.js.
  function camFor(dayEvents, moonStates) {
    if (!dayEvents || !dayEvents.length) return 0;
    var az = [];
    dayEvents.forEach(function (e) {
      (e.moons || []).forEach(function (moonID) {
        var m = findMoonState(moonStates, moonID);
        if (m) az.push(m.m.az / D2R);
      });
    });
    if (!az.length) return 0;
    var LIM = 100, lo = Math.min.apply(null, az.concat([0])), hi = Math.max.apply(null, az.concat([0]));
    if (hi - lo > 2 * LIM) return 0;
    return lo < -LIM ? lo + LIM : hi > LIM ? hi - LIM : 0;
  }
  function findMoonState(moonStates, moonID) {
    for (var i = 0; i < moonStates.length; i++) if (moonStates[i].mo && moonStates[i].mo.id === moonID) return moonStates[i];
    return null;
  }

  // parseDayEvents(events): events is the real /events list (already
  // viewer-filtered server-side — never re-derive visibility here). Returns
  // the subset that carry a recognized MoonNightPayload, each as
  // {type, moons:[ids], raw: <the Event>}.
  var TYPES = ['blood', 'magic', 'conjunction', 'eclipse', 'harvest'];
  function parseDayEvents(events) {
    var out = [];
    (events || []).forEach(function (e) {
      if (!e.payload) return;
      var p;
      try { p = JSON.parse(e.payload); } catch (err) { return; }
      if (!p || TYPES.indexOf(p.type) < 0) return;
      out.push({ type: p.type, moons: p.moons || [], raw: e });
    });
    return out;
  }

  // overlaysFor(dayEvents, moonStates): visual hints for sky_2d's drawn
  // layer. moonStates: the array the state builder placed this render
  // (each {mo, x, y, r, up, ...}), so overlays can be skipped/adjusted for a
  // moon that isn't currently above the horizon.
  function overlaysFor(dayEvents, moonStates) {
    var out = { blood: {}, eclipse: {}, harvest: {}, conjGroups: [], magic: false };
    (dayEvents || []).forEach(function (e) {
      var ups = (e.moons || []).map(function (id) { return findMoonState(moonStates, id); }).filter(function (m) { return m && m.up; });
      if (e.type === 'blood') ups.forEach(function (m) { out.blood[m.mo.id] = true; });
      else if (e.type === 'eclipse') ups.forEach(function (m) { out.eclipse[m.mo.id] = .78; });
      else if (e.type === 'harvest') ups.forEach(function (m) { out.harvest[m.mo.id] = true; });
      else if (e.type === 'conjunction' && ups.length > 1) out.conjGroups.push(ups.map(function (m) { return m.mo.id; }));
      else if (e.type === 'magic') out.magic = true;
    });
    return out;
  }

  window.SkyEvents = { camFor: camFor, parseDayEvents: parseDayEvents, overlaysFor: overlaysFor, TYPES: TYPES };
})();
