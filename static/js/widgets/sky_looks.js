/*
 * sky_looks.js — how weather looks in the sky (window.SkyLooks, window.SkyFX).
 *
 * Ported from the signed sky pane mockup's sky-looks.js (#763, #842). Every
 * weather resolves into one recipe from the 41-effect vocabulary: the painted
 * sky's physics (cloud, darkness, fog, rain, snow, hail, storm, wind), a sky
 * tint, air layers, near particles, aurora, fireballs, sigils, a funnel and
 * lightning. The WebGL painter (sky_gl.js) reads all of it; the basic 2D sky
 * (sky_2d.js) reads the physics and tint.
 *
 * A weather row's preset_id picks its Calendaria built-in. A campaign's own
 * preset draws like the built-in its icon and precipitation are closest to.
 *
 * SkyFX draws on the page what the painter lays into the sky: particles near
 * the viewer, and shapes (sigils, halos, the warm glow of a harvest moon).
 */
(function () {
  'use strict';

  var SW = window.SkyWorld, PAL = SW.PAL, clamp = SW.clamp, smooth01 = SW.smooth01, D2R = SW.D2R;

  /* Sigils: thin glyphs that are no letter or known sign. */
  var SIGILS = ['M5.2 5C5.2 4 3.8 4 3.8 5.1C3.8 6.7 6.6 6.7 6.6 4.9C6.6 2.6 2.4 2.6 2.4 5.2C2.4 7.4 4.4 8.6 6.8 8',
    'M4 5.6a1.3 1.3 0 1 0 2.6 0a1.3 1.3 0 1 0-2.6 0M2 3.4C3.6 1.8 7 1.8 8.2 4.2M8 7.6L6.9 6.8',
    'M5 3.6C5.8 5 4.2 6.6 5 8.4M5 1.4a1.1 1.1 0 1 0 .01 0M7 6.2L8 5.6',
    'M1.8 4.4C2.8 2.4 3.8 2.4 4.6 4.2S6.6 5.8 8.2 3M4 7.4a1 1 0 1 0 2 0a1 1 0 1 0-2 0',
    'M1.8 3C4.6 3 6.8 5 7.2 8M3.4 6.4a1 1 0 1 0 .01 0M8.2 3.4h.01',
    'M2.4 7.2C3.4 3.6 6 2.2 7.8 2.6C7.2 5.8 5 7.8 2.4 7.2M4.8 5.4h.01',
    'M2 5.8C3.4 4.2 5 4.2 6 5.8S8 7 8.4 6M4 2.2a1 1 0 1 0 .01 0',
    'M2.2 3C3.8 4.6 6.2 4.6 7.8 3M3.2 6.8C4.4 5.8 5.6 5.8 6.8 6.8M5 8.2h.01',
    'M2.6 7.8C2.2 5 3.6 3 5.4 3.2S7.8 5.6 7 7.6M4.6 1.6L6.6 2.2M7.8 8.2h.01',
    'M2.2 6.8C3.8 5.2 3.8 3.2 2.6 2.2M4.2 7.6C6.8 6.2 7.4 3.8 6.4 2M8 4.6h.01',
    'M2.2 2.2C3 4.6 5.4 6.2 8 6.4M3.8 8.2C4.6 7.2 5.8 6.8 7 7.4M6.8 2.4a.9 .9 0 1 0 .01 0',
    'M3.4 5.4a1.6 1.6 0 1 0 3.2 0a1.6 1.6 0 1 0-3.2 0M1.8 8.4C3.6 8.2 5.6 7.6 6.8 6.4'], SIGP = null;
  function sigil(i) { if (!SIGP) SIGP = SIGILS.map(function (d) { return new Path2D(d); }); return SIGP[i % SIGP.length]; }

  /* ── How weather looks in the sky. Every weather, built-in or an owner's own, is drawn through one path: a main effect
     and up to two more layers from one vocabulary, a strength and a tint. The vocabulary is Calendaria's 38 effects plus
     motes, sparks and sigils; each is a hand-tuned building block (a state for the painted sky, layers of air, particles
     near the viewer), so an owner chooses blocks and the renderer owns the art. ── */
  var LOOKS = (function(){
    var L = {};
    /* Groups, in the order the look editor shows them. */
    L.GROUPS = [
      ['Clouds and sky', ['clear', 'clouds-light', 'clouds-heavy', 'clouds-overcast', 'fog', 'haze', 'aurora', 'meteors']],
      ['Rain and snow', ['rain', 'rain-heavy', 'lightning', 'sleet', 'hail', 'snow', 'snow-heavy', 'ice', 'rain-acid', 'rain-blood']],
      ['Wind and dust', ['gust', 'sand', 'tornado', 'hurricane', 'leaves', 'petals', 'spores']],
      ['Fire and smoke', ['embers', 'ashfall', 'smoke', 'sparks']],
      ['Magic', ['aether', 'arcane', 'arcane-wind', 'ley-surge', 'nullstatic', 'spectral', 'void', 'veil', 'divine', 'miasma', 'motes', 'sigils']]
    ];
    L.VOCAB = [];
    L.GROUPS.forEach(function(g){ g[1].forEach(function(id){ L.VOCAB.push(id); }); });
    /* Calendaria's own effect for each of the 42 built-in weathers. */
    L.PRESET_EFFECT = {
      'clear':'clear', 'partly-cloudy':'clouds-light', 'cloudy':'clouds-heavy', 'overcast':'clouds-overcast', 'drizzle':'rain', 'rain':'rain', 'fog':'fog', 'mist':'fog',
      'windy':'gust', 'sunshower':'rain', 'snow':'snow', 'sleet':'sleet', 'heat-wave':'haze', 'thunderstorm':'lightning', 'blizzard':'snow-heavy', 'hail':'hail',
      'tornado':'tornado', 'hurricane':'hurricane', 'ice-storm':'ice', 'monsoon':'rain-heavy', 'ashfall':'ashfall', 'sandstorm':'sand', 'luminous-sky':'aurora',
      'sakura-bloom':'petals', 'autumn-leaves':'leaves', 'rolling-fog':'fog', 'wildfire-smoke':'smoke', 'dust-devil':'sand', 'black-sun':'void', 'ley-surge':'ley-surge',
      'aether-haze':'aether', 'nullfront':'nullstatic', 'permafrost-surge':'ice', 'gravewind':'spectral', 'veilfall':'veil', 'arcane-winds':'arcane-wind',
      'acid-rain':'rain-acid', 'blood-rain':'rain-blood', 'meteor-shower':'meteors', 'spore-cloud':'spores', 'divine-light':'divine', 'plague-miasma':'miasma'
    };
    /* The engine's physics for each built-in (cloud cover, wetness, energy, precipitation and its amount), which the
       painted sky follows: a drizzle is lighter than rain, mist thinner than fog, a dust devil gentler than a sandstorm. */
    L.PHYS = {
      'clear':[0, 0, .1, null, 0], 'partly-cloudy':[.35, .05, .15, null, 0], 'cloudy':[.7, .1, .2, null, 0], 'overcast':[.95, .2, .2, null, 0],
      'drizzle':[.9, .4, .15, 'drizzle', .2], 'rain':[.95, .75, .4, 'rain', .55], 'fog':[.8, .3, 0, 'drizzle', .05], 'mist':[.5, .25, .05, null, 0],
      'windy':[.4, .05, .6, null, 0], 'sunshower':[.45, .35, .2, 'rain', .25], 'snow':[.95, .6, .3, 'snow', .5], 'sleet':[.95, .6, .4, 'sleet', .5],
      'heat-wave':[.05, 0, .1, null, 0], 'thunderstorm':[.9, .8, .85, 'rain', .8], 'blizzard':[1, .8, .95, 'snow', .9], 'hail':[.9, .7, .8, 'hail', .7],
      'tornado':[1, .7, 1, 'rain', .7], 'hurricane':[1, 1, 1, 'rain', 1], 'ice-storm':[1, .7, .7, 'hail', .6], 'monsoon':[1, 1, .6, 'rain', .95],
      'ashfall':[.8, .1, .3, null, 0], 'sandstorm':[.6, 0, .9, null, 0], 'luminous-sky':[.1, 0, .1, null, 0], 'sakura-bloom':[.2, 0, .2, null, 0],
      'autumn-leaves':[.4, .05, .5, null, 0], 'rolling-fog':[.85, .35, .15, null, 0], 'wildfire-smoke':[.6, 0, .2, null, 0], 'dust-devil':[.1, 0, .5, null, 0],
      'black-sun':[.5, 0, .5, null, 0], 'ley-surge':[.5, .1, .7, null, 0], 'aether-haze':[.5, .1, .2, null, 0], 'nullfront':[.8, .1, .6, null, 0],
      'permafrost-surge':[.9, .6, .7, 'snow', .7], 'gravewind':[.6, .05, .7, null, 0], 'veilfall':[.9, .3, .3, 'rain', .3], 'arcane-winds':[.4, 0, .8, null, 0],
      'acid-rain':[.95, .7, .4, 'rain', .6], 'blood-rain':[.95, .6, .4, 'rain', .5], 'meteor-shower':[.05, 0, .2, null, 0], 'spore-cloud':[.5, .2, .2, null, 0],
      'divine-light':[.1, 0, .3, null, 0], 'plague-miasma':[.7, .4, .1, null, 0]
    };
    return L;
  })();

  /* ── The 41 building blocks. Each says what the painted sky does (base: cloud, darkness, fog, rain, snow, hail, storm,
     wind, as 0..1 at full strength), how it colours the sky (sky: colour, how much, and a light factor), and what it adds:
     air (sheets, fog banks, haze, streaks, rays drawn into the sky by the shader; depth 0 sky, 1 valleys, 2 near), parts
     (particles near the viewer), aurora, sigils, fireballs, a funnel, a flash. Names are the owner's words. best: the
     hour its preview is shown at. ── */
  LOOKS.AIR = {veil:0, blob:1, haze:2, gust:3, sand:4, rays:5, 'void':6, plume:7};
  LOOKS.EFFECTS = {
    'clear':{n:'Clear', best:12.3, base:{cloud:.08, wind:.2}},
    'clouds-light':{n:'Light clouds', best:12.3, base:{cloud:.38, fog:.02, wind:.3}, sky:['#a9b3c0', .08, 1]},
    'clouds-heavy':{n:'Heavy clouds', best:12.3, base:{cloud:.72, dark:.12, fog:.05, wind:.35}, sky:['#828896', .18, .96]},
    'clouds-overcast':{n:'Overcast', best:12.3, base:{cloud:.95, dark:.22, fog:.08, wind:.3}, sky:['#6e7078', .3, .9]},
    'fog':{n:'Fog', best:10.5, base:{cloud:.35, dark:.05, fog:1, wind:.1}, sky:['#b9bec2', .2, 1]},
    'haze':{n:'Heat haze', best:15, base:{cloud:.08, fog:.22, wind:.15}, sky:['#e9c48f', .22, 1.05],
      air:[{t:'haze', c:'#ffcf8a', k:.85, v:[-.06, .4], d:0}]},
    'aurora':{n:'Aurora', best:22.5, base:{cloud:.06}, sky:['#1b3c3a', .22, 1], aurora:{c0:'#45ffa0', c1:'#c35cff', k:.9, alt:'mid', span:170}},
    'meteors':{n:'Fireballs', best:22.5, base:{cloud:.05}, sky:['#2a1e3a', .14, 1], fire:{rate:.18, c:'#ff8a3d'}},
    'rain':{n:'Rain', best:12.3, base:{cloud:.95, dark:.38, fog:.18, rain:1, wind:.5}, sky:['#5a6478', .25, .92]},
    'rain-heavy':{n:'Downpour', best:12.3, base:{cloud:1, dark:.55, fog:.26, rain:1.5, wind:.75}, sky:['#434a5a', .35, .85], heavy:.8, slant:.12},
    'lightning':{n:'Thunder and lightning', best:16.8, base:{cloud:1, dark:.85, fog:.1, rain:1.25, storm:1, wind:1}, sky:['#2c2f45', .35, .88], flash:'lightning'},
    'sleet':{n:'Sleet', best:12.3, base:{cloud:.95, dark:.3, fog:.2, rain:.7, snow:.45, wind:.55}, sky:['#5e6678', .22, .92]},
    'hail':{n:'Hail', best:12.3, base:{cloud:.95, dark:.5, fog:.12, rain:.35, hail:1, wind:.6}, sky:['#4f566a', .25, .9]},
    'snow':{n:'Snow', best:12.3, base:{cloud:.86, dark:.06, fog:.2, snow:1, wind:.25}, sky:['#c6ceda', .1, 1.02]},
    'snow-heavy':{n:'Blizzard snow', best:12.3, base:{cloud:.98, dark:.15, fog:.45, snow:1.6, wind:.85}, sky:['#cfd6e3', .18, 1]},
    'ice':{n:'Ice crystals', best:11, base:{cloud:.55, dark:.05, fog:.15, wind:.2}, sky:['#a9cfe8', .18, 1.02],
      parts:[{p:'shard', n:26, s:[1.6, 3.4], c:'#bfe6ff', c2:'#eaf7ff', v:[4, 11], wob:4, spin:.6, e:.35, a:.9, mode:'fall'}]},
    'rain-acid':{n:'Acid rain', best:12.3, base:{cloud:.95, dark:.35, fog:.2, rain:1, wind:.5}, sky:['#3d6a2c', .3, .9], rainTint:['#7ee05a', .85],
      air:[{t:'blob', c:'#8fcf55', k:.22, sx:.004, sy:0, v:[-.1, .25], d:1}]},
    'rain-blood':{n:'Blood rain', best:12.3, base:{cloud:.95, dark:.4, fog:.18, rain:1, wind:.5}, sky:['#5a1a20', .35, .88], rainTint:['#c0283a', .9]},
    'gust':{n:'Gusts', best:12.3, base:{cloud:.4, fog:.02, wind:1}, air:[{t:'gust', c:'#eef2f5', k:1, sx:.22, v:[.02, .95], d:0}],
      parts:[{p:'grain', n:36, s:[.8, 1.5], c:'#d8d2c4', c2:'#f3efe6', v:[130, 8], wob:4, e:0, a:.55, mode:'blow'}]},
    'sand':{n:'Blowing sand', best:13, base:{cloud:.5, dark:.1, fog:.55, wind:1}, sky:['#c9a46a', .55, 1],
      air:[{t:'sand', c:'#d9b27a', k:.8, sx:.18, v:[-.1, .9], d:2}], parts:[{p:'grain', n:150, s:[.8, 1.8], c:'#b8894a', c2:'#d9b27a', v:[150, 12], wob:3, e:0, a:.75, mode:'blow'}]},
    'tornado':{n:'Funnel cloud', best:17.2, base:{cloud:1, dark:.7, fog:.15, rain:.5, wind:1}, sky:['#3f4a36', .35, .85], funnel:{c:'#3b3a40', k:1, w:1},
      parts:[{p:'debris', n:42, s:[1, 2.6], c:'#5a4a3a', c2:'#7a6a55', v:[60, -8], e:0, a:.9, mode:'vortex'}]},
    'hurricane':{n:'Gale-driven rain', best:12.3, base:{cloud:1, dark:.75, fog:.3, rain:1.6, wind:1}, sky:['#3a4250', .35, .82], heavy:.9, slant:2.1},
    'leaves':{n:'Falling leaves', best:15.5, base:{cloud:.35, wind:.55}, sky:['#b8894e', .1, 1],
      parts:[{p:'leaf', n:18, s:[3.2, 5.4], c:'#c8641f', c2:'#e0a13a', v:[26, 20], wob:14, spin:1.2, e:0, a:.95, mode:'fall'}]},
    'petals':{n:'Petals', best:14, base:{cloud:.15, wind:.35}, sky:['#e8b8c8', .08, 1.02],
      parts:[{p:'petal', n:22, s:[2.4, 4], c:'#ffb3cf', c2:'#ffe0ea', v:[18, 14], wob:12, spin:1.4, e:0, a:.92, mode:'fall'}]},
    'spores':{n:'Spores', best:15, base:{cloud:.45, fog:.35, wind:.1}, sky:['#6d8a3f', .3, .9],
      parts:[{p:'spore', n:30, s:[1.4, 2.6], c:'#a6cf5a', c2:'#dff0a0', v:[3, -6], wob:6, e:.45, a:.85, mode:'rise'}]},
    'embers':{n:'Embers', best:20.8, base:{cloud:.3, fog:.15, wind:.3}, sky:['#a8683a', .3, .95],
      parts:[{p:'ember', n:24, s:[1.3, 2.4], c:'#ff6a1a', c2:'#ffb347', v:[6, -22], wob:8, fl:1, e:1, a:1, mode:'rise'}]},
    'ashfall':{n:'Falling ash', best:14, base:{cloud:.8, dark:.35, fog:.3, wind:.15}, sky:['#6e5a48', .45, .82],
      parts:[{p:'ash', n:40, s:[1.4, 3], c:'#8a8580', c2:'#b3aea8', v:[5, 10], wob:6, spin:.5, e:0, a:.8, mode:'fall'},
        {p:'ember', n:8, s:[1, 1.8], c:'#ff6a1a', c2:'#ffb347', v:[4, 8], wob:5, fl:1, e:1, a:.9, mode:'fall'}]},
    'smoke':{n:'Smoke', best:15.5, base:{cloud:.6, dark:.25, fog:.4, wind:.25}, sky:['#5b4a3a', .5, .8], sun:{tint:'#ff7a3a', dim:.35},
      air:[{t:'plume', c:'#4f4640', k:.75, sx:.02, sy:.035, v:[-.1, 1], d:0}, {t:'blob', c:'#6b5a4c', k:.3, sx:.006, sy:0, v:[-.1, .5], d:1}]},
    'sparks':{n:'Sparks', best:21, base:{cloud:.1},
      parts:[{p:'spark', n:44, s:[1.6, 2.8], c:'#fff0c0', c2:'#ffd27a', v:[2, 7], wob:3, e:1, a:1, mode:'spark'}]},
    'aether':{n:'Aether glow', best:18, base:{cloud:.3, fog:.4, wind:.1}, sky:['#8a3c78', .35, 1],
      air:[{t:'blob', c:'#ee88dd', k:.6, sx:.008, sy:.002, v:[-.05, .8], d:0, e:.5}]},
    'arcane':{n:'Arcane motes', best:21, base:{cloud:.15}, sky:['#3c64b4', .25, 1],
      parts:[{p:'mote', n:30, s:[1.6, 2.9], c:'#4488ff', c2:'#a8ccff', v:[5, -14], wob:7, fl:.5, e:1, a:1, mode:'rise'}]},
    'arcane-wind':{n:'Arcane wind', best:16, base:{cloud:.35, wind:.9}, sky:['#6e3c82', .3, 1],
      air:[{t:'blob', c:'#b48cff', k:.5, sx:.05, sy:0, v:[0, .85], d:0, rb:1, e:.5}],
      parts:[{p:'mote', n:20, s:[.9, 1.6], c:'#ffffff', c2:'#ffffff', v:[70, -2], wob:5, fl:1, e:1, a:.9, mode:'blow', rb:1}]},
    'ley-surge':{n:'Ley surge', best:21, base:{cloud:.45, dark:.2, wind:.7}, sky:['#3c2a78', .4, .95], flash:'ley',
      parts:[{p:'mote', n:30, s:[1.1, 2.2], c:'#ffffff', c2:'#ffffff', v:[8, -34], wob:9, fl:.8, e:1, a:1, mode:'rise', rb:1}]},
    'nullstatic':{n:'Dead static', best:13, base:{cloud:.7, dark:.3, fog:.1}, sky:['#141018', .6, .55], grey:.7,
      parts:[{p:'pixel', n:55, s:[1, 1.8], c:'#ffffff', c2:'#8a8a8a', v:[0, 0], e:.6, a:.8, mode:'blink'}]},
    'spectral':{n:'Wraiths', best:22, base:{cloud:.5, fog:.3, dark:.15}, sky:['#2f4a40', .4, .9],
      parts:[{p:'wraith', n:12, s:[7, 13], c:'#8fd9c8', c2:'#cff5ec', v:[10, 0], wob:10, e:.55, a:.55, mode:'swirl'}]},
    'void':{n:'Void', best:12.3, base:{cloud:.45, dark:.45}, sky:['#1a0e12', .65, .45], sun:{'void':1}, stars:.6,
      air:[{t:'void', c:'#6e0a22', k:.7, v:[-.1, 1.1], d:0}]},
    'veil':{n:'Veils', best:17, base:{cloud:.5, fog:.2}, sky:['#6c5a8c', .35, .95],
      air:[{t:'veil', c:'#d8c8f0', k:.6, sx:.01, sy:.02, v:[.05, 1.05], d:0}]},
    'divine':{n:'Holy light', best:10, base:{cloud:.15}, sky:['#e8c870', .3, 1.08],
      air:[{t:'rays', c:'#fff0b0', k:.7, v:[-.1, 1.1], d:0, e:.7}],
      parts:[{p:'mote', n:18, s:[1.2, 2.2], c:'#ffdd66', c2:'#fff4c0', v:[2, 7], wob:5, fl:.3, e:1, a:.95, mode:'fall', pulse:1}]},
    'miasma':{n:'Miasma', best:11, base:{cloud:.6, fog:.6, dark:.2}, sky:['#4b5a28', .5, .8],
      air:[{t:'blob', c:'#6b7f35', k:.6, sx:.004, sy:.001, v:[-.15, .35], d:1}]},
    'motes':{n:'Motes of light', best:20, base:{cloud:.1},
      parts:[{p:'mote', n:32, s:[1.5, 2.8], c:'#fff3c4', c2:'#ffffff', v:[4, -4], wob:7, fl:.3, e:.9, a:.95, mode:'drift'}]},
    'sigils':{n:'Sigils', best:21.5, base:{cloud:.1}, sigils:{c:'#c9b3ff', k:.8}}
  };

  /* ── Colour. An owner's colour keeps its hue; its lightness and chroma are placed in a band tuned for what it colours
     (a glow, a lit body, air, a shadow), one band by day and another by night, so a neon pick reads as a luminous version
     of its hue and never as a flat sticker. ── */
  (function(L){
    function okOf(hex){ var q = PAL.lab(hex); return {L:q[0], C:Math.hypot(q[1], q[2]), h:Math.atan2(q[2], q[1])}; }
    function okLin(l, c, h){ return PAL.labLin([l, c * Math.cos(h), c * Math.sin(h)]); }
    L.okOf = okOf; L.okLin = okLin;
    var BANDS = {
      glow:{day:[.74, .93, .04, .16], night:[.64, .86, .05, .15]},
      body:{day:[.55, .85, .02, .14], night:[.42, .72, .02, .12]},
      air:{day:[.62, .92, .01, .11], night:[.40, .70, .01, .10]},
      shade:{day:[.14, .34, .02, .12], night:[.08, .26, .02, .10]}
    };
    L.BANDS = BANDS;
    /* The colour placed in its band (night is 0..1, how dark the sky is), as linear RGB. */
    L.tame = function(hex, role, night){
      var o = okOf(hex), b = BANDS[role] || BANDS.body, n = clamp(night || 0, 0, 1);
      var lo = b.day[0] + (b.night[0] - b.day[0]) * n, hi = b.day[1] + (b.night[1] - b.day[1]) * n;
      var cl = b.day[2] + (b.night[2] - b.day[2]) * n, ch = b.day[3] + (b.night[3] - b.day[3]) * n;
      var l = clamp(o.L, lo, hi), c = o.C < .025 ? Math.min(o.C, ch) : clamp(o.C, cl, ch);
      return okLin(l, c, o.h);
    };
    /* A design colour carried to the owner's hue: its own lightness, the tint's hue, never greyer than a hint of colour. */
    L.hueTo = function(hex, tint){
      if (!tint) return hex;
      var a = okOf(hex), t = okOf(tint), rgb = okLin(a.L, Math.max(a.C, Math.min(.12, t.C) * .9, .06), t.h);
      return L.linHex(rgb);
    };
    L.linHex = function(rgb){ return '#' + rgb.map(function(v){ v = clamp(v, 0, 1); v = v <= .0031308 ? v * 12.92 : 1.055 * Math.pow(v, 1 / 2.4) - .055; return ('0' + Math.round(v * 255).toString(16)).slice(-2); }).join(''); };
    L.srgb = function(rgb, a){ return 'rgba(' + rgb.map(function(v){ v = clamp(v, 0, 1); return Math.round(255 * (v <= .0031308 ? v * 12.92 : 1.055 * Math.pow(v, 1 / 2.4) - .055)); }).join(',') + ',' + (a == null ? 1 : +a.toFixed(3)) + ')'; };
    L.lum = function(c){ return .2126 * c[0] + .7152 * c[1] + .0722 * c[2]; };
    /* Scene light on a colour: a lit body takes the light's colour and level; a glow keeps its own light, warmed a
       little by dusk; fog softens both; nothing may be brighter than the cap (the brightest moon by night). */
    L.lit = function(hex, role, sl){
      var c = L.tame(hex, role === 'glow' ? 'glow' : role, sl.night), out;
      if (role === 'glow') out = [0, 1, 2].map(function(k){ return c[k] * (1 + (sl.col[k] - 1) * .25); });
      else if (role === 'shade') out = [0, 1, 2].map(function(k){ return c[k] * (.6 + .4 * sl.col[k] * sl.amb); });
      else out = [0, 1, 2].map(function(k){ return c[k] * sl.col[k] * Math.max(sl.amb, .1); });
      var y = L.lum(out);
      if (y > sl.cap) out = out.map(function(v){ return v * sl.cap / y; });
      return out;
    };

    /* ── A weather as the renderer sees it: a built-in id, or an owner's kind with its look. ── */
    /* A Chronicle weather row as the renderer sees it. A built-in Calendaria
       preset draws with its own physics; a campaign's own preset id draws like
       the built-in its icon and precipitation are closest to. */
    var SPECS = {};
    L.spec = function(w){
      if (!w) return {id:'clear', like:'clear', builtin:true, label:'Clear skies'};
      var id = w.preset_id || 'clear', key = id + '|' + (w.preset_label || '') + '|' + (w.icon || '') + '|' + JSON.stringify(w.precipitation || null);
      if (SPECS[key]) return SPECS[key];
      var like = L.PHYS[id] ? id : likeOf(w);
      return (SPECS[key] = {id:id, like:like, builtin:true, label:w.preset_label || id});
    };
    function likeOf(w){
      var p = w.precipitation || {}, type = p.type || '', k = p.intensity || 0, icon = String(w.icon || '').toLowerCase();
      if (/storm|thunder|lightning/.test(icon)) return 'thunderstorm';
      if (type === 'hail') return 'hail';
      if (type === 'sleet') return 'sleet';
      if (type === 'snow') return k > .65 ? 'blizzard' : 'snow';
      if (type === 'rain' || type === 'drizzle' || /rain/.test(icon)) return k > .65 ? 'monsoon' : type === 'drizzle' ? 'drizzle' : 'rain';
      if (/fog|mist/.test(icon)) return 'fog';
      if (/wind/.test(icon)) return 'windy';
      if (/overcast/.test(icon)) return 'overcast';
      if (/cloud/.test(icon)) return k > .5 ? 'cloudy' : 'partly-cloudy';
      return 'clear';
    }
    L.effectName = function(id){ return (L.EFFECTS[id] || {}).n || id; };

    /* Precipitation follows the preset's amount: drizzle a third of rain, a monsoon half again as much. */
    function amount(ph){ return ph && ph[4] > 0 ? clamp(ph[4] / .55, .3, 1.8) : 1; }
    /* The sky's dials: every plain number a weather sets, with its value on a still, empty sky and, for the ones the
       pane glides between weathers, how many seconds the glide takes. Any change of weather is just new targets for
       these, so no pair of weathers needs a transition of its own (#945). Dials with no ease change at once. */
    L.DIALS = [
      {n:'cloud', rest:0, ease:.8, what:'how much of the sky is cloud'},
      {n:'dark', rest:0, ease:.8, what:'how far storm cloud darkens the sky'},
      {n:'fog', rest:0, ease:.8, what:'how thick the fog is'},
      {n:'rain', rest:0, ease:.8, what:'how hard it rains (above 1 for a downpour)'},
      {n:'snow', rest:0, ease:.8, what:'how hard it snows'},
      {n:'hail', rest:0, ease:.8, what:'how hard it hails'},
      {n:'storm', rest:0, ease:.8, what:'how stormy: lightning and churn'},
      {n:'wind', rest:.2, ease:.8, what:'how strong the wind is'},
      {n:'grey', rest:0, ease:null, what:'how far the light is washed grey'},
      {n:'light', rest:1, ease:null, what:'how bright the light is, 1 for a plain day'},
      {n:'slant', rest:0, ease:null, what:'how far rain is driven sideways'},
      {n:'heavy', rest:0, ease:null, what:'how heavy the drops are'},
      {n:'stars', rest:1, ease:null, what:'how many stars show, 1 for all'},
      {n:'drift', rest:1, ease:null, what:'how fast fog and haze drift, 1 for a still day'}
    ];
    L.DIAL_NAMES = L.DIALS.map(function(d){ return d.n; });
    function blank(){
      var R = {sky:null, fogTint:null, rainTint:null, aurora:[], air:[], parts:[], sigils:[], fire:[], funnel:null, flash:{lightning:0, ley:0}, sun:{dim:0, 'void':0, tint:null}, effects:[]};
      L.DIALS.forEach(function(d){ R[d.n] = d.rest; });
      return R;
    }
    L.blank = blank;
    function hx(c, tint){ return tint ? L.hueTo(c, tint) : c; }
    /* One building block into a recipe, at weight w (1 for the main effect, less for a second layer) and strength m. */
    function add(R, id, w, m, tint, ph, builtin){
      var E = L.EFFECTS[id]; if (!E) return;
      var b = E.base || {}, main = w >= 1, k = w * m;
      R.effects.push(id);
      if (main && builtin && ph){
        var f = amount(ph);
        R.cloud = ph[0]; R.dark = (b.dark || 0) * (.6 + .4 * Math.min(f, 1.4)); R.fog = id === 'fog' ? clamp(ph[0] * 1.2, .4, 1) : (b.fog || 0);
        R.rain = (b.rain || 0) * f; R.snow = (b.snow || 0) * f; R.hail = (b.hail || 0) * f; R.storm = b.storm || 0;
        R.wind = (b.wind != null ? b.wind : .2) * .4 + (.12 + .88 * ph[2]) * .6;
        /* The preset's own precipitation, where its effect draws none (ice pellets under ice crystals). */
        if (ph[3] && !(b.rain || b.snow || b.hail)){
          var q = .6 * f;
          if (ph[3] === 'rain' || ph[3] === 'drizzle') R.rain += q; else if (ph[3] === 'snow') R.snow += q; else if (ph[3] === 'hail') R.hail += q; else if (ph[3] === 'sleet'){ R.rain += q * .6; R.snow += q * .5; }
        }
      } else {
        var pc = ph ? ph[0] : 0, wk = main ? 1 : .75 * m;
        R.cloud = Math.max(R.cloud, pc, (b.cloud || 0) * (main ? .55 + .45 * Math.min(m, 1) : wk));
        ['dark', 'fog', 'rain', 'snow', 'hail', 'storm'].forEach(function(n){ R[n] = Math.max(R[n], (b[n] || 0) * (main ? Math.min(m, 1.15) : wk)); });
        R.wind = Math.max(R.wind, (b.wind || 0) * (main ? 1 : .8), ph ? .12 + .88 * ph[2] : 0);
      }
      if (E.sky){ var sk = w * E.sky[1] * (main ? Math.min(1, .5 + .5 * m) : .5); R.sky = R.sky ? mixSky(R.sky, [hx(E.sky[0], tint), sk]) : [hx(E.sky[0], tint), sk]; R.light *= 1 + (E.sky[2] - 1) * w; }
      if (E.grey) R.grey = Math.max(R.grey, E.grey * w);
      if (E.rainTint) R.rainTint = [hx(E.rainTint[0], tint), E.rainTint[1] * Math.min(1, k)];
      if (E.slant) R.slant = Math.max(R.slant, E.slant * w); if (E.heavy) R.heavy = Math.max(R.heavy, E.heavy * w);
      if (E.aurora){ var A = E.aurora; R.aurora.push({c0:hx(A.c0, tint), c1:hx(A.c1, tint), k:A.k * k, alt:A.alt, span:A.span, az:0}); }
      (E.air || []).forEach(function(a){ R.air.push(Object.assign({}, a, {c:hx(a.c, tint), k:a.k * k})); });
      (E.parts || []).forEach(function(p){ R.parts.push(Object.assign({}, p, {c:hx(p.c, tint), c2:hx(p.c2, tint), n:p.n * Math.min(1.25, .35 + .65 * k), a:p.a * Math.min(1, .55 + .45 * k), rb:tint ? 0 : p.rb, fx:id})); });
      if (E.sigils) R.sigils.push({c:hx(E.sigils.c, tint), k:E.sigils.k * k});
      if (E.fire) R.fire.push({rate:E.fire.rate * Math.min(1.3, .4 + .6 * k), c:hx(E.fire.c, tint)});
      if (E.funnel) R.funnel = {c:hx(E.funnel.c, tint), k:E.funnel.k * Math.min(1.1, k), w:E.funnel.w * (ph && ph[2] < .6 ? .45 : 1)};
      if (E.flash) R.flash[E.flash] = Math.max(R.flash[E.flash], Math.min(1, k));
      if (E.sun){ if (E.sun.dim) R.sun.dim = Math.max(R.sun.dim, E.sun.dim * w); if (E.sun['void']) R.sun['void'] = Math.max(R.sun['void'], E.sun['void'] * Math.min(1, k)); if (E.sun.tint) R.sun.tint = hx(E.sun.tint, tint); }
      if (E.stars) R.stars = Math.min(R.stars, 1 + (E.stars - 1) * w);
      /* An owner's tint colours the air itself a little: fog and haze take its hue. */
      if (tint && main){ R.fogTint = [tint, .45 * Math.min(1, m)]; R.rainTint = R.rainTint || ((R.rain + R.snow + R.hail) > 0 ? [hx('#a8c4e6', tint), .55 * Math.min(1, m)] : null); }
    }
    function mixSky(a, b){ var t = b[1] / Math.max(1e-6, a[1] + b[1]); return [L.linHex(PAL.mixLin(PAL.hexLin(a[0]), PAL.hexLin(b[0]), t)), Math.min(.8, a[1] + b[1] * .6)]; }

    /* Resolve a weather into one recipe. Built-ins use their Calendaria effect at full strength with the preset's own
       physics; an owner's kind uses its look over the physics of the weather it is like. */
    L.resolve = function(spec){
      if (!spec) return null;
      if (spec._R && spec._Rkey === JSON.stringify(spec.look || null) + spec.like) return spec._R;
      var builtin = !!spec.builtin, like = spec.like || spec.id, ph = L.PHYS[like] || L.PHYS.clear, look = spec.look || {};
      var main = look.effect || L.PRESET_EFFECT[like] || 'clear', acc = (look.accents || []).filter(function(a){ return L.EFFECTS[a] && a !== main; }).slice(0, 2);
      var s = look.strength != null ? clamp(+look.strength, 0, 1) : .6, m = builtin ? 1 : .3 + .95 * s, tint = look.tint || null;
      var R = blank();
      add(R, main, 1, m, tint, ph, builtin && !look.effect);
      acc.forEach(function(a){ add(R, a, .8, m, tint, null, false); });
      /* Rolling fog is thinner overall and lies in banks along the valley that roll by; plain fog is one even veil. */
      if (like === 'rolling-fog' && !look.effect){ R.drift = 3.2; R.fog *= .7; R.air.push({t:'blob', c:'#dde2e6', k:.9, sx:.03, sy:0, v:[-.1, .3], d:1}); }
      if (like === 'dust-devil') R.funnel = {c:'#c9a878', k:.55, w:.32};
      spec._R = R; spec._Rkey = JSON.stringify(spec.look || null) + spec.like;
      return R;
    };
    /* Two recipes part of the way from one to the other: numbers ease across, layers fade out and in. */
    L.blend = function(A, B, t){
      if (!A) return B; if (!B) return A;
      if (t <= 0) return A; if (t >= 1) return B;
      var R = blank(), u = 1 - t;
      L.DIAL_NAMES.forEach(function(n){ R[n] = A[n] * u + B[n] * t; });
      R.sky = A.sky && B.sky ? [L.linHex(PAL.mixLin(PAL.hexLin(A.sky[0]), PAL.hexLin(B.sky[0]), t)), A.sky[1] * u + B.sky[1] * t] : A.sky ? [A.sky[0], A.sky[1] * u] : B.sky ? [B.sky[0], B.sky[1] * t] : null;
      R.fogTint = A.fogTint && B.fogTint ? [B.fogTint[0], A.fogTint[1] * u + B.fogTint[1] * t] : A.fogTint ? [A.fogTint[0], A.fogTint[1] * u] : B.fogTint ? [B.fogTint[0], B.fogTint[1] * t] : null;
      R.rainTint = (A.rainTint && B.rainTint) ? [t < .5 ? A.rainTint[0] : B.rainTint[0], A.rainTint[1] * u + B.rainTint[1] * t] : A.rainTint ? [A.rainTint[0], A.rainTint[1] * u] : B.rainTint ? [B.rainTint[0], B.rainTint[1] * t] : null;
      function fade(list, k){ return list.map(function(x){ var o = Object.assign({}, x); if (o.k != null) o.k *= k; if (o.a != null) o.a *= k; if (o.rate != null) o.rate *= k; return o; }); }
      ['aurora', 'air', 'parts', 'sigils', 'fire'].forEach(function(n){ R[n] = fade(A[n], u).concat(fade(B[n], t)); });
      R.funnel = A.funnel && B.funnel ? Object.assign({}, B.funnel, {k:A.funnel.k * u + B.funnel.k * t}) : A.funnel ? Object.assign({}, A.funnel, {k:A.funnel.k * u}) : B.funnel ? Object.assign({}, B.funnel, {k:B.funnel.k * t}) : null;
      R.flash = {lightning:A.flash.lightning * u + B.flash.lightning * t, ley:A.flash.ley * u + B.flash.ley * t};
      R.sun = {dim:A.sun.dim * u + B.sun.dim * t, 'void':A.sun['void'] * u + B.sun['void'] * t, tint:t < .5 ? A.sun.tint : B.sun.tint};
      R.effects = A.effects.concat(B.effects);
      return R;
    };
    /* A forecast is drawn as the weather it expects, softened by how unsure it is. */
    L.soften = function(R, conf){
      var k = clamp(conf, 0, 1), o = L.blend(blank(), R, .35 + .65 * k);
      o.cloud = R.cloud * (.5 + .5 * k);
      return o;
    };
  })(LOOKS);

  /* ── Effects drawn on the page, for the painter to lay into the sky: particles near the viewer (in front of the
     hills), and the sky's own drawn shapes (behind clouds and hills). Every particle is a function of the one shared
     clock, so any moment can be drawn again exactly, and a still is just one moment. ── */
  var SKYFX = (function(){
    var TAU = Math.PI * 2, F = {};
    /* The small set of tempos every effect moves to, in seconds. */
    var TEMPO = {breath:7.2, sway:3.6, flick:1.3, shimmer:2.4, drift:26};
    F.TEMPO = TEMPO;
    /* Particle counts are capped per sky, whatever an owner asks for. */
    F.CAP = 170;
    function h(i, k){ var x = Math.sin(i * 127.1 + k * 311.7 + 17.3) * 43758.5453; return x - Math.floor(x); }
    F.hash = h;
    function wrap(v, n){ return ((v % n) + n) % n; }

    /* The scene's light, for colouring what is drawn: its level, its colour (normalised), how dark, how foggy, and the
       brightness nothing may pass (the brightest moon up by night; the sun, far brighter, by day). */
    F.sceneLight = function(st, P){
      var dark = st.dark, wx = st.wx, clit = P && P.clit ? P.clit : [1, 1, 1], y = LOOKS.lum(clit) || 1;
      var col = clit.map(function(v){ return clamp(v / y, .35, 1.8); });
      var moon = 0; st.moons.forEach(function(M){ if (M.up && M.m.alt > 0) moon = Math.max(moon, M.m.lit * (M.mo.size || 1) * smooth01(0, 10, M.m.alt / D2R)); });
      var amb = (1 - dark) * (1 - wx.cloud * .3 - wx.dark * .45) + dark * (.07 + .2 * moon) * (1 - wx.cloud * .5);
      var cap = dark > .5 ? .22 + .5 * moon : 1;
      return {amb:clamp(amb, .05, 1.1), col:col, night:dark, fog:wx.fog, cap:cap, moon:moon};
    };

    /* One particle at time t. Positions wrap through the sky with a margin, so none pops in at an edge. */
    function particle(p, i, t, W, H, sc, wind, seed){
      var r1 = h(i, seed), r2 = h(i, seed + 1.3), r3 = h(i, seed + 2.7), r4 = h(i, seed + 4.1), r5 = h(i, seed + 5.9);
      var z = .35 + .65 * r1, sp = .5 + .5 * z, m = 24 * sc, Wm = W + 2 * m, Hm = H + 2 * m;
      var vx = p.v[0] * sc * sp, vy = p.v[1] * sc * sp, wob = (p.wob || 0) * sc * z, per = TEMPO.sway * (.8 + .45 * r4);
      var o = {z:z, a:1, rot:r5 * TAU, flip:1, x:0, y:0};
      if (p.mode === 'blink'){
        var per2 = 1.8 + r4 * 1.6, cyc = Math.floor((t + r5 * per2) / per2), u = (t + r5 * per2) / per2 - cyc;
        o.x = h(i * 7 + cyc, seed) * W; o.y = h(i * 11 + cyc, seed + 3) * H * .92;
        o.a = Math.pow(Math.sin(Math.PI * smooth01(0, .45, u)), 2);
        return o;
      }
      if (p.mode === 'spark'){
        var per3 = 1.3 + r4 * 1.3, c2 = Math.floor((t + r5 * per3) / per3), u2 = (t + r5 * per3) / per3 - c2;
        o.x = h(i * 13 + c2, seed + 1) * W; o.y = h(i * 17 + c2, seed + 2) * H * .85;
        o.x += (h(i, c2 + .5) - .5) * p.v[0] * sc * 2 * u2; o.y += p.v[1] * sc * u2 * 1.4;
        o.a = smooth01(0, .22, u2) * (1 - smooth01(.35, .95, u2));
        o.dir = (h(i, c2 + .9) - .5) * 1.2;
        return o;
      }
      if (p.mode === 'vortex'){
        var ang = r2 * TAU + t * (1.6 + 1.4 * r3) * (r1 > .5 ? 1 : .8), life = wrap(t * (.12 + .1 * r4) + r5, 1);
        var hgt = life * (.25 + .5 * r3), rad = (8 + 26 * hgt + 10 * r4) * sc;
        o.x = (p.cx != null ? p.cx : W * .5) + Math.cos(ang) * rad; o.y = (p.gy != null ? p.gy : H * .86) - hgt * H * .55 - 2 * sc;
        o.a = Math.sin(Math.PI * life) * (Math.sin(ang) > 0 ? 1 : .55); o.rot = ang * 2;
        return o;
      }
      var x0 = r2 * Wm, y0 = r3 * Hm, drift = p.mode === 'fall' || p.mode === 'blow' ? wind * 22 * sc * sp : 0;
      o.x = wrap(x0 + (vx + drift) * t + wob * Math.sin(t * TAU / per + r4 * TAU), Wm) - m;
      o.y = wrap(y0 + vy * t + (p.mode === 'drift' ? wob * .6 * Math.cos(t * TAU / (per * 1.3) + r5 * TAU) : p.mode === 'blow' ? wob * .3 * Math.sin(t * 3 + r4 * 9) : 0), Hm) - m;
      if (p.spin){ o.rot = r5 * TAU + t * p.spin * (r4 > .5 ? 1 : -1) * (.6 + .6 * r3); o.flip = Math.cos(t * p.spin * 1.7 + r2 * TAU); }
      /* Rising things kindle near the ground and burn out as they climb. */
      if (p.mode === 'rise') o.a = smooth01(-m, H * .12, H - o.y) * (1 - smooth01(H * .55, H * .95, H - o.y));
      if (p.fl) o.a *= 1 - p.fl * .35 * (.5 + .5 * Math.sin(t * TAU / (TEMPO.flick * (.8 + .5 * r3)) + r4 * TAU));
      if (p.pulse) o.a *= .7 + .3 * Math.sin(t * TAU / TEMPO.breath + r2 * TAU);
      return o;
    }
    F.particle = particle;

    /* Sprites. Each is drawn in place at its size; glows are soft gradients, bodies are small shaded shapes. */
    function glow(ctx, x, y, r, cc, ce, a){
      var g = ctx.createRadialGradient(x, y, 0, x, y, r * 2.6);
      g.addColorStop(0, LOOKS.srgb(ce, a)); g.addColorStop(.28, LOOKS.srgb(cc, a * .75)); g.addColorStop(1, LOOKS.srgb(cc, 0));
      ctx.fillStyle = g; ctx.beginPath(); ctx.arc(x, y, r * 2.6, 0, TAU); ctx.fill();
    }
    var DRAW = {
      mote:function(ctx, o, r, A, B){ glow(ctx, o.x, o.y, r, A, B, o.a); },
      spore:function(ctx, o, r, A, B){ glow(ctx, o.x, o.y, r * .8, A, B, o.a * .8); },
      ember:function(ctx, o, r, A, B, p){
        var tl = r * 3.2, dx = -(p.v[0] || 0), dy = -(p.v[1] || -1), dl = Math.hypot(dx, dy) || 1;
        var g = ctx.createLinearGradient(o.x, o.y, o.x + dx / dl * tl, o.y + dy / dl * tl);
        g.addColorStop(0, LOOKS.srgb(A, o.a * .7)); g.addColorStop(1, LOOKS.srgb(A, 0));
        ctx.strokeStyle = g; ctx.lineWidth = r * .9; ctx.lineCap = 'round'; ctx.beginPath(); ctx.moveTo(o.x, o.y); ctx.lineTo(o.x + dx / dl * tl, o.y + dy / dl * tl); ctx.stroke();
        glow(ctx, o.x, o.y, r * .75, A, B, o.a);
      },
      spark:function(ctx, o, r, A, B){
        glow(ctx, o.x, o.y, r * .7, A, B, o.a);
        ctx.strokeStyle = LOOKS.srgb(B, o.a * .8); ctx.lineWidth = Math.max(.6, r * .35); ctx.lineCap = 'round';
        var l = r * 2.4 * (.6 + .4 * o.a), c = Math.cos(o.dir || 0), s = Math.sin(o.dir || 0);
        ctx.beginPath(); ctx.moveTo(o.x - c * l, o.y - s * l); ctx.lineTo(o.x + c * l, o.y + s * l); ctx.moveTo(o.x + s * l * .6, o.y - c * l * .6); ctx.lineTo(o.x - s * l * .6, o.y + c * l * .6); ctx.stroke();
      },
      petal:function(ctx, o, r, A, B){
        ctx.save(); ctx.translate(o.x, o.y); ctx.rotate(o.rot); ctx.scale(Math.max(.18, Math.abs(o.flip)), 1);
        var g = ctx.createLinearGradient(0, -r, 0, r); g.addColorStop(0, LOOKS.srgb(B, o.a)); g.addColorStop(1, LOOKS.srgb(A, o.a));
        ctx.fillStyle = g; ctx.beginPath(); ctx.moveTo(0, -r); ctx.quadraticCurveTo(r * .95, -r * .1, 0, r); ctx.quadraticCurveTo(-r * .95, -r * .1, 0, -r); ctx.fill();
        ctx.restore();
      },
      leaf:function(ctx, o, r, A, B, p, i){
        var c = h(i, 9.1) < .5 ? A : B;
        ctx.save(); ctx.translate(o.x, o.y); ctx.rotate(o.rot); ctx.scale(Math.max(.15, Math.abs(o.flip)), 1);
        ctx.fillStyle = LOOKS.srgb(c, o.a); ctx.beginPath(); ctx.moveTo(0, -r * 1.2); ctx.quadraticCurveTo(r * .9, -r * .2, 0, r); ctx.quadraticCurveTo(-r * .9, -r * .2, 0, -r * 1.2); ctx.fill();
        ctx.strokeStyle = LOOKS.srgb(c.map(function(v){ return v * .55; }), o.a * .8); ctx.lineWidth = Math.max(.5, r * .12); ctx.beginPath(); ctx.moveTo(0, -r * .9); ctx.lineTo(0, r * 1.25); ctx.stroke();
        ctx.restore();
      },
      ash:function(ctx, o, r, A, B, p, i){
        ctx.save(); ctx.translate(o.x, o.y); ctx.rotate(o.rot); ctx.scale(Math.max(.25, Math.abs(o.flip)), 1);
        ctx.fillStyle = LOOKS.srgb(h(i, 3.3) < .5 ? A : B, o.a * .85); ctx.beginPath();
        for (var k = 0; k < 5; k++){ var an = k / 5 * TAU, rr = r * (.55 + .45 * h(i, k + 1.7)); ctx[k ? 'lineTo' : 'moveTo'](Math.cos(an) * rr, Math.sin(an) * rr); }
        ctx.closePath(); ctx.fill(); ctx.restore();
      },
      shard:function(ctx, o, r, A, B){
        ctx.save(); ctx.translate(o.x, o.y); ctx.rotate(o.rot);
        ctx.fillStyle = LOOKS.srgb(A, o.a * .75); ctx.beginPath(); ctx.moveTo(0, -r * 1.6); ctx.lineTo(r * .38, 0); ctx.lineTo(0, r * 1.6); ctx.lineTo(-r * .38, 0); ctx.closePath(); ctx.fill();
        /* A glint as a facet turns toward the light: smooth, never a flash. */
        var gl = Math.pow(Math.max(0, Math.cos(o.rot * 2.3)), 14);
        if (gl > .02){ ctx.strokeStyle = LOOKS.srgb(B, o.a * gl); ctx.lineWidth = Math.max(.5, r * .3); ctx.beginPath(); ctx.moveTo(0, -r * 1.5); ctx.lineTo(0, r * 1.5); ctx.stroke(); }
        ctx.restore();
      },
      grain:function(ctx, o, r, A, B, p){
        var l = r * 3.5, s = Math.atan2(p.v[1], p.v[0]);
        ctx.strokeStyle = LOOKS.srgb(o.z > .7 ? B : A, o.a * .7); ctx.lineWidth = Math.max(.5, r * .55);
        ctx.beginPath(); ctx.moveTo(o.x, o.y); ctx.lineTo(o.x - Math.cos(s) * l, o.y - Math.sin(s) * l); ctx.stroke();
      },
      debris:function(ctx, o, r, A, B, p, i){ DRAW.ash(ctx, o, r, A, B, p, i + 40); },
      pixel:function(ctx, o, r, A, B, p, i){ ctx.fillStyle = LOOKS.srgb(h(i, 2.2) < .6 ? A : B, o.a); ctx.fillRect(Math.round(o.x), Math.round(o.y), Math.max(1, Math.round(r)), Math.max(1, Math.round(r))); },
      /* A wraith: a faint shape of mist that drifts and coils, palest and widest where it has been and lifting as it
         goes, like breath in cold air; it has no bright head, so it never reads as a comet. */
      wraith:function(ctx, o, r, A, B, p, i, t, W, H, sc, wind, seed){
        for (var k = 7; k >= 0; k--){
          var q = swirl(p, i, t - k * .35, W, H, sc, seed), rr = r * (.55 + k * .16), al = o.a * .27 * (1 - k / 9), y = q.y - k * r * .22;
          var g = ctx.createRadialGradient(q.x, y, 0, q.x, y, rr);
          g.addColorStop(0, LOOKS.srgb(B, al)); g.addColorStop(.5, LOOKS.srgb(A, al * .55)); g.addColorStop(1, LOOKS.srgb(A, 0));
          ctx.fillStyle = g; ctx.beginPath(); ctx.ellipse(q.x, y, rr * .7, rr, 0, 0, TAU); ctx.fill();
        }
      }
    };
    function swirl(p, i, t, W, H, sc, seed){
      var r2 = h(i, seed + 1.3), r3 = h(i, seed + 2.7), r4 = h(i, seed + 4.1), R = (18 + 30 * r4) * sc, w = .35 + .25 * r3;
      return {x:wrap(r2 * (W + 80) + p.v[0] * sc * t + R * Math.sin(t * w + r3 * TAU), W + 80) - 40, y:H * (.25 + .5 * r3) + R * .45 * Math.cos(t * w * 1.3 + r2 * TAU)};
    }
    /* A rainbow colour for one particle: the hue turns slowly; it is placed in the glow band like any other colour. */
    function rainbow(i, t, sl){ var hh = (h(i, 6.6) + t * .05) % 1; return LOOKS.tame(LOOKS.linHex(LOOKS.okLin(.8, .14, hh * TAU)), 'glow', sl.night); }

    /* The near layer: every particle set of the weather, at time t, onto the surface's own canvas. */
    F.near = function(s, st, dpr){
      var R = st.look, sets = R ? R.parts : [];
      if (!sets.length) return false;
      var cv = s.nearCv || (s.nearCv = document.createElement('canvas')), w = Math.max(1, Math.round(st.W * dpr)), hh = Math.max(1, Math.round(st.H * dpr));
      if (cv.width !== w || cv.height !== hh){ cv.width = w; cv.height = hh; }
      /* Small skies keep enough particles, large enough, to read: a band or a thumbnail is not an empty sky. */
      var ctx = cv.getContext('2d'), sl = st.sl, W = st.W, H = st.H, sc = clamp(H / 150, .65, 1.6), t = st.t, wind = st.wx.wind;
      ctx.setTransform(1, 0, 0, 1, 0, 0); ctx.clearRect(0, 0, w, hh); ctx.setTransform(w / W, 0, 0, hh / H, 0, 0);
      var area = clamp(W * H / (1000 * 150), .5, 1.35), left = F.CAP, fogA = 1 - .45 * sl.fog, n = 0;
      sets.forEach(function(p, si){
        var cnt = Math.min(left, Math.round(p.n * area)); left -= cnt;
        if (cnt <= 0 || p.a <= .01) return;
        var e = p.e || 0, role = e >= .5 ? 'glow' : 'body';
        var A = LOOKS.lit(p.c, role, sl), B = LOOKS.lit(p.c2 || p.c, role, sl);
        if (p.mode === 'vortex' && st.funnelAt){ p = Object.assign({}, p, {cx:st.funnelAt.x, gy:st.funnelAt.gy}); }
        var seed = 3.1 + si * 17.7 + (p.fx ? p.fx.length * 1.9 : 0), draw = DRAW[p.p] || DRAW.mote;
        for (var i = 0; i < cnt; i++){
          var o = particle(p, i, t, W, H, sc, wind, seed);
          o.a *= p.a * fogA * (.55 + .45 * o.z);
          if (o.a <= .01 || o.x < -30 || o.x > W + 30 || o.y < -30 || o.y > H + 30) continue;
          var r = (p.s[0] + (p.s[1] - p.s[0]) * h(i, seed + 8.8)) * sc * (.55 + .45 * o.z) * (1 + .3 * sl.fog);
          var a = A, b = B;
          if (p.rb){ a = rainbow(i, t, sl); b = a.map(function(v){ return Math.min(1, v * 1.25 + .05); }); }
          draw(ctx, o, r, a, b, p, i, t, W, H, sc, wind, seed); n++;
        }
      });
      return n > 0;
    };
    return F;
  })();

  /* ── Shapes drawn into the sky itself (behind clouds and hills): the owner's sky shapes that are drawn rather than
     painted, sigils, a moon's halo ring, the glow where a fallen star came down. Each is placed by its compass point and
     height, sized by its size, lit by the scene, and moves only to the shared tempos. ── */
  (function(F){
    var TAU = Math.PI * 2, h = F.hash, TEMPO = F.TEMPO;
    var SIZE = {small:.62, medium:1, large:1.45};
    F.SIZE = SIZE;
    function soft(ctx, x, y, rx, ry, c, a){
      ctx.save(); ctx.translate(x, y); ctx.scale(1, ry / rx);
      var g = ctx.createRadialGradient(0, 0, 0, 0, 0, rx);
      g.addColorStop(0, LOOKS.srgb(c, a)); g.addColorStop(.35, LOOKS.srgb(c, a * .45)); g.addColorStop(1, LOOKS.srgb(c, 0));
      ctx.fillStyle = g; ctx.beginPath(); ctx.arc(0, 0, rx, 0, TAU); ctx.fill(); ctx.restore();
    }
    F.soft = soft;
    var SHAPES = {
      /* A ribbon: a long, gently curving band of light, soft at the edges and fading at both ends; a slow shimmer runs
         along it. */
      streak:function(ctx, it, st, t){
        var W = st.W, H = st.H, len = W * .34 * it.sz, x0 = it.x - len / 2, tilt = (h(it.seed, 1) - .5) * .5, A = it.col, B = it.hi, N = 40;
        var pts = [];
        for (var i = 0; i <= N; i++){ var u = i / N, x = x0 + u * len, y = it.y + (u - .5) * len * tilt * .35 + Math.sin(u * 3.2 + h(it.seed, 2) * 6) * H * .045 * it.sz + Math.sin(t * TAU / TEMPO.drift + u * 4) * H * .008; pts.push([x, y, u]); }
        [[H * .075 * it.sz, .1], [H * .026 * it.sz, .26], [Math.max(1, H * .007), .75]].forEach(function(L2, k){
          for (var j = 0; j < N; j++){
            var u = pts[j][2], ends = Math.sin(Math.PI * u), sh = .75 + .25 * Math.sin(u * 9 - t * TAU / TEMPO.shimmer);
            ctx.strokeStyle = LOOKS.srgb(k === 2 ? B : A, L2[1] * it.k * Math.pow(ends, 1.4) * (k === 2 ? sh : 1)); ctx.lineWidth = L2[0]; ctx.lineCap = 'round';
            ctx.beginPath(); ctx.moveTo(pts[j][0], pts[j][1]); ctx.lineTo(pts[j + 1][0], pts[j + 1][1]); ctx.stroke();
          }
        });
      },
      /* A glow: light from something out of sight, pooled on the horizon or hanging in the air; it breathes slowly. */
      glow:function(ctx, it, st, t){
        var H = st.H, br = .92 + .08 * Math.sin(t * TAU / TEMPO.breath + it.seed), r = H * .42 * it.sz;
        soft(ctx, it.x, it.y, r * 1.8, r * .8, it.col, .32 * it.k * br);
        soft(ctx, it.x, it.y, r * .6, r * .32, it.hi, .38 * it.k * br);
      },
      /* A ring of pale light, like a halo with nothing at its centre; a brighter arc turns round it very slowly. */
      ring:function(ctx, it, st, t){
        var H = st.H, R = H * .19 * it.sz, a0 = t * TAU / (TEMPO.drift * 1.6) + it.seed;
        for (var k = 0; k < 3; k++){
          ctx.strokeStyle = LOOKS.srgb(it.col, [.06, .16, .45][k] * it.k); ctx.lineWidth = [R * .28, R * .1, Math.max(.9, R * .025)][k];
          ctx.beginPath(); ctx.arc(it.x, it.y, R, 0, TAU); ctx.stroke();
        }
        ctx.strokeStyle = LOOKS.srgb(it.hi, .35 * it.k); ctx.lineWidth = Math.max(1, R * .05); ctx.lineCap = 'round';
        ctx.beginPath(); ctx.arc(it.x, it.y, R, a0, a0 + .9); ctx.stroke();
      },
      /* Wandering stars: points of light brighter than their neighbours, with soft rays, drifting across the sky over
         the event's hours as no star does. A small one is alone; larger ones travel in a loose line. */
      star:function(ctx, it, st, t){
        var H = st.H, n = it.size === 'large' ? 5 : it.size === 'small' ? 1 : 3, hrs = Math.max(.001, it.elapsed);
        for (var k = 0; k < n; k++){
          var ang = (h(it.seed, k + 3) - .5) * .8 - .35, sp = H * (.09 + .05 * h(it.seed, k + 7));
          var x = it.x + (k - (n - 1) / 2) * H * .16 + Math.cos(ang) * sp * hrs + Math.sin(t * TAU / TEMPO.drift + k) * H * .012;
          var y = it.y + (h(it.seed, k + 11) - .5) * H * .12 + Math.sin(ang) * sp * hrs * .5 + Math.cos(t * TAU / (TEMPO.drift * 1.3) + k) * H * .008;
          var r = Math.max(1.1, H * .012) * (k === 0 ? 1.15 : .9), tw = .85 + .15 * Math.sin(t * TAU / TEMPO.shimmer + k * 2.1);
          soft(ctx, x, y, r * 7, r * 7, it.col, .28 * it.k * tw);
          ctx.strokeStyle = LOOKS.srgb(it.hi, .45 * it.k * tw); ctx.lineWidth = Math.max(.6, r * .28); ctx.lineCap = 'round';
          var L2 = r * 5.5 * tw; ctx.beginPath(); ctx.moveTo(x - L2, y); ctx.lineTo(x + L2, y); ctx.moveTo(x, y - L2); ctx.lineTo(x, y + L2); ctx.stroke();
          soft(ctx, x, y, r * 1.6, r * 1.6, it.hi, .95 * it.k);
        }
      },
      /* A door of light standing in the sky: a tall arch, brightest at its threshold, spilling light down; it opens from
         a line of light when it first appears. */
      doorway:function(ctx, it, st, t){
        var H = st.H, dh = H * .34 * it.sz, dw = dh * .42 * clamp(it.open, .04, 1), x = it.x, y = it.y + dh * .5, top = y - dh;
        var br = .94 + .06 * Math.sin(t * TAU / TEMPO.breath + it.seed);
        soft(ctx, x, y - dh * .4, dh * .75, dh * .9, it.col, .22 * it.k * br);
        ctx.save();
        ctx.beginPath(); ctx.moveTo(x - dw / 2, y); ctx.lineTo(x - dw / 2, top + dw / 2); ctx.arc(x, top + dw / 2, dw / 2, Math.PI, 0); ctx.lineTo(x + dw / 2, y); ctx.closePath();
        var g = ctx.createLinearGradient(0, top, 0, y); g.addColorStop(0, LOOKS.srgb(it.col, .5 * it.k * br)); g.addColorStop(1, LOOKS.srgb(it.hi, .92 * it.k * br));
        ctx.fillStyle = g; ctx.fill();
        ctx.strokeStyle = LOOKS.srgb(it.hi, .75 * it.k); ctx.lineWidth = Math.max(.8, H * .005); ctx.stroke();
        ctx.restore();
        /* Light spilling from the threshold, down and outward. */
        var sp = ctx.createLinearGradient(0, y, 0, y + dh * .45); sp.addColorStop(0, LOOKS.srgb(it.hi, .3 * it.k * it.open)); sp.addColorStop(1, LOOKS.srgb(it.col, 0));
        ctx.fillStyle = sp; ctx.beginPath(); ctx.moveTo(x - dw / 2, y); ctx.lineTo(x + dw / 2, y); ctx.lineTo(x + dw * 1.3, y + dh * .45); ctx.lineTo(x - dw * 1.3, y + dh * .45); ctx.closePath(); ctx.fill();
      }
    };
    F.SHAPES = SHAPES;

    /* Sigils: faint glyphs that come and go across the sky, a few at a time; most are barely there. */
    function sigils(ctx, st, set, t){
      var W = st.W, H = st.H, slots = Math.round(clamp(W * H / 16000, 4, 10)), col = LOOKS.lit(set.c, 'glow', st.sl);
      for (var s = 0; s < slots; s++){
        var per = 5.5 + 3.5 * h(s, 1.1), c = Math.floor((t + h(s, 2.2) * per) / per), u = (t + h(s, 2.2) * per) / per - c;
        var env = Math.sin(Math.PI * smooth01(0, 1, u)), peak = h(s * 5 + c, 3.3) < .25 ? .85 : .5;
        var x = h(s * 7 + c, 4.4) * W, y = H * (.08 + .5 * h(s * 11 + c, 5.5)), sz = H * (.12 + .07 * h(s + c, 6.6));
        var path = sigil(Math.floor(h(s * 3 + c, 7.7) * 12));
        ctx.save(); ctx.translate(x, y - u * H * .03); ctx.rotate((h(s + c, 8.8) - .5) * .5); var k = sz / 10; ctx.scale(k, k); ctx.translate(-5, -5);
        ctx.lineCap = 'round'; ctx.lineJoin = 'round';
        /* A soft glow under a fine line, so a sigil reads as light at any size. */
        ctx.lineWidth = 3.6 / k; ctx.strokeStyle = LOOKS.srgb(col, env * peak * set.k * .3); ctx.stroke(path);
        ctx.lineWidth = 1.4 / k; ctx.strokeStyle = LOOKS.srgb(col, env * peak * set.k); ctx.stroke(path);
        ctx.restore();
      }
    }

    /* The far layer: everything on the page's list for this moment, onto the sky's effect canvas (after the moons). */
    F.far = function(ctx, st){
      var t = st.t, n = 0;
      (st.fxItems || []).forEach(function(it){
        if (it.k <= .004) return;
        if (it.shape === 'halo'){ haloRing(ctx, it, st); n++; return; }
        if (it.shape === 'landing'){ F.soft(ctx, it.x, it.y, st.H * .35, st.H * .16, it.col, .5 * it.k); F.soft(ctx, it.x, it.y, st.H * .1, st.H * .05, it.hi, .7 * it.k); n++; return; }
        if (it.shape === 'warm'){ F.soft(ctx, it.x, it.y, it.r * 4.2, it.r * 3.2, it.col, .3 * it.k); n++; return; }
        if (it.shape === 'embers'){ it.pts.forEach(function(q){ F.soft(ctx, q[0], q[1], q[2] * 2.4, q[2] * 2.4, it.col, q[3]); }); n++; return; }
        var fn = SHAPES[it.shape]; if (fn){ fn(ctx, it, st, t); n++; }
      });
      ((st.look && st.look.sigils) || []).forEach(function(set){ if (set.k > .01){ sigils(ctx, st, set, t); n++; } });
      return n > 0;
    };
    /* A moon's halo ring, 22 degrees out: a faint ring of light, a little bluer on the inside. */
    function haloRing(ctx, it, st){
      var R = it.r;
      [[R * .16, .05], [R * .06, .12]].forEach(function(q){ ctx.strokeStyle = LOOKS.srgb(it.col, q[1] * it.k); ctx.lineWidth = q[0]; ctx.beginPath(); ctx.arc(it.x, it.y, R, 0, TAU); ctx.stroke(); });
      ctx.strokeStyle = LOOKS.srgb(it.hi, .1 * it.k); ctx.lineWidth = R * .03; ctx.beginPath(); ctx.arc(it.x, it.y, R * .97, 0, TAU); ctx.stroke();
    }
  })(SKYFX);

  window.SkyLooks = LOOKS;
  window.SkyFX = SKYFX;
  SKYFX.sigilPath = sigil;
})();
