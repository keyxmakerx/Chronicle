/*
 * sky_looks.js — how weather looks in the sky (window.SkyLooks, window.SkyFX).
 *
 * Ported from the signed sky pane mockup's sky-looks.js (#763, #842). Every
 * weather resolves into one recipe from the effect vocabulary: the painted
 * sky's physics (cloud, darkness, fog, rain, snow, hail, storm, wind), a sky
 * tint, air layers, near particles, drawn shapes, aurora, fireballs, sigils,
 * a funnel and lightning. The WebGL painter (sky_gl.js) reads all of it; the basic 2D sky
 * (sky_2d.js) reads the physics and tint.
 *
 * A weather row's preset_id picks its built-in. A campaign's own preset
 * draws like the built-in its icon and precipitation are closest to.
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
     Chronicle's own; each is a hand-tuned building block (a state for the painted sky, layers of air, particles
     near the viewer), so an owner chooses blocks and the renderer owns the art. ── */
  var LOOKS = (function(){
    var L = {};
    /* Groups, in the order the look editor shows them. */
    L.GROUPS = [
      ['Clouds and sky', ['clear', 'clouds-light', 'clouds-heavy', 'clouds-overcast', 'fog', 'haze', 'aurora', 'meteors',
        'mist-low', 'gloom', 'sunbeams', 'red-sky', 'rainbow', 'sun-halo', 'night-clouds', 'frost', 'diamond-dust', 'mammatus', 'lenticular']],
      ['Rain and snow', ['rain', 'rain-heavy', 'lightning', 'sleet', 'hail', 'snow', 'snow-heavy', 'ice', 'rain-acid', 'rain-blood',
        'flurries', 'freezing-rain', 'thundersnow', 'dry-lightning', 'rain-gold', 'rain-ink', 'snow-glow', 'cinders', 'star-rain', 'shardfall', 'st-elmo', 'ball-lightning']],
      ['Wind and dust', ['gust', 'sand', 'tornado', 'hurricane', 'leaves', 'petals', 'spores', 'sea-spray', 'waterspout', 'dust-wall']],
      ['Fire and smoke', ['embers', 'ashfall', 'smoke', 'sparks', 'firestorm', 'horizon-fire', 'brimstone', 'smog', 'witchfire', 'lanterns', 'volcano']],
      ['Living things', ['fireflies', 'wings', 'swarm', 'feathers', 'pollen', 'thistledown', 'gossamer', 'leviathans']],
      ['Magic', ['aether', 'arcane', 'arcane-wind', 'ley-surge', 'nullstatic', 'spectral', 'void', 'veil', 'divine', 'miasma', 'motes', 'sigils',
        'wisps', 'dreamfog', 'shadow', 'rift', 'bubbles', 'fey-lights']]
    ];
    L.VOCAB = [];
    L.GROUPS.forEach(function(g){ g[1].forEach(function(id){ L.VOCAB.push(id); }); });
    /* The effect each built-in weather draws with: Calendaria's own for its 42, ours for the rest. */
    L.PRESET_EFFECT = {
      'clear':'clear', 'partly-cloudy':'clouds-light', 'cloudy':'clouds-heavy', 'overcast':'clouds-overcast', 'drizzle':'rain', 'rain':'rain', 'fog':'fog', 'mist':'fog',
      'windy':'gust', 'sunshower':'rain', 'snow':'snow', 'sleet':'sleet', 'heat-wave':'haze', 'thunderstorm':'lightning', 'blizzard':'snow-heavy', 'hail':'hail',
      'tornado':'tornado', 'hurricane':'hurricane', 'ice-storm':'ice', 'monsoon':'rain-heavy', 'ashfall':'ashfall', 'sandstorm':'sand', 'luminous-sky':'aurora',
      'sakura-bloom':'petals', 'autumn-leaves':'leaves', 'rolling-fog':'fog', 'wildfire-smoke':'smoke', 'dust-devil':'sand', 'black-sun':'void', 'ley-surge':'ley-surge',
      'aether-haze':'aether', 'nullfront':'nullstatic', 'permafrost-surge':'ice', 'gravewind':'spectral', 'veilfall':'veil', 'arcane-winds':'arcane-wind',
      'acid-rain':'rain-acid', 'blood-rain':'rain-blood', 'meteor-shower':'meteors', 'spore-cloud':'spores', 'divine-light':'divine', 'plague-miasma':'miasma',
      'showers':'rain', 'flurries':'flurries', 'hard-frost':'frost', 'ground-mist':'mist-low', 'gloom':'gloom', 'sunbeams':'sunbeams', 'bitter-cold':'diamond-dust',
      'thaw':'mist-low', 'gale':'gust', 'squall':'hurricane', 'dry-lightning':'dry-lightning', 'thundersnow':'thundersnow', 'freezing-rain':'freezing-rain',
      'waterspout':'waterspout', 'firestorm':'firestorm', 'dust-wall':'dust-wall', 'rainbow':'rainbow', 'red-sky':'red-sky', 'sun-halo':'sun-halo',
      'night-clouds':'night-clouds', 'firefly-dusk':'fireflies', 'pollen-haze':'pollen', 'thistledown':'thistledown', 'gossamer':'gossamer', 'sea-spray':'sea-spray',
      'locust-swarm':'swarm', 'smog':'smog', 'butterfly-days':'wings', 'wisp-lights':'wisps', 'dreamfog':'dreamfog', 'shadowfall':'shadow', 'sky-rift':'rift',
      'golden-rain':'rain-gold', 'ink-rain':'rain-ink', 'glass-rain':'shardfall', 'cinder-rain':'cinders', 'star-rain':'star-rain', 'glimmersnow':'snow-glow',
      'witchfire':'witchfire', 'brimstone':'brimstone', 'featherfall':'feathers', 'fey-lights':'fey-lights',
      'mammatus-sky':'mammatus', 'lenticular':'lenticular', 'st-elmos-fire':'st-elmo', 'ball-lightning':'ball-lightning', 'eruption':'volcano', 'leviathans':'leviathans'
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
      'divine-light':[.1, 0, .3, null, 0], 'plague-miasma':[.7, .4, .1, null, 0],
      'showers':[.6, .5, .35, 'rain', .4], 'flurries':[.6, .3, .25, 'snow', .2], 'hard-frost':[.05, 0, .05, null, 0], 'ground-mist':[.2, .15, 0, null, 0],
      'gloom':[1, .15, .1, null, 0], 'sunbeams':[.6, .05, .2, null, 0], 'bitter-cold':[.05, 0, .1, null, 0], 'thaw':[.55, .35, .1, 'drizzle', .1],
      'gale':[.5, .05, .9, null, 0], 'squall':[.95, .8, .95, 'rain', .75], 'dry-lightning':[.8, 0, .85, null, 0], 'thundersnow':[1, .75, .9, 'snow', .8],
      'freezing-rain':[.95, .65, .35, 'rain', .45], 'waterspout':[1, .7, .9, 'rain', .5], 'firestorm':[.8, 0, 1, null, 0], 'dust-wall':[.75, 0, 1, null, 0],
      'rainbow':[.4, .2, .15, 'rain', .1], 'red-sky':[.4, 0, .1, null, 0], 'sun-halo':[.25, 0, .1, null, 0], 'night-clouds':[.05, 0, .05, null, 0],
      'firefly-dusk':[.1, 0, .05, null, 0], 'pollen-haze':[.15, 0, .15, null, 0], 'thistledown':[.2, 0, .3, null, 0], 'gossamer':[.1, 0, .1, null, 0],
      'sea-spray':[.5, .3, .8, null, 0], 'locust-swarm':[.35, 0, .5, null, 0], 'smog':[.5, .05, 0, null, 0], 'butterfly-days':[.2, 0, .15, null, 0],
      'wisp-lights':[.3, .2, 0, null, 0], 'dreamfog':[.4, .3, .05, null, 0], 'shadowfall':[.5, 0, .3, null, 0], 'sky-rift':[.3, 0, .5, null, 0],
      'golden-rain':[.6, .5, .2, 'rain', .4], 'ink-rain':[1, .8, .4, 'rain', .65], 'glass-rain':[.85, .4, .6, 'hail', .4], 'cinder-rain':[.85, 0, .5, null, 0],
      'star-rain':[.05, 0, .2, null, 0], 'glimmersnow':[.8, .5, .2, 'snow', .45], 'witchfire':[.3, 0, .2, null, 0], 'brimstone':[.6, .05, .2, null, 0],
      'featherfall':[.45, 0, .2, null, 0], 'fey-lights':[.1, 0, .1, null, 0],
      'mammatus-sky':[.7, .2, .45, null, 0], 'lenticular':[.12, 0, .55, null, 0], 'st-elmos-fire':[.95, .5, .8, null, 0], 'ball-lightning':[.85, .4, .6, null, 0],
      'eruption':[.3, 0, .6, null, 0], 'leviathans':[.25, 0, .2, null, 0]
    };
    return L;
  })();

  /* ── The building blocks. Each says what the painted sky does (base: cloud, darkness, fog, rain, snow, hail, storm,
     wind, as 0..1 at full strength), how it colours the sky (sky: colour, how much, and a light factor), and what it adds:
     air (sheets, fog banks, haze, streaks, rays drawn into the sky by the shader; depth 0 sky, 1 valleys, 2 near), parts
     (particles near the viewer; band keeps them to a share of the height, wink and flap are their own rhythms), shapes
     (drawn into the sky behind the clouds: a rainbow, a halo, a rift), aurora, sigils, fireballs, a funnel, a flash.
     Names are the owner's words. best: the hour its preview is shown at. ── */
  LOOKS.AIR = {veil:0, blob:1, haze:2, gust:3, sand:4, rays:5, 'void':6, plume:7, pouch:8};
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
    'sigils':{n:'Sigils', best:21.5, base:{cloud:.1}, sigils:{c:'#c9b3ff', k:.8}},
    /* Clouds and sky */
    'mist-low':{n:'Ground mist', best:7.2, base:{cloud:.15, fog:.16, wind:.08}, sky:['#dfe5ea', .1, 1.02],
      air:[{t:'blob', c:'#eef2f5', k:.9, sx:.006, sy:0, v:[-.14, .2], d:1}, {t:'blob', c:'#e3e9ee', k:.5, sx:.003, sy:0, v:[-.15, .12], d:2}]},
    'gloom':{n:'Gloom', best:13, base:{cloud:.98, dark:.62, fog:.14, wind:.15}, sky:['#35343c', .5, .66], grey:.25},
    'sunbeams':{n:'Sunbeams', best:16.6, base:{cloud:.6, dark:.1, wind:.25}, sky:['#e9d7a8', .12, 1.04],
      air:[{t:'rays', c:'#ffeec2', k:.4, v:[-.1, 1.1], d:0, e:.35}]},
    'red-sky':{n:'Red sky', best:18.7, base:{cloud:.42, wind:.25}, sky:['#e0552e', .5, 1], sun:{tint:'#ff5a2a', dim:.1},
      air:[{t:'haze', c:'#ff7a4a', k:.45, v:[-.06, .6], d:0, e:.3}]},
    'rainbow':{n:'Rainbow', best:17.3, base:{cloud:.4, fog:.04, rain:.16, wind:.3}, sky:['#9fb6d6', .1, 1.03], shapes:[{s:'rainbow', k:.9}]},
    'sun-halo':{n:'Halo', best:11.2, base:{cloud:.22, fog:.06, wind:.2}, sky:['#dfe8f2', .14, 1.03], shapes:[{s:'sunring', c:'#f4f7ff', k:.85}]},
    'night-clouds':{n:'Night-shining clouds', best:21.4, base:{cloud:.04}, sky:['#16305c', .2, 1], shapes:[{s:'nightcloud', c:'#9fd8ff', k:.85}]},
    'frost':{n:'Hoarfrost', best:8.1, base:{cloud:.05, fog:.07, wind:.08}, sky:['#cfe6f7', .24, 1.05],
      air:[{t:'blob', c:'#e6f4ff', k:.45, sx:.002, sy:0, v:[-.15, .14], d:1}],
      parts:[{p:'glint', n:60, s:[1.2, 2.4], c:'#dff3ff', c2:'#ffffff', v:[0, 0], e:.9, a:1, mode:'blink', band:[.6, 1]}]},
    'diamond-dust':{n:'Diamond dust', best:10.4, base:{cloud:.04, fog:.1, wind:.1}, sky:['#bfe0f5', .22, 1.05], shapes:[{s:'sunring', c:'#f4f7ff', k:.4}],
      parts:[{p:'glint', n:110, s:[1, 2.1], c:'#e8f6ff', c2:'#ffffff', v:[3, 5], wob:5, e:.9, a:1, mode:'drift'}]},
    /* Rain and snow */
    'flurries':{n:'Flurries', best:12.3, base:{cloud:.55, fog:.06, snow:.38, wind:.45}, sky:['#d3dbe6', .08, 1.02]},
    'freezing-rain':{n:'Freezing rain', best:12.3, base:{cloud:.95, dark:.3, fog:.2, rain:.8, wind:.35}, sky:['#7f97ad', .28, .95], rainTint:['#cfe9ff', .7],
      parts:[{p:'glint', n:26, s:[1, 1.9], c:'#d6efff', c2:'#ffffff', v:[0, 0], e:.8, a:.9, mode:'blink', band:[.58, 1]}]},
    'thundersnow':{n:'Thundersnow', best:16.8, base:{cloud:1, dark:.62, fog:.3, snow:1.35, storm:.9, wind:.9}, sky:['#3d4058', .32, .9], flash:'lightning'},
    'dry-lightning':{n:'Dry lightning', best:18.3, base:{cloud:.85, dark:.5, fog:.04, storm:1, wind:.8}, sky:['#6a4038', .38, .9], flash:'lightning'},
    'rain-gold':{n:'Golden rain', best:16, base:{cloud:.6, dark:.08, fog:.1, rain:.8, wind:.3}, sky:['#e2b64a', .32, 1.06], rainTint:['#ffd65a', .95],
      air:[{t:'haze', c:'#ffd98a', k:.5, v:[-.06, .5], d:0, e:.4}]},
    'rain-ink':{n:'Ink rain', best:12.3, base:{cloud:1, dark:.5, fog:.2, rain:1.1, wind:.4}, sky:['#1f1b4a', .68, .74], rainTint:['#0d0b1c', .95], grey:.1,
      parts:[{p:'grain', n:170, s:[1.4, 2.7], c:'#0b0918', c2:'#1d1a3a', v:[14, 190], wob:1, e:0, a:.85, mode:'fall'}]},
    'snow-glow':{n:'Glowing snow', best:21, base:{cloud:.8, dark:.1, fog:.2, snow:.9, wind:.2}, sky:['#4b3c8a', .32, 1], rainTint:['#c9a8ff', .9],
      parts:[{p:'mote', n:28, s:[1.2, 2.3], c:'#b48cff', c2:'#efe4ff', v:[4, 16], wob:9, fl:.3, e:1, a:.9, mode:'fall'}]},
    'cinders':{n:'Cinder rain', best:20, base:{cloud:.85, dark:.55, fog:.22, wind:.35}, sky:['#5a1f14', .5, .8],
      parts:[{p:'ember', n:80, s:[1.2, 2.3], c:'#ff5a14', c2:'#ffc24a', v:[10, 46], wob:6, fl:.8, e:1, a:1, mode:'fall'},
        {p:'ash', n:20, s:[1.2, 2.4], c:'#5c5450', c2:'#8a827c', v:[6, 22], wob:6, spin:.6, e:0, a:.8, mode:'fall'}]},
    'star-rain':{n:'Star rain', best:22.3, base:{cloud:.08}, sky:['#14224a', .25, 1],
      parts:[{p:'ember', n:44, s:[1.1, 2], c:'#9fc4ff', c2:'#ffffff', v:[-14, 70], wob:2, fl:.3, e:1, a:1, mode:'fall'},
        {p:'glint', n:16, s:[1, 1.8], c:'#cfe2ff', c2:'#ffffff', v:[0, 0], e:1, a:.9, mode:'blink', band:[.7, 1]}]},
    'shardfall':{n:'Glass rain', best:12.3, base:{cloud:.9, dark:.5, fog:.1, hail:.3, wind:.35}, sky:['#2f5a66', .45, .9],
      parts:[{p:'shard', n:70, s:[2, 4], c:'#9fe9f7', c2:'#ffffff', v:[8, 90], wob:2, spin:1.6, e:.8, a:1, mode:'fall'}]},
    /* Wind and dust */
    'sea-spray':{n:'Sea spray', best:12.3, base:{cloud:.6, dark:.1, fog:.3, wind:1}, sky:['#7fa0ad', .32, .96],
      air:[{t:'gust', c:'#f2f8fa', k:1, sx:.25, v:[-.05, .7], d:1}, {t:'blob', c:'#e6f0f3', k:.85, sx:.06, sy:0, v:[-.14, .34], d:2}],
      parts:[{p:'grain', n:170, s:[1.1, 2.3], c:'#e8f4f8', c2:'#ffffff', v:[160, -6], wob:5, e:.3, a:.85, mode:'blow'}]},
    'waterspout':{n:'Waterspout', best:16, base:{cloud:1, dark:.55, fog:.2, rain:.7, wind:1}, sky:['#3f5a66', .35, .86], funnel:{c:'#5d7f8c', k:.9, w:.5},
      parts:[{p:'grain', n:50, s:[.8, 1.5], c:'#cfe3ea', c2:'#ffffff', v:[120, -10], wob:5, e:.1, a:.6, mode:'blow'},
        {p:'debris', n:30, s:[.8, 1.8], c:'#cfe3ea', c2:'#eef7fa', v:[60, -8], e:.1, a:.8, mode:'vortex'}]},
    'dust-wall':{n:'Wall of dust', best:16.5, base:{cloud:.75, dark:.45, fog:.8, wind:1}, sky:['#8a5a2c', .7, .78], sun:{tint:'#ff8a3a', dim:.5},
      air:[{t:'sand', c:'#a56a35', k:1.1, sx:.12, v:[-.15, 1.1], d:2}, {t:'plume', c:'#70441f', k:.6, sx:.03, sy:.01, v:[-.1, .9], d:0}],
      parts:[{p:'grain', n:150, s:[.8, 1.8], c:'#7a4d22', c2:'#b88348', v:[170, 10], wob:4, e:0, a:.8, mode:'blow'}]},
    /* Fire and smoke */
    'firestorm':{n:'Firestorm', best:20.5, base:{cloud:.8, dark:.6, fog:.4, wind:1}, sky:['#7a2410', .6, .8], sun:{tint:'#ff5a2a', dim:.55},
      air:[{t:'plume', c:'#3a2a24', k:.9, sx:.06, sy:.04, v:[-.1, 1.05], d:0}, {t:'blob', c:'#ff6a22', k:.45, sx:.03, sy:0, v:[-.15, .3], d:1, e:.8}],
      parts:[{p:'ember', n:120, s:[1.2, 2.6], c:'#ff5a14', c2:'#ffd27a', v:[120, -26], wob:10, fl:1, e:1, a:1, mode:'blow'}]},
    'horizon-fire':{n:'Fire on the horizon', best:21.5, base:{cloud:.4, dark:.2, fog:.15, wind:.3}, sky:['#6a2010', .46, .9], shapes:[{s:'hglow', c:'#ff4a1a', k:.95}],
      air:[{t:'plume', c:'#33261f', k:.45, sx:.02, sy:.03, v:[0, 1], d:0}],
      parts:[{p:'ember', n:24, s:[1.1, 2.2], c:'#ff6a1a', c2:'#ffb347', v:[8, -16], wob:8, fl:1, e:1, a:.9, mode:'rise'}]},
    'brimstone':{n:'Brimstone', best:14, base:{cloud:.6, dark:.25, fog:.5, wind:.2}, sky:['#a8932a', .55, .86], sun:{tint:'#ffd23a', dim:.3},
      air:[{t:'haze', c:'#d6c23a', k:.9, v:[-.1, .7], d:0}, {t:'blob', c:'#b8a52e', k:.4, sx:.006, sy:.002, v:[-.15, .35], d:1}],
      parts:[{p:'ember', n:12, s:[1, 1.9], c:'#e8e04a', c2:'#fff9b0', v:[5, 12], wob:6, fl:.8, e:1, a:.85, mode:'fall'}]},
    'smog':{n:'Smog', best:9, base:{cloud:.5, dark:.2, fog:.55, wind:.05}, sky:['#7a6a52', .55, .84], sun:{tint:'#ffb070', dim:.45}, grey:.2,
      air:[{t:'haze', c:'#8a7658', k:.9, v:[-.1, .6], d:0}, {t:'blob', c:'#6e5f4a', k:.5, sx:.003, sy:0, v:[-.15, .3], d:1}]},
    'witchfire':{n:'Witchfire', best:21.5, base:{cloud:.3, fog:.2, wind:.2}, sky:['#0f3a3c', .4, .92],
      air:[{t:'blob', c:'#1fd6a8', k:.3, sx:.01, sy:.004, v:[-.15, .3], d:1, e:.6}],
      parts:[{p:'ember', n:38, s:[1.4, 2.8], c:'#2affc4', c2:'#c8fff0', v:[4, -26], wob:10, fl:1, e:1, a:1, mode:'rise'}]},
    'lanterns':{n:'Sky lanterns', best:21, base:{cloud:.08}, sky:['#2a2040', .15, 1],
      parts:[{p:'lantern', n:30, s:[3, 5.6], c:'#ff9a3a', c2:'#ffe2a0', v:[3, -9], wob:6, fl:.25, e:1, a:1, mode:'rise'}]},
    /* Living things */
    'fireflies':{n:'Fireflies', best:21, base:{cloud:.12, wind:.05}, sky:['#1c3324', .14, 1],
      parts:[{p:'mote', n:44, s:[1.2, 2.2], c:'#c8ff5a', c2:'#f4ffc8', v:[3, 0], wob:10, e:1, a:1, mode:'wander', band:[.5, .98], wink:2.6}]},
    'wings':{n:'Fluttering wings', best:15.5, base:{cloud:.2, wind:.2}, sky:['#f0d9a8', .06, 1.02],
      parts:[{p:'wing', n:22, s:[2.6, 4.4], c:'#f2a23a', c2:'#fff0c8', v:[10, 0], wob:14, e:0, a:.95, mode:'wander', band:[.18, .9], flap:9}]},
    'swarm':{n:'Swarm', best:15, base:{cloud:.35, dark:.3, fog:.2, wind:.6}, sky:['#5e4a2a', .45, .82], sun:{tint:'#d9a05a', dim:.4},
      air:[{t:'blob', c:'#3d2f1c', k:.75, sx:.07, sy:.01, v:[.1, .9], d:0}],
      parts:[{p:'grain', n:240, s:[1.3, 2.5], c:'#221a0e', c2:'#4a3a22', v:[95, 6], wob:18, e:0, a:1, mode:'blow'}]},
    'feathers':{n:'Feathers', best:14, base:{cloud:.45, wind:.25}, sky:['#aab6d4', .22, .97],
      parts:[{p:'feather', n:34, s:[3.8, 6.6], c:'#ffffff', c2:'#b5adc2', v:[12, 16], wob:20, spin:.7, e:0, a:.95, mode:'fall'}]},
    'pollen':{n:'Pollen', best:15, base:{cloud:.15, fog:.2, wind:.2}, sky:['#e3cf6a', .3, 1.03],
      air:[{t:'haze', c:'#f0dc7a', k:.7, v:[-.08, .55], d:0}],
      parts:[{p:'spore', n:60, s:[.9, 1.7], c:'#f2d84a', c2:'#fff4b0', v:[8, 1], wob:9, e:.3, a:.8, mode:'drift'}]},
    'thistledown':{n:'Thistledown', best:15.5, base:{cloud:.2, wind:.4}, sky:['#dfe9d6', .06, 1.02],
      parts:[{p:'fluff', n:40, s:[2.8, 4.8], c:'#ffffff', c2:'#e9efe2', v:[24, -3], wob:16, spin:.3, e:.15, a:.9, mode:'drift'}]},
    'gossamer':{n:'Gossamer', best:9.5, base:{cloud:.12, fog:.1, wind:.15}, sky:['#c4d4e6', .16, 1],
      parts:[{p:'strand', n:26, s:[12, 26], c:'#f4f9ff', c2:'#ffffff', v:[14, -1], wob:8, e:.6, a:.95, mode:'drift'}]},
    /* Magic */
    'wisps':{n:'Will-o’-wisps', best:22, base:{cloud:.3, fog:.4, wind:.05}, sky:['#12303a', .3, .95],
      air:[{t:'blob', c:'#9fd9d0', k:.35, sx:.004, sy:0, v:[-.15, .25], d:1}],
      parts:[{p:'wisp', n:11, s:[3, 5.5], c:'#7af2e0', c2:'#eafffb', v:[6, 0], wob:22, e:1, a:.95, mode:'wander', band:[.55, .95], pulse:1}]},
    'dreamfog':{n:'Dream fog', best:19, base:{cloud:.4, fog:.85, wind:.08}, sky:['#c9a8e0', .35, 1.02],
      air:[{t:'blob', c:'#ffffff', k:.7, sx:.006, sy:.002, v:[-.1, .8], d:0, rb:1, e:.45}, {t:'veil', c:'#f0d8ff', k:.35, sx:.008, sy:.01, v:[.1, 1], d:0}]},
    'shadow':{n:'Shadowfall', best:14, base:{cloud:.5, dark:.75, fog:.2, wind:.2}, sky:['#0e0a1c', .7, .42], sun:{dim:.8}, stars:.5, grey:.3,
      air:[{t:'void', c:'#2a1450', k:.55, v:[-.1, .7], d:0}, {t:'blob', c:'#0a0614', k:.7, sx:.012, sy:.002, v:[-.15, .4], d:1}],
      parts:[{p:'grain', n:70, s:[1.2, 2.4], c:'#05030a', c2:'#1a0f30', v:[3, 26], wob:6, e:0, a:.8, mode:'fall'}]},
    'rift':{n:'Sky rift', best:20.5, base:{cloud:.3, dark:.15}, sky:['#3a2470', .35, .95], shapes:[{s:'rift', c:'#b48cff', k:.95}],
      air:[{t:'rays', c:'#c9a8ff', k:.35, v:[.1, 1.1], d:0, e:.6}],
      parts:[{p:'mote', n:18, s:[1.2, 2.2], c:'#c9a8ff', c2:'#ffffff', v:[3, -12], wob:8, fl:.5, e:1, a:.9, mode:'rise'}]},
    'bubbles':{n:'Bubbles', best:15, base:{cloud:.15, wind:.15}, sky:['#bfe6f2', .08, 1.03],
      parts:[{p:'bubble', n:40, s:[3, 6.6], c:'#ffffff', c2:'#bfefff', v:[5, -11], wob:12, e:.5, a:1, mode:'drift', rb:1}]},
    'fey-lights':{n:'Fey lights', best:21.5, base:{cloud:.12}, sky:['#1e3a36', .2, 1], aurora:{c0:'#ff9ad6', c1:'#7affc8', k:.35, alt:'low', span:120},
      parts:[{p:'mote', n:38, s:[1.4, 2.8], c:'#ffffff', c2:'#ffffff', v:[5, 0], wob:14, fl:.4, e:1, a:1, mode:'wander', band:[.15, .9], rb:1}]},
    /* Clouds and sky: pouches hanging under a storm deck, warm underneath at sunset; lens clouds standing still over the
       highest peaks while the wind blows. */
    'mammatus':{n:'Hanging pouches', best:19.4, base:{cloud:.55, dark:.25, fog:.04, wind:.3}, sky:['#8a6a70', .22, .95],
      air:[{t:'pouch', c:'#a3969a', k:1, sx:.004, sy:0, v:[.32, 1.2], d:0}]},
    'lenticular':{n:'Lens clouds', best:18.9, base:{cloud:.1, wind:.55}, sky:['#c9a0b4', .12, 1.02], shapes:[{s:'lens', k:1}]},
    /* Storms: blue fire on the crowns of the near wood, and slow balls of lightning wandering low over the land. Both
       are their own light, so they show most in the dark (night: 1), and in a short pane their size has a floor (rmin),
       so a docked strip still reads them. */
    'st-elmo':{n:'St Elmo’s fire', best:21.6, base:{cloud:.95, dark:.6, fog:.08, wind:.7}, sky:['#232a48', .35, .82],
      parts:[{p:'elmo', n:16, s:[3.4, 5.6], rmin:3.6, c:'#6fa8ff', c2:'#eef4ff', v:[0, 0], e:1, a:1, mode:'ridge', night:1}]},
    'ball-lightning':{n:'Ball lightning', best:21.2, base:{cloud:.9, dark:.5, fog:.12, wind:.4}, sky:['#2c2f48', .3, .85],
      parts:[{p:'ball', n:3, s:[5, 7.4], rmin:4.4, c:'#b8ccff', c2:'#ffffff', v:[7, 0], wob:18, swing:.03, low:1, e:1, a:1, mode:'wander', band:[.82, .93], pulse:1, night:1}]},
    /* Fire and smoke: a mountain erupting beyond the far range, its plume leaning with the wind, ash falling near. */
    'volcano':{n:'Eruption', best:20.6, base:{cloud:.25, dark:.3, fog:.12, wind:.4}, sky:['#6a2a14', .42, .86], sun:{tint:'#ff7a3a', dim:.3}, shapes:[{s:'eruption', c:'#ff5a1a', k:1}],
      parts:[{p:'ash', n:26, s:[1.2, 2.6], c:'#6e6862', c2:'#9a948e', v:[8, 9], wob:6, spin:.5, e:0, a:.75, mode:'fall'}]},
    /* Living things: great shapes swimming slowly far off, paler the farther they are; by night faint lights along
       their flanks. */
    'leviathans':{n:'Sky leviathans', best:16.8, base:{cloud:.25, fog:.06, wind:.2}, sky:['#a9b9cc', .08, 1], shapes:[{s:'leviathan', c:'#33405a', k:1}]}
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
       pane glides between weathers, its pace: the time constant of its glide in seconds, so it is most of the way there
       in three. Any change of weather is just new targets for these, so no pair of weathers needs a transition of its
       own (#945). Dials with no ease fade across with the look's layers (FADE_S). */
    L.DIALS = [
      {n:'cloud', rest:0, ease:2.2, what:'how much of the sky is cloud'},
      {n:'dark', rest:0, ease:1.8, what:'how far storm cloud darkens the sky'},
      {n:'fog', rest:0, ease:3, what:'how thick the fog is'},
      {n:'rain', rest:0, ease:1.2, what:'how hard it rains (above 1 for a downpour)'},
      {n:'snow', rest:0, ease:1.6, what:'how hard it snows'},
      {n:'hail', rest:0, ease:.6, what:'how hard it hails'},
      {n:'storm', rest:0, ease:1.4, what:'how stormy: lightning and churn'},
      {n:'wind', rest:.2, ease:1, what:'how strong the wind is'},
      {n:'grey', rest:0, ease:null, what:'how far the light is washed grey'},
      {n:'light', rest:1, ease:null, what:'how bright the light is, 1 for a plain day'},
      {n:'slant', rest:0, ease:null, what:'how far rain is driven sideways'},
      {n:'heavy', rest:0, ease:null, what:'how heavy the drops are'},
      {n:'stars', rest:1, ease:null, what:'how many stars show, 1 for all'},
      {n:'drift', rest:1, ease:null, what:'how fast fog and haze drift, 1 for a still day'}
    ];
    /* A change of weather fades the look's layers (dust, haze, a funnel, the sky's tint) and the dials with no pace of
       their own across over this many seconds, so nothing switches at once. */
    L.FADE_S = 6;
    L.DIAL_NAMES = L.DIALS.map(function(d){ return d.n; });
    function blank(){
      var R = {sky:null, fogTint:null, rainTint:null, aurora:[], air:[], parts:[], shapes:[], sigils:[], fire:[], funnel:null, flash:{lightning:0, ley:0}, sun:{dim:0, 'void':0, tint:null, tk:0}, effects:[]};
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
      /* pi: the set's place in its own effect, which seeds its particles, so they keep their places while a change of
         weather lays another weather's sets before them. */
      (E.parts || []).forEach(function(p, j){ R.parts.push(Object.assign({}, p, {c:hx(p.c, tint), c2:hx(p.c2, tint), n:p.n * Math.min(1.25, .35 + .65 * k), a:p.a * Math.min(1, .55 + .45 * k), rb:tint ? 0 : p.rb, fx:id, pi:j})); });
      (E.shapes || []).forEach(function(q){ R.shapes.push({s:q.s, c:q.c ? hx(q.c, tint) : null, k:q.k * Math.min(1, k)}); });
      if (E.sigils) R.sigils.push({c:hx(E.sigils.c, tint), k:E.sigils.k * k});
      if (E.fire) R.fire.push({rate:E.fire.rate * Math.min(1.3, .4 + .6 * k), c:hx(E.fire.c, tint)});
      if (E.funnel) R.funnel = {c:hx(E.funnel.c, tint), k:E.funnel.k * Math.min(1.1, k), w:E.funnel.w * (ph && ph[2] < .6 ? .45 : 1)};
      if (E.flash) R.flash[E.flash] = Math.max(R.flash[E.flash], Math.min(1, k));
      if (E.sun){ if (E.sun.dim) R.sun.dim = Math.max(R.sun.dim, E.sun.dim * w); if (E.sun['void']) R.sun['void'] = Math.max(R.sun['void'], E.sun['void'] * Math.min(1, k)); if (E.sun.tint){ R.sun.tint = hx(E.sun.tint, tint); R.sun.tk = 1; } }
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
      ['aurora', 'air', 'parts', 'shapes', 'sigils', 'fire'].forEach(function(n){ R[n] = fade(A[n] || [], u).concat(fade(B[n] || [], t)); });
      R.funnel = A.funnel && B.funnel ? Object.assign({}, B.funnel, {k:A.funnel.k * u + B.funnel.k * t}) : A.funnel ? Object.assign({}, A.funnel, {k:A.funnel.k * u}) : B.funnel ? Object.assign({}, B.funnel, {k:B.funnel.k * t}) : null;
      R.flash = {lightning:A.flash.lightning * u + B.flash.lightning * t, ley:A.flash.ley * u + B.flash.ley * t};
      /* The sun's smoky tint fades in and out with its strength; between two tints its colour mixes. */
      var ta = A.sun.tint, tb = B.sun.tint;
      R.sun = {dim:A.sun.dim * u + B.sun.dim * t, 'void':A.sun['void'] * u + B.sun['void'] * t, tint:ta && tb ? L.linHex(PAL.mixLin(PAL.hexLin(ta), PAL.hexLin(tb), t)) : ta || tb,
        tk:(ta ? (A.sun.tk != null ? A.sun.tk : 1) : 0) * u + (tb ? (B.sun.tk != null ? B.sun.tk : 1) : 0) * t};
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
        o.x = h(i * 7 + cyc, seed) * W; o.y = p.band ? H * (p.band[0] + (p.band[1] - p.band[0]) * h(i * 11 + cyc, seed + 3)) : h(i * 11 + cyc, seed + 3) * H * .92;
        o.a = Math.pow(Math.sin(Math.PI * smooth01(0, .45, u)), 2);
        return o;
      }
      /* Wandering things (fireflies, moths, wisps) keep to a band of the sky and loop about a slow drift; wink is a
         glow that comes and goes, flap a pair of wings beating. swing caps the rise and fall (a share of the height),
         and low keeps a set down in front of the middle range, never up against the sky over the far one. */
      if (p.mode === 'wander'){
        var bd = p.band || [.12, .9], Rw = (10 + 22 * r4) * sc * (p.wob || 12) / 12, wv = .3 + .3 * r3, mw = 24 * sc, Ww = W + 2 * mw;
        o.x = wrap(r2 * Ww + p.v[0] * sc * (.5 + .5 * z) * t + Rw * Math.sin(t * wv + r3 * TAU) + Rw * .4 * Math.sin(t * wv * 2.3 + r5 * TAU), Ww) - mw;
        o.y = H * (bd[0] + (bd[1] - bd[0]) * r3) + (p.swing ? Math.min(Rw * .5, p.swing * H) : Rw * .5) * Math.cos(t * wv * 1.3 + r2 * TAU);
        if (p.low && F.ridgeAt) o.y = Math.max(o.y, F.ridgeAt(1, o.x, W, H) + 3 * sc);
        if (p.wink){ var pw = p.wink * (.7 + .6 * r4), uw = wrap(t / pw + r5, 1); o.a = Math.pow(Math.sin(Math.PI * smooth01(0, .6, uw)), 2); }
        if (p.flap){ o.flip = Math.sin(t * p.flap * (.8 + .4 * r1) + r2 * TAU); o.rot = (r5 - .5) * .9 + .3 * Math.sin(t * wv * 2 + r4 * TAU); }
        if (p.fl) o.a *= 1 - p.fl * .35 * (.5 + .5 * Math.sin(t * TAU / (TEMPO.flick * (.8 + .5 * r3)) + r4 * TAU));
        if (p.pulse) o.a *= .7 + .3 * Math.sin(t * TAU / TEMPO.breath + r2 * TAU);
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
      /* On the ridge: a fire standing on a tree crown of the near wood. Each burns on one crown for a few seconds, dies
         down and kindles on another, crackling while it burns; rot carries the crackle's step for the drawing. */
      if (p.mode === 'ridge'){
        var cr = F.crowns ? F.crowns(W, H) : [], per4 = 3.4 + 3 * r4, c4 = Math.floor((t + r5 * per4) / per4), u4 = (t + r5 * per4) / per4 - c4;
        if (!cr.length){ o.a = 0; return o; }
        var q4 = cr[Math.floor(h(i * 7 + c4, seed + 6.1) * cr.length) % cr.length], fq = Math.floor(t * 7 + r4 * 50);
        o.x = q4[0]; o.y = q4[1]; o.rot = fq;
        o.a = smooth01(0, .14, u4) * (1 - smooth01(.72, 1, u4)) * (.72 + .28 * h(i * 3 + fq, seed));
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
    /* A particle set's seed: the whole effect id and the set's place in it, so two weathers' sets never draw the same
       particles, and a set keeps its particles whatever lies before it in a blended list. */
    function seedOf(p, si){ return 3.1 + (p.pi != null ? p.pi : si) * 17.7 + (p.fx ? SW.hash01(p.fx) * 97 : 0); }
    F.seedOf = seedOf;

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
      /* St Elmo's fire: a cold glow on a crown's tip, a wider bloom round it in the dark, and fine streamers crackling up
         and out from it. */
      elmo:function(ctx, o, r, A, B, p, i){
        var y0 = o.y - r * .15;
        glow(ctx, o.x, y0, r * .8, A, A, o.a * .2);
        glow(ctx, o.x, y0, r * .5, A, B, o.a * .95);
        ctx.strokeStyle = LOOKS.srgb(B, o.a * .85); ctx.lineWidth = Math.max(.6, r * .13); ctx.lineCap = 'round'; ctx.lineJoin = 'round';
        for (var k = 0; k < 6; k++){
          var an = -Math.PI / 2 + (h(i * 5 + k, o.rot) - .5) * 2.1, ln = r * (.8 + h(i * 7 + k, o.rot + 1)), x = o.x, y = y0;
          ctx.beginPath(); ctx.moveTo(x, y);
          for (var sg = 1; sg <= 3; sg++){ var a2 = an + (h(k * 3 + sg, o.rot + 2) - .5) * .9; x += Math.cos(a2) * ln / 3; y += Math.sin(a2) * ln / 3; ctx.lineTo(x, y); }
          ctx.stroke();
        }
      },
      /* Ball lightning: a hot white core in a cold halo, short arcs that snap on and off round it, and the faint pool of
         its light on the nearest ground under it, fainter the higher it floats. */
      ball:function(ctx, o, r, A, B, p, i, t, W, H){
        if (F.ridgeAt){
          for (var gi = 1, gy = H + 1; gi <= 3; gi++){ gy = F.ridgeAt(gi, o.x, W, H, true); if (gy > o.y) break; }
          var pa = o.a * .2 * (1 - smooth01(r, r * 9, gy - o.y));
          if (gy < H + r && pa > .01) F.soft(ctx, o.x, gy + r * .2, r * 5, r * 1.1, A, pa);
        }
        F.soft(ctx, o.x, o.y, r * 2.6, r * 2.6, A, o.a * .3);
        glow(ctx, o.x, o.y, r * .75, A, B, o.a);
        var fq = Math.floor(t * 5 + i * 7);
        ctx.strokeStyle = LOOKS.srgb(B, o.a * .9); ctx.lineWidth = Math.max(.6, r * .12); ctx.lineCap = 'round';
        for (var k = 0; k < 4; k++){
          if (h(fq * 4 + k, i + 2.3) < .45) continue;
          var a0 = h(fq, k + i) * TAU, rr = r * (1 + .6 * h(fq + 3, k)), x = o.x + Math.cos(a0) * r * .5, y = o.y + Math.sin(a0) * r * .5;
          ctx.beginPath(); ctx.moveTo(x, y);
          for (var sg = 1; sg <= 3; sg++){ var a1 = a0 + (h(fq + sg, k + 9) - .5) * 1.4; x += Math.cos(a1) * rr / 2.5; y += Math.sin(a1) * rr / 2.5; ctx.lineTo(x, y); }
          ctx.stroke();
        }
      },
      /* A glint: a point of light that turns to the eye and away, as frost and falling ice do. */
      glint:function(ctx, o, r, A, B, p, i, t){
        var tw = .25 + .75 * Math.pow(.5 + .5 * Math.sin(t * TAU / (TEMPO.shimmer * (.5 + .9 * h(i, 4.4))) + h(i, 5.5) * TAU), 3), a = o.a * tw;
        if (a <= .02) return;
        glow(ctx, o.x, o.y, r * .55, A, B, a * .8);
        ctx.strokeStyle = LOOKS.srgb(B, a); ctx.lineWidth = Math.max(.5, r * .3); ctx.lineCap = 'round';
        var l = r * 2.1 * tw; ctx.beginPath(); ctx.moveTo(o.x - l, o.y); ctx.lineTo(o.x + l, o.y); ctx.moveTo(o.x, o.y - l); ctx.lineTo(o.x, o.y + l); ctx.stroke();
      },
      /* Thistledown: a seed hanging under a tuft of fine hairs. */
      fluff:function(ctx, o, r, A, B){
        ctx.save(); ctx.translate(o.x, o.y); ctx.rotate(Math.sin(o.rot) * .5);
        ctx.strokeStyle = LOOKS.srgb(A, o.a * .55); ctx.lineWidth = Math.max(.4, r * .1); ctx.lineCap = 'round'; ctx.beginPath();
        for (var k = 0; k < 9; k++){ var an = -Math.PI * (.08 + .84 * k / 8); ctx.moveTo(0, 0); ctx.lineTo(Math.cos(an) * r * 1.5, Math.sin(an) * r * 1.5); }
        ctx.stroke();
        ctx.strokeStyle = LOOKS.srgb(B, o.a * .7); ctx.beginPath(); ctx.moveTo(0, 0); ctx.lineTo(0, r * 1.1); ctx.stroke();
        ctx.fillStyle = LOOKS.srgb(B.map(function(v){ return v * .55; }), o.a * .9); ctx.beginPath(); ctx.ellipse(0, r * 1.2, r * .16, r * .3, 0, 0, TAU); ctx.fill();
        ctx.restore();
      },
      /* Gossamer: one long thread of spider silk, seen where the light runs along it. */
      strand:function(ctx, o, r, A, B, p, i, t){
        var bend = (h(i, 1.9) - .5) * r * .7 + Math.sin(t * TAU / TEMPO.sway + h(i, 2.8) * TAU) * r * .12, tilt = (h(i, 6.1) - .5) * .5;
        ctx.save(); ctx.translate(o.x, o.y); ctx.rotate(tilt); ctx.lineCap = 'round';
        ctx.strokeStyle = LOOKS.srgb(A, o.a * .35); ctx.lineWidth = Math.max(.4, r * .02);
        ctx.beginPath(); ctx.moveTo(-r, 0); ctx.quadraticCurveTo(0, bend, r, 0); ctx.stroke();
        var u = .5 + .5 * Math.sin(t * TAU / (TEMPO.shimmer * 2.5) + h(i, 7.7) * TAU), x0 = -r + 2 * r * u * .8, g = ctx.createLinearGradient(x0, 0, x0 + r * .4, 0);
        g.addColorStop(0, LOOKS.srgb(B, 0)); g.addColorStop(.5, LOOKS.srgb(B, o.a)); g.addColorStop(1, LOOKS.srgb(B, 0));
        ctx.strokeStyle = g; ctx.lineWidth = Math.max(.6, r * .035);
        ctx.beginPath(); ctx.moveTo(-r, 0); ctx.quadraticCurveTo(0, bend, r, 0); ctx.stroke();
        ctx.restore();
      },
      /* A feather: a curved quill with a soft vane, turning as it falls. */
      feather:function(ctx, o, r, A, B){
        ctx.save(); ctx.translate(o.x, o.y); ctx.rotate(o.rot); ctx.scale(Math.max(.22, Math.abs(o.flip)), 1);
        var g = ctx.createLinearGradient(-r * .5, 0, r * .5, 0); g.addColorStop(0, LOOKS.srgb(B, o.a)); g.addColorStop(.5, LOOKS.srgb(A, o.a)); g.addColorStop(1, LOOKS.srgb(B, o.a * .9));
        ctx.fillStyle = g; ctx.beginPath(); ctx.moveTo(0, -r * 1.5); ctx.bezierCurveTo(r * .62, -r * .9, r * .5, r * .5, r * .06, r * 1.05); ctx.bezierCurveTo(-r * .5, r * .5, -r * .58, -r * .8, 0, -r * 1.5); ctx.fill();
        ctx.strokeStyle = LOOKS.srgb(B.map(function(v){ return v * .7; }), o.a * .8); ctx.lineWidth = Math.max(.4, r * .07); ctx.lineCap = 'round';
        ctx.beginPath(); ctx.moveTo(0, -r * 1.35); ctx.quadraticCurveTo(r * .1, 0, r * .05, r * 1.5); ctx.stroke();
        ctx.restore();
      },
      /* A pair of wings: a moth or a butterfly, beating. */
      wing:function(ctx, o, r, A, B, p, i){
        var c = h(i, 9.1) < .6 ? A : B, beat = Math.max(.12, Math.abs(o.flip));
        ctx.save(); ctx.translate(o.x, o.y); ctx.rotate(o.rot);
        [-1, 1].forEach(function(sd){
          ctx.save(); ctx.scale(sd * beat, 1);
          ctx.fillStyle = LOOKS.srgb(c, o.a); ctx.beginPath(); ctx.moveTo(0, -r * .1); ctx.bezierCurveTo(r * .5, -r * 1.2, r * 1.5, -r * .9, r * 1.05, 0); ctx.bezierCurveTo(r * 1.3, r * .7, r * .5, r * .95, 0, r * .25); ctx.closePath(); ctx.fill();
          ctx.fillStyle = LOOKS.srgb(c.map(function(v){ return v * .45; }), o.a * .7); ctx.beginPath(); ctx.arc(r * .82, -r * .35, r * .16, 0, TAU); ctx.fill();
          ctx.restore();
        });
        ctx.strokeStyle = LOOKS.srgb(c.map(function(v){ return v * .3; }), o.a); ctx.lineWidth = Math.max(.6, r * .16); ctx.lineCap = 'round';
        ctx.beginPath(); ctx.moveTo(0, -r * .4); ctx.lineTo(0, r * .45); ctx.stroke();
        ctx.restore();
      },
      /* A bubble: a thin film with one bright window on it. */
      bubble:function(ctx, o, r, A, B){
        var g = ctx.createRadialGradient(o.x, o.y, r * .55, o.x, o.y, r);
        g.addColorStop(0, LOOKS.srgb(A, 0)); g.addColorStop(.82, LOOKS.srgb(A, o.a * .22)); g.addColorStop(1, LOOKS.srgb(B, o.a * .7));
        ctx.fillStyle = g; ctx.beginPath(); ctx.arc(o.x, o.y, r, 0, TAU); ctx.fill();
        ctx.strokeStyle = LOOKS.srgb(B, o.a * .9); ctx.lineWidth = Math.max(.5, r * .12); ctx.lineCap = 'round';
        ctx.beginPath(); ctx.arc(o.x, o.y, r * .68, -2.5, -1.7); ctx.stroke();
      },
      /* A paper lantern: a warm body with a dark rim under it, and the glow it carries up. */
      lantern:function(ctx, o, r, A, B){
        glow(ctx, o.x, o.y, r * 1.1, A, B, o.a * .55);
        var g = ctx.createLinearGradient(0, o.y - r, 0, o.y + r); g.addColorStop(0, LOOKS.srgb(A, o.a)); g.addColorStop(.7, LOOKS.srgb(B, o.a)); g.addColorStop(1, LOOKS.srgb(B, o.a));
        ctx.fillStyle = g; ctx.beginPath(); ctx.moveTo(o.x - r * .5, o.y + r * .8); ctx.quadraticCurveTo(o.x - r * .95, o.y - r * .2, o.x - r * .45, o.y - r * .9);
        ctx.lineTo(o.x + r * .45, o.y - r * .9); ctx.quadraticCurveTo(o.x + r * .95, o.y - r * .2, o.x + r * .5, o.y + r * .8); ctx.closePath(); ctx.fill();
        ctx.fillStyle = LOOKS.srgb(A.map(function(v){ return v * .3; }), o.a * .85); ctx.fillRect(o.x - r * .5, o.y + r * .74, r, Math.max(.6, r * .16));
      },
      /* A wisp: a small cold flame with a bright heart, leaning as it wanders. */
      wisp:function(ctx, o, r, A, B, p, i, t){
        var lean = Math.sin(t * TAU / TEMPO.sway + h(i, 3.9) * TAU) * r * .5;
        for (var k = 3; k >= 1; k--) glow(ctx, o.x + lean * k * .4, o.y - k * r * .75, r * (1 - k * .2), A, A, o.a * .22 * (1 - k / 5));
        glow(ctx, o.x, o.y, r * 1.6, A, A, o.a * .3);
        glow(ctx, o.x, o.y, r * .7, A, B, o.a);
      },
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
      /* A set that is its own light (night) shows most in the dark: under a storm's darkness or at night. */
      var dim = .25 + .75 * Math.max(st.dark, clamp(st.wx.dark / .6, 0, 1) * .2);
      sets.forEach(function(p, si){
        var cnt = Math.min(left, Math.round(p.n * area)); left -= cnt;
        if (cnt <= 0 || p.a <= .01) return;
        var e = p.e || 0, role = e >= .5 ? 'glow' : 'body';
        var A = LOOKS.lit(p.c, role, sl), B = LOOKS.lit(p.c2 || p.c, role, sl);
        if (p.mode === 'vortex' && st.funnelAt){ p = Object.assign({}, p, {cx:st.funnelAt.x, gy:st.funnelAt.gy}); }
        var seed = seedOf(p, si), draw = DRAW[p.p] || DRAW.mote, clip = false;
        /* Fire on the wood's crowns stands behind the foreground ridge: it is cut off where that ridge rises over it. */
        if (p.mode === 'ridge' && F.ridgeAt){
          ctx.save(); ctx.beginPath(); ctx.moveTo(0, -1);
          for (var cx = 0; cx <= W + 4; cx += 4) ctx.lineTo(cx, F.ridgeAt(3, cx, W, H));
          ctx.lineTo(W + 4, -1); ctx.closePath(); ctx.clip(); clip = true;
        }
        for (var i = 0; i < cnt; i++){
          var o = particle(p, i, t, W, H, sc, wind, seed);
          o.a *= p.a * fogA * (.55 + .45 * o.z) * (p.night ? dim : 1);
          if (o.a <= .01 || o.x < -30 || o.x > W + 30 || o.y < -30 || o.y > H + 30) continue;
          var r = Math.max(H < 180 ? p.rmin || 0 : 0, (p.s[0] + (p.s[1] - p.s[0]) * h(i, seed + 8.8)) * sc * (.55 + .45 * o.z)) * (1 + .3 * sl.fog);
          var a = A, b = B;
          if (p.rb){ a = rainbow(i, t, sl); b = a.map(function(v){ return Math.min(1, v * 1.25 + .05); }); }
          draw(ctx, o, r, a, b, p, i, t, W, H, sc, wind, seed); n++;
        }
        if (clip) ctx.restore();
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
    /* A puff of smoke: like soft, but dense across its middle, so a column of them reads as body, not as glows. */
    function puff(ctx, x, y, rx, ry, c, a){
      ctx.save(); ctx.translate(x, y); ctx.scale(1, ry / rx);
      var g = ctx.createRadialGradient(0, 0, 0, 0, 0, rx);
      g.addColorStop(0, LOOKS.srgb(c, a)); g.addColorStop(.45, LOOKS.srgb(c, a * .8)); g.addColorStop(1, LOOKS.srgb(c, 0));
      ctx.fillStyle = g; ctx.beginPath(); ctx.arc(0, 0, rx, 0, TAU); ctx.fill(); ctx.restore();
    }
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

    /* ── Shapes a weather asks for (the look's `shapes`), placed from the sky itself: a rainbow stands opposite the sun,
       a halo rings the sun or the brightest moon, night clouds hang in the twilight, a rift and a far fire take their
       place from the land. Each becomes an item on the page's list, drawn with the rest of the far layer. ── */
    function dirOf(v){ return {alt:Math.asin(clamp(v[1], -1, 1)), az:Math.atan2(v[0], v[2])}; }
    function cross(a, b){ return [a[1] * b[2] - a[2] * b[1], a[2] * b[0] - a[0] * b[2], a[0] * b[1] - a[1] * b[0]]; }
    function unit(a){ var l = Math.hypot(a[0], a[1], a[2]) || 1; return [a[0] / l, a[1] / l, a[2] / l]; }
    /* The circle of sky `deg` degrees from the direction c, as points on the page; parts under the horizon are left out. */
    function ringPts(st, c, deg, n){
      var up = Math.abs(c[1]) > .98 ? [1, 0, 0] : [0, 1, 0], u = unit(cross(up, c)), w = cross(c, u), cr = Math.cos(deg * D2R), sr = Math.sin(deg * D2R), runs = [], run = null;
      for (var i = 0; i <= n; i++){
        var th = i / n * TAU, q = [0, 1, 2].map(function(k){ return cr * c[k] + sr * (Math.cos(th) * u[k] + Math.sin(th) * w[k]); }), d = dirOf(q);
        if (d.alt <= 0){ run = null; continue; }
        var pt = SW.PROJ.at(st.L, d.alt, d.az);
        /* A step right across the page is the panorama's seam behind the viewer, not a line to draw. */
        if (run && Math.abs(pt.x - run[run.length - 1][0]) > st.W * .4) run = null;
        if (!run){ run = []; runs.push(run); }
        run.push([pt.x, pt.y, smooth01(0, 5, d.alt / D2R)]);
      }
      return runs;
    }
    function strokeRuns(ctx, runs, col, a, lw){
      ctx.lineWidth = lw; ctx.lineCap = 'butt'; ctx.lineJoin = 'round';
      runs.forEach(function(run){
        for (var j = 0; j + 1 < run.length; j++){
          ctx.strokeStyle = LOOKS.srgb(col, a * Math.min(run[j][2], run[j + 1][2]));
          ctx.beginPath(); ctx.moveTo(run[j][0], run[j][1]); ctx.lineTo(run[j + 1][0], run[j + 1][1]); ctx.stroke();
        }
      });
    }
    var BOW = ['#ff4a4a', '#ff9a3a', '#ffe14a', '#5ad66a', '#4aa8ff', '#7a5cff'];
    SHAPES.rainbow = function(ctx, it, st){
      var px = (st.L.hor - st.L.top) / 90, band = Math.max(1, px * .5);
      BOW.forEach(function(c, k){ strokeRuns(ctx, ringPts(st, it.dir, 42.3 - k * .42, 72), PAL.hexLin(c), .62 * it.k, band * 1.25); });
      /* The second bow, fainter, wider and the other way round, with the dark band between them left alone. */
      BOW.forEach(function(c, k){ strokeRuns(ctx, ringPts(st, it.dir, 50.4 + k * .5, 72), PAL.hexLin(c), .17 * it.k, band * 1.4); });
      strokeRuns(ctx, ringPts(st, it.dir, 38.5, 72), [1, 1, 1], .05 * it.k, px * 6);
    };
    /* A halo is drawn round where its body stands on the page: the panorama stretches the sky sideways, and a ring
       that followed it would come out an oval, which no halo is. */
    SHAPES.sunring = function(ctx, it, st){
      var R = (st.L.hor - st.L.top) * .34, c = SW.PROJ.at(st.L, it.alt, it.az);
      [[R * .11, .09], [R * .035, .3]].forEach(function(q){ ctx.strokeStyle = LOOKS.srgb(it.col, q[1] * it.k); ctx.lineWidth = q[0]; ctx.beginPath(); ctx.arc(c.x, c.y, R, 0, TAU); ctx.stroke(); });
      ctx.strokeStyle = LOOKS.srgb(it.warm, .22 * it.k); ctx.lineWidth = Math.max(.8, R * .022); ctx.beginPath(); ctx.arc(c.x, c.y, R * .965, 0, TAU); ctx.stroke();
      /* Sun dogs: a bright patch on the ring at the sun's own height, either side, with a tail pointing away. */
      [-1, 1].forEach(function(sd){
        var x = c.x + sd * R, r = R * .11;
        soft(ctx, x + sd * r * 1.2, c.y, r * 2.6, r * .8, it.col, .22 * it.k * it.dogs);
        soft(ctx, x, c.y, r, r * .85, it.warm, .55 * it.k * it.dogs);
      });
    };
    /* A colour seen far off, as the painted far range is: part of the way into the horizon's colour, and darker at dusk
       and by night. */
    function farTone(st, c, k){
      var belt = smooth01(-8, -1, st.altEff) * (1 - smooth01(4, 10, st.altEff)) * (1 - st.wx.cloud);
      return PAL.scale(PAL.mixLin(c, st.P.hor, k), 1 - Math.max(st.dark, belt) * .25);
    }
    /* Lens clouds: smooth lenses stacked over a peak, lit on top by the clouds' own light and shaded under, their
       undersides warmed by a low sun. They stand still while the wind blows, which is what makes them uncanny. They are
       solid cloud: thick air hazes their colour toward the horizon's, never lets the sky through. A low sun just
       behind them lights their edges. */
    SHAPES.lens = function(ctx, it, st){
      var H = st.H, W = st.W, P = st.P, w0 = H * .62 * it.sz, hz = .22 + .45 * (1 - (it.vis != null ? it.vis : 1));
      var top = farTone(st, PAL.mixLin(P.clit, [1, 1, 1], .1), hz), mid = farTone(st, PAL.mixLin(P.clit, P.cshade, .45), hz);
      var low = farTone(st, PAL.mixLin(P.cshade, PAL.mixLin(P.glow, P.clit, .3), it.warm * .65), hz);
      var back = smooth01(-3, 2, st.altSun) * (1 - smooth01(8, 30, st.altSun)) * Math.exp(-Math.pow((it.x - st.sp.x) / (W * .2), 2)) * (1 - st.wx.cloud * .6);
      var rim = PAL.mixLin(P.rim, [1, 1, 1], .3);
      for (var k = 0; k < 4; k++){
        var w = w0 * (1 - k * .17) * (.92 + .16 * h(it.seed, k)), hh = H * .028 * it.sz * (1 - k * .08);
        var y = it.y - k * hh * 1.35, x = it.x + (h(it.seed, k + 3) - .5) * w * .18;
        soft(ctx, x, y, w * .62, hh * 2.2, mid, .14 * it.k);
        var g = ctx.createLinearGradient(0, y - hh * 1.2, 0, y + hh * .8);
        g.addColorStop(0, LOOKS.srgb(top, it.k)); g.addColorStop(.55, LOOKS.srgb(mid, it.k)); g.addColorStop(1, LOOKS.srgb(low, it.k));
        ctx.fillStyle = g; ctx.beginPath(); ctx.moveTo(x - w / 2, y); ctx.bezierCurveTo(x - w * .3, y - hh * 2.1, x + w * .3, y - hh * 2.1, x + w / 2, y);
        ctx.bezierCurveTo(x + w * .25, y + hh * 1.1, x - w * .25, y + hh * 1.1, x - w / 2, y); ctx.fill();
        if (back > .02){
          ctx.save(); ctx.globalCompositeOperation = 'lighter'; ctx.strokeStyle = LOOKS.srgb(rim, .45 * back * it.k); ctx.lineWidth = Math.max(.8, hh * .4);
          ctx.beginPath(); ctx.moveTo(x - w * .46, y - hh * .2); ctx.bezierCurveTo(x - w * .28, y - hh * 2, x + w * .28, y - hh * 2, x + w * .46, y - hh * .2); ctx.stroke(); ctx.restore();
        }
      }
    };
    /* An eruption beyond the far range: a dark cone whose foot the range hides, its crater glowing, lava thrown up in
       arcs and running down its flanks, and a plume of ash leaning with the wind, lit by the sun on one side and from
       below by the crater; now and then lightning in the plume. The glow is its own light, so it rules the night and
       is faint by day; the cone and the ash take the scene's light and haze like the range in front of them. */
    SHAPES.eruption = function(ctx, it, st, t){
      var H = st.H, P = st.P, x = it.x, base = it.y, ch = H * .22 * it.sz, py = base - ch, cw = ch * .16, wind = .3 + st.wx.wind;
      var lava = it.k * (.1 + .9 * it.night), bloom = it.k * (.1 + .9 * it.night), day = 1 - it.night, side = st.sp.x < x ? -1 : 1;
      var sunK = day * smooth01(-3, 8, st.altSun) * (1 - st.wx.cloud * .6);
      /* By day the cone is darker than the range in front (it is bare rock and ash) and hazes less, as it is so big. */
      var cone = farTone(st, PAL.scale(P.h0, .74 - .24 * day), .32 - .17 * day);
      var ashD = farTone(st, PAL.mixLin(P.cshade, [.05, .045, .04], .78), .18), ashL = farTone(st, PAL.mixLin(P.clit, [.42, .38, .34], .55), .22);
      /* Where the plume is at age u: rising, leaning with the wind, swaying a little. */
      function at(u, j){ return [x + Math.pow(u, 1.4) * H * .5 * wind + Math.sin(u * 7 + j) * H * .02, py - u * H * .75]; }
      /* The plume, oldest puffs first, so the newest sit low over the crater; each lit from below while it is young. A
         puff swells in at the crater rather than appearing there; by day the column is denser and wider. */
      for (var j = 13; j >= 0; j--){
        var u = (j / 14 + t / 46) % 1, pp = at(u, j), px2 = pp[0], py2 = pp[1];
        var rr = H * (.035 + .16 * u) * (1 + .4 * day), fade = it.k * smooth01(0, .05, u) * (1 - smooth01(.68, 1, u)), under = Math.exp(-u * 2.6);
        puff(ctx, px2, py2, rr * 1.4, rr * 1.15, PAL.mixLin(ashD, it.col, under * .5 * bloom), Math.min(1, (.82 + .3 * day) * fade));
        if (sunK > .02) soft(ctx, px2 + side * rr * .35, py2 - rr * .2, rr * .9, rr * .75, ashL, .38 * fade * sunK);
        if (under * bloom > .03) soft(ctx, px2, py2 + rr * .45, rr * .9, rr * .55, it.col, .55 * fade * under * bloom);
      }
      /* Lightning born of the ash, every few seconds, for a moment: a fine jagged channel down the plume's own path
         with one branch, a hot core in a sheath and a glow lighting the ash round it, as the sky's own bolts are. */
      var lc = Math.floor(t / 3.3), lu = t / 3.3 - lc;
      if (!st.rm && lu < .07 && h(lc, 4.2) < .7){
        var f = it.k * (1 - lu / .07), pts = [], n = 12, brn = [];
        for (var q = 0; q <= n; q++){
          var uq = .42 - .3 * q / n, pq = at(uq, 0), jx = q && q < n ? (h(lc * 13 + q, 1.7) - .5) * H * .035 : 0;
          pts.push([pq[0] + jx, pq[1]]);
        }
        var b0 = 4 + Math.floor(h(lc, 2.9) * 4), bd = h(lc, 3.7) < .5 ? -1 : 1;
        brn.push(pts[b0]);
        for (var q3 = 1; q3 <= 5; q3++){ var lp = brn[q3 - 1]; brn.push([lp[0] + bd * H * (.01 + .012 * h(lc + q3, 5.1)), lp[1] + H * (.012 + .01 * h(lc + q3, 6.3))]); }
        var mid = pts[Math.floor(n / 2)];
        soft(ctx, mid[0], mid[1], H * .14, H * .16, [.72, .78, 1], .32 * f);
        var lw = Math.max(.6, H * .0035);
        [[lw * 5, [.6, .66, 1], .14], [lw * 2.2, [.78, .84, 1], .45], [lw, [1, 1, 1], 1]].forEach(function(L2, li){
          ctx.strokeStyle = LOOKS.srgb(L2[1], L2[2] * f); ctx.lineWidth = L2[0]; ctx.lineCap = 'round'; ctx.lineJoin = 'round';
          ctx.beginPath(); pts.forEach(function(pq, qi){ if (qi) ctx.lineTo(pq[0], pq[1]); else ctx.moveTo(pq[0], pq[1]); }); ctx.stroke();
          ctx.lineWidth = L2[0] * .6; ctx.strokeStyle = LOOKS.srgb(L2[1], L2[2] * f * .7);
          ctx.beginPath(); brn.forEach(function(pq, qi){ if (qi) ctx.lineTo(pq[0], pq[1]); else ctx.moveTo(pq[0], pq[1]); }); ctx.stroke();
        });
      }
      /* The cone, carried down past the range's foot so no edge of it ever shows; its sunward flank lighter, with
         the ridge light the range has. */
      var g = ctx.createLinearGradient(x - ch, 0, x + ch, 0), c0 = PAL.scale(cone, 1 - .2 * side * sunK), c1 = PAL.scale(cone, 1 + .2 * side * sunK);
      g.addColorStop(0, LOOKS.srgb(c0, it.k)); g.addColorStop(1, LOOKS.srgb(c1, it.k));
      ctx.fillStyle = g; ctx.beginPath(); ctx.moveTo(x - ch * 4, H + 2); ctx.lineTo(x - ch * 2.4, base + H * .04);
      ctx.quadraticCurveTo(x - ch * .5, base - ch * .55, x - cw, py); ctx.lineTo(x + cw, py);
      ctx.quadraticCurveTo(x + ch * .5, base - ch * .55, x + ch * 2.4, base + H * .04); ctx.lineTo(x + ch * 4, H + 2); ctx.closePath(); ctx.fill();
      ctx.save(); ctx.globalCompositeOperation = 'lighter'; ctx.strokeStyle = LOOKS.srgb(P.rim, .16 * sunK * it.k); ctx.lineWidth = Math.max(.8, H * .008);
      ctx.beginPath(); ctx.moveTo(x + side * cw, py); ctx.quadraticCurveTo(x + side * ch * .5, base - ch * .55, x + side * ch * 1.6, base - ch * .05); ctx.stroke(); ctx.restore();
      /* Lava down the flanks, flickering. */
      ctx.lineCap = 'round';
      for (var fl = 0; fl < 3; fl++){
        var sd = fl === 1 ? -1 : 1, fk = .7 + .3 * Math.sin(t * TAU / TEMPO.flick + fl * 2);
        ctx.strokeStyle = LOOKS.srgb(it.col, lava * fk * .9); ctx.lineWidth = Math.max(.8, H * .006);
        ctx.beginPath(); ctx.moveTo(x + sd * cw * .5, py + 1);
        for (var q2 = 1; q2 <= 4; q2++) ctx.lineTo(x + sd * ch * (.08 + q2 * (.12 + .08 * fl)) + (h(fl, q2) - .5) * ch * .1, py + q2 * ch * (.17 + .04 * fl));
        ctx.stroke();
      }
      soft(ctx, x, py, ch * .7, ch * .35, it.col, .7 * bloom);
      soft(ctx, x, py + 1, cw * 1.6, cw * .7, it.hi, lava);
      /* Lava thrown up: each bomb rises and falls on its own arc and cools as it goes. */
      for (var b = 0; b < 22; b++){
        var per = 1.5 + 1.3 * h(b, 1.1), uu = ((t + h(b, 2.2) * per) / per) % 1, vx = (h(b, 3.3) - .5) * ch * 1.3, vy = ch * (.7 + .8 * h(b, 4.4));
        var ba = lava * (1 - uu) * (1 - uu), br = Math.max(1, H * .007);
        if (ba > .02) soft(ctx, x + vx * uu, py - vy * uu + vy * uu * uu, br, br, it.hi, ba);
      }
    };
    /* Sky leviathans: great shapes swimming slowly across far off, the farther the paler and the more lost in the air
       (their colour hazes toward the sky's; their bodies stay solid, so the sun never shows through one), lit a little
       on their backs by day; by night faint lights along their flanks. */
    SHAPES.leviathan = function(ctx, it, st, t){
      var W = st.W, H = st.H, L = st.L, P = st.P, night = it.night;
      [.38, .62, 1].forEach(function(z, n){
        var len = H * .62 * z * it.sz, sp = H * .05 * z, span = W + len * 2, x0 = (.3 + .33 * n + .1 * h(it.seed, n + 1)) * span;
        var x = ((x0 + sp * t) % span + span) % span - len, y = L.top + (L.hor - L.top) * (.28 + .32 * h(it.seed, n + 5)) + Math.sin(t * TAU / (TEMPO.drift * 1.3) + n * 2) * H * .02;
        var hz0 = .78 * (1 - z) + .12, haze = hz0 + (.92 - hz0) * st.wx.fog * .7, c = PAL.mixLin(it.col, P.mid, haze), back = PAL.mixLin(c, P.clit, .2 * (1 - night)), bh = len * .2, beat = Math.sin(t * TAU / (TEMPO.breath * 1.1) + n * 1.7) * .32;
        var a = it.k, g = ctx.createLinearGradient(0, -bh * .7, 0, bh * .6);
        g.addColorStop(0, LOOKS.srgb(back, a)); g.addColorStop(1, LOOKS.srgb(PAL.scale(c, .85), a));
        ctx.save(); ctx.translate(x, y); ctx.rotate(Math.sin(t * TAU / (TEMPO.drift * 1.3) + n * 2 + 1.2) * .05);
        ctx.fillStyle = g;
        ctx.beginPath(); ctx.moveTo(len * .5, bh * .08);
        ctx.bezierCurveTo(len * .44, -bh * .62, len * .05, -bh * .72, -len * .3, -bh * .2);
        ctx.quadraticCurveTo(-len * .42, -bh * .04, -len * .47, 0);
        ctx.quadraticCurveTo(-len * .4, bh * .2, -len * .28, bh * .24);
        ctx.bezierCurveTo(len * .05, bh * .64, len * .44, bh * .5, len * .5, bh * .08); ctx.fill();
        /* The flukes beat about the tail's root; a long fin strokes slowly under the body. */
        ctx.save(); ctx.translate(-len * .46, 0); ctx.rotate(beat);
        ctx.beginPath(); ctx.moveTo(0, 0); ctx.quadraticCurveTo(-len * .08, -bh * .2, -len * .17, -bh * .62); ctx.quadraticCurveTo(-len * .1, -bh * .1, -len * .06, 0);
        ctx.quadraticCurveTo(-len * .1, bh * .1, -len * .17, bh * .62); ctx.quadraticCurveTo(-len * .08, bh * .2, 0, 0); ctx.fill(); ctx.restore();
        ctx.save(); ctx.translate(len * .16, bh * .38); ctx.rotate(.5 + beat * .6);
        ctx.beginPath(); ctx.moveTo(0, 0); ctx.quadraticCurveTo(-len * .06, bh * .5, -len * .2, bh * .7); ctx.quadraticCurveTo(-len * .08, bh * .25, -len * .04, -bh * .05); ctx.fill(); ctx.restore();
        if (night > .05) for (var q = 0; q < 7; q++){
          var pu = .6 + .4 * Math.sin(t * TAU / TEMPO.breath + q * .9 + n), lr = Math.max(1.4, H * .014 * z);
          soft(ctx, len * (.36 - q * .1), bh * (.12 + .05 * Math.sin(q)), lr, lr, it.hi, night * pu * (.5 + .5 * z) * it.k);
        }
        ctx.restore();
      });
    };
    /* The n highest true peaks of a ridge in the middle of the view (each the top of the ridge for a twentieth of the
       width either side), at least a fifth of the width apart; a ridge with too few peaks lends its highest points. */
    function peaks(st, i, n){
      var W = st.W, H = st.H, out = [], pts = [], step = 3, win = Math.max(1, Math.round(W * .05 / step));
      for (var x = W * .06; x <= W * .94; x += step) pts.push([x, F.ridgeAt ? F.ridgeAt(i, x, W, H) : st.L.hor]);
      var inner = pts.filter(function(p){ return p[0] >= W * .1 && p[0] <= W * .9; });
      var tops = pts.filter(function(p, j){
        if (p[0] < W * .1 || p[0] > W * .9) return false;
        for (var d = -win; d <= win; d++){ var q = pts[j + d]; if (q && q[1] < p[1]) return false; }
        return true;
      });
      var byY = function(a, b){ return a[1] - b[1]; }, far = function(p){ return out.every(function(o){ return Math.abs(o[0] - p[0]) > W * .2; }); };
      tops.sort(byY).forEach(function(p){ if (out.length < n && far(p)) out.push(p); });
      inner.sort(byY).forEach(function(p){ if (out.length < n && far(p)) out.push(p); });
      return out;
    }
    F.peaks = peaks;
    F.lookItems = function(st){
      var R = st.look, sl = st.sl, W = st.W, H = st.H, L = st.L;
      if (!R || !R.shapes || !R.shapes.length) return;
      var clear = (1 - st.wx.cloud * .7) * (1 - st.wx.fog * .5), sunAlt = st.sun.alt / D2R, night = smooth01(.25, .75, st.dark);
      R.shapes.forEach(function(q, n){
        if (q.k <= .01) return;
        if (q.s === 'rainbow'){
          /* A bow needs a low sun behind the viewer: none at night, none under a high sun. */
          var k = q.k * smooth01(-1, 4, sunAlt) * (1 - smooth01(36, 42, sunAlt));
          if (k > .01) st.fxItems.push({shape:'rainbow', dir:st.sun.v.map(function(v){ return -v; }), k:k, still:true});
        } else if (q.s === 'sunring'){
          var body = sunAlt > -2 ? {v:st.sun.v, k:1, dogs:1} : null;
          if (!body) st.moons.forEach(function(M){ if (M.up && M.m.alt > 0 && (!body || M.m.lit > body.k)) body = {v:M.m.v, k:M.m.lit, dogs:.4}; });
          if (body && body.k > .2){ var bd = dirOf(body.v); st.fxItems.push({shape:'sunring', alt:bd.alt, az:bd.az, k:q.k * clamp(body.k, 0, 1), dogs:body.dogs, col:LOOKS.lit(q.c || '#f4f7ff', 'glow', sl), warm:LOOKS.lit('#ffd9b0', 'glow', sl), still:true}); }
        } else if (q.s === 'nightcloud'){
          /* Night-shining clouds: seen only in deep twilight and after, low over where the sun went. */
          for (var j = 0; j < 7; j++) st.fxItems.push({shape:'streak', x:W * (.1 + .13 * j + .05 * h(j, 3.1)), y:L.top + (L.hor - L.top) * (.56 + .22 * h(j, 5.7) + .05 * (j % 2)), sz:.42 + .34 * h(j, 7.3),
            k:q.k * night * clear * (.55 + .45 * h(j, 9.9)), col:LOOKS.lit(q.c, 'glow', sl), hi:LOOKS.lit('#eaf6ff', 'glow', sl), seed:11 + j * 3.7});
        } else if (q.s === 'rift'){
          st.fxItems.push({shape:'doorway', x:W * .5, y:L.top + (L.hor - L.top) * .5, sz:1.25, open:1, k:q.k, col:LOOKS.lit(q.c, 'glow', sl), hi:LOOKS.lit('#f4ecff', 'glow', sl), seed:5 + n});
        } else if (q.s === 'hglow'){
          st.fxItems.push({shape:'glow', x:W * .34, y:L.hor, sz:1.3, k:q.k * (.45 + .55 * night), col:LOOKS.lit(q.c, 'glow', sl), hi:LOOKS.lit('#ffd27a', 'glow', sl), seed:3 + n});
        } else if (q.s === 'lens'){
          /* Over the two highest peaks of the far range, warm underneath while the sun is low, kept whole inside a
             narrow pane. Their strength is the change of weather's alone; cloud and fog haze them instead (vis). */
          var warm = smooth01(12, 1, sunAlt) * smooth01(-8, -1.5, sunAlt);
          peaks(st, 0, 2).forEach(function(pt, j){
            var sz = j ? .72 : 1, hw = H * .31 * sz + 4, lx = W > hw * 2 ? clamp(pt[0], hw, W - hw) : W / 2;
            st.fxItems.push({shape:'lens', x:lx, y:pt[1] - H * (.09 + .03 * j), sz:sz, k:q.k, vis:clear, warm:warm, seed:7 + j * 5, still:true});
          });
        } else if (q.s === 'eruption'){
          /* On the far ridge; the sky over it glows with the crater by night. */
          var ex = W * .7, ey = (F.ridgeAt ? F.ridgeAt(0, ex, W, H) : L.hor) + H * .02;
          st.fxItems.push({shape:'glow', x:ex, y:ey - H * .2, sz:1.1, k:q.k * (.06 + .94 * night), col:LOOKS.lit(q.c, 'glow', sl), hi:LOOKS.lit('#ffb347', 'glow', sl), seed:9});
          st.fxItems.push({shape:'eruption', x:ex, y:ey, sz:1.25, k:q.k, night:night, col:LOOKS.lit(q.c, 'glow', sl), hi:LOOKS.lit('#ffd27a', 'glow', sl)});
        } else if (q.s === 'leviathan'){
          st.fxItems.push({shape:'leviathan', sz:1, k:q.k, night:night, col:LOOKS.lit(q.c, 'shade', sl), hi:LOOKS.lit('#8ff0ff', 'glow', sl), seed:13});
        }
      });
    };

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

    /* Shapes built of overlapping parts fade as one, so a cone half faded in never shows the plume through it: drawn
       whole on a scratch canvas, then laid in at their strength. */
    var WHOLE = {lens:1, eruption:1, leviathan:1}, SCRATCH = null;
    function whole(ctx, it, st, t, fn){
      var cv = ctx.canvas, sc = SCRATCH || (SCRATCH = document.createElement('canvas'));
      if (sc.width !== cv.width || sc.height !== cv.height){ sc.width = cv.width; sc.height = cv.height; }
      var g = sc.getContext('2d');
      g.setTransform(1, 0, 0, 1, 0, 0); g.clearRect(0, 0, sc.width, sc.height); g.setTransform(ctx.getTransform());
      fn(g, Object.assign({}, it, {k:1}), st, t);
      ctx.save(); ctx.setTransform(1, 0, 0, 1, 0, 0); ctx.globalAlpha *= it.k; ctx.drawImage(sc, 0, 0); ctx.restore();
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
        var fn = SHAPES[it.shape]; if (fn){ if (WHOLE[it.shape] && it.k < .995) whole(ctx, it, st, t, fn); else fn(ctx, it, st, t); n++; }
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
