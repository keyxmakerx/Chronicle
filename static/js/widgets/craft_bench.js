/**
 * craft_bench.js -- the crafting bench widget (data-widget="craft_bench")
 *
 * Mounts a crafting scene: CraftScene plays it, a skin draws it (CraftDesk
 * for now), and in build mode CraftBenchEditor arranges it. This file is the
 * glue: it reads the mount's data attributes, injects the bench's styles
 * once, and hands each part its data. craft_bench.ai.md is the guide.
 *
 * Attributes:
 *   data-mode         "play" (default) or "build"
 *   data-thumbs       base URL of the tray's room pictures (build mode)
 *   data-campaign-id, data-calendar-id
 *                     a calendar for the window's sky; without them the
 *                     window shows the demo calendar below
 *
 * Until the crafting app stores recipes and benches, the widget plays the
 * demo below and saves nothing.
 */
(function () {
  'use strict';

  /* The demo bench: two recipes that between them use every kind of step,
     four ingredients, and a made-up calendar for the window. Generic on
     purpose: a campaign's own recipes come from the crafting app. */
  var DEMO = {
    items: {
      bloodmoss: {name: 'Bloodmoss', color: '#b3263a', desc: 'A red moss from old battlefields. It stops bleeding.'},
      water: {name: 'Spring water', color: '#9fc6e6', desc: 'Drawn cold from the spring above the town.'},
      saltglass: {name: 'Saltglass', color: '#9fe3e0', desc: 'Brittle crystals from the salt flats. They bite when wet.'},
      bile: {name: 'Wyrmbile', color: '#5fbf3a', desc: 'Bottled from a drake. Do not smell it.'}
    },
    recipes: {
      heal: {name: 'Healing potion', kind: 'Steady brew', who: 'the hero', stat: 'Reason', mod: 0, goal: 30, start: 20,
        base: '#cfe3f2', paste: '#7e1a2a', mid: '#a63a4a', final: '#ff4d6d',
        steps: [{t: 'add', item: 'bloodmoss', to: 'mortar'}, {t: 'grind', turns: 3}, {t: 'tip'}, {t: 'pour', item: 'water'}, {t: 'simmer', secs: 5}]},
      caustic: {name: 'Caustic flask', kind: 'Volatile', who: 'the hero', stat: 'Reason', mod: 0, goal: 45, start: 34,
        base: '#d9eef0', paste: '#6fc9c4', mid: '#79c9a8', final: '#a6ff3d',
        steps: [{t: 'add', item: 'saltglass', to: 'mortar'}, {t: 'grind', turns: 2}, {t: 'tip'}, {t: 'add', item: 'bile', to: 'flask'}, {t: 'strike', n: 3}]}
    },
    info: {
      cat: {name: 'Soot', text: 'The workshop cat. Sleeps through anything.', more: 'Soot has slept through two fires and a drake on the roof. Pet at your own risk.', acts: [['Pet the cat', 'pet']]},
      book: {name: 'Recipe book', text: 'Recipes, copied out by hand.'},
      tray: {name: 'Tray', text: 'What was brought from the pack.'}
    },
    calendar: {
      name: 'demo', mode: 'fantasy', tracks_real_time: false, hemisphere: 'north', hours_per_day: 24, minutes_per_hour: 60, leap_year_every: 0, leap_year_offset: 0,
      months: ['Thaw', 'Seedtime', 'Rains', 'Blossom', 'Highsun', 'Harvest', 'Leaffall', 'Mists', 'Frost', 'Deepwinter', 'Longnight', 'Waking'].map(function (n) { return {name: n, days: 30}; }),
      moons: [{id: 1, name: 'The Lantern', cycle_days: 30, phase_offset: 4, color: '#e5e7eb', size: 1, phase_mode: 'auto'}],
      current_year: 100, current_month: 9, current_day: 8, weather: {preset_id: 'clear', preset_label: 'Clear'}
    }
  };

  var CSS = [
    '@font-face{font-family:"Crafting Fell";src:url(/static/fonts/customize/imfell-latin-400.woff2) format("woff2");font-weight:400;font-style:normal;font-display:swap}',
    '.cr-stage{position:relative;aspect-ratio:12/7;overflow:hidden;outline:none;user-select:none;-webkit-user-select:none}',
    '.cr-stage:focus-visible{box-shadow:inset 0 0 0 3px var(--color-accent)}',
    '.cr-svg{display:block;width:100%;height:100%;touch-action:none}',
    '.cr-fx{position:absolute;inset:0;width:100%;height:100%;pointer-events:none}',
    '.cr-grab{cursor:grab}.cr-picking{cursor:crosshair}',
    '.cr-hint{position:absolute;left:50%;bottom:14px;transform:translateX(-50%);display:flex;align-items:center;gap:10px;max-width:calc(100% - 24px);padding:7px 8px 7px 16px;border-radius:999px;font-size:13.5px;line-height:1.35}',
    '.cr-hint .cr-acts{display:flex;gap:6px;flex:none}',
    '.cr-hint .cr-acts:empty{display:none}',
    '.cr-hint:has(.cr-acts:empty){padding-right:16px}',
    '.cr-btn{font:inherit;font-size:13px;font-weight:600;border-radius:999px;padding:5px 14px;cursor:pointer;border:1px solid transparent}',
    '.cr-card{position:absolute;z-index:4;max-width:250px;padding:10px 12px;border-radius:10px;font-size:12.5px;line-height:1.4;pointer-events:none;opacity:0;transform:translateY(4px);transition:opacity .15s var(--ease-out),transform .15s var(--ease-out);display:flex;flex-direction:column;gap:3px}',
    '.cr-card.show{opacity:1;transform:none}.cr-card.pinned{pointer-events:auto;padding-right:30px}',
    '.cr-card .cr-meta{font-size:11px;letter-spacing:.04em}.cr-card p{margin:0}.cr-card .cr-foot{font-size:11px;margin-top:2px}',
    '.cr-card ol{margin:4px 0 2px;padding-left:18px}',
    '.cr-x{position:absolute;top:4px;right:6px;background:none;border:0;font-size:18px;line-height:1;cursor:pointer;padding:2px 4px}',
    '.cr-menu{position:absolute;z-index:6;min-width:190px;padding:6px;border-radius:10px;display:flex;flex-direction:column}',
    '.cr-menu .cr-mhead{font-size:11px;text-transform:uppercase;letter-spacing:.08em;padding:4px 10px 6px}',
    '.cr-menu button{font:inherit;font-size:13px;text-align:left;background:none;border:0;border-radius:6px;padding:7px 10px;cursor:pointer}',
    '.crb-scene{--sky1:#0b1430;--sky2:#2a3b66;--lit:#ffc478;--vig:.55;background:#120c08}',
    '.crb-scene[data-time="dusk"]{--sky1:#2c2550;--sky2:#e0895a;--vig:.6}',
    '.crb-scene[data-time="dusk"] .d-stars{opacity:0}.crb-scene[data-time="dusk"] .d-moon{opacity:.35}',
    '.crb-scene[data-light="lantern"]{--lit:#ffe2aa}',
    '.crb-scene[data-light="witch"]{--lit:#b096ff}',
    '@keyframes crb-flick{0%{transform:scale(1,1) skewX(0)}35%{transform:scale(.96,1.08) skewX(-3deg)}70%{transform:scale(1.03,.95) skewX(2deg)}100%{transform:scale(.98,1.04) skewX(-1deg)}}',
    '.d-cflame,.d-bf{transform-box:fill-box;transform-origin:50% 100%;animation:crb-flick 1.3s ease-in-out infinite alternate}',
    '.d-bf{animation-duration:.45s}',
    '@keyframes crb-beam{0%,100%{opacity:0}45%,55%{opacity:.22}}',
    '.d-beam{animation:crb-beam 5s ease-in-out infinite}.d-beacon{animation:crb-beam 5s ease-in-out infinite;opacity:.9}',
    '@keyframes crb-caps{0%,100%{opacity:.75}50%{opacity:1}}',
    '.d-caps{animation:crb-caps 3.2s ease-in-out infinite}.d-jarglow{animation:crb-caps 3.2s ease-in-out infinite}',
    '@keyframes crb-glint{0%,100%{opacity:.6}50%{opacity:1}}.d-glint{animation:crb-glint 2.4s ease-in-out infinite}',
    '@keyframes crb-hot{0%,100%{filter:drop-shadow(0 0 2px rgb(255 214 120 / .35))}50%{filter:drop-shadow(0 0 12px rgb(255 214 120 / .95))}}',
    '.cr-hot{animation:crb-hot 1.4s ease-in-out infinite}',
    '.d-label{font-family:"Crafting Fell",Georgia,serif;font-size:8.5px;fill:#3a2a1a}',
    '.cr-bel-top{transform-box:fill-box;transform-origin:0% 100%;transition:transform .16s var(--ease-out)}',
    '.cr-bellows.pump .cr-bel-top{transform:rotate(9deg)}',
    '.cr-bel-mid{transform-box:fill-box;transform-origin:50% 100%;transition:transform .16s var(--ease-out)}',
    '.cr-bellows.pump .cr-bel-mid{transform:scaleY(.5)}',
    '.bk-h{font-family:"Crafting Fell",Georgia,serif;font-size:22px;fill:#2a1a10}',
    '.bk-h2{font-family:"Crafting Fell",Georgia,serif;font-size:19px;fill:#2a1a10}',
    '.bk-k,.bk-t,.bk-n,.bk-s,.bk-g{font-family:"Crafting Fell",Georgia,serif;fill:#4a3826}',
    '.bk-k{font-size:12px;font-style:italic}.bk-t{font-size:13.5px}.bk-n{font-size:11px;font-style:italic}',
    '.bk-s{font-size:14px}.bk-s.is-now{fill:#7a1a1a}.bk-s.is-done{fill:#6a5a46}',
    '.bk-g{font-size:10.5px;font-style:italic}.bk-g.is-perfect{fill:#2f6a2a}.bk-g.is-miss{fill:#8a1a1a}',
    '.bk-rule{stroke:#6a5236;stroke-width:1;fill:none}',
    '.bk-tick{stroke:#2f5a2a;stroke-width:2.4;fill:none;stroke-linecap:round;stroke-linejoin:round}',
    '.bk-cross{stroke:#8a1a1a;stroke-width:2.2;fill:none;stroke-linecap:round}',
    '.bk-now{fill:#7a1a1a}.bk-dot{fill:#9a8a6e}',
    '.bk-bar{fill:none;stroke:#5a4630;stroke-width:1.2}.bk-fill{fill:#7a1a1a;opacity:.75;transition:width .8s var(--ease-out)}',
    '.bk-v.is-edge{fill:#2f6a2a}.bk-v.is-bane{fill:#8a1a1a}',
    '.bk-stamp{fill:none;stroke:#a0201a;stroke-width:3}.bk-stampt{font-family:"Oswald","Arial Narrow",sans-serif;font-size:20px;letter-spacing:.14em;text-transform:uppercase;fill:#a0201a}',
    '.cr-g-track{fill:none;stroke:#3a2c1a;stroke-width:12;stroke-linecap:round}',
    '.cr-g-band{fill:none;stroke:#c99a3c;stroke-width:12;opacity:.8}',
    '.cr-g-band.is-in{stroke:#ffd77a;opacity:1;filter:drop-shadow(0 0 6px rgb(255 215 122 / .9))}',
    '.cr-g-time{fill:none;stroke:#e4c27a;stroke-width:3;opacity:.75;stroke-linecap:round}',
    '.cr-g-needle{stroke:#ff6a4a;stroke-width:4;stroke-linecap:round;filter:drop-shadow(0 0 4px rgb(255 106 74 / .7))}',
    '.cr-g-hub{fill:#e4c27a}',
    '.cr-g-lab{font-family:Inter,system-ui,sans-serif;font-size:10px;letter-spacing:.1em;text-transform:uppercase;fill:#e4c27a}',
    '.cr-b-track{fill:#2a1f14;stroke:#6e5222}',
    '.cr-b-zone{fill:#c99a3c;opacity:.6}.cr-b-perfect{fill:#ffd77a}',
    '.cr-b-mark{stroke:#fff;stroke-width:4;stroke-linecap:round;filter:drop-shadow(0 0 5px rgb(255 255 255 / .9))}',
    '.cr-b-pip{fill:none;stroke:#e4c27a;stroke-width:1.5}.cr-b-pip.is-perfect{fill:#ffd77a}.cr-b-pip.is-good{fill:#c99a3c}.cr-b-pip.is-miss{fill:#c0392b;stroke:#c0392b}',
    '.cr-mark{stroke:#e6f4ff;stroke-opacity:.6;stroke-width:2}.cr-mark-gold{stroke:#ffd77a;stroke-opacity:1;stroke-width:3}',
    '.cr-stream{stroke-width:5;stroke-linecap:round;opacity:.8}',
    '.cr-ringpath{fill:none;stroke:#ffd77a;stroke-width:4;stroke-linecap:round;opacity:.85;filter:drop-shadow(0 0 4px rgb(255 215 122 / .8))}',
    '.cr-liqtop{opacity:.75}',
    '@keyframes crb-pop{0%{opacity:0;transform:translateY(10px) scale(.8)}20%{opacity:1;transform:none}100%{opacity:0;transform:translateY(-26px)}}',
    '.cr-pop{font-family:"Crafting Fell",Georgia,serif;font-size:30px;transform-box:fill-box;transform-origin:50% 50%;animation:crb-pop 1.1s var(--ease-out) forwards;paint-order:stroke;stroke:#1a0f07;stroke-width:4px}',
    '.cr-pop.is-perfect{fill:#ffd77a}.cr-pop.is-good{fill:#f3e6cc}.cr-pop.is-miss{fill:#ff7a6a}.cr-pop.is-say{fill:#f3e6cc;font-size:22px}',
    '.cr-die-face{fill:#f1e6cf;stroke:#6b5a3e;stroke-width:1.5}.cr-die-edge{fill:none;stroke:#b9a682;stroke-width:1}',
    '.cr-die-n{font-family:"Oswald","Arial Narrow",sans-serif;font-size:19px;font-weight:600;fill:#2a1a10}',
    '.cr-dice.is-final .cr-die{animation:crb-diepop .4s var(--ease-out)}',
    '@keyframes crb-diepop{0%{transform:scale(1.25)}100%{transform:scale(1)}}',
    '.cr-die{transform-box:fill-box;transform-origin:50% 50%}',
    '.cr-hint{background:rgb(20 14 9 / .8);color:#f3e6cc;border:1px solid rgb(228 194 122 / .35);backdrop-filter:blur(6px);-webkit-backdrop-filter:blur(6px);box-shadow:0 8px 24px -8px rgb(0 0 0 / .6)}',
    '.cr-btn{background:rgb(255 255 255 / .08);color:#f3e6cc;border-color:rgb(228 194 122 / .4)}',
    '.cr-btn:hover{background:rgb(255 255 255 / .16)}',
    '.cr-btn-primary{background:#e4b04a;color:#1d140c;border-color:#e4b04a}.cr-btn-primary:hover{background:#ffd77a}',
    '@keyframes crb-breathe{0%,100%{transform:scaleY(1)}50%{transform:scaleY(1.035)}}',
    '.d-cat-body{transform-box:fill-box;transform-origin:50% 100%;animation:crb-breathe 4.2s ease-in-out infinite}',
    '@keyframes crb-tail{0%,86%,100%{transform:rotate(0)}90%{transform:rotate(-16deg)}94%{transform:rotate(6deg)}}',
    '.d-cat-tail{transform-box:fill-box;transform-origin:0% 0%;animation:crb-tail 7s ease-in-out infinite}',
    '.crb-scene[data-time="dusk"] .d-moonshaft{opacity:.3}',
    '.cr-hov{filter:drop-shadow(0 0 7px rgb(255 214 120 / .7)) brightness(1.12)}',
    '@keyframes crb-wag{0%,100%{transform:rotate(0)}25%{transform:rotate(-22deg)}75%{transform:rotate(10deg)}}',
    '.petted .d-cat-tail{animation:crb-wag .45s ease-in-out 3}',
    '.petted .d-cat-body{animation:crb-breathe .5s ease-in-out 3}',
    '@keyframes crb-flip{0%{transform:translate(948px,380px) rotate(0) translate(0,36px)}100%{transform:translate(948px,380px) rotate(180deg) translate(0,36px)}}',
    '.d-hg.flip{transform-origin:0 0;animation:crb-flip .8s var(--ease-out)}',
    '.cr-card{background:rgb(24 17 11 / .92);color:#efe2c8;border:1px solid rgb(228 194 122 / .4);box-shadow:0 14px 30px -10px rgb(0 0 0 / .7)}',
    '.cr-card b{font-family:"Crafting Fell",Georgia,serif;font-size:17px;font-weight:400;color:#ffe2a8}',
    '.cr-card .cr-meta{color:#b9a27a}.cr-card .cr-now{color:#ffd77a}.cr-card .cr-foot{color:#8f7d60}',
    '.cr-menu{background:rgb(24 17 11 / .96);border:1px solid rgb(228 194 122 / .4);box-shadow:0 14px 30px -10px rgb(0 0 0 / .7)}',
    '.cr-menu .cr-mhead{color:#b9a27a}.cr-menu button{color:#efe2c8}.cr-menu button:hover,.cr-menu button:focus-visible{background:rgb(228 194 122 / .16);outline:none}',
    '.cr-x{color:#efe2c8}',
    '.d-wallglow,.d-deskglow,.d-moonshaft,.d-bglow,.d-jarglow,.d-flash,.d-bloom1,.d-bloom2,.d-refl,.d-caustic{pointer-events:none}',
    '.d-grainov{position:absolute;inset:0;width:100%;height:100%;pointer-events:none;mix-blend-mode:overlay;opacity:.14;background-size:160px 160px}',
    '.d-lite .d-dofg{filter:none}',
    '.d-calsky .d-fallsky{display:none}',
    '.crb-scene .d-drops{display:none}.crb-scene[data-weather="rain"] .d-drops{display:inline}.d-lite .d-grainov{display:none}',
    '@keyframes crb-bob{0%,100%{transform:translateY(0)}50%{transform:translateY(-5px)}}.d-orbf{animation:crb-bob 3.6s ease-in-out infinite}',
    '@keyframes crb-blink{0%,91%,100%{opacity:1}93%,96%{opacity:0}}.d-eyes{animation:crb-blink 5.5s infinite}',
    '.d-wisp{animation:crb-caps 2.6s ease-in-out infinite}',
    '@keyframes crb-spin{to{transform:rotate(360deg)}}.d-swirl{transform-box:view-box;transform-origin:0 -30px;animation:crb-spin 9s linear infinite}',
    '@media (max-width:640px){.cr-stage{aspect-ratio:auto}.cr-svg{height:auto;aspect-ratio:12/7}.cr-fx{height:auto;aspect-ratio:12/7}.d-grainov{height:auto;aspect-ratio:12/7}.cr-hint{position:static;transform:none;margin:8px;border-radius:10px;font-size:13px;justify-content:space-between}.cr-btn{padding:4px 10px}}',
    '.crb{display:flex;flex-direction:column;min-width:0;color:var(--color-text-body);font-size:14px;line-height:1.55;border:1px solid var(--color-border);border-radius:12px;overflow:hidden;background:var(--color-bg-primary)}',
    '.crb.is-playing .crb-tray{display:none}',
    '.crb-bar{display:flex;flex-wrap:wrap;align-items:center;gap:8px 12px;padding:8px 12px;border-bottom:1px solid var(--color-border);background:var(--color-bg-secondary)}',
    '.crb-hint{font-size:12.5px;color:var(--color-text-secondary);flex:1 1 200px}',
    '.crb-try{display:flex;gap:6px;align-items:center}',
    '.crb-status{font-size:12px;color:var(--color-text-muted);white-space:nowrap}',
    '.crb-scene.is-arrange .cr-hint{display:none}',
    '.crb-seg{display:flex;flex-wrap:wrap;flex:none;border:1px solid var(--color-border);border-radius:8px;overflow:hidden;background:var(--color-bg-tertiary)}',
    '.crb-seg button{flex:1 1 auto;font:inherit;font-size:12px;padding:6px 10px;border:0;background:none;color:var(--color-text-body);cursor:pointer;white-space:nowrap}',
    '.crb-seg button+button{border-left:1px solid var(--color-border)}',
    '.crb-seg button[aria-pressed="true"]{background:var(--color-bg-secondary);color:var(--color-accent);font-weight:600;box-shadow:inset 0 -2px 0 var(--color-accent)}',
    '.crb-btn{font:inherit;font-size:13px;font-weight:500;color:var(--color-text-body);background:var(--color-bg-secondary);border:1px solid var(--color-border);border-radius:8px;padding:6px 12px;cursor:pointer;box-shadow:var(--elev-resting);display:inline-flex;align-items:center;gap:6px;transition:box-shadow var(--dur-standard) var(--ease-out),transform var(--dur-standard) var(--ease-out)}',
    '.crb-btn:hover:not(:disabled){box-shadow:var(--elev-hover);transform:translateY(-1px)}',
    '.crb-btn:disabled{opacity:.5;cursor:default}',
    '.crb-btn[aria-pressed="true"]{color:var(--color-accent);border-color:var(--color-accent)}',
    '.crb-btn.crb-sm{font-size:12.5px;padding:5px 10px}',
    '.crb-in{font:inherit;font-size:13px;color:var(--color-text-primary);background:var(--color-input-bg);border:1px solid var(--color-input-border);border-radius:8px;padding:6px 9px;min-width:0}',
    '.crb-in.crb-sm{font-size:12.5px;padding:3px 6px}',
    'select.crb-in{cursor:pointer}',
    '.crb-txt{font:inherit;font-size:13px;border:1px solid var(--color-input-border);border-radius:8px;padding:7px 9px;background:var(--color-input-bg);color:var(--color-text-primary);width:100%;resize:vertical}',
    '.crb-in:focus,.crb-txt:focus{outline:2px solid var(--color-accent);outline-offset:-1px;border-color:var(--color-accent)}',
    '.crb-fl{display:flex;flex-direction:column;gap:5px;min-width:0}',
    '.crb-fl>span{font-size:12.5px;font-weight:500;color:var(--color-text-primary)}',
    '.crb-fine{font-size:12px;color:var(--color-text-secondary);line-height:1.45;margin:0}',
    '.crb-pill{display:inline-flex;align-items:center;font-size:11px;font-weight:600;padding:1px 8px;border-radius:999px;background:var(--color-bg-tertiary);color:var(--color-text-secondary);border:1px solid var(--color-border);white-space:nowrap}',
    '.crb-ib{display:inline-flex;align-items:center;justify-content:center;width:28px;height:28px;border-radius:6px;border:0;background:none;color:var(--color-text-secondary);cursor:pointer;flex:none}',
    '.crb-ib:hover{background:var(--color-bg-tertiary);color:var(--color-text-primary)}',
    '.crb-grid2{display:grid;grid-template-columns:1fr 1fr;gap:8px}',
    '.crb-sec{display:flex;flex-direction:column;gap:10px}',
    '.crb-sec>h4{margin:0;font-size:11px;text-transform:uppercase;letter-spacing:.08em;color:var(--color-warn);font-weight:700;padding-bottom:4px;border-bottom:1px solid var(--color-border-light)}',
    '.crb-ctl{display:flex;flex-direction:column;gap:5px}',
    '.crb-ctl>span{font-size:12.5px;color:var(--color-text-primary);font-weight:500}',
    '.crb-wrap{position:relative;min-width:0}',
    '.crb-pcard{position:absolute;z-index:8;width:292px;max-height:calc(100% - 20px);overflow:auto;display:flex;flex-direction:column;gap:12px;padding:12px 14px 14px;background:var(--color-bg-secondary);border:1px solid var(--color-border);border-top:3px solid var(--color-warn);border-radius:12px;box-shadow:var(--elev-dragged);transition:opacity .12s}',
    '.crb-wrap.is-dragging .crb-pcard{opacity:0;pointer-events:none}',
    '.crb-ihead{display:flex;align-items:center;gap:8px}',
    '.crb-ihead .crb-title{flex:1;min-width:0;font-size:15px;font-weight:700;padding:5px 8px;border-color:transparent;background:transparent}',
    '.crb-ihead .crb-title:hover{border-color:var(--color-border)}.crb-ihead .crb-title:focus{border-color:var(--color-accent);background:var(--color-input-bg)}',
    '.crb-lock{display:flex;gap:7px;align-items:flex-start;font-size:12px;color:var(--color-text-secondary);background:var(--color-bg-tertiary);border-radius:8px;padding:8px 10px;margin:0}',
    '.crb-lock svg{flex:none;margin-top:2px}',
    '.crb-more summary{font-size:12.5px;font-weight:500;color:var(--color-text-primary);cursor:pointer;margin-bottom:6px}',
    '.crb-more summary small{font-weight:400;color:var(--color-text-secondary)}',
    '.crb-arow{font:inherit;display:flex;flex-direction:column;gap:1px;text-align:left;width:100%;padding:7px 10px;border:1px solid var(--color-border);border-radius:8px;background:var(--color-bg-primary);cursor:pointer;color:var(--color-text-body)}',
    '.crb-arow:hover{border-color:var(--color-input-border)}.crb-arow b{font-size:13px;font-weight:600;color:var(--color-text-primary)}.crb-arow small{font-size:11.5px;color:var(--color-text-secondary)}',
    '.crb-actc{border:1px solid var(--color-border);border-radius:10px;padding:10px;display:flex;flex-direction:column;gap:8px;background:var(--color-bg-primary)}',
    '.crb-acth{display:flex;gap:6px;align-items:center}.crb-acth .crb-in{flex:1;font-weight:600}',
    '.crb-rowb{display:flex;flex-wrap:wrap;gap:6px}',
    '.crb-pos{padding-top:10px;border-top:1px solid var(--color-border-light)}',
    '.crb-postxt{margin:-6px 0 0}.crb-postxt:empty{display:none}',
    '.crb-tray{border-top:1px solid var(--color-border);background:var(--color-bg-secondary)}',
    '.crb-ttabs{display:flex;gap:2px;padding:6px 10px 0;overflow-x:auto;border-bottom:1px solid var(--color-border);scrollbar-width:none}',
    '.crb-ttabs button{font:inherit;font-size:12.5px;font-weight:500;padding:7px 12px 8px;border:0;background:none;color:var(--color-text-secondary);cursor:pointer;white-space:nowrap;border-bottom:2px solid transparent;margin-bottom:-1px}',
    '.crb-ttabs button:hover{color:var(--color-text-primary)}',
    '.crb-ttabs button[aria-selected="true"]{color:var(--color-text-primary);font-weight:600;border-bottom-color:var(--color-warn)}',
    '.crb-tbody{display:flex;align-items:flex-start;gap:22px;padding:12px 14px 14px;overflow-x:auto;min-height:150px}',
    '.crb-tgrp{flex:none;display:flex;flex-direction:column}',
    '.crb-tgrp ul{list-style:none;margin:0 0 10px;padding:0;display:flex;flex-wrap:wrap;gap:6px;max-width:640px}',
    '.crb-tside{gap:10px;min-width:230px}',
    '.crb-grp{font-size:11px;text-transform:uppercase;letter-spacing:.08em;color:var(--color-text-secondary);font-weight:700;margin:0 0 6px}',
    '.crb-grp small{text-transform:none;letter-spacing:0;font-weight:400}',
    '.crb-tths{display:flex;gap:8px}',
    '.crb-th{font:inherit;display:flex;flex-direction:column;gap:5px;padding:0;border:0;background:none;cursor:pointer;color:var(--color-text-body);text-align:left}',
    '.crb-th img,.crb-thnone{display:block;height:76px;width:auto;aspect-ratio:4/3;border-radius:8px;box-shadow:0 0 0 1px var(--color-border);transition:transform .12s,box-shadow .12s;background:#1a130d}',
    '.crb-th:hover img{transform:translateY(-2px);box-shadow:0 0 0 1px var(--color-input-border),var(--elev-hover)}',
    '.crb-th span{font-size:12px;line-height:1.25;max-width:150px}',
    '.crb-th[aria-pressed="true"] img,.crb-th[aria-pressed="true"] .crb-thnone{box-shadow:0 0 0 2px var(--color-accent),0 0 0 4px var(--color-bg-secondary)}',
    '.crb-th[aria-pressed="true"] span{color:var(--color-accent);font-weight:600}',
    '.crb-th:focus-visible{outline:none}.crb-th:focus-visible img{box-shadow:0 0 0 3px var(--color-accent)}',
    '.crb-tnote{flex:none;align-self:center;max-width:220px;font-size:12px;color:var(--color-text-secondary);line-height:1.45;margin:0}',
    '.crb-prow{display:inline-flex;align-items:center;gap:6px;font:inherit;font-size:12.5px;background:var(--color-bg-secondary);border:1px solid var(--color-border);border-radius:999px;padding:4px 11px;cursor:pointer;color:var(--color-text-body)}',
    '.crb-prow small{font-size:11px;color:var(--color-warn)}',
    '.crb-prow .crb-lk{color:var(--color-text-secondary);display:flex}',
    '.crb-prow:hover,.crb-prow.is-hov{background:var(--color-bg-tertiary);border-color:var(--color-input-border)}',
    '.crb-prow[aria-current="true"]{background:rgb(var(--color-warn-rgb) / .14);border-color:var(--color-warn);color:var(--color-text-primary);font-weight:600}',
    '@media (max-width:720px){.crb-pcard{position:static;width:auto;max-height:none;border-radius:0;border-left:0;border-right:0;box-shadow:none}.crb-tbody{min-height:0}.crb-status{flex-basis:100%}}',
    '@media (prefers-reduced-motion:reduce){.crb-scene *,.crb *{animation:none!important;transition:none!important}}',
    'html[data-cz-reduce] .crb-scene *,html[data-view-motion="calm"] .crb-scene *{animation:none!important;transition:none!important}'
  ].join('\n');

  function injectStyle() {
    if (document.getElementById('crb-style')) return;
    var st = document.createElement('style'); st.id = 'crb-style'; st.textContent = CSS; document.head.appendChild(st);
  }

  Chronicle.register('craft_bench', {
    init: function (el) {
      if (!window.CraftScene || !window.CraftDesk) return;
      injectStyle();
      var ds = el.dataset, build = ds.mode === 'build' && window.CraftBenchEditor;
      var sky = ds.campaignId && ds.calendarId ? {campaignId: ds.campaignId, calendarId: ds.calendarId} : {calendar: DEMO.calendar};
      var skin = CraftDesk.create({sky: sky, thumbs: ds.thumbs || '', info: DEMO.info});
      var data = {recipes: DEMO.recipes, items: DEMO.items, recipe: 'heal'};
      if (build) {
        el._crb = CraftBenchEditor.start(el, Object.assign({skin: skin}, data));
      } else {
        var box = document.createElement('div'); box.className = 'crb-scene'; el.innerHTML = ''; el.appendChild(box);
        ['time', 'weather', 'light'].forEach(function (k) { box.setAttribute('data-' + k, skin.opts[k]); });
        el._crb = CraftScene.mount(box, skin, data);
      }
    },
    destroy: function (el) {
      if (el._crb) { el._crb.destroy(); el._crb = null; }
      el.innerHTML = '';
    }
  });
})();
