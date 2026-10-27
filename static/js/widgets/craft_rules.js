/**
 * craft_rules.js -- the crafting rules editor widget (data-widget="craft_rules")
 *
 * One recipe's "When… then…" rules and how the moments players see look:
 * the "When it can be made" tab (craft_rules_editor.js) and the How it looks tab
 * (craft_rules_looks.js, on the bench in craft_rules_stage.js), over one
 * shared state. What a rule means lives in craft_rules_logic.js; this file
 * is the glue: it reads the mount's attributes, injects the styles once,
 * reads the calendar (or uses the demo one), and switches tabs.
 * craft_rules.ai.md is the guide.
 *
 * Attributes:
 *   data-campaign-id, data-calendar-id
 *                     a calendar to write the rules against, read as the
 *                     viewer may see it; without them the demo calendar below
 *
 * Until the crafting app stores recipes, the widget edits the demo recipes
 * below and saves nothing.
 */
(function () {
  'use strict';
  var R = window.CraftRules, FX = window.ItemLookFX;

  var CSS = [
    '@font-face{font-family:"Crafting Fell";src:url(/static/fonts/customize/imfell-latin-400.woff2) format("woff2");font-weight:400;font-style:normal;font-display:swap}',
    '.crr{--crr-help:rgb(var(--color-ok-rgb));--crr-help-s:rgb(var(--color-ok-rgb) / .12);--crr-risk:rgb(var(--color-warn-rgb));--crr-risk-s:rgb(var(--color-warn-rgb) / .13);',
    '  --crr-block:rgb(var(--color-bad-rgb));--crr-block-s:rgb(var(--color-bad-rgb) / .11);--crr-change:var(--color-info);--crr-change-s:color-mix(in srgb,var(--color-info) 13%,transparent);',
    '  --crr-only:#0e7490;--crr-only-s:rgb(14 116 144 / .11);--crr-moon:#7c3aed;--crr-moon-s:rgb(124 58 237 / .1);',
    '  --crr-acc:var(--color-accent);--crr-acc-s:rgb(var(--color-accent-rgb) / .12);--crr-ink:var(--color-accent-link,var(--color-accent));--crr-field:var(--color-input-border,var(--color-border));',
    '  --crr-dm:rgb(var(--color-warn-rgb));display:flex;flex-direction:column;border:1px solid var(--color-border);border-radius:14px;background:var(--color-bg-primary);box-shadow:var(--elev-resting);color:var(--color-text-body);font-size:14px;position:relative}',
    '.dark .crr{--crr-only:#67e8f9;--crr-only-s:rgb(103 232 249 / .11);--crr-moon:#c4b5fd;--crr-moon-s:rgb(196 181 253 / .12)}',
    '.crr .crr-help{--c:var(--crr-help);--cs:var(--crr-help-s)}.crr .crr-risk{--c:var(--crr-risk);--cs:var(--crr-risk-s)}.crr .crr-block{--c:var(--crr-block);--cs:var(--crr-block-s)}',
    '.crr .crr-change{--c:var(--crr-change);--cs:var(--crr-change-s)}.crr .crr-only{--c:var(--crr-only);--cs:var(--crr-only-s)}.crr .crr-moon{--c:var(--crr-moon);--cs:var(--crr-moon-s)}',
    '.crr .crr-when{--c:var(--color-text-secondary);--cs:var(--color-bg-tertiary)}',
    '.crr .crr-grow{flex:1}.crr .crr-r{margin-left:auto}',
    '.crr .crr-sr{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0);white-space:nowrap}',
    '.crr .btn{gap:7px}',
    /* Head, banner, tabs */
    '.crr-head{display:flex;flex-wrap:wrap;align-items:center;gap:10px 14px;padding:12px 16px;border-bottom:1px solid var(--color-border);background:var(--color-card-bg);border-radius:14px 14px 0 0}',
    '.crr-head .crr-t{flex:1 1 260px;min-width:0;display:flex;flex-direction:column}',
    '.crr-head .crr-t b{font-family:var(--font-heading,inherit);font-size:19px;color:var(--color-text-primary);line-height:1.2}',
    '.crr-head .crr-t small{font-size:12px;color:var(--color-text-secondary)}',
    '.crr-head .crr-t small b{font:inherit;font-weight:600;color:var(--color-text-body)}',
    '.crr-banner{display:flex;flex-wrap:wrap;align-items:center;gap:6px 12px;padding:8px 16px;border-bottom:1px solid var(--color-border-light);background:var(--color-bg-secondary);font-size:12.5px;color:var(--color-text-secondary)}',
    '.crr-banner b{color:var(--color-text-primary)}',
    '.crr-banner.is-bad{background:var(--crr-risk-s);color:var(--color-text-body)}',
    '.crr-tabs{position:relative;display:flex;gap:4px;padding:0 12px;border-bottom:1px solid var(--color-border);background:var(--color-card-bg);overflow-x:auto;scrollbar-width:none}',
    '.crr-tabs [role="tab"]{font:inherit;display:inline-flex;align-items:center;gap:8px;min-height:44px;font-size:12px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;padding:0 12px;border:0;background:none;color:var(--color-text-secondary);cursor:pointer;white-space:nowrap}',
    '.crr-tabs [role="tab"][aria-selected="true"]{color:var(--color-text-primary)}',
    '.crr-tabs [role="tab"]:focus-visible{outline:2px solid var(--crr-acc);outline-offset:-4px;border-radius:8px}',
    '.crr-tabbar{position:absolute;bottom:-1px;height:2px;background:var(--crr-acc);border-radius:2px;transition:left var(--dur-standard) var(--ease-out),width var(--dur-standard) var(--ease-out)}',
    '.crr-body{padding:18px 20px 24px;display:flex;flex-direction:column;gap:18px}',
    '.crr-load{padding:32px 20px;color:var(--color-text-secondary);font-size:13px}',
    '.crr-pane{display:flex;flex-direction:column;gap:18px}',
    '.crr-sech{display:flex;align-items:baseline;gap:10px;flex-wrap:wrap}',
    '.crr-sech h4,.crr-shh h4,.crr-fold-hd h4{margin:0;font-size:12px;text-transform:uppercase;letter-spacing:.08em;color:var(--color-text-primary);font-weight:700}',
    '.crr-sech small{font-size:12.5px;color:var(--color-text-secondary)}',
    '.crr-hint{font-size:12px;color:var(--color-text-secondary);margin:0}.crr-hint b{color:var(--color-text-body)}',
    '.crr-grp{font-size:10.5px;text-transform:uppercase;letter-spacing:.08em;font-weight:700;color:var(--color-text-secondary);margin:2px 0 -2px}',
    '.crr-in{font:inherit;width:100%;padding:8px 10px;border-radius:8px;font-size:13px;background:var(--color-input-bg,var(--color-bg-primary));border:1px solid var(--crr-field);color:var(--color-text-primary)}',
    '.crr-in:focus{outline:none;border-color:var(--crr-acc);box-shadow:0 0 0 3px var(--crr-acc-s)}',
    '.crr-in.crr-sm{width:auto;padding:2px 6px;font-size:12px;min-height:28px}',
    '.crr-area{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px;resize:vertical}',
    '.crr-ctl{display:flex;flex-direction:column;gap:4px;font-size:12px;color:var(--color-text-secondary)}',
    '.crr-ib{display:inline-flex;align-items:center;justify-content:center;width:32px;height:32px;border-radius:8px;border:0;background:none;color:var(--color-text-secondary);cursor:pointer;flex:none}',
    '.crr-ib:hover,.crr-ib:focus-visible{background:var(--color-bg-tertiary);color:var(--color-text-primary)}',
    /* Your AI */
    '.crr-aibar{display:flex;flex-wrap:wrap;align-items:center;gap:8px 12px;padding:10px 12px 10px 14px;border-radius:12px;border:1px solid var(--color-border-light);background:var(--color-bg-secondary);font-size:12.5px;color:var(--color-text-secondary)}',
    '.crr-aibar>.fa-solid{color:var(--crr-acc)}.crr-aibar>span:first-of-type{flex:1 1 280px}',
    '.crr-fold{display:flex;flex-direction:column;gap:12px;padding:14px;border-radius:12px;border:1px solid var(--color-border);background:var(--color-card-bg);box-shadow:var(--elev-hover);animation:crr-drape var(--dur-standard) var(--ease-out)}',
    '.crr-fold[hidden]{display:none}',
    '@keyframes crr-drape{from{opacity:0;transform:translateY(-6px)}}',
    '.crr-fold-hd{display:flex;align-items:center;gap:10px}.crr-fold-hd h4{flex:1}',
    '.crr-brief{max-height:260px;overflow:auto}',
    /* The calendar strip */
    '.crr-tl{border:1px solid var(--color-border);border-radius:12px;background:var(--color-card-bg);overflow:hidden}',
    '.crr-tl-scroll{overflow-x:auto;scrollbar-width:thin}',
    '.crr-tl-grid{display:grid;grid-template-columns:150px repeat(var(--n),32px);min-width:max-content;position:relative}',
    '.crr-tl-grid>*{min-height:32px;display:flex;align-items:center;justify-content:center;font-size:11px;color:var(--color-text-secondary)}',
    '.crr-tl-lab{position:sticky;left:0;z-index:2;background:var(--color-card-bg);justify-content:flex-start!important;padding:0 10px;gap:7px;font-size:12px!important;color:var(--color-text-body)!important;border-right:1px solid var(--color-border-light);white-space:nowrap;overflow:hidden}',
    '.crr-tl-lab b{color:var(--color-text-primary);font-weight:600;overflow:hidden;text-overflow:ellipsis}',
    'button.crr-tl-lab{font:inherit;border-top:0;border-bottom:0;border-left:0;cursor:pointer;text-align:left}',
    'button.crr-tl-lab:hover b,button.crr-tl-lab:focus-visible b{text-decoration:underline}',
    '.crr-tl-rule.is-draft{opacity:.7;font-style:italic}',
    '.crr-tl-mon{justify-content:flex-start!important;padding-left:6px;font-weight:700;color:var(--color-text-primary)!important;text-transform:uppercase;letter-spacing:.06em;font-size:10.5px!important;border-left:1px solid var(--color-border-light)}',
    '.crr-tl-day{font:inherit;font-size:11px;border:0;background:none;color:var(--color-text-secondary);cursor:pointer;font-variant-numeric:tabular-nums;padding:0}',
    '.crr-tl-day.is-today{color:var(--crr-ink);font-weight:700}',
    '.crr-tl-day.is-sel{color:var(--color-text-primary);font-weight:700}',
    '.crr-tl-day:focus-visible{outline:2px solid var(--crr-acc);outline-offset:-3px;border-radius:6px}',
    '.crr-tl-grid .is-today{background:var(--crr-acc-s)}',
    '.crr-tl-grid .is-sel{box-shadow:inset 2px 0 0 var(--crr-acc),inset -2px 0 0 var(--crr-acc)}',
    '.crr-tl-cell{position:relative;cursor:pointer}',
    '.crr-tl-cell .fa-solid{font-size:13px}',
    '.crr-tl-sep{grid-column:1/-1;min-height:1px!important;height:1px;background:var(--color-border-light)}',
    '.crr-bar{position:absolute;left:0;right:0;top:9px;bottom:9px;background:var(--c);opacity:.85;transform-origin:left;animation:crr-grow var(--dur-large) var(--ease-out) both}',
    '.crr-bar.start{left:4px;border-top-left-radius:6px;border-bottom-left-radius:6px}.crr-bar.end{right:4px;border-top-right-radius:6px;border-bottom-right-radius:6px}',
    '.crr-bar.maybe{background:repeating-linear-gradient(135deg,var(--c) 0 4px,transparent 4px 8px);opacity:.8;box-shadow:inset 0 0 0 1.5px var(--c)}',
    '@keyframes crr-grow{from{transform:scaleX(0)}}',
    '.crr-tl-note{justify-content:flex-start!important;gap:6px;padding-left:10px;font-size:11.5px!important;white-space:nowrap}',
    '.crr-tl-note.crr-hand{color:var(--crr-moon)!important;font-style:italic;background:repeating-linear-gradient(135deg,var(--crr-moon-s) 0 6px,transparent 6px 12px)}',
    '.crr-tl-note.crr-hand select{font-style:normal}',
    '.crr-tl-note.crr-warnrow{color:var(--crr-risk)!important;background:repeating-linear-gradient(135deg,var(--crr-risk-s) 0 6px,transparent 6px 12px)}',
    '.crr-tl-note.crr-quietrow{color:var(--color-text-secondary)!important}',
    '.crr-rn{flex:none;width:20px;height:20px;border-radius:6px;background:var(--cs);color:var(--c);font-weight:700;font-size:11px;display:inline-flex;align-items:center;justify-content:center}',
    '.crr-yes{color:var(--crr-help);display:inline-flex}.crr-no{color:var(--crr-block);display:inline-flex}',
    '.crr-tl-can b{color:var(--color-text-primary)}',
    '.crr-tl-foot{display:flex;flex-wrap:wrap;gap:6px 14px;padding:8px 12px;border-top:1px solid var(--color-border-light);font-size:11.5px;color:var(--color-text-secondary)}',
    '.crr-key{display:inline-flex;align-items:center;gap:6px}.crr-key i{width:16px;height:8px;border-radius:3px;background:var(--c)}',
    '.crr-key i.m{background:repeating-linear-gradient(135deg,var(--c) 0 3px,transparent 3px 6px);box-shadow:inset 0 0 0 1px var(--c)}',
    '.crr-dayp{display:flex;flex-wrap:wrap;gap:10px 16px;align-items:flex-start;padding:12px 14px;border-top:1px solid var(--color-border);background:var(--color-bg-primary)}',
    '.crr-dt{min-width:180px;display:flex;flex-direction:column;align-items:flex-start;gap:2px}',
    '.crr-dt b{font-size:14px;color:var(--color-text-primary)}.crr-dt small{font-size:12px;color:var(--color-text-secondary)}',
    '.crr-verd{display:inline-flex;align-items:center;gap:6px;font-size:12.5px;font-weight:700;padding:3px 10px;border-radius:999px;background:var(--crr-help-s);color:var(--crr-help);margin-top:6px}',
    '.crr-verd.no{background:var(--crr-block-s);color:var(--crr-block)}.crr-verd.crr-help,.crr-verd.crr-risk{background:var(--cs);color:var(--c);margin-top:2px}',
    '.crr-dayp ul{list-style:none;margin:0;padding:0;display:flex;flex-wrap:wrap;gap:6px;flex:1}',
    '.crr-dayp li{display:inline-flex;align-items:center;gap:7px;font-size:12.5px;padding:4px 10px 4px 8px;border-radius:999px;background:var(--cs);color:var(--color-text-primary)}',
    '.crr-dayp li .fa-solid{color:var(--c);font-size:12px}',
    '.crr-dayp li em{font-style:normal;color:var(--color-text-secondary);font-size:11.5px}',
    '.crr-dayp li.crr-quiet{border-radius:10px;color:var(--color-text-secondary)}',
    /* Rule cards */
    '.crr-rlist{list-style:none;margin:0;padding:0;display:flex;flex-direction:column;gap:10px}',
    '.crr-rc{border:1px solid var(--color-border);border-radius:12px;background:var(--color-card-bg);box-shadow:var(--elev-resting);transition:box-shadow var(--dur-micro) var(--ease-out),border-color var(--dur-micro) var(--ease-out)}',
    '.crr-rc:hover{box-shadow:var(--elev-hover)}',
    '.crr-rc.is-open{border-color:var(--crr-acc);box-shadow:inset 2px 0 0 var(--crr-acc),var(--elev-hover)}',
    '.crr-rc.is-draft{border-style:dashed;border-color:var(--crr-dm)}',
    '.crr-rc.is-warned{border-color:var(--crr-risk)}',
    '.crr-rc.is-new{animation:crr-drop var(--dur-large) var(--ease-out)}',
    '@keyframes crr-drop{from{opacity:0;transform:translateY(-8px)}}',
    '.crr-rhd{display:flex;align-items:center;gap:12px;padding:12px 12px 12px 14px;cursor:pointer;border-radius:12px}',
    '.crr-rhd:focus-visible{outline:2px solid var(--crr-acc);outline-offset:-2px}',
    '.crr-flow{display:flex;align-items:center;gap:8px;flex:1;min-width:0;flex-wrap:wrap}',
    '.crr-tile{display:inline-flex;align-items:center;gap:10px;padding:6px 12px 6px 6px;border-radius:12px;background:var(--cs);min-width:0;max-width:100%}',
    '.crr-ti{width:34px;height:34px;border-radius:9px;background:var(--color-card-bg);display:flex;align-items:center;justify-content:center;color:var(--c);flex:none;box-shadow:var(--elev-resting);font-size:15px}',
    '.crr-tt{display:flex;flex-direction:column;min-width:0;line-height:1.2}',
    '.crr-tt small{font-size:10.5px;text-transform:uppercase;letter-spacing:.08em;font-weight:700;color:var(--c)}',
    '.crr-tile.crr-when .crr-tt small{color:var(--color-text-secondary)}',
    '.crr-tt b{font-size:13.5px;color:var(--color-text-primary);font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
    '.crr-tt span{font-size:11.5px;color:var(--color-text-secondary);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
    '.crr-plus{font-size:12px;font-weight:700;color:var(--color-text-secondary)}',
    '.crr-arrow{color:var(--color-text-secondary);flex:none;font-size:13px}',
    '.crr-rside{display:flex;align-items:center;gap:10px;flex:none}',
    '.crr-stc{display:inline-flex;align-items:center;gap:5px;font-size:11.5px;font-weight:600;padding:2px 9px 2px 7px;border-radius:999px;background:var(--color-bg-tertiary);color:var(--color-text-secondary);white-space:nowrap}',
    '.crr-stc i{width:7px;height:7px;border-radius:50%;background:currentColor;opacity:.75}',
    '.crr-stc.is-on{background:var(--crr-help-s);color:var(--crr-help)}.crr-stc.is-warn{background:var(--crr-risk-s);color:var(--crr-risk)}.crr-stc.is-hand{background:var(--crr-moon-s);color:var(--crr-moon)}',
    '.crr-visi{display:inline-flex;color:var(--color-text-secondary)}',
    '.crr-chev{color:var(--color-text-secondary);transition:transform var(--dur-standard) var(--ease-out)}',
    '.crr-rc.is-open .crr-chev{transform:rotate(180deg)}',
    '.crr-rsent{margin:0;padding:0 14px 12px 46px;font-size:13px;color:var(--color-text-secondary);line-height:1.45}',
    '.crr-rsent b{color:var(--color-text-body);font-weight:600}',
    '.crr-rwarn{display:flex;align-items:center;gap:10px;flex-wrap:wrap;margin:0 14px 12px 46px;padding:8px 10px;border-radius:10px;background:var(--crr-risk-s);color:var(--color-text-body);font-size:12.5px}',
    '.crr-rwarn>.fa-solid{color:var(--crr-risk)}.crr-rwarn .btn{margin-left:auto}',
    '.crr-rdraft{display:flex;align-items:center;gap:10px;flex-wrap:wrap;padding:10px 14px;border-top:1px dashed var(--crr-dm);background:var(--crr-risk-s);font-size:12.5px;border-radius:0 0 12px 12px}',
    '.crr-rdraft>.fa-solid{color:var(--crr-dm)}.crr-rdraft span{flex:1 1 220px}',
    '.crr-rexp{display:grid;grid-template-rows:0fr;transition:grid-template-rows var(--dur-large) var(--ease-out)}',
    '.crr-rc.is-open .crr-rexp{grid-template-rows:1fr}',
    '.crr-rexp>div{overflow:hidden;min-height:0}',
    '.crr-rin{padding:4px 16px 16px 46px;display:flex;flex-direction:column;gap:16px}',
    '.crr-step{display:grid;grid-template-columns:72px minmax(0,1fr);gap:10px}',
    '.crr-lbl{font-size:12px;text-transform:uppercase;letter-spacing:.08em;font-weight:700;color:var(--c);padding-top:8px}',
    '.crr-step.crr-when .crr-lbl{color:var(--color-text-secondary)}',
    '.crr-body{display:flex;flex-direction:column;gap:10px;min-width:0}',
    '.crr-step .crr-body{padding:0}',
    '.crr-cond{display:flex;flex-direction:column;gap:10px;padding:12px;border-radius:12px;border:1px solid var(--color-border-light);background:var(--color-bg-primary);position:relative}',
    '.crr-cond+.crr-cond::before{content:"and";position:absolute;top:-17px;left:14px;font-size:11.5px;font-weight:700;color:var(--color-text-secondary);background:var(--color-card-bg);padding:0 6px}',
    '.crr-rm{position:absolute;top:6px;right:6px}',
    '.crr-chips{display:flex;flex-wrap:wrap;gap:6px}',
    '.crr-cx{font:inherit;display:inline-flex;align-items:center;gap:7px;min-height:32px;font-size:12.5px;font-weight:600;padding:5px 11px 5px 9px;border-radius:999px;border:1px solid var(--color-border);background:var(--color-card-bg);color:var(--color-text-body);cursor:pointer;white-space:nowrap;transition:background var(--dur-micro),border-color var(--dur-micro)}',
    '.crr-cx:hover{border-color:var(--crr-field)}',
    '.crr-cx[aria-pressed="true"]{background:var(--crr-acc-s);border-color:var(--crr-acc);color:var(--crr-ink)}',
    '.crr-cx .crr-moon,.crr-tx .crr-moon,.crr-ti .crr-moon,.crr-tl-lab .crr-moon{flex:none;display:block}',
    '.crr-tiles{display:grid;grid-template-columns:repeat(auto-fill,minmax(100px,1fr));gap:6px}',
    '.crr-tiles.crr-effs{grid-template-columns:repeat(auto-fill,minmax(108px,1fr))}',
    '.crr-tx{font:inherit;display:flex;flex-direction:column;align-items:center;gap:6px;padding:10px 6px 8px;border-radius:12px;border:1px solid var(--color-border);background:var(--color-card-bg);color:var(--color-text-body);cursor:pointer;text-align:center;font-size:12px;line-height:1.2;transition:background var(--dur-micro),border-color var(--dur-micro)}',
    '.crr-tx:hover{border-color:var(--crr-field)}',
    '.crr-tx>.fa-solid{font-size:18px;color:var(--c,var(--color-text-secondary))}',
    '.crr-tx[aria-pressed="true"]{background:var(--cs,var(--crr-acc-s));border-color:var(--c,var(--crr-acc));color:var(--color-text-primary);font-weight:600}',
    '.crr-tx:not(.crr-help):not(.crr-risk):not(.crr-block):not(.crr-change):not(.crr-only):not(.crr-moon)[aria-pressed="true"]{background:var(--crr-acc-s);border-color:var(--crr-acc)}',
    '.crr-seen{font-size:10px;font-weight:600;color:var(--crr-help)}',
    '.crr-ring{display:grid;grid-template-columns:repeat(8,minmax(0,1fr));gap:4px}',
    '.crr-ring .crr-tx{padding:8px 2px 6px;font-size:11px}',
    '.crr-addand{align-self:flex-start;font-size:12.5px;border:0;background:none;cursor:pointer;padding:0}',
    '.crr-dice{display:flex;align-items:center;gap:10px;flex-wrap:wrap}',
    '.crr-seg{display:inline-flex;flex-wrap:wrap;border:1px solid var(--color-border);border-radius:10px;overflow:hidden;background:var(--color-card-bg)}',
    '.crr-seg button{font:inherit;font-size:12px;min-height:32px;padding:0 10px;border:0;border-right:1px solid var(--color-border-light);background:none;color:var(--color-text-body);cursor:pointer}',
    '.crr-seg button:last-child{border-right:0}',
    '.crr-seg button[aria-pressed="true"]{background:var(--crr-acc-s);color:var(--crr-ink);font-weight:700}',
    '.crr-stepper{display:inline-flex;align-items:center;border:1px solid var(--color-border);border-radius:10px;overflow:hidden;background:var(--color-card-bg)}',
    '.crr-stepper button{font:inherit;width:34px;height:34px;border:0;background:var(--color-bg-tertiary);cursor:pointer;color:var(--color-text-primary)}',
    '.crr-stepper output{min-width:46px;text-align:center;font-weight:700;color:var(--color-text-primary)}',
    '.crr-vcards{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:8px}',
    '.crr-vc{font:inherit;display:flex;flex-direction:column;align-items:flex-start;gap:4px;padding:10px 12px;border-radius:12px;border:1px solid var(--color-border);background:var(--color-card-bg);cursor:pointer;text-align:left;color:var(--color-text-body)}',
    '.crr-vc b{font-size:13px;color:var(--color-text-primary);display:flex;align-items:center;gap:8px}',
    '.crr-vc small{font-size:11.5px;color:var(--color-text-secondary);line-height:1.35}',
    '.crr-vc[aria-pressed="true"]{border-color:var(--crr-acc);background:var(--crr-acc-s);box-shadow:inset 0 0 0 1px var(--crr-acc)}',
    '.crr-lookstrip{display:flex;flex-wrap:wrap;align-items:center;gap:6px 10px;padding:10px 12px;border-radius:12px;background:var(--color-bg-primary);border:1px solid var(--color-border-light)}',
    '.crr-lk{display:inline-flex;align-items:center;gap:6px;font-size:12px;padding:3px 9px 3px 7px;border-radius:999px;background:var(--color-bg-tertiary);color:var(--color-text-body)}',
    '.crr-lk i{width:9px;height:9px;border-radius:50%}',
    '.crr-racts{display:flex;align-items:center;gap:8px;flex-wrap:wrap}',
    '.crr-danger{color:var(--crr-block)!important}',
    '.crr-addcard{font:inherit;display:flex;align-items:center;gap:12px;width:100%;padding:14px 16px;border-radius:12px;border:1.5px dashed var(--crr-field);background:none;color:var(--color-text-secondary);cursor:pointer;text-align:left;transition:border-color var(--dur-micro),background var(--dur-micro)}',
    '.crr-addcard:hover{border-color:var(--crr-acc);background:var(--crr-acc-s);color:var(--crr-ink)}',
    '.crr-addcard span{display:flex;flex-direction:column}.crr-addcard b{font-size:14px;color:var(--color-text-primary)}.crr-addcard small{font-size:12px}',
    '.crr-startgrid{display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:10px}',
    '.crr-st{font:inherit;display:flex;flex-direction:column;gap:8px;padding:12px;border-radius:12px;border:1px solid var(--color-border);background:var(--color-bg-primary);cursor:pointer;text-align:left;color:var(--color-text-body);transition:box-shadow var(--dur-micro),border-color var(--dur-micro)}',
    '.crr-st:hover{box-shadow:var(--elev-hover);border-color:var(--crr-field)}',
    '.crr-st b{font-size:14px;color:var(--color-text-primary)}.crr-st small{font-size:12px;color:var(--color-text-secondary);line-height:1.4}',
    '.crr-st.crr-blank{border-style:dashed;justify-content:center}.crr-st.crr-blank>.fa-solid{color:var(--crr-acc)}',
    '.crr-mini{display:flex;flex-direction:column;align-items:flex-start;gap:2px}',
    '.crr-mini .crr-tile{padding:4px 8px 4px 4px;gap:7px}.crr-mini .crr-ti{width:26px;height:26px;border-radius:7px;font-size:12px}',
    '.crr-mini .crr-tt b{font-size:12px}.crr-mini .crr-tt span{display:none}.crr-mini .crr-arrow{transform:rotate(90deg);margin-left:10px}',
    /* What the AI writes */
    '.crr-aij{display:none;margin:0 14px 12px 46px;padding:10px 12px;border-radius:10px;background:var(--color-bg-primary);border:1px solid var(--color-border-light);font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:11.5px;line-height:1.5;color:var(--color-text-body);white-space:pre-wrap;word-break:break-word}',
    '.crr-aicap{display:none;margin:0 14px 4px 46px;font-size:10.5px;text-transform:uppercase;letter-spacing:.08em;font-weight:700;color:var(--color-text-secondary)}',
    '.crr.is-ai .crr-aij,.crr.is-ai .crr-aicap,.crr-brief:not([hidden]){display:block}',
    '.crr-aij.crr-block{margin:0}.crr-aicap.crr-block{margin:0 0 -8px}',
    '.crr-aij .k{color:var(--crr-ink)}.crr-aij .s{color:var(--crr-help)}.crr-aij .n{color:var(--crr-risk)}',
    /* How it looks */
    '.crr-fxgrid{display:grid;grid-template-columns:200px minmax(0,1fr);gap:18px;align-items:start}',
    '.crr-orail{display:flex;flex-direction:column;gap:3px}',
    '.crr-orail .crr-grp{margin:8px 0 2px 4px}.crr-orail .crr-grp:first-child{margin-top:0}',
    '.crr-ob{font:inherit;display:flex;align-items:center;gap:9px;width:100%;min-height:44px;padding:6px 8px;border-radius:10px;border:1px solid transparent;background:none;cursor:pointer;text-align:left;color:var(--color-text-body);transition:background var(--dur-micro),border-color var(--dur-micro)}',
    '.crr-ob:hover{background:var(--color-bg-tertiary)}',
    '.crr-ob[aria-pressed="true"]{background:var(--crr-acc-s);border-color:var(--crr-acc)}',
    '.crr-pl{width:26px;height:26px;border-radius:50%;flex:none;display:flex;align-items:center;justify-content:center;background:var(--color-bg-tertiary);color:var(--color-text-secondary);font-size:10px}',
    '.crr-ob[aria-pressed="true"] .crr-pl{background:var(--crr-acc);color:var(--color-accent-on-fill,#fff)}',
    '.crr-on{flex:1;min-width:0;display:flex;flex-direction:column;line-height:1.25}',
    '.crr-on b{font-size:12.5px;font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;color:var(--color-text-primary)}',
    '.crr-on small{font-size:11px;color:var(--color-text-secondary);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
    '.crr-dl{display:inline-flex;gap:3px;flex:none}.crr-dl i{width:7px;height:7px;border-radius:50%}.crr-dl i.mv{border:1.5px solid var(--color-text-secondary)}',
    '.crr-railnote{display:flex;gap:6px;align-items:flex-start;margin-top:8px!important}',
    '.crr-fxmain{display:flex;flex-direction:column;gap:12px;min-width:0}',
    '.crr-stage{position:relative;border-radius:12px;overflow:hidden;background:#15110e;box-shadow:var(--elev-hover)}',
    '.crr-stage canvas{display:block;width:100%;aspect-ratio:16/9}',
    '.crr-stg-tag{position:absolute;left:12px;top:10px;font-size:11px;letter-spacing:.08em;text-transform:uppercase;color:rgb(255 236 210 / .65);font-weight:600;pointer-events:none}',
    '.crr-stg-cap{position:absolute;right:12px;top:8px;font-family:"Crafting Fell",Georgia,serif;font-size:17px;color:rgb(255 236 210 / .88);pointer-events:none;max-width:55%;text-align:right}',
    '.crr-spots{position:absolute;inset:0;pointer-events:none}',
    '.crr-spot{position:absolute;transform:translate(-50%,-50%);pointer-events:auto;font:inherit;width:44px;height:44px;border-radius:50%;border:2px dashed rgb(255 240 220 / .55);background:rgb(20 14 10 / .3);cursor:pointer;display:grid;place-items:center;padding:0;transition:border-color var(--dur-micro),background var(--dur-micro),opacity var(--dur-large)}',
    '.crr-spot i{width:13px;height:13px;border-radius:50%}',
    '.crr-spot u{position:absolute;top:-6px;right:-6px;width:20px;height:20px;border-radius:50%;background:#f6ead2;color:#2b2218;font-size:10px;text-decoration:none;display:grid;place-items:center}',
    '.crr-spot.is-full{border-style:solid;border-color:rgb(255 240 220 / .85)}',
    '.crr-slab{position:absolute;top:100%;left:50%;transform:translateX(-50%);margin-top:3px;font-size:11px;font-weight:600;color:#f6ead2;white-space:nowrap;pointer-events:none;background:rgb(20 14 10 / .7);padding:1px 8px;border-radius:9px}',
    '.crr-spot.lab-up .crr-slab{top:auto;bottom:100%;margin:0 0 3px}',
    '.crr-spot:hover,.crr-spot:focus-visible{border-color:#f6ead2;background:rgb(246 234 210 / .22);outline:none}',
    '.crr-spots.is-armed .crr-spot{border-color:#f6ead2;animation:crr-pulse 1.2s ease-in-out infinite}',
    '.crr-spot.is-hot{background:rgb(246 234 210 / .38);border-color:#fff}',
    '@keyframes crr-pulse{50%{box-shadow:0 0 0 7px rgb(246 234 210 / .25)}}',
    '.crr-spots.is-playing .crr-spot{opacity:.15}.crr-spots.is-playing.is-armed .crr-spot{opacity:1}',
    '.crr-fxtoast{position:absolute;left:50%;bottom:12px;transform:translateX(-50%);padding:4px 6px 4px 12px;border-radius:10px;background:rgb(17 24 39 / .9);color:#f9fafb;font-size:12.5px;display:flex;gap:6px;align-items:center;white-space:nowrap}',
    '.crr-fxtoast[hidden]{display:none}.crr-fxtoast .quiet-word{color:#c7d2fe;border:0;background:none;cursor:pointer;padding:0 6px}',
    '.crr-fxbar{display:flex;align-items:center;gap:10px;flex-wrap:wrap}',
    '.crr-fxbar .quiet-word{border:0;background:none;cursor:pointer;padding:0}',
    '.crr-fxedit{display:flex;flex-direction:column;gap:10px}.crr-fxedit .crr-ctl{max-width:360px}',
    '.crr-spotlist{list-style:none;margin:0;padding:0;display:grid;grid-template-columns:repeat(auto-fill,minmax(230px,1fr));gap:6px}',
    '.crr-spotlist li{display:grid;grid-template-columns:12px minmax(0,1fr) auto;gap:10px;align-items:center;min-height:44px;padding:4px 6px 4px 10px;border-radius:9px;background:var(--color-card-bg);border:1px solid var(--color-border-light)}',
    '.crr-dot{width:12px;height:12px;border-radius:50%;border:1.5px dashed var(--color-text-secondary)}',
    '.crr-sn{display:flex;flex-direction:column;line-height:1.25;min-width:0}.crr-sn b{font-size:12.5px;color:var(--color-text-primary)}.crr-sn em{font-style:normal;font-size:11px;color:var(--color-text-secondary)}',
    '.crr-sv{display:flex;gap:4px;flex-wrap:wrap;justify-content:flex-end}',
    '.crr-chipv{display:inline-flex;align-items:center;font-size:11.5px;font-weight:600;padding:0 0 0 9px;border-radius:999px;background:var(--crr-acc-s);color:var(--crr-ink)}',
    '.crr-chipv.is-mv{background:var(--color-bg-tertiary);color:var(--color-text-body)}',
    '.crr-chipv button{font:inherit;border:0;background:none;color:inherit;cursor:pointer;font-size:11px;width:32px;height:32px;border-radius:50%}',
    '.crr-chipv button:hover,.crr-chipv button:focus-visible{background:rgb(0 0 0 / .08)}',
    '.crr-shelf{display:flex;flex-direction:column;gap:8px}',
    '.crr-shh{display:flex;align-items:baseline;gap:10px;flex-wrap:wrap}',
    '.crr-looks{display:grid;grid-template-columns:repeat(auto-fill,minmax(96px,1fr));gap:6px}',
    '.crr-lk2{font:inherit;display:flex;flex-direction:column;gap:3px;padding:0 0 6px;border-radius:10px;border:1px solid var(--color-border);background:var(--color-card-bg);cursor:grab;overflow:hidden;color:var(--color-text-body);text-align:left;touch-action:none;user-select:none;-webkit-user-select:none;transition:border-color var(--dur-micro),box-shadow var(--dur-micro)}',
    '.crr-lk2 canvas{width:100%;height:44px;display:block}',
    '.crr-lk2 b{font-size:11.5px;font-weight:600;color:var(--color-text-primary);padding:0 7px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
    '.crr-lk2 small{font-size:10.5px;color:var(--color-text-secondary);padding:0 7px}',
    '.crr-lk2:hover{border-color:var(--crr-field);box-shadow:var(--elev-hover)}',
    '.crr-lk2.is-armed{border-color:var(--crr-acc);box-shadow:inset 0 0 0 2px var(--crr-acc)}',
    '.crr-lk2.is-lifted{opacity:.35}',
    '.crr-lk2.is-mv{padding-top:8px}.crr-mvi{font-size:16px;color:var(--color-text-secondary);padding:0 7px}',
    '.crr-ghost{position:fixed;z-index:200;pointer-events:none;transform:translate(-50%,-70%);display:flex;align-items:center;gap:7px;padding:7px 11px;border-radius:10px;background:var(--color-card-bg);box-shadow:var(--elev-dragged);font-size:12px;font-weight:600;color:var(--color-text-primary)}',
    '.crr-ghost i{width:12px;height:12px;border-radius:50%}',
    /* Smaller screens */
    '@media (max-width:1000px){.crr-fxgrid{grid-template-columns:minmax(0,1fr)}.crr-orail{flex-direction:row;overflow-x:auto;scrollbar-width:thin;padding-bottom:4px}',
    '  .crr-orail .crr-grp,.crr-railnote{display:none}.crr-ob{width:auto;flex:none;max-width:220px}}',
    '@media (max-width:760px){.crr-vcards{grid-template-columns:1fr}.crr-looks{grid-template-columns:repeat(3,minmax(0,1fr))}.crr-spot{width:36px;height:36px}.crr-slab{font-size:9.5px}.crr-stg-cap{font-size:14px}.crr-tl-foot>span:last-child{display:none}}',
    '@media (max-width:620px){.crr-body{padding:14px 12px 20px}.crr-rhd{flex-wrap:wrap;padding:12px}.crr-rside{width:100%;padding-left:32px;min-width:0}.crr-rside .crr-stc{min-width:0;overflow:hidden;text-overflow:ellipsis}.crr-stg-cap{display:none}',
    '  .crr-rsent,.crr-aij,.crr-aicap,.crr-rwarn{margin-left:12px}.crr-rsent{padding:0 12px 12px}.crr-rin{padding:4px 12px 14px}',
    '  .crr-step{grid-template-columns:1fr;gap:4px}.crr-ring{grid-template-columns:repeat(4,minmax(0,1fr))}.crr-tiles.crr-effs{grid-template-columns:repeat(3,minmax(0,1fr))}',
    '  .crr-tl-grid{grid-template-columns:112px repeat(var(--n),32px)}.crr-dayp{flex-direction:column}.crr-dt{min-width:0}.crr-dayp ul{flex:none}}'
  ].join('\n');

  function injectStyle() {
    if (document.getElementById('crr-style')) return;
    var st = document.createElement('style'); st.id = 'crr-style'; st.textContent = CSS; document.head.appendChild(st);
  }

  /* ---------- The demo: a made-up calendar and three recipes ----------
     Generic on purpose: a campaign's own recipes come from the crafting app.
     Two moons that cycle and one set by hand, four seasons, a few events, a
     rolled event and a month of weather, so every kind of rule has
     something to point at. */
  var MONTHS = ['Thaw', 'Seedtime', 'Rains', 'Blossom', 'Midsummer', 'Harvest', 'Leaffall', 'Mists', 'Gloaming', 'Deepwinter', 'Longnight', 'Waking'];
  var DEMO_CAL = {
    name: 'Demo calendar', mode: 'fantasy', tracks_real_time: false, leap_year_every: 0, leap_year_offset: 0,
    months: MONTHS.map(function (n) { return {name: n, days: 30}; }),
    moons: [
      {id: 1, name: 'The Lantern', cycle_days: 30, phase_offset: 4, color: '#f2d27a', phase_mode: 'auto'},
      {id: 2, name: 'The Ember', cycle_days: 19, phase_offset: 11, color: '#e0603a', phase_mode: 'auto'},
      {id: 3, name: 'The Veil', phase_mode: 'manual', color: '#a78bfa', state_names: {full: 'Open', half: 'Parted', crescent: 'Thin', dark: 'Drawn', vanished: 'Gone'},
        marks: [{year: 100, month: 9, day: 1, state: 'dark', shown: true}]}
    ],
    seasons: [{name: 'Greening', start_month: 1, start_day: 1, end_month: 3, end_day: 30}, {name: 'High Sun', start_month: 4, start_day: 1, end_month: 6, end_day: 30},
      {name: 'Fading', start_month: 7, start_day: 1, end_month: 9, end_day: 30}, {name: 'Frost', start_month: 10, start_day: 1, end_month: 12, end_day: 30}],
    current_year: 100, current_month: 9, current_day: 8
  };
  var DEMO_WEATHER = ['Clear', 'Clear', 'Partly Cloudy', 'Cloudy', 'Rain', 'Rain', 'Fog', 'Clear', 'Windy', 'Ley Surge', 'Clear', 'Overcast', 'Drizzle', 'Rain', 'Thunderstorm',
    'Clear', 'Clear', 'Mist', 'Cloudy', 'Blood Rain', 'Clear', 'Partly Cloudy', 'Snow', 'Sleet', 'Clear', 'Fog', 'Cloudy', 'Clear', 'Snow', 'Clear'];
  var LISTS = {benches: ['Alchemy bench', 'Forge', 'Field kit'], skills: ['Alchemy', 'Herbalism', 'Smithing'], items: ['Bloodnight draught', 'Spoiled draught', 'Moonwater'],
    marks: ['Moonlit', 'Twice-moonlit', 'Fresh-picked'], recipes: ['Umbral salts', 'Lantern oil'], tables: ['Moon quirks', 'Brewing mishaps']};
  function demoInput() {
    var y = 100, m = 9, d = 8, weather = {}, i;
    for (i = 0; i < DEMO_WEATHER.length; i++) { weather[y + '-' + m + '-' + d] = DEMO_WEATHER[i]; d++; if (d > 30) { d = 1; m++; } }
    return {
      calendar: DEMO_CAL, weather: weather,
      events: [{name: 'Lantern Festival', year: 100, month: 9, day: 15}, {name: 'Market day', year: 100, month: 9, day: 10}, {name: 'Market day', year: 100, month: 9, day: 17},
        {name: 'Market day', year: 100, month: 9, day: 24}, {name: 'Market day', year: 100, month: 10, day: 1}, {name: 'First Frost', year: 100, month: 10, day: 1}],
      eventNames: ['Lantern Festival', 'Market day', 'First Frost', 'The Long Night'],
      rolls: [{event: 'Ember omen', result: 'Smoulder', year: 100, month: 9, day: 12}, {event: 'Ember omen', result: 'Wildfire', year: 100, month: 9, day: 26},
        {event: 'Ember omen', result: 'Calm', year: 100, month: 10, day: 6}],
      rolled: [{name: 'Ember omen', results: ['Calm', 'Smoulder', 'Wildfire']}],
      lists: LISTS, knownDays: DEMO_WEATHER.length
    };
  }
  function S2(o) {
    var out = {};
    Object.keys(o).forEach(function (k) { var v = o[k]; out[k] = Array.isArray(v) ? {look: v[0], move: v[1]} : R.MOVES[v] ? {move: v} : {look: v}; });
    return out;
  }
  function moments(name) {
    return {success: {spots: S2({work: 'dawn', result: 'glass'}), words: name}, progress: {spots: S2({heat: 'ember'}), words: 'More work to do'},
      failure: {spots: S2({work: ['rot', 'shake'], heat: 'dim'}), words: 'Botched'}, breakthrough: {spots: S2({work: 'storm', window: 'glass', result: 'dawn'}), words: 'Breakthrough!'}};
  }
  function rule(when, then, chance, players, line, look) { return {when: when, then: then, chance: chance, players: players, bookLine: line, look: look}; }
  var DEMO_RULES = {
    heal: [rule([{kind: 'season', season: 'Frost'}], {kind: 'bane'}, 'always', 'book', 'Bloodmoss is thin and bitter in the frost months.', {spots: S2({window: 'frost'}), words: 'The moss is thin'})],
    caustic: [],
    moonlit: [
      rule([{kind: 'moon', moon: 'The Lantern', phase: 'full'}], {kind: 'edge'}, 'always', 'book', 'Brewed under a full Lantern, the moss gives up its colour easily.', {spots: S2({window: 'dawn', work: 'bloom'}), words: 'The Lantern is full'}),
      rule([{kind: 'moon', moon: 'The Ember', phase: 'full'}], {kind: 'cannot'}, 'always', 'book', 'The Ember’s full light thins it to water. Wait for it to wane.', {spots: S2({window: 'frost', heat: 'dim'}), words: 'Not tonight'}),
      rule([{kind: 'moon', moon: 'The Veil', phase: 'full'}], {kind: 'quirk', table: 'Moon quirks'}, '1 in 4', 'secret', '', null),
      rule([{kind: 'rolled', event: 'Ember omen', result: 'Wildfire'}], {kind: 'becomes', item: 'Bloodnight draught'}, 'always', 'whenItHappens', '', {spots: S2({window: 'rot', result: 'ember'}), words: 'Bloodnight draught'}),
      rule([{kind: 'weather', weather: 'Blood Rain'}], {kind: 'extra', amount: 1}, '1 in 6', 'whenItHappens', '', {spots: S2({window: 'ember', result: 'bloom'}), words: 'One extra'}),
      rule([{kind: 'event', event: 'Lantern Festival'}], {kind: 'discover', recipe: 'Umbral salts'}, '1 in 2', 'secret', '', null),
      rule([{kind: 'moon', moon: 'The Lantern', phase: 'full'}, {kind: 'weather', weather: 'Clear'}], {kind: 'mark', mark: 'Moonlit'}, 'always', 'whenItHappens', '', {spots: S2({window: 'glass', result: 'dawn'}), words: 'Moonlit'})
    ]
  };
  var RECIPES = [
    {key: 'heal', name: 'Healing potion', ings: [[1, 'Bloodmoss'], [1, 'Spring water']], goal: 30, stat: 'Reason', liquid: '#b4232f'},
    {key: 'caustic', name: 'Caustic flask', ings: [[1, 'Saltglass'], [1, 'Wyrmbile']], goal: 45, stat: 'Reason', liquid: '#7fae2e'},
    {key: 'moonlit', name: 'Moonlit draught', ings: [[2, 'Bloodmoss'], [1, 'Wolf fang'], [1, 'Spring water']], goal: 45, stat: 'Reason', liquid: '#9b1c2c'}
  ];
  /* With a real calendar the demo recipes keep their shape and get sample
     rules written in that calendar's names, since the demo's names aren't
     on it. */
  function sampleRules(W, key) {
    if (key === 'caustic') return [];
    if (key === 'heal') { var last = W.seasons[W.seasons.length - 1]; return last ? [rule([{kind: 'season', season: last.name}], {kind: 'bane'}, 'always', 'book', '', null)] : []; }
    return R.starters(W).filter(function (s) { return s.key !== 'night'; }).map(function (s) { return R.ruleData(s.rule); });
  }
  function recipes(W, real, fx) {
    return RECIPES.map(function (r) {
      var rules = R.cleanRules(real ? sampleRules(W, r.key) : DEMO_RULES[r.key], fx);
      rules.forEach(function (q) { if (!q.look && q.players !== 'secret') q.look = R.defLook(W, q); q._def = q.look ? R.clone(q.look) : null; });
      var looks = moments(r.name), clean = {};
      Object.keys(looks).forEach(function (k) { clean[k] = R.cleanLook(looks[k], fx.spots, fx.looks); });
      return {key: r.key, name: r.name, ings: r.ings, goal: r.goal, stat: r.stat, liquid: r.liquid, rules: rules, looks: clean, defaults: R.clone(clean)};
    });
  }

  /* ---------- Reading a real calendar ---------- */
  function getJSON(url) {
    return (window.Chronicle && Chronicle.apiFetch ? Chronicle.apiFetch(url) : fetch(url, {credentials: 'same-origin', headers: {Accept: 'application/json'}}))
      .then(function (r) { if (!r.ok) throw new Error('http ' + r.status); return r.json(); });
  }
  /* The months the next `span` days fall in, on the calendar's own month
     lengths. */
  function monthsAhead(cal, span) {
    var Sky = window.SkyWorld, y = cal.current_year, m = cal.current_month, d = cal.current_day, out = [{y: y, m: m}], n = (cal.months || []).length;
    if (!n || !Sky) return out;
    for (var i = 1; i < span; i++) {
      d++;
      if (d > Sky.monthDays(cal, m - 1, y)) { d = 1; m++; if (m > n) { m = 1; y++; } out.push({y: y, m: m}); }
    }
    return out;
  }
  /* load reads what the rules need, as the viewer may see it: the calendar,
     its events, rolls and weather for the days ahead, every event's name,
     the owner's own kinds of weather and each rolled event's results. A part
     that can't be read is left empty rather than failing the whole page. */
  function load(cid, calid, span) {
    var base = '/campaigns/' + encodeURIComponent(cid) + '/calendars/' + encodeURIComponent(calid);
    function each(ms, path, empty) { return Promise.all(ms.map(function (x) { return getJSON(base + path + '?year=' + x.y + '&month=' + x.m).catch(function () { return empty; }); })); }
    return getJSON(base).then(function (cal) {
      var ms = monthsAhead(cal, span);
      return Promise.all([each(ms, '/events', []), each(ms, '/weather/days', []), each(ms, '/rolls', {}),
        getJSON(base + '/events/index').catch(function () { return {data: []}; }), getJSON(base + '/weather/settings').catch(function () { return null; })]).then(function (got) {
        var names = {}, events = [], weather = {}, rolls = [], index = (got[3] && got[3].data) || [];
        index.forEach(function (e) { if (e && e.id && e.name) names[e.id] = e.name; });
        ms.forEach(function (x, i) {
          var list = Array.isArray(got[0][i]) ? got[0][i] : (got[0][i] && got[0][i].data) || [];
          list.forEach(function (e) { if (e && e.id && e.name) names[e.id] = e.name; });
          events = events.concat(R.flattenEvents(list, x.y, x.m));
          var wd = Array.isArray(got[1][i]) ? got[1][i] : (got[1][i] && got[1][i].data) || [];
          wd.forEach(function (w) { if (w && w.preset_label) weather[w.year + '-' + w.month + '-' + w.day] = {label: w.preset_label, icon: w.icon}; });
        });
        var rolledIds = [];
        ms.forEach(function (x, i) {
          rolls = rolls.concat(R.flattenRolls(got[2][i], names));
          Object.keys(got[2][i] || {}).forEach(function (id) { if (names[id] && rolledIds.indexOf(id) < 0) rolledIds.push(id); });
        });
        return Promise.all(rolledIds.map(function (id) {
          return getJSON(base + '/events/' + encodeURIComponent(id) + '/roll?year=' + cal.current_year).then(function (v) {
            var res = ((v && v.config && v.config.results) || []).map(function (b) { return b.name; });
            ((v && v.nights) || []).forEach(function (nt) { if (nt.result && res.indexOf(nt.result) < 0) res.push(nt.result); });
            return {name: names[id], results: res};
          }, function () { return {name: names[id], results: []}; });
        })).then(function (rolled) {
          rolls.forEach(function (r) { var e = rolled.filter(function (x) { return x.name === r.event; })[0]; if (e && e.results.indexOf(r.result) < 0) e.results.push(r.result); });
          return {calendar: cal, events: events, eventNames: index.map(function (e) { return e.name; }), weather: weather, rolls: rolls, rolled: rolled,
            weatherKinds: (got[4] && got[4].kinds) || [], lists: LISTS, knownDays: span, truncated: !!(got[3] && got[3].truncated)};
        });
      });
    });
  }

  /* ---------- The widget ---------- */
  function fa(n) { return '<i class="fa-solid fa-' + n + '" aria-hidden="true"></i>'; }
  function calm() { return (document.documentElement.getAttribute('data-motion') || 'full') !== 'full'; }

  /* Counts mounts, so two editors on a page keep their tab ids apart. */
  var mounts = 0;
  function start(el, input, note) {
    var fx = {spots: FX.MOMENT_SPOTS, looks: FX.LOOKS}, W = R.world(Object.assign({sky: window.SkyWorld, span: 30}, input));
    var S = {W: W, at: 0, fx: fx, pick: 2, recipes: recipes(W, !!input.real, fx), calm: calm};
    var ed = null, looks = null, tab = 'rules', uid = 'crr' + (++mounts);
    S.rec = function () { return S.recipes[S.pick]; };
    S.setDay = function (d) { S.at = d; };
    S.changed = function () {};
    S.say = function (msg) { var lv = el.querySelector('.crr-live'); if (lv) { lv.textContent = ''; setTimeout(function () { lv.textContent = msg; }, 30); } };
    S.goLook = function (id) { show('looks'); looks.select(id); };
    function head() {
      var r = S.rec();
      el.querySelector('.crr-t').innerHTML = '<b>' + R.esc(r.name) + '</b><small>' + r.ings.map(function (g) { return g[0] + ' ' + R.esc(g[1]); }).join(' · ') + ' → <b>' + R.esc(r.name) +
        '</b> · 2d10 + ' + R.esc(r.stat) + ', goal ' + r.goal + '</small>';
      Array.prototype.forEach.call(el.querySelectorAll('[data-recipe]'), function (b) { b.setAttribute('aria-pressed', String(+b.getAttribute('data-recipe') === S.pick)); });
    }
    function moveBar() {
      var b = el.querySelector('[role="tab"][aria-selected="true"]'), bar = el.querySelector('.crr-tabbar');
      if (b && bar && b.offsetWidth) { bar.style.left = b.offsetLeft + 'px'; bar.style.width = b.offsetWidth + 'px'; }
    }
    function show(k) {
      tab = k;
      Array.prototype.forEach.call(el.querySelectorAll('[role="tab"]'), function (b) { var on = b.getAttribute('data-tab') === k; b.setAttribute('aria-selected', String(on)); b.tabIndex = on ? 0 : -1; });
      Array.prototype.forEach.call(el.querySelectorAll('[role="tabpanel"]'), function (p) { p.hidden = p.getAttribute('data-pane') !== k; });
      if (k === 'looks') { if (!looks) looks = CraftRulesLooks.mount(el.querySelector('[data-pane="looks"]'), S); else looks.refresh(); }
      else if (ed) ed.rules();
      moveBar();
    }
    el.innerHTML = '<div class="crr">' +
      '<div class="crr-head"><div class="crr-t"></div><div class="crr-chips" role="group" aria-label="Recipe">' + S.recipes.map(function (r, i) {
        return '<button type="button" class="crr-cx" data-recipe="' + i + '" aria-pressed="false"><span style="width:10px;height:10px;border-radius:50%;background:' + r.liquid + '"></span>' + R.esc(r.name) + '</button>';
      }).join('') + '</div><button type="button" class="btn btn-secondary btn-sm" data-ai aria-pressed="false">' + fa('code') + '<span>Show what the AI writes</span></button></div>' +
      '<div class="crr-banner' + (note.bad ? ' is-bad' : '') + '">' + note.html + '</div>' +
      '<div class="crr-tabs" role="tablist" aria-label="Rule editor"><button type="button" role="tab" id="' + uid + '-t-rules" data-tab="rules" aria-controls="' + uid + '-p-rules" aria-selected="true">' + fa('list-check') + 'When it can be made</button>' +
      '<button type="button" role="tab" id="' + uid + '-t-looks" data-tab="looks" aria-controls="' + uid + '-p-looks" aria-selected="false" tabindex="-1">' + fa('wand-magic-sparkles') + 'How it looks</button><span class="crr-tabbar"></span></div>' +
      '<div class="crr-body"><div role="tabpanel" id="' + uid + '-p-rules" aria-labelledby="' + uid + '-t-rules" data-pane="rules"></div>' +
      '<div role="tabpanel" id="' + uid + '-p-looks" aria-labelledby="' + uid + '-t-looks" data-pane="looks" hidden></div></div><div class="crr-sr crr-live" aria-live="polite"></div></div>';
    head();
    ed = CraftRulesEditor.mount(el.querySelector('[data-pane="rules"]'), S);
    function onClick(e) {
      var rb = e.target.closest('[data-recipe]');
      if (rb) { S.pick = +rb.getAttribute('data-recipe'); head(); ed.refresh(); if (looks) looks.refresh(); return; }
      var t = e.target.closest('[role="tab"]'); if (t) { show(t.getAttribute('data-tab')); return; }
      var ai = e.target.closest('[data-ai]');
      if (ai) { var on = ai.getAttribute('aria-pressed') !== 'true'; ai.setAttribute('aria-pressed', String(on)); el.querySelector('.crr').classList.toggle('is-ai', on); }
    }
    function onKey(e) {
      var t = e.target; if (t.getAttribute('role') !== 'tab') return;
      var ts = Array.prototype.slice.call(el.querySelectorAll('[role="tab"]')), i = ts.indexOf(t), n = e.key === 'ArrowRight' ? i + 1 : e.key === 'ArrowLeft' ? i - 1 : null;
      if (n == null) return; e.preventDefault(); n = (n + ts.length) % ts.length; ts[n].focus(); show(ts[n].getAttribute('data-tab'));
    }
    el.addEventListener('click', onClick); el.addEventListener('keydown', onKey); window.addEventListener('resize', moveBar);
    requestAnimationFrame(moveBar);
    return {destroy: function () {
      el.removeEventListener('click', onClick); el.removeEventListener('keydown', onKey); window.removeEventListener('resize', moveBar);
      if (ed) ed.destroy(); if (looks) looks.destroy();
    }};
  }

  Chronicle.register('craft_rules', {
    init: function (el) {
      injectStyle();
      if (!R || !FX || !window.CraftRulesEditor || !window.CraftRulesLooks || !window.SkyWorld) {
        el.innerHTML = '<p class="crr-load" role="status">The rules editor couldn’t load. Reload the page to try again.</p>';
        return;
      }
      var ds = el.dataset, demo = '<span><b>Demo calendar.</b> Open this page with <code>?campaign_id=…&amp;calendar_id=…</code> to write rules against your own calendar. Nothing here is saved.</span>';
      if (!ds.campaignId || !ds.calendarId) { el._crr = start(el, demoInput(), {html: demo}); return; }
      el.innerHTML = '<p class="crr-load" role="status">Reading the calendar…</p>';
      el._crrGen = (el._crrGen || 0) + 1;
      var gen = el._crrGen;
      load(ds.campaignId, ds.calendarId, 30).then(function (input) {
        if (gen !== el._crrGen || !el.isConnected) return;
        input.real = true;
        el._crr = start(el, input, {html: '<span><b>' + R.esc(input.calendar.name || 'Your calendar') + '</b>, as you can see it: its moons, events, rolls and the next 30 days of weather. The sample rules are written in its names. Nothing here is saved.</span>' +
          (input.truncated ? '<span>It has more events than one list holds, so the event picker shows the first ones.</span>' : '')});
      }, function (err) {
        if (gen !== el._crrGen || !el.isConnected) return;
        if (window.console) console.warn('[craft-rules] the calendar could not be read', err);
        el._crr = start(el, demoInput(), {bad: true, html: '<span><b>That calendar couldn’t be read</b> (it may not exist, or you may not be able to see it). Showing the demo calendar instead. Nothing here is saved.</span>'});
      });
    },
    destroy: function (el) {
      el._crrGen = (el._crrGen || 0) + 1;
      if (el._crr) { el._crr.destroy(); el._crr = null; }
      el.innerHTML = '';
    }
  });
})();
