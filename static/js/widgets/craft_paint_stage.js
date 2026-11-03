/**
 * craft_paint_stage.js -- runs a painted bench (CraftStage) and registers the
 * craft_paint widget
 *
 * A painted scene (CraftPaint.define) is drawn into an offscreen canvas, then
 * finished at one of four effect levels:
 *   gpu    one pass on the page's shared WebGL2 context (gl_shared.js, the
 *          one the skies paint with): heat haze, a shockwave off each blow,
 *          colour fringes on the brightest light, bloom from the blurred
 *          mips, and rays traced toward the window's light
 *   canvas a small blurred copy of the bright parts laid back as bloom, rays
 *          from it, and a streak on each blow
 *   basic  the scene alone
 *   still  one frame and a Play button
 * Every level keeps the baked fire. It starts at gpu (canvas on a device that
 * looks weak, or one the page already found slow), and steps down a level
 * after about two seconds of slow frames. The page keeps one slow-device
 * verdict (SharedGL): a stage leaving gpu for slowness turns the skies basic,
 * and a sky found slow moves a stage off gpu.
 * Only one stage animates at a time, it pauses off screen and in a hidden tab,
 * drawing is capped at twice the screen's pixel density, Calm motion waits for
 * Play and Off shows the still frame. craft_paint.ai.md is the guide.
 */
window.CraftStage = window.CraftStage || (function () {
  'use strict';
  var W = 640, H = 360;
  var LEVELS = ['gpu', 'canvas', 'basic', 'still'];
  var NAMES = ['Graphics card', 'Canvas glow', 'Plain canvas', 'Still frame'];
  // A frame slower than SLOW_MS on average (about 40 fps) for SLOW_S seconds steps the level down.
  var SLOW_MS = 24, SLOW_S = 2, WARM = 30;

  // 'off', 'calm' or 'full', from the device setting and Chronicle's own motion choices.
  function motionMode(root, reduce) {
    if (root.getAttribute('data-motion') === 'off') return 'off';
    if (reduce || root.getAttribute('data-motion') === 'calm' || root.getAttribute('data-view-motion') === 'calm' || root.hasAttribute('data-cz-reduce')) return 'calm';
    return 'full';
  }
  // A device with little memory, few cores or data saver on starts at canvas rather than gpu.
  function startLevel(nav) {
    nav = nav || {};
    var weak = (nav.deviceMemory && nav.deviceMemory <= 4) || (nav.connection && nav.connection.saveData) || (nav.hardwareConcurrency && nav.hardwareConcurrency <= 4);
    return weak ? 1 : 0;
  }
  // Watches the gap between frames. tick(ms) is true once frames have run slow for long enough.
  function governor() {
    var o = { ema: 16.7, slowFor: 0, warm: WARM };
    o.tick = function (gap) {
      if (o.warm > 0) { o.warm--; return false; }
      gap = Math.min(100, gap);
      o.ema = o.ema * .9 + gap * .1;
      if (o.ema > SLOW_MS) o.slowFor += gap / 1000; else o.slowFor = Math.max(0, o.slowFor - gap / 2000);
      return o.slowFor > SLOW_S;
    };
    o.reset = function () { o.ema = 16.7; o.slowFor = 0; o.warm = WARM; };
    return o;
  }
  function levelOf(name) { var i = LEVELS.indexOf(String(name || '')); return i; }
  function now() { return performance.now(); }

  /* ---- the window's sky ---- */
  // The calendar's sky code paints the window from the campaign's calendar as players see it
  // (no hidden moons), or from a demo calendar; without the sky scripts the scene's drawn night stays.
  var DEMO = { name: 'demo', mode: 'fantasy', hours_per_day: 24, minutes_per_hour: 60, leap_year_every: 0, leap_year_offset: 0, hemisphere: 'north',
    months: ['Thaw', 'Seedtime', 'Rains', 'Blossom', 'Highsun', 'Harvest', 'Leaffall', 'Mists', 'Frost', 'Deepwinter', 'Longnight', 'Waking'].map(function (n) { return { name: n, days: 30 }; }),
    moons: [{ id: 1, name: 'The Lantern', cycle_days: 30, phase_offset: 4, color: '#e5e7eb', size: 1, phase_mode: 'auto' }],
    current_year: 100, current_month: 9, current_day: 8, current_hour: 21, current_minute: 40, weather: null };
  // ready() hears when a calendar read from the server arrives, so a paused stage can repaint its window.
  function Sky(src, calm, ready) {
    var s = { cv: null, light: null, data: null, model: null, key: '', last: -1, lastLight: -1, dead: false, hold: {} };
    function getJSON(u) { return window.SkyPane.Scene.getJSON(u); }
    function load() {
      if (!window.SkyPane || !window.SkyPane.Scene) return;
      if (src.calendar) { s.data = { calendar: src.calendar, weather: src.calendar.weather || null, seed: src.calendar.name || 'sky' }; return; }
      if (!src.campaignId || !src.calendarId) return;
      var base = '/campaigns/' + encodeURIComponent(src.campaignId) + '/calendars/' + encodeURIComponent(src.calendarId);
      getJSON(base + '?audience=players').then(function (cal) {
        cal.moons = (cal.moons || []).filter(function (x) { return x && !x.hidden_from_players; });
        return getJSON(base + '/weather/days?year=' + cal.current_year + '&month=' + cal.current_month + '&audience=players').then(function (days) {
          days = Array.isArray(days) ? days : (days && days.data) || [];
          return days.filter(function (x) { return x && x.year === cal.current_year && x.month === cal.current_month && x.day === cal.current_day; })[0] || null;
        }, function () { return null; }).then(function (wx) { if (s.dead) return; s.data = { calendar: cal, weather: wx || cal.weather || null, seed: src.calendarId }; if (ready) ready(); });
      }).catch(function (err) { if (window.console) console.warn('[craft-paint] the calendar could not be read', err); });
    }
    // Paints at most ten times a second into a canvas twice the window's size; rich uses the
    // painted (GPU) sky where the calendar's sky code has one, otherwise its basic sky.
    s.paint = function (t, win, rich) {
      var S = window.SkyPane && window.SkyPane.Scene;
      if (!S || !window.SkyWorld || !window.Sky2D || !s.data) return null;
      var w = win[2] * 2, h = win[3] * 2;
      if (!s.cv) { s.cv = document.createElement('canvas'); s.cv.width = w; s.cv.height = h; }
      var key = s.data.seed + '|' + rich;
      if (key !== s.key) {
        s.key = key; s.last = -1; s.lastLight = -1;
        s.model = { calendar: Object.assign({}, s.data.calendar), todayEvents: [], dayWeather: s.data.weather, skym: window.SkyWorld.makeSkym(s.data.calendar),
          landSeed: S.seed(String(s.data.seed)), width: win[2], height: win[3], reduced: calm, glide: null };
      }
      if (s.last >= 0 && t - s.last < .1) return s;
      var dt = s.last < 0 ? 0 : Math.min(1, t - s.last); s.last = t;
      try {
        var g = s.cv.getContext('2d'), st = S.state(s.model, calm ? 0 : t, dt);
        g.setTransform(1, 0, 0, 1, 0, 0); g.clearRect(0, 0, w, h);
        if (rich) S.paint(g, s.hold, st, 2, 0, true); else window.Sky2D.draw(g, st, 2, 0);
      } catch (e) { return null; }
      if (s.lastLight < 0 || t - s.lastLight > .5) {
        s.lastLight = t;
        // The sky's average colour sets the light through the window: its tint, strength and how much is daylight.
        var one = s.one || (s.one = document.createElement('canvas')), og;
        one.width = one.height = 1; og = one.getContext('2d', { willReadFrequently: true });
        og.clearRect(0, 0, 1, 1); og.drawImage(s.cv, 0, 0, 1, 1);
        var d = og.getImageData(0, 0, 1, 1).data, m = Math.max(d[0], d[1], d[2], 1), lum = (d[0] * .3 + d[1] * .59 + d[2] * .11) / 255;
        s.light = { c: [d[0] / m * 230, d[1] / m * 230, d[2] / m * 230], k: .9 + lum * 2.4, day: Math.max(0, lum - .15) };
      }
      return s;
    };
    s.destroy = function () { s.dead = true; };
    load();
    return s;
  }

  /* ---- the finishing passes ---- */
  function finishCanvas(cv, base, look, basic) {
    var g = cv.getContext('2d'), small = document.createElement('canvas'), blur = document.createElement('canvas');
    small.width = blur.width = 160; small.height = blur.height = 90;
    var sg = small.getContext('2d'), bg = blur.getContext('2d'), hasFilter = 'filter' in sg, lastHits = base.sc.hits || 0, streak = 0;
    var L = look.light, X = look.hit;
    return { frame: function (t, dt) {
      g.setTransform(1, 0, 0, 1, 0, 0); g.drawImage(base.cv, 0, 0, cv.width, cv.height);
      if (basic) return;
      var k = cv.width / W; g.setTransform(k, 0, 0, k, 0, 0);
      // Bloom: a small copy of only the bright parts, blurred and laid back over the frame in light.
      if (hasFilter) {
        sg.filter = 'brightness(.5) contrast(3.2) saturate(1.4)'; sg.clearRect(0, 0, 160, 90); sg.drawImage(base.cv, 0, 0, 160, 90);
        bg.filter = 'blur(3px)'; bg.clearRect(0, 0, 160, 90); bg.drawImage(small, 0, 0);
        g.save(); g.globalCompositeOperation = 'lighter'; g.globalAlpha = .55; g.drawImage(blur, 0, 0, W, H);
        // Rays: the bright copy stretched away from the window's light a few times, fading.
        if (L) for (var i = 1; i <= 5; i++) { var z = 1 + i * .06; g.globalAlpha = .07 * (6 - i) / 5; g.drawImage(blur, L[0] - L[0] * z, L[1] - L[1] * z, W * z, H * z); }
        g.restore();
      }
      // A streak of light along each blow.
      if ((base.sc.hits || 0) !== lastHits) { lastHits = base.sc.hits; streak = 1; }
      if (streak > 0 && X) {
        streak -= dt * 3.5; g.save(); g.globalCompositeOperation = 'lighter';
        var lg = g.createLinearGradient(X[0] - 180, 0, X[0] + 180, 0);
        lg.addColorStop(0, 'rgba(255,200,140,0)'); lg.addColorStop(.5, 'rgba(255,230,190,' + Math.max(0, streak) * .5 + ')'); lg.addColorStop(1, 'rgba(255,200,140,0)');
        g.fillStyle = lg; g.fillRect(X[0] - 180, X[1] - 1, 360, 3); g.restore();
      }
    } };
  }
  /* The graphics-card finish runs on the page's one WebGL2 context (gl_shared.js), shared with the skies: it draws
     into that canvas's lower-left corner and the picture is copied into this stage's own 2D canvas at once. */
  var FS = ['#version 300 es', 'precision highp float;', 'out vec4 o;',
    'uniform sampler2D T;uniform float t,age;uniform vec2 uRes,hit,L;uniform vec4 hz0,hz1;',
    'float h(vec2 p){return fract(sin(dot(p,vec2(127.1,311.7)))*43758.5453);}',
    'float n(vec2 p){vec2 i=floor(p),f=fract(p);f=f*f*(3.-2.*f);return mix(mix(h(i),h(i+vec2(1,0)),f.x),mix(h(i+vec2(0,1)),h(i+vec2(1,1)),f.x),f.y);}',
    'float box(vec2 p,vec4 r){return smoothstep(r.x,r.x+.04,p.x)*smoothstep(r.z,r.z-.04,p.x)*smoothstep(r.y,r.y+.06,p.y)*smoothstep(r.w,r.w-.06,p.y);}',
    'vec3 bright(vec3 c,float k){return max(c-k,0.);}',
    'void main(){',
    // p runs from the top-left, like the scene; a is the frame's shape, so rings stay round.
    ' vec2 uv=gl_FragCoord.xy/uRes,p=vec2(uv.x,1.-uv.y),a=vec2(16./9.,1.);',
    // Heat haze: the pixels behind the hot places are bent by rising noise.
    ' float m=box(p,hz0)+box(p,hz1)*.6;',
    ' vec2 off=vec2(n(p*vec2(38.,14.)+vec2(0.,t*3.))-.5,n(p*vec2(30.,10.)+vec2(5.,t*2.4))-.5)*.006*m;',
    // A shockwave ring off each blow.
    ' float d=length((p-hit)*a),r=age*.55,band=exp(-pow((d-r)/.018,2.))*max(0.,1.-age*1.6);',
    ' off+=normalize((p-hit)*a+1e-5)*band*.012;',
    ' vec2 q=p+off;vec3 c=texture(T,q).rgb;',
    // Colour fringes on only the brightest light.
    ' float lum=dot(c,vec3(.3,.59,.11));vec2 cd=(p-.5)*.004*smoothstep(.6,1.,lum);',
    ' c.r=mix(c.r,texture(T,q+cd).r,.7);c.b=mix(c.b,texture(T,q-cd).b,.7);',
    // Bloom gathered from the blurred mip levels.
    ' vec3 b=bright(textureLod(T,q,2.).rgb,.55)*.5+bright(textureLod(T,q,3.).rgb,.5)*.7+bright(textureLod(T,q,4.).rgb,.45)*.9+bright(textureLod(T,q,5.).rgb,.4)*1.1;',
    // Rays: march from this pixel toward the window's light, gathering its cool light.
    ' vec3 g=vec3(0.);vec2 st=(L-p)/24.;vec2 pp=p;float w=1.;',
    ' for(int i=0;i<24;i++){pp+=st;g+=bright(textureLod(T,pp,3.).rgb,.35)*vec3(.7,.8,1.)*w;w*=.93;}',
    ' g*=.05*smoothstep(L.x-.52,L.x-.07,p.x);',
    ' c+=b*.85+g+(h(p*900.+t)-.5)*.025;',
    ' o=vec4(c,1.);}'].join('\n');
  // null where there is no shared context (no WebGL2, a software-only browser, a device already found slow) or the
  // shader will not build, so the stage starts a level lower. failed is set when the first copy comes out empty.
  function finishGPU(cv, base, look) {
    var GS = window.SharedGL, C = GS && !GS.isSlow() ? GS.context() : null;
    if (!C || C.lost) return null;
    var gl = C.gl, g = cv.getContext('2d'), P = null, tex = null, gen = -1, checked = false;
    var X = look.hit || [-9, -9], L = look.light || [W, 0], hz = (look.haze || []).concat([[0, 0, 0, 0], [0, 0, 0, 0]]);
    function build() {
      P = GS.program(gl, FS, 'craft-paint'); GS.finish(gl, P);
      tex = gl.createTexture(); gl.bindTexture(gl.TEXTURE_2D, tex);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR); gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE); gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      gen = C.gen;
    }
    try { build(); } catch (e) { if (window.console) console.warn('[craft-paint] the graphics-card finish could not start', e); return null; }
    var lastHits = base.sc.hits || 0, age = 9;
    function plain() { g.setTransform(1, 0, 0, 1, 0, 0); g.drawImage(base.cv, 0, 0, cv.width, cv.height); }
    var o = { failed: false, frame: function (t, dt) {
      var w = cv.width, hh = cv.height;
      if ((base.sc.hits || 0) !== lastHits) { lastHits = base.sc.hits; age = 0; }
      age += dt;
      // While the context is lost the plain scene shows; after a restore the program and texture are made again.
      if (C.lost || gl.isContextLost()) return plain();
      if (gen !== C.gen) { try { build(); } catch (e) { o.failed = true; return plain(); } }
      GS.fit(w, hh); gl.viewport(0, 0, w, hh); gl.useProgram(P.p);
      gl.activeTexture(gl.TEXTURE0); gl.bindTexture(gl.TEXTURE_2D, tex);
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, base.cv); gl.generateMipmap(gl.TEXTURE_2D);
      GS.setU(gl, P, 'T', 0); GS.setU(gl, P, 'uRes', [w, hh]); GS.setU(gl, P, 't', t); GS.setU(gl, P, 'age', age);
      GS.setU(gl, P, 'hit', [X[0] / W, X[1] / H]); GS.setU(gl, P, 'L', [L[0] / W, L[1] / H]);
      GS.setU(gl, P, 'hz0', [hz[0][0] / W, hz[0][1] / H, hz[0][2] / W, hz[0][3] / H]); GS.setU(gl, P, 'hz1', [hz[1][0] / W, hz[1][1] / H, hz[1][2] / W, hz[1][3] / H]);
      gl.disable(gl.BLEND); gl.drawArrays(gl.TRIANGLES, 0, 3);
      g.setTransform(1, 0, 0, 1, 0, 0); g.drawImage(C.canvas, 0, C.canvas.height - hh, w, hh, 0, 0, w, hh);
      // The scene is opaque, so a clear pixel in the first copy means this browser can't hand the picture over.
      if (!checked) {
        checked = true;
        var px = null; try { px = g.getImageData(w >> 1, hh >> 1, 1, 1).data; } catch (e) { /* unreadable: trust the copy */ }
        if (px && px[3] === 0) { o.failed = true; plain(); }
      }
    } };
    return o;
  }

  /* ---- the stage ---- */
  var stages = [], active = null, raf = 0, lastT = 0;
  function loop(n) {
    raf = 0;
    var st = active;
    if (!st || !st.vis || document.hidden) { lastT = 0; return; }
    var gap = lastT ? n - lastT : 0, dt = lastT ? Math.min(.05, gap / 1000) : .016;
    lastT = n; st.t += dt;
    var a = now();
    draw(st, dt);
    if (st.burn) { var end = now() + 30; while (now() < end) { /* the Design Lab's slow-computer switch */ } }
    st.ms = st.ms * .92 + (now() - a) * .08; st.fpsN++;
    if (n - st.fpsT > 500) { st.fps = st.fpsN * 1000 / (n - st.fpsT); st.fpsN = 0; st.fpsT = n; readout(st); }
    if (gap && st.auto && st.gov.tick(gap)) {
      st.log.push(NAMES[st.level] + ' ran at ' + Math.round(1000 / st.gov.ema) + ' frames a second, so it stepped down.');
      // One verdict for the page: too slow for the graphics card here turns the skies basic too (not for the Lab's pretend).
      if (st.level === 0 && !st.burn && window.SharedGL) SharedGL.markSlow();
      setLevel(st, st.level + 1);
      if (st.level === 3) { pause(st, 'Paused to keep this page smooth. Play anyway'); return; }
    }
    raf = requestAnimationFrame(loop);
  }
  function kick() { if (!raf && active) { lastT = 0; raf = requestAnimationFrame(loop); } }
  function draw(st, dt) {
    var sc = st.base.sc, look = sc.look || {}, sky = look.window ? st.sky.paint(st.t, look.window, st.level === 0) : null;
    if (sky) { sc.sky = sky.cv; sc.skyLight = sky.light; }
    sc.rich = st.level <= 1;
    sc.frame(st.t, dt);
    st.eng.frame(st.t, dt);
    if (st.eng.failed) { st.log.push('The graphics card could not hand the picture over here, so it moved to ' + NAMES[1] + '.'); setLevel(st, 1); }
  }
  // The stage's own canvas is always 2D: the graphics-card level paints on the shared context and copies in.
  function setLevel(st, n) {
    var look = st.base.sc.look || {};
    st.eng = n === 0 ? finishGPU(st.cv, st.base, look) : null;
    if (n === 0 && !st.eng) {
      st.log.push((window.SharedGL && SharedGL.isSlow() ? 'This device was already found slow for graphics-card effects' : 'There is no graphics-card painter here') + ', so it starts at ' + NAMES[1] + '.');
      n = 1;
    }
    if (!st.eng) st.eng = finishCanvas(st.cv, st.base, look, n >= 2);
    st.level = n; st.gov.reset(); st.host.setAttribute('data-level', LEVELS[n]);
    draw(st, .016); readout(st);
  }
  function cover(st, text) {
    st.coverBtn.hidden = !text; st.pauseBtn.hidden = !!text || st.mode === 'off';
    if (text) st.coverBtn.querySelector('span').lastChild.textContent = ' ' + text;
  }
  function start(st) {
    if (st.dead || st.mode === 'off') return;
    if (active && active !== st) pause(active, 'Play ' + active.name);
    // Play on the still frame means run anyway: the plainest moving level, with no more stepping down.
    if (st.level === 3) { st.auto = false; setLevel(st, 2); }
    st.userPaused = false; active = st; cover(st, null);
    st.fpsT = now(); st.fpsN = 0; st.host.setAttribute('data-running', 'true'); kick(); readout(st);
  }
  function pause(st, text) {
    if (active === st) active = null;
    st.host.setAttribute('data-running', 'false');
    cover(st, st.mode === 'off' ? null : text || 'Play ' + st.name); readout(st);
  }
  // When a stage comes into view and nothing else is playing, it plays, unless motion is reduced or
  // the viewer paused it. A stage leaving the view lets the next one in view take over.
  // It waits a turn, so every stage's view report from the same scroll is in before one is picked, and the
  // pick is the first in the page's order, whichever report came first.
  var settling = 0;
  function settle() {
    if (settling) return;
    settling = setTimeout(function () {
      settling = 0;
      if (active) return;
      var next = stages.filter(function (s) { return s.vis && !s.userPaused && s.mode === 'full' && s.level < 3 && !s.dead; })
        .sort(function (a, b) { return a.box.compareDocumentPosition(b.box) & 4 ? -1 : 1; })[0];
      if (next) start(next);
    }, 0);
  }
  function readout(st) {
    if (!st.read) return;
    var bits = ['Level: ' + NAMES[st.level] + (st.auto ? '' : ' (chosen)')];
    if (active === st) bits.push(st.ms.toFixed(1) + ' ms of work a frame', Math.round(st.fps) + ' frames a second'); else bits.push('paused');
    st.read.textContent = bits.join(' · ');
    st.logEl.innerHTML = '';
    (st.log.length ? st.log : ['No steps down yet.']).forEach(function (x) { var li = document.createElement('li'); li.textContent = x; st.logEl.appendChild(li); });
    st.labBtns.forEach(function (b) { b.setAttribute('aria-pressed', String(b.getAttribute('data-l') === (st.auto ? 'auto' : LEVELS[st.level]))); });
  }

  // opts: scene (a CraftPaint scene name), level ('auto' or one of LEVELS), sky ({campaignId,
  // calendarId} or {calendar}), name (said on the Play button, 'the forge'), lab (show the level
  // picker and the readout).
  function mount(host, opts) {
    var make = window.CraftPaint && window.CraftPaint.scenes[opts.scene];
    if (!make) return null;
    var reduce = !!(window.matchMedia && matchMedia('(prefers-reduced-motion: reduce)').matches);
    var dpr = Math.min(2, window.devicePixelRatio || 1), bs = Math.min(1.5, dpr);
    var box = document.createElement('div'); box.className = 'cpaint';
    var cv = document.createElement('canvas'); cv.className = 'cpaint-cv'; cv.width = W * dpr; cv.height = H * dpr;
    var off = document.createElement('canvas'); off.width = Math.round(W * bs); off.height = Math.round(H * bs);
    var og = off.getContext('2d'); og.scale(bs, bs);
    var sc = make(og, off);
    cv.setAttribute('role', 'img'); cv.setAttribute('aria-label', sc.label || opts.scene);
    box.innerHTML = '<button type="button" class="cpaint-cover" hidden><span><i class="fa-solid fa-play" aria-hidden="true"></i> Play</span></button>' +
      '<button type="button" class="cpaint-pause" hidden aria-label="Pause"><i class="fa-solid fa-pause" aria-hidden="true"></i></button>';
    box.insertBefore(cv, box.firstChild);
    host.innerHTML = ''; host.appendChild(box);
    var forced = levelOf(opts.level), mode = motionMode(document.documentElement, reduce);
    var st = { host: host, box: box, cv: cv, base: { cv: off, g: og, sc: sc }, name: opts.name || 'the scene', mode: mode, auto: forced < 0 && mode !== 'off',
      gov: governor(), log: [], t: 1.2, ms: 0, fps: 0, fpsN: 0, fpsT: 0, vis: false, dead: false, userPaused: false, level: 0, burn: false,
      sky: Sky(opts.sky || { calendar: DEMO }, mode !== 'full', function () { if (active !== st && !st.dead) draw(st, 0); }), coverBtn: box.querySelector('.cpaint-cover'), pauseBtn: box.querySelector('.cpaint-pause'), labBtns: [] };
    st.pauseBtn.setAttribute('aria-label', 'Pause ' + st.name);
    if (opts.lab) lab(st);
    // And the other way round: once the sky finds the device slow, a stage on the graphics card steps down.
    if (window.SharedGL) SharedGL.onSlow(function () {
      if (st.dead || !st.auto || st.level !== 0) return;
      st.log.push('The sky found this device slow, so it moved to ' + NAMES[1] + '.'); setLevel(st, 1); readout(st);
    });
    setLevel(st, mode === 'off' ? 3 : forced >= 0 ? forced : firstLevel(st));
    pause(st, mode === 'calm' ? 'Calm motion is on. Play ' + st.name : null);
    stages.push(st);
    st.coverBtn.addEventListener('click', function () { start(st); });
    st.pauseBtn.addEventListener('click', function () { st.userPaused = true; pause(st); st.coverBtn.focus(); });
    box.addEventListener('click', function (e) { if (active === st && !e.target.closest('button') && sc.poke) sc.poke(); });
    // In view means some of it shows: a stage whose edge only touches the window's counts as intersecting with
    // nothing showing, and the second threshold reports the moment it does show.
    st.io = new IntersectionObserver(function (es) {
      var e = es[es.length - 1];
      st.vis = e.isIntersecting && e.intersectionRatio > 0;
      if (st.vis) { if (active === st) kick(); else settle(); }
      else if (active === st) { pause(st); settle(); }
    }, { threshold: [0, 0.01] });
    st.io.observe(box);
    var api = {
      level: function () { return LEVELS[st.level]; },
      setLevel: function (name) { choose(st, name); },
      running: function () { return active === st; },
      play: function () { start(st); },
      pause: function () { st.userPaused = true; pause(st); },
      poke: function () { if (sc.poke) sc.poke(); },
      steps: function () { return st.log.slice(); },
      destroy: function () {
        st.dead = true; if (active === st) active = null;
        st.io.disconnect(); st.sky.destroy();
        stages.splice(stages.indexOf(st), 1); settle();
      }
    };
    return api;
  }
  // The level an adjusting stage starts at, said in the readout when the device's own figures lower it.
  function firstLevel(st) {
    var n = startLevel(window.navigator);
    if (n > 0) st.log.push('This device has little memory, few cores or data saver on, so it starts at ' + NAMES[n] + '.');
    return n;
  }
  // Hold a level, or 'auto' to start again from the top and adjust.
  function choose(st, name) {
    if (st.mode === 'off') return;
    if (name === 'auto') { st.auto = true; st.log = []; setLevel(st, firstLevel(st)); }
    else if (levelOf(name) >= 0) { st.auto = false; setLevel(st, levelOf(name)); }
    if (st.level === 3) pause(st); else if (active === st) kick();
  }
  // The Design Lab's controls: pick a level or leave it to adjust itself, and pretend the computer is slow.
  function lab(st) {
    var bar = document.createElement('div'); bar.className = 'cpaint-lab';
    bar.innerHTML = '<div class="cpaint-levels" role="group" aria-label="Effect level">' +
      ['auto'].concat(LEVELS).map(function (l, i) { return '<button type="button" data-l="' + l + '">' + (i ? NAMES[i - 1] : 'Adjust itself') + '</button>'; }).join('') + '</div>' +
      '<button type="button" class="cpaint-slow" aria-pressed="false">Pretend this is a slow computer</button>' +
      '<p class="cpaint-read" aria-live="polite"></p><ul class="cpaint-log"></ul>';
    st.box.parentNode.appendChild(bar);
    st.read = bar.querySelector('.cpaint-read'); st.logEl = bar.querySelector('.cpaint-log');
    st.labBtns = Array.prototype.slice.call(bar.querySelectorAll('[data-l]'));
    bar.addEventListener('click', function (e) {
      var b = e.target.closest('button'); if (!b) return;
      if (b.hasAttribute('data-l')) { choose(st, b.getAttribute('data-l')); if (st.level < 3 && active !== st) start(st); }
      else { st.burn = !st.burn; b.setAttribute('aria-pressed', String(st.burn)); if (st.burn && active !== st) start(st); }
      readout(st);
    });
  }
  document.addEventListener('visibilitychange', function () { if (!document.hidden) kick(); });

  var CSS = [
    '.cpaint{position:relative;aspect-ratio:16/9;max-width:100%;overflow:hidden;border-radius:10px;background:#0b0807;cursor:pointer}',
    '.cpaint-cv{display:block;width:100%;height:100%}',
    '.cpaint-cover{position:absolute;inset:0;display:flex;align-items:center;justify-content:center;border:0;padding:16px;background:rgba(8,6,5,.32);color:#fff;font:inherit;cursor:pointer}',
    '.cpaint-cover span{display:inline-flex;align-items:center;gap:8px;padding:9px 16px;border-radius:999px;background:rgba(18,14,12,.82);border:1px solid rgba(255,255,255,.2);font-weight:600;font-size:.875rem;text-align:center}',
    '.cpaint-cover:focus-visible span,.cpaint-pause:focus-visible{outline:2px solid var(--color-accent,#6366f1);outline-offset:2px}',
    '.cpaint-pause{position:absolute;right:10px;bottom:10px;width:34px;height:34px;display:flex;align-items:center;justify-content:center;border-radius:999px;border:1px solid rgba(255,255,255,.2);background:rgba(18,14,12,.6);color:#fff;opacity:0;transition:opacity .15s ease-out}',
    '.cpaint:hover .cpaint-pause,.cpaint-pause:focus-visible{opacity:1}',
    '@media (hover:none){.cpaint-pause{opacity:1}}',
    '.cpaint-lab{display:flex;flex-direction:column;gap:6px;margin-top:10px;font-size:.8125rem;color:var(--color-text-secondary)}',
    '.cpaint-levels{display:flex;flex-wrap:wrap;gap:2px 14px;border-bottom:1px solid var(--color-border)}',
    '.cpaint-levels button{padding:6px 0;border:0;border-bottom:2px solid transparent;margin-bottom:-1px;background:none;color:var(--color-text-secondary);font:inherit;font-size:.75rem;font-variant:all-small-caps;letter-spacing:.04em;cursor:pointer}',
    '.cpaint-levels button[aria-pressed="true"]{color:var(--color-text-primary);border-bottom-color:var(--color-accent)}',
    '.cpaint-slow{align-self:flex-start;padding:0;border:0;background:none;color:var(--color-accent);font:inherit;cursor:pointer}',
    '.cpaint-slow[aria-pressed="true"]{text-decoration:underline}',
    '.cpaint-read{margin:0;font-variant-numeric:tabular-nums}',
    '.cpaint-log{margin:0;padding-left:18px;font-size:.75rem}',
    '@media (prefers-reduced-motion:reduce){.cpaint-pause{transition:none}}'
  ].join('\n');
  function injectStyle() {
    if (document.getElementById('cpaint-style')) return;
    var s = document.createElement('style'); s.id = 'cpaint-style'; s.textContent = CSS; document.head.appendChild(s);
  }

  /* The widget. Attributes: data-scene (default forge), data-level (auto, or a level to hold), data-lab
     (the level picker and readout), data-name (what Play names), and data-campaign-id plus
     data-calendar-id for the window's sky; without both the window shows a demo calendar. */
  if (window.Chronicle && Chronicle.register) Chronicle.register('craft_paint', {
    init: function (el) {
      injectStyle();
      var ds = el.dataset, sky = ds.campaignId && ds.calendarId ? { campaignId: ds.campaignId, calendarId: ds.calendarId } : { calendar: DEMO };
      el._paint = mount(el, { scene: ds.scene || 'forge', level: ds.level || 'auto', lab: ds.lab != null, name: ds.name || 'the forge', sky: sky });
    },
    destroy: function (el) { if (el._paint) { el._paint.destroy(); el._paint = null; } el.innerHTML = ''; }
  });

  return { LEVELS: LEVELS, NAMES: NAMES, mount: mount, motionMode: motionMode, startLevel: startLevel, governor: governor, levelOf: levelOf };
})();
