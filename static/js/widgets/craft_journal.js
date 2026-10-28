/**
 * craft_journal.js -- the paper journal skin for the crafting bench (window.CraftJournal)
 *
 * The crafter's own notebook, open on a desk and seen from above. It plays on
 * CraftScene like every skin, with the same steps, grading and roll, drawn in
 * ink on the page. What only paper does lives here: the quill writes each new
 * line as it happens, a miss leaves an ink blot that stays with the recipe,
 * changing recipe turns the page, and a finished recipe is sealed in wax with
 * the crafter's signet.
 *
 * The desk, binding, paper and props are painted once into a picture, since
 * every texture is filter work and far too slow to redraw each frame. The ink
 * and the moving parts stay live over it, and a small light map multiplies the
 * candle and the window's light over everything.
 *
 * The date in the margin, the moons and the window's light come from the
 * campaign's calendar as players see it (?audience=players): this is a
 * player's book, so a Director previewing it sees no secret moon either.
 *
 * CraftJournal.create(opts) makes one skin for one scene:
 *   opts.sky     {campaignId, calendarId} for the live calendar, or
 *                {calendar} to show that calendar object as it is
 *   opts.thumbs  the folder holding the build tray's pictures
 *   opts.info    hover cards to add or change, by piece id
 *   opts.look    a saved look ({stock, ink, hand, ...}) to start from
 */
window.CraftJournal = (function () {
  function create(opts) {
  opts = opts || {};
  var host = {scope: function (h) { return h; }, id: function (n) { return n; }, calm: false}, API = null, dead = false, urls = [];
  function sc(h){return host.scope(h);}
  /* The owner's choices. Each is a small table of values, so a look is data,
     never a second copy of the drawing code. */
  var O={stock:'parchment',ink:'gall',hand:'kalam',seal:'scarlet',emblem:'moth',binding:'oxblood',light:'candle',window:'calendar',age:'worn',
    note:'Measure twice, pour once.',kit:[]};
  var STOCK={
    parchment:{name:'Parchment',pp:'#ecdcba',cut:'#f6ecd4',under:'#d6c39a',edge:'#b9a57c',tone:'#8a5a20',fibre:[.42,.3,.14],laid:true},
    cotton:{name:'Cotton rag',pp:'#efe9dc',cut:'#faf6ec',under:'#dbd2bf',edge:'#c4b9a2',tone:'#7a6440',fibre:[.38,.34,.26]},
    field:{name:'Field book',pp:'#e8e2cc',cut:'#f4efdd',under:'#d4ccb1',edge:'#bdb497',tone:'#6a5a30',fibre:[.36,.32,.2],grid:true},
    vellum:{name:'Vellum',pp:'#eee1c8',cut:'#f8efdc',under:'#dcc9a6',edge:'#c7b08a',tone:'#94643a',fibre:[.5,.36,.22],mottle:true},
    grimoire:{name:'Grimoire',pp:'#2d2622',cut:'#3b332d',under:'#1e1a17',edge:'#5b4d41',tone:'#000000',fibre:[.85,.75,.6],dark:true}
  };
  // Each ink on light paper, and the pale version of it that grimoire pages take
  var INK={gall:{name:'Iron gall',c:'#2c2119',d:'#e9dcc0'},sepia:{name:'Sepia',c:'#5c3a20',d:'#e6bf92'},indigo:{name:'Indigo',c:'#23325f',d:'#aec2f2'},
    viridian:{name:'Viridian',c:'#1d4a3a',d:'#9ed9c0'},crimson:{name:'Crimson',c:'#6c1a22',d:'#f2a0a0'},gold:{name:'Gold',c:'#8a6418',d:'#f0cb6c'}};
  // k evens out how big each hand writes, so every hand fills a line the same
  var HAND={kalam:{name:'Quick',f:'"Crafting Kalam",cursive',k:1},caveat:{name:'Loose',f:'"Crafting Caveat",cursive',k:1.12},
    apple:{name:'Flowing',f:'"Crafting Apple",cursive',k:.74},fine:{name:'Fine',f:'"Crafting Cedarville",cursive',k:.94},fell:{name:'Old print',f:'"Crafting Fell",Georgia,serif',k:.96}};
  var SEAL={scarlet:{name:'Scarlet',c:'#a3201c'},oxblood:{name:'Oxblood',c:'#5e1616'},forest:{name:'Forest',c:'#2e5226'},
    midnight:{name:'Midnight',c:'#1f2d52'},black:{name:'Black',c:'#1e1b1b'},gold:{name:'Gold',c:'#b8892c'}};
  var EMBLEM={
    moth:{name:'Moth',d:'M0,-3 C-4,-12 -15,-14 -16,-6 C-17,1 -8,3 -2,1 C-9,4 -13,11 -8,14 C-4,16 -1,9 0,4 C1,9 4,16 8,14 C13,11 9,4 2,1 C8,3 17,1 16,-6 C15,-14 4,-12 0,-3 Z M0,-4 L0,9 M-1,-5 C-3,-10 -6,-12 -8,-13 M1,-5 C3,-10 6,-12 8,-13'},
    key:{name:'Key',d:'M-11,-6 a6,6 0 1 0 0.01,0 Z M-5,-6 L13,-6 M8,-6 L8,-1 M12,-6 L12,0 M-11,-9 a3,3 0 1 0 0.01,0'},
    crescent:{name:'Two moons',d:'M-2,-12 A12,12 0 1 0 -2,12 A9,9 0 1 1 -2,-12 Z M10,-6 a4,4 0 1 0 0.01,0 Z'},
    leaf:{name:'Leaf',d:'M-12,12 C-14,-4 -2,-14 13,-13 C13,2 3,13 -12,12 Z M-12,12 L7,-7 M-5,5 L-5,-2 M0,0 L0,-6 M-5,5 L2,5 M0,0 L6,0'},
    flask:{name:'Flask',d:'M-3,-14 L3,-14 M-2,-14 L-2,-5 C-11,-2 -12,11 -4,13 L4,13 C12,11 11,-2 2,-5 L2,-14 M-8,5 L8,5'}
  };
  var BIND={oxblood:{name:'Oxblood',c:'#5a1e18',r:'#b8892c'},forest:{name:'Forest',c:'#233a28',r:'#9a2a22'},black:{name:'Black',c:'#1d1918',r:'#7a1a1a'},
    tan:{name:'Saddle',c:'#7a4c26',r:'#2a3f6a'},midnight:{name:'Midnight',c:'#1d2840',r:'#c9a54a'}};
  /* What lights the page, and how it moves. The window's light follows the
     calendar unless the owner picks other weather to preview. */
  var LIGHT={candle:{name:'Candle',c:[255,196,128],r:880,flick:1,glow:'255,190,110'},lantern:{name:'Lantern',c:[255,222,170],r:1040,flick:.35,glow:'255,214,150'},
    witch:{name:'Witchlight',c:[186,160,255],r:820,flick:.5,bob:1,glow:'176,150,255'},none:{name:'Snuffed',c:[0,0,0],r:1,flick:0,glow:'0,0,0'}};
  var WIN={calendar:{name:'From the calendar'},rain:{name:'Rain'},snow:{name:'Snow'},morning:{name:'Morning'}};
  var AGE={clean:{name:'New'},worn:{name:'Kept'},battered:{name:'Battered'}};
  var TABLES={stock:STOCK,ink:INK,hand:HAND,seal:SEAL,emblem:EMBLEM,binding:BIND,light:LIGHT,window:WIN,age:AGE};
  // The build tray's words for every choice, in the order the tray shows them
  var CHOICES={};Object.keys(TABLES).forEach(function(k){var T=TABLES[k];CHOICES[k]=Object.keys(T).map(function(v){return [v,T[v].name];});});
  // A saved look is checked against the tables, so an unknown value keeps the default.
  function setLook(o){Object.keys(o||{}).forEach(function(k){var v=o[k];
    if(TABLES[k]&&TABLES[k].hasOwnProperty(v))O[k]=v;
    else if(k==='note')O.note=String(v==null?'':v).slice(0,120);});}
  setLook(opts.look);

  function stock(){return STOCK[O.stock];}
  function inkHex(){var i=INK[O.ink];return stock().dark?i.d:i.c;}
  function ink(){return CraftScene.rgb(inkHex());}
  function esc(t){return CraftScene.esc(t);}

  var L={
    mortar:{x:700,y:524,r:58},
    flask:{x:884,y:404,r:74,neck:252,neckW:16},
    burner:{x:884,y:562},
    slots:[[200,568],[338,562],[476,570]],
    gauge:{x:704,y:330},bar:{x:672,y:140,w:320},dice:{x:1010,y:450},result:{x:1012,y:356},
    pour:{x:831,y:209,rot:125,mouth:[872,238]},
    tip:{x:838,y:196,rot:65,mx:876,my:250},
    seal:{x:520,y:604},candle:{x:1150,y:92},signet:{x:1152,y:556},quill:{x:1134,y:338}
  };
  // Where each ingredient's card was taped, so its tape stays on the page once it is used
  var SPOT={},ITEMC={};
  function itemSpot(k,i){var h=L.slots[i%L.slots.length];SPOT[k]=h;return h;}

  /* A tiny seeded random, so the baked stains and fibres land in the same
     place every time the page is drawn. */
  function rng(seed){var s=seed>>>0;return function(){s=(s*1664525+1013904223)>>>0;return s/4294967296;};}
  function f1(v){return v.toFixed(1);}
  var R=function(a,b){return CraftScene.rnd(a,b);};

  var PAGE_L='M122,40 C300,31 520,31 597,49 L597,669 C520,677 300,678 122,671 Q115,356 122,40 Z';
  var PAGE_R='M1078,40 C900,31 680,31 603,49 L603,669 C680,677 900,678 1078,671 Q1085,356 1078,40 Z';

  /* Tonight, from the calendar as players see it: the date line, the moons
     for the margin and the weather for the window. Moons hidden from players
     are never drawn, whoever is looking. */
  var SKY={ok:false,line:'',label:'',moons:[],wx:'clear',day:false};
  var HAND_PHASE={full:.5,half:.25,crescent:.125,dark:0,vanished:0};
  function phaseName(p){var n=['new','a waxing crescent','at first quarter','waxing gibbous','full','waning gibbous','at last quarter','a waning crescent'];return n[Math.round(p*8)%8];}
  function skyFrom(cal,dayWx){var W2=window.SkyWorld;SKY.ok=false;if(!cal||!cal.months||!cal.months.length)return;
    var y=+cal.current_year||1,m=+cal.current_month||1,d=+cal.current_day||1,mo=cal.months[m-1]||{},hpd=+cal.hours_per_day||24,h=+cal.current_hour||0;
    var wx=dayWx||cal.weather||{},id=String(wx.preset_id||wx.preset_label||'').toLowerCase();
    SKY.label=String(wx.preset_label||'');
    SKY.wx=/snow|sleet|blizzard|hail/.test(id)?'snow':/rain|storm|drizzle|shower|thunder/.test(id)?'rain':'clear';
    SKY.day=cal.hasOwnProperty('current_hour')&&h/hpd>=.27&&h/hpd<.77;
    SKY.line=(mo.name?mo.name+' ':'')+d+', '+y+(cal.epoch_name?' '+cal.epoch_name:'')+(SKY.label?' · '+SKY.label.toLowerCase():'');
    SKY.moons=[];
    if(W2&&W2.dayIndex){var t=W2.dayIndex(cal,y,m,d)+(h/hpd);
      (cal.moons||[]).filter(function(x){return x&&!x.hidden_from_players;}).slice(0,3).forEach(function(x){
        var manual=x.phase_mode==='manual',p=manual?HAND_PHASE[W2.handState(cal,x,Math.floor(t))]||0:W2.phaseAt(x,t),cy=+x.cycle_days||0,note='';
        if(!manual&&cy>0){var toFull=((.5-p)%1+1)%1*cy,toNew=((1-p)%1+1)%1*cy,k=Math.round(Math.min(toFull,toNew));
          note=k<1?'':(toFull<toNew?'full':'new')+' in '+k+(k===1?' day':' days');}
        SKY.moons.push({name:String(x.name||'A moon'),p:p,color:/^#[0-9a-f]{6}$/i.test(x.color)?x.color:'#e5e7eb',note:note});});}
    SKY.ok=true;}
  function getJSON(url){return (window.Chronicle&&Chronicle.apiFetch?Chronicle.apiFetch(url):fetch(url,{credentials:'same-origin',headers:{Accept:'application/json'}}))
    .then(function(r){if(!r.ok)throw new Error('http '+r.status);return r.json();});}
  function loadSky(){var o=opts.sky||{};
    if(o.calendar){skyFrom(o.calendar,null);skyChanged();return;}
    if(!o.campaignId||!o.calendarId)return;
    var base='/campaigns/'+encodeURIComponent(o.campaignId)+'/calendars/'+encodeURIComponent(o.calendarId);
    getJSON(base+'?audience=players').then(function(cal){
      return getJSON(base+'/weather/days?year='+cal.current_year+'&month='+cal.current_month+'&audience=players').then(function(days){
        days=Array.isArray(days)?days:(days&&days.data)||[];
        return days.filter(function(x){return x&&x.year===cal.current_year&&x.month===cal.current_month&&x.day===cal.current_day;})[0]||null;},function(){return null;})
        .then(function(wx){if(dead)return;skyFrom(cal,wx);skyChanged();});
    }).catch(function(err){if(window.console)console.warn('[craft-journal] the calendar could not be read',err);});}
  function skyChanged(){tonightInfo();if(API){API.rain(!!winLook().rain);API.rerender();}}
  /* The still picture: desk, binding, page block, paper, wear and props, lit
     flat (the light map adds the light). It is drawn as its own SVG document
     and shown as an image, so it carries no web fonts and no CSS variables.
     Its two lit textures use a distant light at half strength, doubled
     afterwards, so nothing clips inside the filter (craft_bench.ai.md). */
  function bake(){
    var st=stock(),bd=BIND[O.binding],R=rng(7),age=O.age,dark=!!st.dark;
    var fb=st.fibre;
    var d='<defs>'+
      '<filter id="wood" x="0" y="0" width="100%" height="100%"><feTurbulence type="fractalNoise" baseFrequency=".0035 .085" numOctaves="4" seed="3"/><feColorMatrix values="0 0 0 0 .2  0 0 0 0 .11  0 0 0 0 .05  0 0 0 -1.6 1.25"/><feComposite in2="SourceGraphic" operator="in"/></filter>'+
      '<filter id="woodfine" x="0" y="0" width="100%" height="100%"><feTurbulence type="fractalNoise" baseFrequency=".02 .6" numOctaves="2" seed="8"/><feColorMatrix values="0 0 0 0 .08  0 0 0 0 .04  0 0 0 0 .02  0 0 0 .55 -.18"/><feComposite in2="SourceGraphic" operator="in"/></filter>'+
      '<filter id="lth" x="0" y="0" width="100%" height="100%"><feTurbulence type="fractalNoise" baseFrequency=".75" numOctaves="3" seed="11" result="n"/><feDiffuseLighting in="n" surfaceScale="1.8" lighting-color="#808080" result="l"><feDistantLight azimuth="-40" elevation="58"/></feDiffuseLighting>'+
        '<feTurbulence type="fractalNoise" baseFrequency=".012" numOctaves="3" seed="4" result="m"/><feColorMatrix in="m" values="0 0 0 0 1  0 0 0 0 1  0 0 0 0 1  .7 .7 .7 0 -.6" result="mm"/>'+
        '<feComposite in="l" in2="SourceGraphic" operator="arithmetic" k1="2.1" k2="0" k3="0" k4="0" result="lit"/><feComposite in="mm" in2="lit" operator="arithmetic" k1="0" k2="-.16" k3="1" k4="0" result="mot"/><feComposite in="mot" in2="SourceAlpha" operator="in"/></filter>'+
      '<filter id="fibre" x="0" y="0" width="100%" height="100%"><feTurbulence type="fractalNoise" baseFrequency=".01 .11" numOctaves="3" seed="4"/><feColorMatrix values="0 0 0 0 '+fb[0]+'  0 0 0 0 '+fb[1]+'  0 0 0 0 '+fb[2]+'  0 0 0 '+(dark?.16:.13)+' 0"/><feComposite in2="SourceGraphic" operator="in"/></filter>'+
      '<filter id="tooth" x="0" y="0" width="100%" height="100%"><feTurbulence type="fractalNoise" baseFrequency=".85" numOctaves="2" seed="5" result="n"/><feDiffuseLighting in="n" surfaceScale=".7" lighting-color="#808080"><feDistantLight azimuth="-40" elevation="62"/></feDiffuseLighting><feColorMatrix values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  -.66 0 0 0 .3"/><feComposite in2="SourceGraphic" operator="in"/></filter>'+
      '<filter id="mottle" x="0" y="0" width="100%" height="100%"><feTurbulence type="fractalNoise" baseFrequency=".008" numOctaves="4" seed="21"/><feColorMatrix values="0 0 0 0 .55  0 0 0 0 .36  0 0 0 0 .16  0 0 0 '+(st.mottle?.55:dark?.0:.28)+' -.12"/><feComposite in2="SourceGraphic" operator="in"/></filter>'+
      '<filter id="deckle" x="-2%" y="-2%" width="104%" height="104%"><feTurbulence type="fractalNoise" baseFrequency=".09" numOctaves="2" seed="6"/><feDisplacementMap in="SourceGraphic" scale="3.2"/></filter>'+
      '<filter id="wash" x="-20%" y="-20%" width="140%" height="140%"><feTurbulence type="fractalNoise" baseFrequency=".05" numOctaves="3" seed="9"/><feDisplacementMap in="SourceGraphic" scale="9"/></filter>'+
      '<filter id="b2"><feGaussianBlur stdDeviation="2"/></filter><filter id="b6" x="-20%" y="-20%" width="140%" height="140%"><feGaussianBlur stdDeviation="6"/></filter><filter id="b14" x="-20%" y="-20%" width="140%" height="140%"><feGaussianBlur stdDeviation="14"/></filter>'+
      '<linearGradient id="gutL" x1="0" x2="1"><stop offset="0" stop-color="#000" stop-opacity="0"/><stop offset=".55" stop-color="#2a1a08" stop-opacity=".1"/><stop offset=".86" stop-color="#2a1a08" stop-opacity=".42"/><stop offset="1" stop-color="#1a0e04" stop-opacity=".72"/></linearGradient>'+
      '<linearGradient id="gutR" x1="1" x2="0"><stop offset="0" stop-color="#000" stop-opacity="0"/><stop offset=".55" stop-color="#2a1a08" stop-opacity=".1"/><stop offset=".86" stop-color="#2a1a08" stop-opacity=".42"/><stop offset="1" stop-color="#1a0e04" stop-opacity=".72"/></linearGradient>'+
      /* The page bulges up off the spine, so a soft band of it faces the light */
      '<linearGradient id="hump" x1="0" x2="1"><stop offset="0" stop-color="#fff" stop-opacity="0"/><stop offset=".5" stop-color="#fff" stop-opacity="'+(dark?.05:.22)+'"/><stop offset="1" stop-color="#fff" stop-opacity="0"/></linearGradient>'+
      '<radialGradient id="toneE" cx=".5" cy=".5" r=".72"><stop offset=".7" stop-color="'+st.tone+'" stop-opacity="0"/><stop offset="1" stop-color="'+st.tone+'" stop-opacity="'+(age==='clean'?.06:age==='worn'?.2:.34)+'"/></radialGradient>'+
      '<linearGradient id="rib" x1="0" x2="1"><stop offset="0" stop-color="#000" stop-opacity=".35"/><stop offset=".35" stop-color="#fff" stop-opacity=".18"/><stop offset=".6" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".35"/></linearGradient>'+
      '<radialGradient id="brass" cx=".35" cy=".3" r=".8"><stop offset="0" stop-color="#f3d58a"/><stop offset=".45" stop-color="#b98a34"/><stop offset="1" stop-color="#5a3c12"/></radialGradient>'+
      '<radialGradient id="glassk" cx=".35" cy=".3" r=".9"><stop offset="0" stop-color="#5a5462"/><stop offset=".5" stop-color="#1c1820"/><stop offset="1" stop-color="#0c0a0e"/></radialGradient>'+
      '<radialGradient id="waxtop" cx=".4" cy=".35" r=".7"><stop offset="0" stop-color="#fff8e6"/><stop offset=".7" stop-color="#efe0bf"/><stop offset="1" stop-color="#cdb88e"/></radialGradient>'+
      '<pattern id="grid" width="22" height="22" patternUnits="userSpaceOnUse"><path d="M22,0 V22 M0,22 H22" stroke="#4f7fb4" stroke-opacity=".2" stroke-width="1" fill="none"/></pattern>'+
      '<pattern id="laid" width="34" height="5" patternUnits="userSpaceOnUse"><path d="M0,2.5 H34" stroke="#7a5a2a" stroke-opacity=".035" stroke-width="1"/><path d="M17,0 V5" stroke="#7a5a2a" stroke-opacity=".035" stroke-width="1.2"/></pattern>'+
      '<clipPath id="pages"><path d="'+PAGE_L+' '+PAGE_R+'"/></clipPath>'+
      '</defs>';
    var s='<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="700" viewBox="0 0 1200 700">'+d;
    /* The desk: walnut planks, the far edge falling into dark */
    s+='<rect width="1200" height="700" fill="#5a3a22"/><rect width="1200" height="700" fill="#000" filter="url(#wood)"/><rect width="1200" height="700" fill="#000" filter="url(#woodfine)"/>';
    [0,232,470].forEach(function(y,i){s+='<path d="M0,'+y+' H1200" stroke="#120904" stroke-width="3" stroke-opacity=".8"/><path d="M0,'+(y+2.5)+' H1200" stroke="#a2744a" stroke-width="1" stroke-opacity=".25"/>';});
    /* Desk props, left: a bundle of drying herbs and a few spilled seeds */
    s+='<g transform="translate(46,170) rotate(-12)"><path d="M0,0 C4,-60 -6,-120 -2,-170" stroke="#000" stroke-opacity=".35" stroke-width="10" filter="url(#b6)" transform="translate(-10,14)"/>';
    for(var h=0;h<9;h++){var a=-12+h*3,len=120+R()*50;s+='<path d="M0,0 C'+f1(a*.6)+','+f1(-len*.4)+' '+f1(a*1.4)+','+f1(-len*.8)+' '+f1(a*2)+','+f1(-len)+'" stroke="#5d6b3a" stroke-width="2" fill="none"/>';
      for(var q=0;q<7;q++){var t=.3+q*.1,lx=a*2*t,ly=-len*t;s+='<ellipse cx="'+f1(lx+(q%2?5:-5))+'" cy="'+f1(ly)+'" rx="7" ry="2.6" fill="'+(h%3?'#7d8a52':'#8f7aa8')+'" transform="rotate('+(q%2?-35:35)+' '+f1(lx)+' '+f1(ly)+')"/>';}}
    s+='<path d="M-14,-6 Q0,-18 14,-6 L12,6 Q0,-4 -12,6 Z" fill="#b39468"/><path d="M-12,0 Q0,-10 12,0" stroke="#8a6a40" fill="none"/></g>';
    for(var k=0;k<14;k++)s+='<ellipse cx="'+f1(30+R()*60)+'" cy="'+f1(330+R()*120)+'" rx="2.4" ry="1.5" fill="#3a2a14" transform="rotate('+f1(R()*180)+')" opacity=".85"/>';
    s+='<g transform="translate(58,560) rotate(24)"><path d="M0,-44 C24,-30 26,20 0,46 C-26,20 -24,-30 0,-44 Z" fill="#8a5a24" opacity=".85"/><path d="M0,-40 V44 M0,-14 L14,-24 M0,6 L-15,-6 M0,22 L13,12" stroke="#5a3814" fill="none"/></g>';
    /* Desk props, right: the candle in its dish, the inkwell, a stick of sealing wax */
    var C=L.candle;
    s+='<g transform="translate('+C.x+','+C.y+')"><ellipse cx="-16" cy="20" rx="60" ry="56" fill="#000" opacity=".4" filter="url(#b14)"/><circle r="56" fill="url(#brass)"/><circle r="47" fill="none" stroke="#5a3c12" stroke-opacity=".6" stroke-width="2"/>'+
      '<path d="M44,24 a16,16 0 1 1 18,10" fill="none" stroke="url(#brass)" stroke-width="7"/><circle r="27" fill="url(#waxtop)"/><path d="M-27,4 q-6,10 -2,16 q4,4 6,-4 M20,-14 q8,6 5,14" fill="#ead9b3"/><circle r="27" fill="none" stroke="#a89068" stroke-opacity=".5"/><circle r="6" fill="#d8c294"/></g>';
    var I={x:1150,y:318};
    s+='<g transform="translate('+I.x+','+I.y+')"><ellipse cx="-14" cy="16" rx="40" ry="38" fill="#000" opacity=".5" filter="url(#b6)"/><path d="M-30,-6 L-24,-28 L24,-28 L30,-6 L24,24 L-24,24 Z" fill="url(#glassk)"/><path d="M-24,-28 L24,-28 L30,-6 L24,24" fill="none" stroke="#fff" stroke-opacity=".18"/><circle r="15" fill="#0e0b12"/><circle r="15" fill="none" stroke="#b98a34" stroke-width="3"/><path d="M-6,-6 a9,9 0 0 1 9,-4" stroke="#fff" stroke-opacity=".35" stroke-width="2" fill="none"/></g>';
    s+='<g transform="translate(1166,628) rotate(-78)"><rect x="-58" y="-9" width="104" height="18" rx="3" fill="#000" opacity=".35" filter="url(#b6)" transform="translate(6,8)"/><rect x="-58" y="-9" width="104" height="18" rx="3" fill="'+SEAL[O.seal].c+'"/><rect x="-58" y="-9" width="104" height="6" rx="3" fill="#fff" opacity=".18"/><path d="M-58,-9 q-8,9 0,18" fill="'+SEAL[O.seal].c+'" filter="url(#b2)"/></g>';
    /* The book's shadow on the desk, thrown away from the candle */
    s+='<rect x="96" y="30" width="992" height="664" rx="16" fill="#000" opacity=".62" filter="url(#b14)" transform="translate(-16,18)"/>';
    /* The binding */
    s+='<rect x="104" y="22" width="992" height="662" rx="12" fill="'+bd.c+'"/><rect x="104" y="22" width="992" height="662" rx="12" fill="'+bd.c+'" filter="url(#lth)"/>'+
      '<rect x="110" y="28" width="980" height="650" rx="9" fill="none" stroke="#000" stroke-opacity=".25" stroke-width="2"/>'+
      '<path d="M598,22 V684" stroke="#000" stroke-opacity=".35" stroke-width="6" filter="url(#b2)"/>';
    s+='<rect x="590" y="23" width="20" height="9" rx="2" fill="#d9c49a"/><path d="M592,27.5 h16" stroke="'+bd.r+'" stroke-width="2.4" stroke-dasharray="2 2"/>'+
      '<rect x="590" y="675" width="20" height="9" rx="2" fill="#d9c49a"/><path d="M592,679.5 h16" stroke="'+bd.r+'" stroke-width="2.4" stroke-dasharray="2 2"/>';
    /* The page block: the edges of the leaves under the open spread */
    for(var j=6;j>=1;j--){var o=j*1.6;
      s+='<path d="'+PAGE_L+'" fill="'+st.under+'" transform="translate('+(-o)+','+(o*.7)+')"/><path d="'+PAGE_L+'" fill="none" stroke="#000" stroke-opacity=".09" transform="translate('+(-o)+','+(o*.7)+')"/>';
      s+='<path d="'+PAGE_R+'" fill="'+st.under+'" transform="translate('+o+','+(o*.7)+')"/><path d="'+PAGE_R+'" fill="none" stroke="#000" stroke-opacity=".09" transform="translate('+o+','+(o*.7)+')"/>';}
    /* The paper itself */
    s+='<g filter="url(#deckle)"><path d="'+PAGE_L+' '+PAGE_R+'" fill="'+st.pp+'"/></g>';
    s+='<g clip-path="url(#pages)">'+
      '<rect width="1200" height="700" fill="#000" filter="url(#mottle)"/>'+
      (st.laid?'<rect width="1200" height="700" fill="url(#laid)"/>':'')+
      (st.grid?'<rect x="150" y="66" width="420" height="580" fill="url(#grid)"/><rect x="630" y="66" width="420" height="580" fill="url(#grid)"/>':'')+
      '<rect width="1200" height="700" fill="#000" filter="url(#fibre)"/><rect width="1200" height="700" fill="#000" filter="url(#tooth)"/>'+
      '<rect x="100" y="0" width="1000" height="700" fill="url(#toneE)"/>';
    if(st.mottle){for(var v=0;v<5;v++){var vx=180+R()*820,vy=100+R()*500;s+='<path d="M'+f1(vx)+','+f1(vy)+' c'+f1(30+R()*40)+','+f1(-10+R()*20)+' '+f1(60+R()*50)+','+f1(R()*40)+' '+f1(110+R()*60)+','+f1(-20+R()*40)+'" stroke="#a87c4a" stroke-opacity=".12" stroke-width="'+f1(1+R()*2)+'" fill="none" filter="url(#b2)"/>';}}
    if(age!=='clean'){
      var n=age==='worn'?26:60;for(var fx=0;fx<n;fx++){var x=140+R()*920,y=60+R()*590,r=.8+R()*(age==='worn'?2.4:3.6);s+='<circle cx="'+f1(x)+'" cy="'+f1(y)+'" r="'+f1(r)+'" fill="#8a5420" fill-opacity="'+f1(.08+R()*.14)+'"/>';}
      s+='<g filter="url(#wash)"><circle cx="1004" cy="108" r="46" fill="none" stroke="#7a4a18" stroke-opacity="'+(dark?.25:.2)+'" stroke-width="5"/><circle cx="1004" cy="108" r="42" fill="#7a4a18" fill-opacity=".05"/></g>';
      s+='<path d="M180,640 q30,-14 70,-4 q34,10 60,-6" stroke="#7a4a18" stroke-opacity=".08" stroke-width="16" fill="none" filter="url(#b6)"/>';}
    if(age==='battered'){
      s+='<g filter="url(#wash)"><path d="M122,40 L230,36 Q170,58 132,52 Q118,90 122,170 Z M1078,40 L960,36 Q1030,56 1068,50 Q1082,90 1078,190 Z M122,671 L260,676 Q170,654 130,660 Q116,620 120,560 Z" fill="#2a1406" fill-opacity=".55"/></g>'+
        '<g filter="url(#wash)"><path d="M760,300 c40,-30 120,-10 130,40 c8,50 -60,80 -110,60 c-40,-16 -50,-70 -20,-100 Z" fill="#6a3a12" fill-opacity=".09"/></g>'+
        '<path d="M330,80 l18,40 l-8,30 l22,36" stroke="#5a3a1a" stroke-opacity=".25" fill="none"/>';}
    /* The curve of the pages: dark into the spine, a lit hump beside it, a little fall-off at the fore-edge */
    s+='<rect x="440" y="0" width="160" height="700" fill="url(#gutL)"/><rect x="600" y="0" width="160" height="700" fill="url(#gutR)"/>'+
      '<rect x="470" y="0" width="70" height="700" fill="url(#hump)"/><rect x="660" y="0" width="70" height="700" fill="url(#hump)"/>'+
      '<rect x="100" y="0" width="40" height="700" fill="url(#gutR)" opacity=".35"/><rect x="1060" y="0" width="40" height="700" fill="url(#gutL)" opacity=".35"/>';
    s+='</g>';
    s+='<path d="'+PAGE_L+' '+PAGE_R+'" fill="none" stroke="'+st.edge+'" stroke-opacity=".7" filter="url(#deckle)"/>';
    /* A dog-eared corner on a page that has been used */
    if(age!=='clean')s+='<path d="M1078,630 L1078,671 L1040,672 Z" fill="'+st.under+'"/><path d="M1078,630 L1040,672 L1046,640 Z" fill="'+st.cut+'"/><path d="M1078,630 L1040,672 L1046,640 Z" fill="#000" opacity=".06"/><path d="M1078,630 L1040,672" stroke="#000" stroke-opacity=".2"/>';
    /* The ribbon, laid across the page and off the bottom edge onto the desk */
    var RB='M599,40 C598,300 599,560 592,640 C584,690 566,702 552,726';
    s+='<path d="'+RB+'" stroke="#000" stroke-opacity=".4" stroke-width="11" fill="none" filter="url(#b6)" transform="translate(-6,7)"/>'+
      '<path d="'+RB+'" stroke="'+bd.r+'" stroke-width="9" fill="none"/><path d="'+RB+'" stroke="url(#rib)" stroke-width="9" fill="none" opacity=".7"/>'+
      '<path d="M546,722 l6,-14 l6,14" fill="#000" opacity=".25"/>';
    return s+'</svg>';
  }
  /* Live layer: ink, washes and the things that move. Colours come from the
     scene's CSS variables, which pick() sets from the tables above. */
  function defs(){return ''+
    '<filter id="p-rough" x="-5%" y="-5%" width="110%" height="110%"><feTurbulence type="fractalNoise" baseFrequency=".04" numOctaves="2" seed="2"/><feDisplacementMap in="SourceGraphic" scale="2.4"/></filter>'+
    /* Watercolour: a wobbly edge, then pigment pooled darker along it */
    '<filter id="p-wc" x="-15%" y="-15%" width="130%" height="130%"><feTurbulence type="fractalNoise" baseFrequency=".035" numOctaves="3" seed="9" result="n"/><feDisplacementMap in="SourceGraphic" in2="n" scale="9" result="d"/>'+
      '<feMorphology in="d" operator="erode" radius="2.2" result="e"/><feComposite in="d" in2="e" operator="out" result="edge"/><feGaussianBlur in="edge" stdDeviation="1.1" result="eb"/>'+
      '<feTurbulence type="fractalNoise" baseFrequency=".6" numOctaves="2" seed="12" result="g"/><feColorMatrix in="g" values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 -.9 1.15" result="ga"/><feComposite in="d" in2="ga" operator="in" result="dg"/>'+
      '<feMerge><feMergeNode in="dg"/><feMergeNode in="eb"/></feMerge></filter>'+
    '<filter id="p-wash" x="-15%" y="-15%" width="130%" height="130%"><feTurbulence type="fractalNoise" baseFrequency=".045" numOctaves="3" seed="9" result="n"/><feDisplacementMap in="SourceGraphic" in2="n" scale="7" result="d"/><feGaussianBlur in="d" stdDeviation=".8"/></filter>'+
    '<filter id="p-soft" x="-30%" y="-30%" width="160%" height="160%"><feGaussianBlur stdDeviation="5"/></filter>'+
    '<filter id="p-soft2" x="-30%" y="-30%" width="160%" height="160%"><feGaussianBlur stdDeviation="2.2"/></filter>'+
    '<linearGradient id="p-inkfade" x1="0" x2="1"><stop offset="0" style="stop-color:var(--ink)"/><stop offset=".7" style="stop-color:var(--ink)" stop-opacity=".86"/><stop offset="1" style="stop-color:var(--ink)" stop-opacity=".7"/></linearGradient>'+
    '<radialGradient id="p-waxg" cx=".62" cy=".32" r=".8"><stop offset="0" stop-color="#fff" stop-opacity=".38"/><stop offset=".35" stop-color="#fff" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".35"/></radialGradient>'+
    '<radialGradient id="p-brass2" cx=".35" cy=".3" r=".8"><stop offset="0" stop-color="#f3d58a"/><stop offset=".45" stop-color="#b98a34"/><stop offset="1" stop-color="#5a3c12"/></radialGradient>'+
    '<linearGradient id="p-vane" x1="0" x2="1"><stop offset="0" stop-color="#d9d2c4"/><stop offset=".5" stop-color="#fbf8f0"/><stop offset="1" stop-color="#cfc6b4"/></linearGradient>'+
    '<pattern id="p-hatch" width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(40)"><line x1="0" y1="0" x2="0" y2="6" style="stroke:var(--ink)" stroke-width="1.1" stroke-opacity=".5"/></pattern>'+
    '<pattern id="p-xhatch" width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(40)"><path d="M0,0 V6 M0,3 H6" style="stroke:var(--ink)" stroke-width="1" stroke-opacity=".45"/></pattern>'+
    '<clipPath id="p-pages"><path d="'+PAGE_L+' '+PAGE_R+'"/></clipPath>';
  }
  function background(){
    // The picture goes in once it has loaded, so an empty group means "still painting"
    return '<g class="pj-bakeg" data-bake="page"></g>'+
      /* Hit areas for the props painted into the picture */
      '<g fill="transparent"><circle data-prop="candle" cx="'+L.candle.x+'" cy="'+L.candle.y+'" r="56"/><rect data-prop="inkwell" x="1118" y="288" width="66" height="60"/>'+
      '<rect data-prop="herbs" x="0" y="0" width="80" height="200"/><rect data-prop="waxstick" x="1150" y="570" width="34" height="110"/><rect data-prop="ribbon" x="540" y="640" width="40" height="60"/></g>';
  }
  function tape(x,y,rot,w){return '<g transform="rotate('+rot+' '+x+' '+y+')"><rect x="'+(x-(w||46)/2)+'" y="'+(y-9)+'" width="'+(w||46)+'" height="18" fill="#f1e6c2" fill-opacity=".62" filter="url(#p-rough)"/><rect x="'+(x-(w||46)/2)+'" y="'+(y-9)+'" width="'+(w||46)+'" height="4" fill="#fff" fill-opacity=".18"/></g>';}
  function wash(d,col,op,extra){return '<path d="'+d+'" fill="'+col+'" fill-opacity="'+(op||.4)+'" class="p-washed" '+(extra||'')+'/>';}

  var art={
    burner:function(L){var f=L.flask,b=L.burner,ry=f.y+f.r*.78,x=f.x;
      return '<g data-prop="burner"><g filter="url(#p-rough)"><path d="M'+(x-52)+','+ry+' L'+(x-76)+',650 M'+(x+52)+','+ry+' L'+(x+76)+',650 M'+x+','+(ry+12)+' L'+x+',656" class="p-ink"/>'+
        '<ellipse cx="'+x+'" cy="'+ry+'" rx="56" ry="11" class="p-ink"/><path d="M'+(x-50)+','+(ry+4)+' L'+(x-72)+',646" class="p-ink p-thin" transform="translate(5,0)"/>'+
        '<ellipse cx="'+x+'" cy="640" rx="40" ry="9" class="p-ink"/><path d="M'+(x-40)+',640 Q'+(x-44)+',606 '+(x-18)+',598 L'+(x+18)+',598 Q'+(x+44)+',606 '+(x+40)+',640" class="p-ink" fill="url(#p-hatch)" fill-opacity=".0"/>'+
        '<path d="M'+(x+14)+',604 L'+(x+34)+',636 M'+(x+22)+',601 L'+(x+38)+',627 M'+(x+6)+',606 L'+(x+26)+',638 M'+(x-2)+',610 L'+(x+16)+',639" class="p-ink p-thin"/>'+
        '<rect x="'+(x-12)+'" y="586" width="24" height="12" class="p-ink p-fillpaper"/><rect x="'+(x-5)+'" y="'+(b.y+4)+'" width="10" height="'+(586-b.y-4)+'" class="p-ink p-fillpaper"/></g>'+
        wash('M'+(x-40)+',640 Q'+(x-44)+',606 '+(x-18)+',598 L'+(x+18)+',598 Q'+(x+44)+',606 '+(x+40)+',640 Z','#b0843a',.32,'filter="url(#p-wc)"')+'</g>'+
        '<g class="cr-flame">'+wash('M'+x+','+(b.y-54)+' C'+(x+18)+','+(b.y-28)+' '+(x+14)+','+(b.y-6)+' '+x+','+b.y+' C'+(x-14)+','+(b.y-6)+' '+(x-18)+','+(b.y-28)+' '+x+','+(b.y-54)+' Z','#3f74d8',.5,'filter="url(#p-wc)"')+
        wash('M'+x+','+(b.y-30)+' C'+(x+8)+','+(b.y-18)+' '+(x+7)+','+(b.y-6)+' '+x+','+(b.y-2)+' C'+(x-7)+','+(b.y-6)+' '+(x-8)+','+(b.y-18)+' '+x+','+(b.y-30)+' Z','#f0b13a',.55)+
        '<path d="M'+x+','+(b.y-50)+' C'+(x+14)+','+(b.y-28)+' '+(x+12)+','+(b.y-8)+' '+x+','+(b.y-2)+' C'+(x-12)+','+(b.y-8)+' '+(x-14)+','+(b.y-28)+' '+x+','+(b.y-50)+' Z" class="p-ink p-thin p-flick"/></g>'+
        '<g class="cr-bellows" data-prop="bellows" transform="translate(1004,610) scale(.9)"><g filter="url(#p-rough)"><path d="M-62,6 L-20,2 L-20,10 Z" class="p-ink p-fillpaper"/>'+
        '<path d="M-22,4 L40,-6 Q62,-4 62,8 Q62,18 40,20 L-22,10 Z" class="p-ink p-fillpaper"/><path class="cr-bel-mid" d="M-20,6 L44,0 L44,16 L-20,10 Z" fill="url(#p-hatch)"/>'+
        '<path class="cr-bel-top p-ink p-fillpaper" d="M-22,4 L40,-10 Q64,-10 64,0 Q62,6 40,4 Z"/><path d="M64,-2 L84,-8" class="p-ink" style="stroke-width:4"/></g>'+wash('M-22,4 L40,-6 Q62,-4 62,8 Q62,18 40,20 L-22,10 Z','#8a5a2a',.3)+'</g>';},
    flask:function(L){var f=L.flask,nw=f.neckW,top=f.neck;
      return '<clipPath id="cr-fclip"><circle cx="'+f.x+'" cy="'+f.y+'" r="'+(f.r-3)+'"/><rect x="'+(f.x-nw+3)+'" y="'+(top+4)+'" width="'+(nw*2-6)+'" height="'+(f.y-top)+'"/></clipPath>'+
        '<g class="cr-liqslot p-liq"></g>'+
        '<g filter="url(#p-rough)"><path d="M'+(f.x-nw)+','+top+' L'+(f.x-nw)+','+(f.y-f.r*.82)+' A'+f.r+','+f.r+' 0 1 0 '+(f.x+nw)+','+(f.y-f.r*.82)+' L'+(f.x+nw)+','+top+'" class="p-ink" style="stroke-width:2.2"/>'+
        '<ellipse cx="'+f.x+'" cy="'+top+'" rx="'+(nw+4)+'" ry="5" class="p-ink"/>'+
        '<path d="M'+(f.x-f.r*.7)+','+(f.y-f.r*.22)+' A'+(f.r*.76)+','+(f.r*.76)+' 0 0 1 '+(f.x-f.r*.25)+','+(f.y-f.r*.68)+'" class="p-ink p-thin"/>'+
        '<path d="M'+(f.x+f.r*.42)+','+(f.y+f.r*.7)+' l12,-10 M'+(f.x+f.r*.56)+','+(f.y+f.r*.58)+' l12,-10 M'+(f.x+f.r*.68)+','+(f.y+f.r*.44)+' l11,-9 M'+(f.x+f.r*.78)+','+(f.y+f.r*.28)+' l9,-7" class="p-ink p-thin"/>'+
        '<path d="M'+(f.x+nw-4)+','+(top+10)+' L'+(f.x+nw-4)+','+(f.y-f.r*.86)+'" class="p-ink p-thin"/></g>';},
    mortar:function(r){return '<g filter="url(#p-rough)">'+wash('M'+(-r)+',0 C'+(-r)+','+(r*.9)+' '+(-r*.45)+','+(r*1.05)+' 0,'+(r*1.05)+' C'+(r*.45)+','+(r*1.05)+' '+r+','+(r*.9)+' '+r+',0 Z','#7d838e',.34,'filter="url(#p-wc)"')+
        '<path d="M'+(-r)+',0 C'+(-r)+','+(r*.9)+' '+(-r*.45)+','+(r*1.05)+' 0,'+(r*1.05)+' C'+(r*.45)+','+(r*1.05)+' '+r+','+(r*.9)+' '+r+',0" class="p-ink"/>'+
        '<path d="M'+(r*.3)+','+(r*.4)+' L'+(r*.5)+','+(r*.85)+' M'+(r*.5)+','+(r*.3)+' L'+(r*.7)+','+(r*.7)+' M'+(r*.7)+','+(r*.2)+' L'+(r*.85)+','+(r*.5)+' M'+(r*.15)+','+(r*.55)+' L'+(r*.3)+','+(r*.95)+'" class="p-ink p-thin"/>'+
        '<ellipse cx="0" cy="'+(r*1.04)+'" rx="'+(r*.5)+'" ry="'+(r*.11)+'" class="p-ink"/><ellipse cx="0" cy="0" rx="'+r+'" ry="'+(r*.28)+'" class="p-ink p-fillpaper"/>'+
        wash('M'+(-r*.84)+',2 a'+(r*.84)+','+(r*.21)+' 0 1 0 '+(r*1.68)+',0 a'+(r*.84)+','+(r*.21)+' 0 1 0 '+(-r*1.68)+',0','#3a3428',.38)+'</g>'+
        '<ellipse class="cr-paste p-washed" cx="0" cy="6" rx="'+(r*.64)+'" ry="'+(r*.13)+'" style="opacity:0"/>';},
    pestle:function(){return '<g filter="url(#p-rough)"><rect x="-7" y="-102" width="14" height="98" rx="7" class="p-ink p-fillpaper"/><path d="M2,-94 L2,-14" class="p-ink p-thin"/><ellipse cx="0" cy="0" rx="11" ry="13" class="p-ink p-fillpaper"/><path d="M3,-6 a7,9 0 0 1 4,8" class="p-ink p-thin"/></g>';},
    /* Specimens: a card taped to the page; picking one up lifts it off the paper */
    item:function(k,it){it=it||{};ITEMC[k]=it.color||'#8a6a40';var lab=esc(String(it.name||k).toLowerCase());
      var c='<g class="p-lift"><rect x="-64" y="-60" width="128" height="132" class="p-card"/>'+tape(-40,-58,-24,40)+tape(42,-56,20,40)+'</g>';
      if(k==='bloodmoss')return c+'<g filter="url(#p-rough)">'+wash('M-30,10 C-34,-8 -14,-22 0,-20 C18,-24 34,-6 30,12 C26,26 -26,28 -30,10 Z','#b3263a',.52,'filter="url(#p-wc)"')+
        '<circle cx="-16" cy="6" r="13" class="p-ink"/><circle cx="2" cy="-6" r="15" class="p-ink"/><circle cx="16" cy="8" r="12" class="p-ink"/><circle cx="-2" cy="14" r="11" class="p-ink"/>'+
        '<path d="M-4,-20 q-3,-12 -10,-16 M4,-20 q2,-10 9,-14 M0,-21 v-13 M-20,12 l6,6 M10,16 l6,4" class="p-ink p-thin"/><path d="M-26,12 Q0,22 26,12" class="p-ink"/></g><text class="p-hand p-itemlab" x="0" y="56" text-anchor="middle">'+lab+'</text>';
      if(k==='water')return c+'<g filter="url(#p-rough)">'+wash('M-22,4 L22,4 L22,30 Q22,36 16,36 L-16,36 Q-22,36 -22,30 Z','#6aa6d8',.45,'filter="url(#p-wc)"')+
        '<path d="M-24,32 Q-24,38 -18,38 L18,38 Q24,38 24,32 L24,-8 Q24,-20 10,-24 L10,-40 L-10,-40 L-10,-24 Q-24,-20 -24,-8 Z" class="p-ink p-fillnone"/>'+
        '<rect x="-9" y="-50" width="18" height="11" class="p-ink" fill="url(#p-hatch)"/><path d="M-17,-6 L-17,30" class="p-ink p-thin"/></g><text class="p-hand p-itemlab" x="0" y="58" text-anchor="middle">'+lab+'</text>';
      if(k==='saltglass')return c+'<g filter="url(#p-rough)">'+wash('M-30,22 L-28,-12 L4,-40 L30,-12 L30,22 Z','#5fc9c4',.38,'filter="url(#p-wc)"')+
        '<path d="M-26,22 L-30,-6 L-20,-16 L-14,22 Z M-10,24 L-8,-30 L4,-40 L12,-28 L10,24 Z M12,24 L18,-6 L28,-12 L30,24 Z M4,-40 L2,24" class="p-ink"/></g><text class="p-hand p-itemlab" x="0" y="56" text-anchor="middle">'+lab+'</text>';
      if(k==='bile')return c+'<g filter="url(#p-rough)">'+wash('M-12,-26 L12,-26 L12,20 Q12,28 0,28 Q-12,28 -12,20 Z','#4f9a2a',.52,'filter="url(#p-wc)"')+
        '<rect x="-12" y="-28" width="24" height="56" rx="9" class="p-ink"/><rect x="-10" y="-40" width="20" height="12" class="p-ink" fill="#8a1a1a" fill-opacity=".55"/></g><text class="p-hand p-itemlab" x="0" y="56" text-anchor="middle">'+lab+'</text>';
      /* Any other ingredient: a stoppered phial washed in its own colour */
      var CP=window.CraftPictures;
      if(CP&&it.picture&&CP.has(it.picture))return c+'<g filter="url(#p-rough)" class="cpx-touch"><svg class="'+CP.cls(it.picture)+'" x="-36" y="-50" width="72" height="72" viewBox="0 0 40 40" overflow="visible">'+CP.inner(it.picture,it.color)+'</svg></g><text class="p-hand p-itemlab" x="0" y="58" text-anchor="middle">'+lab+'</text>';
      return c+'<g filter="url(#p-rough)">'+wash('M-14,-8 L14,-8 L16,26 Q16,34 0,34 Q-16,34 -16,26 Z',ITEMC[k],.5,'filter="url(#p-wc)"')+
        '<path d="M-10,-30 L-10,-10 Q-20,-4 -18,26 Q-17,36 0,36 Q17,36 18,26 Q20,-4 10,-10 L10,-30" class="p-ink p-fillnone"/><rect x="-12" y="-42" width="24" height="12" rx="2" class="p-ink" fill="url(#p-hatch)"/>'+
        '<path d="M-12,4 L-12,26" class="p-ink p-thin"/></g><text class="p-hand p-itemlab" x="0" y="58" text-anchor="middle">'+lab+'</text>';},
    bottle:function(col,name){return '<circle cx="0" cy="-34" r="46" fill="'+col+'" fill-opacity=".22" filter="url(#p-wc)" class="p-washed"/>'+
      '<g filter="url(#p-rough)"><circle cx="0" cy="-30" r="26" fill="'+col+'" fill-opacity=".55" class="p-washed"/><circle cx="0" cy="-30" r="27" class="p-ink" style="stroke-width:2"/>'+
      '<path d="M-8,-56 L-8,-70 L8,-70 L8,-56" class="p-ink"/><rect x="-9" y="-80" width="18" height="11" class="p-ink" fill="url(#p-hatch)"/><path d="M-16,-38 A18,18 0 0 1 -5,-48" class="p-ink p-thin"/><path d="M10,-14 l8,-6 M14,-8 l7,-5" class="p-ink p-thin"/></g>'+
      '<text class="p-hand p-itemlab" x="0" y="14" text-anchor="middle">'+esc(String(name).toLowerCase())+'</text>';},
    gaugeFrame:function(){return '<g transform="rotate(-1.5)"><rect x="-96" y="-88" width="192" height="122" class="p-slip"/>'+tape(-70,-86,-18,38)+tape(72,-84,16,38)+'</g>';},
    barFrame:function(w){return '<g transform="rotate(.8 '+(w/2)+' 0)"><rect x="-22" y="-34" width="'+(w+44)+'" height="86" class="p-slip"/>'+tape(6,-32,-14,40)+tape(w-6,-32,12,40)+'</g>';}
  };

  /* The quill and the signet are real objects on the desk: drawn solid and
     shaded, with a shadow that grows as they lift off the paper. */
  function quillArt(){return '<g class="pj-qshadow" filter="url(#p-soft)" opacity=".42"><path d="M0,0 L10,-16 C40,-70 110,-150 196,-196 C160,-140 90,-70 14,-10 Z" fill="#000"/></g>'+
    '<g class="pj-qbody"><path d="M0,0 L10,-16 L16,-12 Z" fill="#1a1410"/><path d="M8,-13 L14,-22" stroke="#3a2a1a" stroke-width="3"/>'+
    '<path d="M12,-18 C40,-70 110,-150 196,-196 C160,-140 90,-70 16,-12 Z" fill="url(#p-vane)"/>'+
    '<path d="M14,-16 C60,-80 130,-150 196,-196" stroke="#a8a090" stroke-width="1.3" fill="none"/>'+
    '<path d="M40,-48 l-10,-14 M60,-72 l-12,-16 M84,-98 l-12,-16 M110,-124 l-12,-14 M136,-148 l-10,-12 M48,-58 l10,12 M74,-86 l12,12 M100,-112 l12,10 M128,-138 l10,8" stroke="#c9c0ae" stroke-width="1" fill="none"/>'+
    '<path d="M150,-160 l-6,-16 M170,-176 l-4,-14" stroke="#e8e1d2" stroke-width="2" fill="none"/></g>';}
  function signetArt(){return '<g class="pj-sshadow" filter="url(#p-soft)" opacity=".5"><circle r="30" fill="#000"/><rect x="-12" y="-100" width="24" height="90" rx="10" fill="#000"/></g>'+
    '<g class="pj-sbody"><rect x="-11" y="-96" width="22" height="86" rx="10" fill="#3a2414"/><rect x="-11" y="-96" width="22" height="86" rx="10" fill="url(#p-waxg)"/><circle cx="0" cy="-98" r="14" fill="#4a2e18"/><circle cx="0" cy="-98" r="14" fill="url(#p-waxg)"/>'+
    '<circle r="28" fill="url(#p-brass2)"/><circle r="21" fill="none" stroke="#6a4818" stroke-width="2"/><g class="pj-semb" transform="scale(.9)" opacity=".7"></g></g>';}
  function emblemSVG(k,cls){var e=EMBLEM[k]||EMBLEM.moth;return '<path class="'+cls+'" d="'+e.d+'"/>';}
  function candleFlame(){var C=L.candle;return '<g class="pj-candle" transform="translate('+C.x+','+C.y+')"><g class="pj-flame"><ellipse rx="9" ry="9" fill="#fff4d0" opacity=".85" filter="url(#p-soft2)"/><circle r="4" fill="#fffbe8"/><circle r="2" fill="#2a1a10" opacity=".7"/></g></g>';}
  /* Writing. Anything new on the page is written by the quill: it flies over,
     the line appears behind the nib, and the quill goes back to the inkwell.
     What was already on the page when it opened is simply there. */
  var W={done:{},prog:{},len:{},queue:[],active:null,silent:true,wet:{},idle:0,blots:{},sealed:{},sealState:null};
  var Q={x:L.quill.x,y:L.quill.y,h:0,rot:0,home:true};
  function wr(key,x,y,inner,w){
    if(W.silent||W.done[key]){W.done[key]=1;return inner;}
    if(!(key in W.prog)){W.prog[key]=0;W.queue.push({key:key,x:x,y:y,w:w||0});}
    var id='wc-'+key.replace(/[^a-z0-9]/gi,'_');
    return '<clipPath id="'+id+'"><rect class="pj-rev" data-k="'+esc(key)+'" x="'+(x-6)+'" y="'+(y-46)+'" height="66" width="'+(W.prog[key]*((W.len[key]||w||200)+14)).toFixed(1)+'"/></clipPath><g clip-path="url(#'+id+')">'+inner+'</g>';
  }
  function wtext(key,x,y,cls,txt,extra){return wr(key,x,y,'<text class="p-hand '+cls+(W.wet[key]>performance.now()?' is-wet':'')+'" data-w="'+esc(key)+'" x="'+x+'" y="'+y+'" '+(extra||'')+'>'+txt+'</text>');}

  /* A moon sketched in the margin: the outline in ink, the lit part left as
     bright paper under the moon's own colour, the dark part shaded and
     hatched, so the phase reads even on a page in shadow. p runs new 0, full .5. */
  function moonSketch(m,cx,cy,r,i){
    var a=2*Math.PI*m.p,k=Math.cos(a),wax=m.p<.5,rx=Math.abs(k)*r,col=m.color==='#e5e7eb'?'#b8c4d8':m.color;
    var lit='M'+cx+','+(cy-r)+' A'+r+','+r+' 0 0 '+(wax?1:0)+' '+cx+','+(cy+r)+' A'+rx.toFixed(2)+','+r+' 0 0 '+((k>0)===wax?0:1)+' '+cx+','+(cy-r)+' Z';
    var dark='M'+cx+','+(cy-r)+' A'+r+','+r+' 0 0 '+(wax?0:1)+' '+cx+','+(cy+r)+' A'+rx.toFixed(2)+','+r+' 0 0 '+((k>0)===wax?0:1)+' '+cx+','+(cy-r)+' Z';
    return '<g data-prop="moon-'+i+'"><path d="'+dark+'" style="fill:var(--ink)" fill-opacity=".5"/><path d="'+dark+'" fill="url(#p-hatch)"/>'+
      '<path d="'+lit+'" fill="#fbf4de" fill-opacity=".92"/>'+wash(lit,col,.5,'filter="url(#p-wc)"')+
      '<circle cx="'+cx+'" cy="'+cy+'" r="'+r+'" class="p-ink p-thin" filter="url(#p-rough)"/>'+
      '<circle cx="'+cx+'" cy="'+cy+'" r="'+(r+8)+'" fill="transparent"/></g>';}
  // One moon is named under its sketch; several share a caption, and each still has its own card.
  function moonRow(moons){var n=moons.length,s='',x0=542-(n-1)*40;
    moons.forEach(function(m,i){s+=moonSketch(m,x0+i*40,72,15,i);});
    return s+'<text class="p-small" x="'+(x0+(n-1)*20)+'" y="106" text-anchor="middle">'+esc(n===1?moons[0].name:'the moons tonight')+'</text>';}

  function book(c){var r=c.r,S=c.S,s='<g class="p-book">';
    /* Left page: tonight, from the calendar, then the recipe */
    if(SKY.ok){s+='<g data-prop="tonight"><text class="p-hand p-date" x="154" y="82">'+esc(SKY.line)+'</text></g>';
      if(SKY.moons.length)s+=moonRow(SKY.moons);}
    s+='<text class="p-hand p-title" x="154" y="134">'+esc(r.name)+'</text><text class="p-small" x="156" y="160">'+esc(r.kind)+' · for '+esc(r.who)+'</text>'+
      '<path d="M154,170 Q260,165 384,171" class="p-ink p-thin"/>';
    var gi=0;
    c.steps.forEach(function(st,i){var y=206+i*42,done=i<c.i||c.phase!=='step',g=null;if(st.graded){g=done?c.res[gi]:null;gi++;}
      s+='<rect x="158" y="'+(y-16)+'" width="17" height="17" class="p-ink p-box" transform="rotate('+((i%2)?1.5:-1.2)+' 166 '+(y-7)+')"/>';
      if(done){var miss=g&&g.grade==='miss';
        s+=wr('tick-'+i,158,y,miss?'<path d="M160,'+(y-15)+' L174,'+y+' M174,'+(y-15)+' L160,'+y+'" class="p-ink p-mark is-miss"/>':'<path d="M160,'+(y-7)+' L166,'+(y-1)+' L180,'+(y-21)+'" class="p-ink p-mark"/>',22);}
      // The slip covers the page from x=400, so the longest step name is said shorter here
      s+='<text class="p-hand p-step'+(i===c.i&&c.phase==='step'?' is-now':'')+(done?' is-done':'')+'" x="188" y="'+y+'" fill="url(#p-inkfade)">'+(st.t==='strike'?'Catch it, '+st.n+' times':esc(c.name(st)))+'</text>';
      if(g)s+=wtext('grade-'+i,190,y+17,'p-grade is-'+g.grade,g.grade==='perfect'?'perfect \u2736':g.grade==='good'?'good':'slipped');
      if(i===c.i&&c.phase==='step')s+='<path d="M136,'+(y-8)+' l14,0 m-5,-5 l5,5 l-5,5" class="p-ink p-arrow"/>';});
    /* A used specimen leaves its tape and a faint stain where its card was */
    c.steps.forEach(function(st,i){var h=SPOT[st.item];if(h&&st.t==='add'&&st.to==='mortar'&&(i<c.i||c.phase!=='step')){
      s+='<g>'+wash('M'+(h[0]-56)+','+(h[1]-50)+' h112 v120 h-112 Z',ITEMC[st.item]||'#8a6a40',.1,'filter="url(#p-wc)"')+tape(h[0]-40,h[1]-58,-24,40)+tape(h[0]+42,h[1]-56,20,40)+
        '<path d="M'+(h[0]-58)+','+(h[1]-52)+' l-4,8 M'+(h[0]+60)+','+(h[1]-48)+' l5,7" class="p-ink p-thin" opacity=".4"/></g>';}});
    /* The project, on a slip tucked into the page */
    var px=412;s+='<g transform="rotate(-1.2 '+px+' 200)"><rect x="'+(px-12)+'" y="184" width="176" height="196" class="p-slip"/>'+tape(px+76,182,-6,52)+
      '<text class="p-small" x="'+px+'" y="214">The project</text>';
    var old=r.start,now=S.points,k=now/r.goal;
    if(now!==old){s+='<text class="p-hand p-num is-struck" x="'+px+'" y="250">'+old+'</text><path d="M'+(px-3)+',240 l34,-6" class="p-ink"/>'+wtext('pts-'+now,px+44,250,'p-num',now+' <tspan class="p-of">of '+r.goal+'</tspan>');}
    else s+='<text class="p-hand p-num" x="'+px+'" y="250">'+now+' <tspan class="p-of">of '+r.goal+'</tspan></text>';
    s+='<rect x="'+px+'" y="262" width="150" height="12" class="p-ink p-thin" filter="url(#p-rough)"/><rect x="'+(px+1)+'" y="263" width="'+(148*k).toFixed(1)+'" height="10" fill="url(#p-xhatch)"/>'+
      '<text class="p-small" x="'+px+'" y="298">Roll 2d10 + '+esc(r.stat)+'</text>';
    var v=c.verdict;s+=wtext('verd-'+v.label,px,328,'p-verd is-'+v.k,esc(v.label));
    if(S.roll)s+=wtext('roll',px,358,'p-rolled',S.roll.a+' + '+S.roll.b+(v.v?(v.v>0?' + 2':' \u2212 2'):'')+' = '+S.roll.total);
    s+='</g>';
    if(S.phase==='done'&&S.points<r.goal)s+=wtext('togo',154,420,'p-notet','+'+S.roll.total+' tonight. '+(r.goal-S.points)+' more at the next respite.');
    if(O.note){var ls=wrapT(O.note,24);s+='<g transform="rotate(-4 420 450)">';ls.forEach(function(l,i){s+=wtext('note-'+i+'-'+l,404,(446+i*24),'p-notet',esc(l));});s+='</g>';}
    /* Right page */
    s+='<text class="p-run" x="640" y="72">the bench</text><text class="p-hand p-cap" x="642" y="100">fig. \u2014 the bench, as I keep it</text>'+
      '<text class="p-run" x="156" y="656">37</text><text class="p-run" x="1044" y="656" text-anchor="end">38</text>';
    /* The seal, once it is pressed */
    if(W.sealed[S.rk])s+='<g transform="translate('+L.seal.x+','+L.seal.y+')">'+waxSeal(40,1)+'</g>';
    return s+'</g>';}
  function wrapT(t,n){var w=String(t).split(/\s+/),lines=[],cur='';w.forEach(function(x){if((cur+' '+x).trim().length>n){lines.push(cur.trim());cur=x;}else cur+=' '+x;});if(cur.trim())lines.push(cur.trim());return lines.slice(0,4);}

  /* Wax: an uneven puddle, a pressed disc inside it, and the emblem sunk into
     the disc. Light comes from the candle, top right, so the impression's
     upper-right walls catch it and its lower-left walls fall into shade. */
  function blobPath(r,seed){var R2=rng(seed),n=16,pts=[];for(var i=0;i<n;i++){var a=i/n*Math.PI*2,rr=r*(.9+R2()*.2);pts.push([Math.cos(a)*rr,Math.sin(a)*rr]);}
    var d='M'+f1((pts[0][0]+pts[n-1][0])/2)+','+f1((pts[0][1]+pts[n-1][1])/2);for(var j=0;j<n;j++){var p=pts[j],q=pts[(j+1)%n];d+=' Q'+f1(p[0])+','+f1(p[1])+' '+f1((p[0]+q[0])/2)+','+f1((p[1]+q[1])/2);}return d+' Z';}
  function waxSeal(r,k){var e=emblemSVG(O.emblem,'pj-emb');
    return '<g class="pj-wax" style="opacity:'+k+'"><path d="'+blobPath(r+6,5)+'" fill="#000" opacity=".3" filter="url(#p-soft2)" transform="translate(-4,5)"/>'+
      '<path d="'+blobPath(r,5)+'" class="pj-waxc"/><path d="'+blobPath(r,5)+'" fill="url(#p-waxg)"/>'+
      '<circle r="'+(r*.72)+'" class="pj-waxc" style="filter:brightness(.9)"/><path d="M'+(-r*.72)+',0 A'+(r*.72)+','+(r*.72)+' 0 0 0 0,'+(r*.72)+'" stroke="#fff" stroke-opacity=".25" stroke-width="2" fill="none"/>'+
      '<path d="M0,'+(-r*.72)+' A'+(r*.72)+','+(r*.72)+' 0 0 1 '+(r*.72)+',0" stroke="#000" stroke-opacity=".35" stroke-width="2" fill="none"/>'+
      '<g transform="translate(-1,1.2)" class="pj-emb-lo">'+e+'</g><g transform="translate(1,-1)" class="pj-emb-hi">'+e+'</g><g class="pj-emb-mid">'+e+'</g>'+
      '<ellipse cx="'+(r*.3)+'" cy="'+(-r*.42)+'" rx="'+(r*.22)+'" ry="'+(r*.1)+'" fill="#fff" opacity=".28" transform="rotate(-30 '+(r*.3)+' '+(-r*.42)+')"/></g>';}

  /* Each frame: run the quill through its queue of lines to write. */
  function writing(dt,api){
    if(!W.active&&W.queue.length){var j=W.queue.shift();var el=api.svg.querySelector('[data-w="'+cssq(j.key)+'"]');
      W.len[j.key]=el&&el.getComputedTextLength?el.getComputedTextLength():(j.w||30);W.active={j:j,t:0,ph:'fly'};
      api.tween(Q,{x:j.x,y:j.y-4,h:24,rot:0},.3,function(){if(W.active)W.active.ph='write';Q.h=2;});Q.home=false;}
    var a=W.active;if(a&&a.ph==='write'){var len=W.len[a.j.key]||30,dur=Math.max(.22,len/300);a.t+=dt;var k=Math.min(1,a.t/dur);
      W.prog[a.j.key]=k;Q.x=a.j.x+k*len;Q.y=a.j.y-5+Math.sin(a.t*34)*3;Q.rot=Math.sin(a.t*17)*2;
      if(Math.random()<dt*12)api.sfx('scratch');
      var rv=api.svg.querySelector('.pj-rev[data-k="'+cssq(a.j.key)+'"]');if(rv)rv.setAttribute('width',(k*(len+14)).toFixed(1));
      if(k>=1){W.done[a.j.key]=1;W.wet[a.j.key]=performance.now()+1600;W.active=null;W.idle=0;
        if(Math.random()<.5)inkDrip(api,Q.x,Q.y+4);}}
    if(!W.active&&!W.queue.length&&!Q.home&&!Q.busy){W.idle+=dt;if(W.idle>.3){Q.home=true;api.tween(Q,{x:L.quill.x,y:L.quill.y,h:0,rot:0},.55,null,'inout');}}
    var sh=Q.home&&Q.h<1?0:Q.h+4;Q.h=Math.max(0,Q.h);
    if(!parts2.qb)return;
    var dx=Q.x-L.candle.x,dy=Q.y-L.candle.y,dl=Math.hypot(dx,dy)||1;
    parts2.qb.setAttribute('transform','translate('+f1(Q.x)+','+f1(Q.y)+') rotate('+f1(Q.rot)+') scale('+(1+Q.h*.004).toFixed(3)+')');
    parts2.qs.setAttribute('transform','translate('+f1(Q.x+dx/dl*sh)+','+f1(Q.y+dy/dl*sh)+') rotate('+f1(Q.rot)+')');
    parts2.qs.setAttribute('opacity',(.42-Math.min(.25,Q.h*.006)).toFixed(2));
  }
  function cssq(s){return String(s).replace(/["\\]/g,'\\$&');}
  var parts2={};
  /* A drop of ink now and then, where the nib lifts off. */
  function inkDrip(api,x,y){if(!stock()||Math.random()<.5)return;var b=api.svg.querySelector('.pj-blots');if(!b)return;
    var g=document.createElementNS('http://www.w3.org/2000/svg','g');g.innerHTML='<circle cx="'+f1(x)+'" cy="'+f1(y)+'" r="'+(1.2+Math.random()*1.4).toFixed(1)+'" class="pj-drop"/>';b.appendChild(g);}
  /* The light map: a small canvas multiplied over the whole scene. Ambient
     night, the candle's warm pool, and the window's light lying across the
     left page in four panes. A second canvas adds the glow you see in the
     air: the flame's bloom and dust turning in the window light. */
  // The glow was a screen blend; drawn plainly it needs less, or it washes the page out.
  var GLOW=.45;
  // The desk's walnut under its grain: the shade over the desk is matched to it, as the page's is to the paper.
  var DESK=[74,48,28];
  var LM={c:null,x:null,g:null,gx:null,turn:null,tx:null,n:0,t:0,motes:[],rain:[],snuff:0,flare:0};
  var QUAD=[[-80,30],[300,-60],[540,300],[160,420]];
  function qpt(u,v){var a=QUAD,x=(1-u)*(1-v)*a[0][0]+u*(1-v)*a[1][0]+u*v*a[2][0]+(1-u)*v*a[3][0],y=(1-u)*(1-v)*a[0][1]+u*(1-v)*a[1][1]+u*v*a[2][1]+(1-u)*v*a[3][1];return [x,y];}
  function pane(ctx,u0,u1,v0,v1,s){var p=[qpt(u0,v0),qpt(u1,v0),qpt(u1,v1),qpt(u0,v1)];ctx.beginPath();ctx.moveTo(p[0][0]*s,p[0][1]*s);for(var i=1;i<4;i++)ctx.lineTo(p[i][0]*s,p[i][1]*s);ctx.closePath();}
  function panes(ctx,s){[[0,.47],[.53,1]].forEach(function(u){[[0,.47],[.53,1]].forEach(function(v){pane(ctx,u[0],u[1],v[0],v[1],s);ctx.fill();});});}
  function winLook(){var w=O.window;
    if(w==='calendar')w=SKY.wx!=='clear'?SKY.wx:SKY.day?'morning':'night';
    if(w==='morning')return {amb:[176,164,148],win:[255,230,186],k:1,day:1};
    if(w==='rain')return {amb:[52,56,74],win:[92,104,132],k:.85,rain:1};
    if(w==='snow')return {amb:[64,68,88],win:[150,160,188],k:.9,snow:1};
    return {amb:[56,60,82],win:[128,142,180],k:1};}
  // The window's light on the desk is soft-edged; a blur is costly, so it is drawn once per colour and reused.
  function paneLight(col,like){var k=col.join(',')+'/'+like.width;if(LM.pk===k)return LM.pc;
    var c=LM.pc||document.createElement('canvas');c.width=like.width;c.height=like.height;var x=c.getContext('2d'),s=c.width/1200;
    x.clearRect(0,0,c.width,c.height);x.filter='blur('+(2.5*s*4).toFixed(1)+'px)';x.fillStyle='rgb('+k.split('/')[0]+')';panes(x,s);x.filter='none';
    LM.pc=c;LM.pk=k;return c;}
  // The light changes slowly enough to redraw at 30 a second, which halves its cost.
  function lightFrame(dt,clock){LM.t+=dt;if(LM.t<1/31)return;dt=LM.t;LM.t=0;var c=LM.x,s=c.canvas.width/1200,Lg=LIGHT[O.light],wl=winLook();
    LM.n+=(Math.random()-.5)*.24;LM.n*=.92;var fl=host.calm?1:1+(LM.n*.5+Math.sin(clock*6.3)*.03+Math.sin(clock*13.1)*.02)*Lg.flick;
    LM.flare=Math.max(0,LM.flare-dt*.8);var on=O.light==='none'?0:1-LM.snuff;fl*=1+LM.flare*.25;
    var cx=L.candle.x,cy=L.candle.y;if(Lg.bob&&!host.calm){cx+=Math.sin(clock*.7)*14;cy+=Math.sin(clock*1.1)*10;}
    c.globalCompositeOperation='source-over';c.filter='none';c.fillStyle='rgb('+wl.amb.join(',')+')';c.fillRect(0,0,c.canvas.width,c.canvas.height);
    c.globalCompositeOperation='lighter';
    if(on>0){var R=Lg.r*(wl.day?.7:1)*(.96+.04*fl),g=c.createRadialGradient(cx*s,cy*s,0,cx*s,cy*s,R*s),col=Lg.c,k=on*(wl.day?.45:1)*fl;
      function st(o,m){g.addColorStop(o,'rgba('+Math.round(col[0]*m*k)+','+Math.round(col[1]*m*k)+','+Math.round(col[2]*m*k)+',1)');}
      st(0,1.25);st(.12,1.05);st(.32,.72);st(.6,.34);st(.85,.1);st(1,0);c.fillStyle=g;c.fillRect(0,0,c.canvas.width,c.canvas.height);}
    c.drawImage(paneLight(wl.win,c.canvas),0,0);
    /* Rain running down the glass throws moving shadows into the window light */
    if(wl.rain){c.save();pane(c,0,1,0,1,s);c.clip();c.globalCompositeOperation='source-over';c.strokeStyle='rgba(24,28,40,.75)';c.lineWidth=2.2*s*2;
      if(LM.rain.length<40)LM.rain.push({u:Math.random(),v:Math.random()*-1,sp:.05+Math.random()*.12,w:Math.random()*6});
      LM.rain.forEach(function(d){d.v+=d.sp*dt;if(d.v>1.1){d.v=-.2;d.u=Math.random();}var p0=qpt(d.u,d.v),p1=qpt(d.u+.004*Math.sin(clock+d.w),d.v+.08);c.beginPath();c.moveTo(p0[0]*s,p0[1]*s);c.lineTo(p1[0]*s,p1[1]*s);c.stroke();
        c.fillStyle='rgba(30,34,48,.5)';c.beginPath();c.arc(p1[0]*s,p1[1]*s,1.6*s*2,0,6.283);c.fill();});c.restore();}
    if(wl.snow){c.save();pane(c,0,1,0,1,s);c.clip();c.globalCompositeOperation='source-over';c.fillStyle='rgba(40,44,60,.35)';
      if(LM.rain.length<40)LM.rain.push({u:Math.random(),v:Math.random(),sp:.02+Math.random()*.03,w:Math.random()*6});
      LM.rain.forEach(function(d){d.v+=d.sp*dt;d.u+=Math.sin(clock*.8+d.w)*.0008;if(d.v>1.05){d.v=-.05;d.u=Math.random();}var p=qpt(d.u,d.v);c.beginPath();c.arc(p[0]*s,p[1]*s,(1.2+d.w*.25)*s*2,0,6.283);c.fill();});c.restore();}
    /* The light map is a multiply, but a blended layer over the whole scene costs a
       full extra pass every frame, a third of the frame rate on a phone. So it is
       turned into a plain see-through shade: exact over the page's own paper and
       the desk's wood, close over everything else. The glow goes on the same layer. */
    if(LM.stock!==O.stock){LM.stock=O.stock;var hx=STOCK[O.stock].pp;LM.paper=[1,3,5].map(function(j){return parseInt(hx.substr(j,2),16);});}
    var id=c.getImageData(0,0,c.canvas.width,c.canvas.height),d=id.data,P=LM.paper;
    var cw=c.canvas.width,ch=c.canvas.height,x0=118*s,x1=1082*s,y0=36*s,y1=676*s,i=0;
    for(var py=0;py<ch;py++)for(var px=0;px<cw;px++,i+=4){var mn=d[i]<d[i+1]?d[i]:d[i+1];if(d[i+2]<mn)mn=d[i+2];var a=255-mn;
      if(a<1){d[i+3]=0;continue;}var Q=px>=x0&&px<x1&&py>=y0&&py<y1?P:DESK;
      d[i]=Q[0]*(d[i]-mn)/a;d[i+1]=Q[1]*(d[i+1]-mn)/a;d[i+2]=Q[2]*(d[i+2]-mn)/a;d[i+3]=a;}
    LM.ox.putImageData(id,0,0);
    var gx=LM.gx,gs=gx.canvas.width/1200;gx.clearRect(0,0,gx.canvas.width,gx.canvas.height);gx.drawImage(LM.o,0,0,gx.canvas.width,gx.canvas.height);
    if(on>0){var gg=gx.createRadialGradient(cx*gs,cy*gs,0,cx*gs,cy*gs,150*gs*fl);gg.addColorStop(0,'rgba('+Lg.glow+','+(.55*on*GLOW)+')');gg.addColorStop(.3,'rgba('+Lg.glow+','+(.16*on*GLOW)+')');gg.addColorStop(1,'rgba('+Lg.glow+',0)');gx.fillStyle=gg;gx.fillRect(0,0,gx.canvas.width,gx.canvas.height);}
    if(!host.calm&&LM.motes.length<34)LM.motes.push({u:Math.random(),v:Math.random(),du:(Math.random()-.5)*.01,dv:(Math.random()-.5)*.01,r:.6+Math.random()*1.3,t:Math.random()*6});
    var mc=wl.day?'255,236,200':'200,214,255';
    LM.motes.forEach(function(m){m.u+=m.du*dt;m.v+=m.dv*dt+.002*dt;m.t+=dt;if(m.u<0||m.u>1||m.v<0||m.v>1){m.u=Math.random();m.v=Math.random();}
      var p=qpt(m.u,m.v),a=(.25+.25*Math.sin(m.t*1.3))*(wl.day?1:.8);gx.fillStyle='rgba('+mc+','+a.toFixed(2)+')';gx.beginPath();gx.arc(p[0]*gs,p[1]*gs,m.r*gs*1.4,0,6.283);gx.fill();});
    /* The flame on the desk follows the same flicker */
    var fe=parts2.flame;if(fe){fe.style.opacity=on;fe.setAttribute('transform','translate('+(cx-L.candle.x).toFixed(1)+','+(cy-L.candle.y).toFixed(1)+') scale('+(.92+fl*.1).toFixed(3)+')');}
  }

  /* Turning to another recipe: a leaf lifts off the right page, stands on the
     spine and lays down on the left. The ink fades as it passes and the new
     recipe is there when it lands. */
  function turn(api,swap){var cv=LM.tx,s=cv.canvas.width/1200,t=0,st=stock(),grp=['.cr-book','.cr-app','.cr-items','.cr-ui','.pj-blots','.pj-seal'].map(function(q){return api.svg.querySelector(q);});
    if(host.calm||!cv){swap();return;}api.sfx('page');var swapped=false,last=performance.now();
    (function fr(now){if(dead)return;var dt=Math.min(.05,(now-last)/1000);last=now;t+=dt/.95;var k=Math.min(1,t),e=k<.5?2*k*k:1-Math.pow(-2*k+2,2)/2,th=e*Math.PI;
      var op=k<.25?1-k/.25:k>.75?(k-.75)/.25:0;grp.forEach(function(g){if(g)g.style.opacity=op.toFixed(2);});
      if(k>=.5&&!swapped){swapped=true;swap();}
      cv.clearRect(0,0,cv.canvas.width,cv.canvas.height);
      if(k<1){var w=478*Math.cos(th),lift=Math.sin(th),x0=600,x1=600+w,top=40-lift*18,bot=670+lift*18,bow=lift*40;
        /* The shadow the leaf throws on the page under it */
        var shw=60*lift+8,sx=x1+(w>=0?1:-1)*shw,sg=cv.createLinearGradient(x1*s,0,sx*s,0);sg.addColorStop(0,'rgba(20,10,4,'+(.35*lift+.05)+')');sg.addColorStop(1,'rgba(20,10,4,0)');
        cv.fillStyle=sg;cv.fillRect(Math.min(x1,sx)*s,40*s,Math.abs(sx-x1)*s,630*s);
        cv.beginPath();cv.moveTo(x0*s,(40)*s);cv.quadraticCurveTo(((x0+x1)/2)*s,(top-bow*.2)*s,x1*s,top*s);cv.quadraticCurveTo((x1+(w>=0?bow:-bow)*.3)*s,355*s,x1*s,bot*s);
        cv.quadraticCurveTo(((x0+x1)/2)*s,(bot+bow*.2)*s,x0*s,670*s);cv.closePath();
        var lg=cv.createLinearGradient(x0*s,0,x1*s,0),dk=w>=0?.18+.3*lift:.12+.25*lift;
        lg.addColorStop(0,'rgba(0,0,0,'+(dk+.15)+')');lg.addColorStop(.35,'rgba(0,0,0,'+(dk*.4)+')');lg.addColorStop(.8,'rgba(255,255,255,'+(.1*lift)+')');lg.addColorStop(1,'rgba(0,0,0,'+(dk*.6)+')');
        cv.fillStyle=st.pp;cv.fill();cv.fillStyle=lg;cv.fill();cv.strokeStyle=st.edge;cv.lineWidth=1;cv.stroke();
        requestAnimationFrame(fr);}
      else grp.forEach(function(g){if(g)g.style.opacity='';});})(last);}

  /* Sealing a finished recipe: drag the signet onto the pencilled ring, the
     wax runs, the signet presses, and the seal is left in the page. */
  var SG={x:L.signet.x,y:L.signet.y,h:0,drag:null,fin:null};
  function placeSignet(){if(!parts2.sb)return;var dx=SG.x-L.candle.x,dy=SG.y-L.candle.y,dl=Math.hypot(dx,dy)||1,sh=SG.h+3;
    parts2.sb.setAttribute('transform','translate('+f1(SG.x)+','+f1(SG.y)+') scale('+(1+SG.h*.006).toFixed(3)+')');
    parts2.ss.setAttribute('transform','translate('+f1(SG.x+dx/dl*sh)+','+f1(SG.y+dy/dl*sh)+')');parts2.ss.setAttribute('opacity',(.5-Math.min(.3,SG.h*.007)).toFixed(2));}
  function sealRing(on){var g=parts2.seal;if(!g)return;g.innerHTML=on?'<g class="pj-ring"><circle cx="'+L.seal.x+'" cy="'+L.seal.y+'" r="44" class="pj-pencil"/><text class="p-small" x="'+L.seal.x+'" y="'+(L.seal.y+64)+'" text-anchor="middle">seal here</text></g>':'';}
  function startSeal(fin){W.sealState='ready';SG.fin=fin;sealRing(true);parts2.sg.classList.add('cr-hot');
    API.say('Seal the recipe: drag the signet onto the ring',[{label:'Seal it for me',fn:function(){if(W.sealState==='ready'||W.sealState==='picked')pressSeal();}}]);}
  function pressSeal(){W.sealState='press';parts2.sg.classList.remove('cr-hot');var t=API.tween,sx=L.seal.x,sy=L.seal.y,g=parts2.seal;
    t(SG,{x:sx+6,y:sy-4,h:34},.35,function(){
      g.insertAdjacentHTML('beforeend',sc('<g class="pj-pour" transform="translate('+sx+','+sy+') scale(.1)">'+waxSeal(40,1)+'</g>'));var pr=g.querySelector('.pj-pour');pr.querySelectorAll('.pj-emb-lo,.pj-emb-hi,.pj-emb-mid').forEach(function(e){e.style.opacity=0;});
      var o={k:.1};API.sfx('wax');t(o,{k:1},.6);(function grow(){if(dead)return;pr.setAttribute('transform','translate('+sx+','+sy+') scale('+o.k.toFixed(3)+')');if(o.k<1)requestAnimationFrame(grow);})();
      for(var i=0;i<10;i++)API.P.emit({type:'blob',x:sx+CraftScene.rnd(-6,6),y:sy-20,vx:CraftScene.rnd(-30,30),vy:CraftScene.rnd(20,80),ay:300,life:.5,size:CraftScene.rnd(1.5,3),color:CraftScene.rgb(SEAL[O.seal].c),alpha:.9});
      API.later(function(){t(SG,{x:sx,y:sy,h:0},.16,function(){API.sfx('seal');API.state().shake=.2;
        for(var j=0;j<14;j++){var a=CraftScene.rnd(0,6.283);API.P.emit({type:'blob',x:sx+Math.cos(a)*40,y:sy+Math.sin(a)*40,vx:Math.cos(a)*40,vy:Math.sin(a)*40,drag:.2,life:.4,size:CraftScene.rnd(1,2.2),color:CraftScene.rgb(SEAL[O.seal].c),alpha:.8});}
        API.later(function(){t(SG,{x:sx+10,y:sy-10,h:30},.3,function(){
          W.sealed[API.state().rk]=1;g.innerHTML='';API.rerender();W.sealState='done';
          t(SG,{x:L.signet.x,y:L.signet.y,h:0},.6,function(){if(SG.fin)SG.fin();},'inout');});},450);});},650);});}
  function down(e,p){if(W.sealState!=='ready'&&W.sealState!=='picked')return false;
    var onS=Math.hypot(p.x-SG.x,p.y-SG.y)<46||(e.target.closest&&e.target.closest('.pj-signet'));
    if(W.sealState==='picked'){if(Math.hypot(p.x-L.seal.x,p.y-L.seal.y)<80){pressSeal();return true;}if(!onS)return false;}
    if(!onS)return false;SG.drag={ox:p.x-SG.x,oy:p.y-SG.y,sx:p.x,sy:p.y,m:0};SG.h=22;try{API.svg.setPointerCapture(e.pointerId);}catch(_){}return true;}
  function move(e,p){if(!SG.drag)return false;SG.drag.m=Math.max(SG.drag.m,Math.hypot(p.x-SG.drag.sx,p.y-SG.drag.sy));SG.x=p.x-SG.drag.ox;SG.y=p.y-SG.drag.oy;return true;}
  function up(e,p){if(!SG.drag)return false;var d=SG.drag;SG.drag=null;
    if(d.m<6){W.sealState='picked';API.say('Now click the ring to press the seal');SG.h=14;return true;}
    if(Math.hypot(SG.x-L.seal.x,SG.y-L.seal.y)<80)pressSeal();else API.tween(SG,{x:L.signet.x,y:L.signet.y,h:0},.4);return true;}

  /* Ink blots: a slip leaves its mark on the page, and the page keeps it. */
  function blotSVG(b){var R2=rng(b.seed),s='<g class="pj-blot" transform="translate('+f1(b.x)+','+f1(b.y)+')"><g class="pj-bi"><path d="'+blobPath(b.r,b.seed)+'" class="pj-blotc" filter="url(#p-wc)"/>';
    for(var i=0;i<9;i++){var a=R2()*6.283,d=b.r*(1.3+R2()*1.4);s+='<circle cx="'+f1(Math.cos(a)*d)+'" cy="'+f1(Math.sin(a)*d)+'" r="'+f1(.8+R2()*2.6)+'" class="pj-blotc"/>';}
    s+='<path d="M'+f1(b.r*.6)+','+f1(b.r*.2)+' q'+f1(b.r*.8)+','+f1(b.r*.3)+' '+f1(b.r*1.4)+','+f1(b.r*1.2)+'" class="pj-blotc" style="fill:none;stroke-width:2.4;stroke:var(--ink)" filter="url(#p-rough)"/>';
    return s+'</g></g>';}
  function drawBlots(rk){var g=API&&API.svg.querySelector('.pj-blots');if(!g)return;var list=W.blots[rk]||[];g.innerHTML=sc(list.map(blotSVG).join(''));}

  var fx={
    dust:function(P,x,y,col){P.emit({type:'blob',x:x,y:y,vx:R(-40,40),vy:R(-50,-10),ay:240,life:R(.4,.8),size:R(1,2.2),color:CraftScene.rgb(col),alpha:.7});},
    drop:function(P,x,y,col,spill){P.emit({type:'blob',x:x,y:y,vx:spill?R(-90,90):R(-10,10),vy:spill?R(-60,20):R(30,80),ay:900,life:spill?.7:.35,size:R(2,3.6),color:CraftScene.rgb(col),alpha:.6});},
    fizz:function(P,x,y,col){for(var i=0;i<34;i++)P.emit({type:'bubble',x:x+R(-30,30),y:y+R(-5,20),vx:R(-20,20),vy:R(-150,-40),life:R(.5,1.1),size:R(2,6),color:ink(),alpha:.7});},
    spark:function(P,x,y,g){var n=g==='perfect'?26:g==='good'?10:0;
      for(var i=0;i<n;i++){var a=R(0,6.283),sp=R(140,360);P.emit({type:'istreak',x:x,y:y,vx:Math.cos(a)*sp,vy:Math.sin(a)*sp,drag:.08,life:R(.3,.6),size:R(1,2),color:ink(),len:.05,alpha:.8});}
      if(g!=='miss')P.emit({type:'ring',x:x,y:y,life:.6,size:16,grow:240,color:g==='perfect'?'176,132,40':ink(),alpha:.7,lw:2.2});
      if(g==='perfect')for(var j=0;j<14;j++){var b=R(0,6.283),sp2=R(40,140);P.emit({type:'blob',x:x,y:y,vx:Math.cos(b)*sp2,vy:Math.sin(b)*sp2-40,ay:120,drag:.04,life:R(.8,1.5),size:R(1.2,2.6),color:'201,160,64',alpha:.9});}},
    smoke:function(P,x,y){},
    steam:function(P,x,y,k){P.emit({type:'soft',x:x+R(-5,5),y:y,vx:R(-12,12),vy:R(-60,-35),life:R(1.6,2.4),size:R(7,12),grow:18,color:stock().dark?'200,190,170':'120,108,96',alpha:.12*(k||1),fade:'inout'});},
    success:function(P,x,y,col){var c=CraftScene.rgb(col);for(var i=0;i<10;i++)P.emit({type:'soft',x:x+R(-40,40),y:y+R(-30,30),life:R(1.6,2.6),size:R(26,50),grow:40,color:c,alpha:.3});
      P.emit({type:'ring',x:x,y:y,life:1,size:12,grow:260,color:'176,132,40',alpha:.8,lw:2});
      for(var j=0;j<30;j++){var a=R(0,6.283),sp=R(60,240);P.emit({type:j%3?'istreak':'blob',x:x,y:y,vx:Math.cos(a)*sp,vy:Math.sin(a)*sp,drag:.05,life:R(.6,1.4),size:j%3?1.4:R(1.5,3),color:j%2?'190,146,52':ink(),len:.05});}},
    bubbles:function(P,F,ly,col,heat,dt){var rate=3+heat*heat*55;if(Math.random()<rate*dt){var bot=F.y+F.r*.85,y=R(Math.max(ly+6,bot-30),bot),hw=Math.sqrt(Math.max(0,F.r*F.r-(y-F.y)*(y-F.y)))*.8,sp=R(40,90)*(.6+heat);
        P.emit({type:'bubble',x:F.x+R(-hw,hw),y:y,vx:R(-6,6),vy:-sp,life:Math.max(.1,(y-ly)/sp),size:R(1.4,3.4)*(.7+heat*.6),color:ink(),alpha:.55,fade:'in'});}
      if(heat>.4&&Math.random()<heat*4*dt)fx.steam(P,F.x,F.neck-4,heat);}
  };
  /* The journal's own layers over the engine's: canvases for the page turn,
     the light map and the glow, and live groups for the blots, the seal, the
     signet, the quill and the candle's flame. */
  function offscreen(){var c=document.createElement('canvas');c.width=200;c.height=117;return c;}
  function canvas(cls,w,h,after){var c=document.createElement('canvas');c.className=cls;c.width=w;c.height=h;c.setAttribute('aria-hidden','true');after.insertAdjacentElement('afterend',c);return c;}
  function split(art,mark){var p=art.split(mark);return [p[0],mark+p[1]];}
  function mounted(api){API=api;var st=api.stage,fxc=st.querySelector('.cr-fx');st.classList.add('crj');
    LM.turn=canvas('pj-turn',900,525,fxc);LM.tx=LM.turn.getContext('2d');
    LM.c=offscreen();LM.x=LM.c.getContext('2d',{willReadFrequently:true});LM.o=offscreen();LM.ox=LM.o.getContext('2d');
    LM.g=canvas('pj-light',600,350,LM.turn);LM.gx=LM.g.getContext('2d');
    var q=split(quillArt(),'<g class="pj-qbody">'),g=split(signetArt(),'<g class="pj-sbody">');
    api.svg.querySelector('.cr-top').insertAdjacentHTML('beforebegin',sc('<g class="pj-blots"></g><g class="pj-seal"></g>'+
      '<g class="pj-signet" data-prop="signet"><g class="pj-ss">'+g[0]+'</g><g class="pj-sb">'+g[1]+'</g></g>'+
      '<g class="pj-quill" data-prop="quill"><g class="pj-qs">'+q[0]+'</g><g class="pj-qb">'+q[1]+'</g></g>'+candleFlame()));
    var $=function(s){return api.svg.querySelector(s);};
    parts2={qb:$('.pj-qb'),qs:$('.pj-qs'),sb:$('.pj-sb'),ss:$('.pj-ss'),sg:$('.pj-signet'),seal:$('.pj-seal'),flame:$('.pj-flame')};
    applyVars();rebake();emblemOnSignet();tonightInfo();loadSky();
  }
  function emblemOnSignet(){var e=API&&API.svg.querySelector('.pj-semb');if(e)e.innerHTML=emblemSVG(O.emblem,'pj-semb-p');}
  /* The still picture is drawn as an SVG document, decoded off to the side,
     and swapped in only once it has loaded, so a change of paper never
     flashes an empty desk. */
  var bakeN=0,bakeUrl=null;
  function rebake(){var n=++bakeN,url=URL.createObjectURL(new Blob([bake()],{type:'image/svg+xml'})),img=new Image();urls.push(url);
    img.onload=function(){if(dead||n!==bakeN){URL.revokeObjectURL(url);return;}var gg=API.svg.querySelector('.pj-bakeg');if(!gg)return;
      var im=gg.querySelector('image');if(!im){im=document.createElementNS('http://www.w3.org/2000/svg','image');['x','y'].forEach(function(a){im.setAttribute(a,'0');});
        im.setAttribute('width','1200');im.setAttribute('height','700');im.setAttribute('preserveAspectRatio','none');gg.appendChild(im);}
      im.setAttribute('href',url);var was=bakeUrl;bakeUrl=url;if(was)API.later(function(){URL.revokeObjectURL(was);},500);API.stage.classList.add('pj-ready');};
    img.src=url;}
  function applyVars(){var r=API.stage,st=stock(),h=HAND[O.hand];
    r.style.setProperty('--pp',st.pp);r.style.setProperty('--pp-cut',st.cut);r.style.setProperty('--pp-edge',st.edge);r.style.setProperty('--ink',inkHex());
    r.style.setProperty('--hand',h.f);r.style.setProperty('--hs',h.k);r.style.setProperty('--seal',SEAL[O.seal].c);
    r.setAttribute('data-dark',st.dark?'1':'0');}
  // Choices that change the still picture; the rest are live colours and fonts
  var BAKED={stock:1,age:1,binding:1,seal:1};
  function refresh(rebaked){if(!API)return;applyVars();if(rebaked)rebake();emblemOnSignet();LM.rain=[];API.rain(!!winLook().rain);
    API.rerender();drawBlots(API.state().rk);try{API.svg.dispatchEvent(new CustomEvent('craft:redrawn'));}catch(e){}}
  function setOpt(k,v){var o={};o[k]=v;setLook(o);if(k==='light')LM.snuff=0;refresh(!!BAKED[k]);}
  function onReset(S){W.done={};W.prog={};W.queue=[];W.active=null;W.silent=true;W.sealState=null;SG.drag=null;SG.x=L.signet.x;SG.y=L.signet.y;SG.h=0;sealRing(false);
    if(parts2.sg)parts2.sg.classList.remove('cr-hot');drawBlots(S.rk);}
  // What is on the page when it opens is simply there; only what follows is written by the quill.
  function bookW(c){var s=book(c);W.silent=host.calm;return s;}
  function onFeedback(g,x,y,S){if(g!=='miss')return;var list=W.blots[S.rk]=W.blots[S.rk]||[];
    list.push({x:x+R(-46,46),y:y+R(20,60),r:R(8,13),seed:Math.floor(R(1,9999))});drawBlots(S.rk);
    var b=API.svg.querySelectorAll('.pj-blots .pj-blot');if(b.length)b[b.length-1].classList.add('is-new');API.sfx('drop');}
  // The engine waits for the seal before it says the work is finished.
  function onDone(S,fin){startSeal(fin);return true;}
  function turnTo(go){turn(API,go);}

  var INFO={
    book:{name:'Journal',text:'Recipes and notes in your own hand. Everything new on the page is written by the quill as it happens.'},
    candle:{name:'Candle',text:'Beeswax in a brass dish. It lights the page.',acts:[['Trim the wick','trim'],['Snuff it','snuff']]},
    inkwell:{name:'Inkwell',text:'Iron gall, mixed in the spring.'},
    quill:{name:'Quill',text:'Goose feather, cut for a right hand. It writes everything new on the page.',acts:[['Doodle in the margin','doodle']]},
    signet:{name:'Signet',text:'Brass, with your mark. A finished recipe gets sealed.'},
    waxstick:{name:'Sealing wax',text:'Melted over the candle for the seal.'},
    herbs:{name:'Drying herbs',text:'Lavender and rosemary, hung to dry, set down for a moment.'},
    ribbon:{name:'Ribbon',text:'Keeps the place.'},
    burner:{name:'Oil burner',text:'Drawn from life, mostly.',acts:[['Turn the flame up','flame']]},
    bellows:{name:'Bellows',text:'Pump them to feed the flame.',acts:[['Give it a pump','pump']]}};
  Object.keys(opts.info||{}).forEach(function(k){INFO[k]=opts.info[k];});
  function tonightInfo(){
    INFO.tonight={name:'Tonight, from the calendar',text:(SKY.ok?SKY.line+'. ':'')+'The light through the window and the moons in the margin follow the campaign’s calendar.'};
    SKY.moons.forEach(function(m,i){INFO['moon-'+i]={name:m.name,text:'Tonight it is '+phaseName(m.p)+(m.note?', '+m.note:'')+'.'};});}
  var DOODLES=['M0,-10 L3,-3 L10,-3 L4,2 L6,10 L0,5 L-6,10 L-4,2 L-10,-3 L-3,-3 Z','M-12,0 C-8,-10 8,-10 12,0 C8,10 -8,10 -12,0 Z M-2,0 a2,2 0 1 0 4,0 a2,2 0 1 0 -4,0','M-10,6 C-10,-8 10,-8 10,6 M-6,6 L-8,12 M6,6 L8,12 M0,-2 L0,4'];
  function act(id,a,api){
    if(a==='flame'){api.heat(.4);api.sfx('pump');}if(a==='pump'){api.pump();api.heat(.3);api.sfx('pump');}
    if(a==='trim'){LM.flare=1;LM.snuff=0;api.sfx('scratch');}
    if(a==='snuff'){var on=LM.snuff<.5;API.tween(LM,{snuff:on?1:0},on?.8:.4);INFO.candle.acts[1]=on?['Light it','snuff']:['Snuff it','snuff'];
      if(on)for(var i=0;i<12;i++)api.P.emit({type:'soft',x:L.candle.x+R(-3,3),y:L.candle.y,vx:R(-14,8),vy:R(-30,-10),life:R(1.6,2.6),size:R(4,8),grow:14,color:'210,205,200',alpha:.35,fade:'inout'});}
    if(a==='doodle'){var x=R(150,380),y=R(560,640),key='doodle-'+Math.floor(R(1,99999));
      W.queue.push({key:key,x:x-12,y:y+8,w:24});W.prog[key]=0;W.len[key]=30;
      API.svg.querySelector('.pj-blots').insertAdjacentHTML('beforeend',sc('<clipPath id="wc-'+key+'"><rect class="pj-rev" data-k="'+key+'" x="'+f1(x-18)+'" y="'+f1(y-38)+'" height="66" width="'+(host.calm?66:0)+'"/></clipPath>'+
        '<g clip-path="url(#wc-'+key+')"><path d="'+DOODLES[Math.floor(R(0,3))]+'" transform="translate('+f1(x)+','+f1(y)+') scale(1.4)" class="p-ink p-thin" filter="url(#p-rough)"/></g>'));}
  }
  /* The engine draws the heat band and the timing zone; on paper they are
     washes. A style, since a stylesheet can't name the scene's own filter id. */
  function washUI(){API.svg.querySelectorAll('.cr-g-band,.cr-b-zone').forEach(function(e){if(!e.style.filter)e.style.filter='url(#'+host.id('p-wash')+')';});}
  function ambient(P,dt,S,clock){if(!API)return;writing(dt,API);lightFrame(dt,clock);placeSignet();washUI();
    var lifted=S.drag&&S.drag.k==='item'?S.drag.it.g:null;API.svg.querySelectorAll('.cr-item').forEach(function(g){g.classList.toggle('is-lifted',g===lifted||(S.sel&&S.sel.g===g));});}

  /* Ready-made looks: each sets every journal choice at once. They leave the
     window alone, so its light keeps following the calendar. */
  var LOOKS={
    herbalist:{name:'Herbalist’s notebook',note:'Parchment, iron gall, a quick hand, scarlet wax',o:{stock:'parchment',ink:'gall',hand:'kalam',seal:'scarlet',emblem:'moth',binding:'oxblood',light:'candle',age:'worn'}},
    grimoire:{name:'Grimoire',note:'Black pages, gold ink, old print, witchlight',o:{stock:'grimoire',ink:'gold',hand:'fell',seal:'black',emblem:'crescent',binding:'black',light:'witch',age:'battered'}},
    field:{name:'Field book',note:'Squared paper, indigo, a loose hand, a lantern',o:{stock:'field',ink:'indigo',hand:'caveat',seal:'forest',emblem:'leaf',binding:'tan',light:'lantern',age:'worn'}},
    court:{name:'Court alchemist',note:'Cotton rag, sepia, a fine hand, midnight wax',o:{stock:'cotton',ink:'sepia',hand:'fine',seal:'midnight',emblem:'key',binding:'midnight',light:'candle',age:'clean'}}};
  function look(id){var Lk=LOOKS[id];if(!Lk)return;setLook(Lk.o);refresh(true);}
  /* What the build-mode editor needs to know about the journal. Everything is
     drawn into the page or painted into the desk, so nothing moves: a
     Director renames pieces and gives them actions. */
  var LOCK_TOOL='Drawn into the page where every step can find it, so it stays put.';
  var LOCK_DESK='Painted into the desk under the book, so it stays put.';
  var PIECES={
    mortar:{kind:'tool',sel:'.cr-mortar, .cr-pestle',lock:LOCK_TOOL,uses:'Add, grind and tip use the mortar.'},
    flask:{kind:'tool',sel:'.cr-flaskg',lock:LOCK_TOOL,fixed:['Swirl it'],uses:'Add, pour and every finale use the flask.'},
    burner:{kind:'tool',sel:'[data-prop="burner"]',lock:LOCK_TOOL,uses:'Keep it steady uses the burner and bellows.'},
    bellows:{kind:'tool',sel:'[data-prop="bellows"]',lock:LOCK_TOOL,uses:'Keep it steady uses the burner and bellows.'},
    book:{kind:'piece',sel:'.cr-book',lock:'The recipe is written on it, so it stays where players can read it.',fixed:['Read the recipe']},
    quill:{kind:'piece',sel:'.pj-quill',lock:'It writes everything new on the page and goes back to the inkwell.'},
    signet:{kind:'piece',sel:'.pj-signet',lock:'It waits beside the page to seal finished recipes.'},
    candle:{kind:'piece',sel:'[data-prop="candle"]',lock:LOCK_DESK},
    inkwell:{kind:'piece',sel:'[data-prop="inkwell"]',lock:LOCK_DESK},
    waxstick:{kind:'piece',sel:'[data-prop="waxstick"]',lock:LOCK_DESK},
    herbs:{kind:'piece',sel:'[data-prop="herbs"]',lock:LOCK_DESK},
    ribbon:{kind:'piece',sel:'[data-prop="ribbon"]',lock:LOCK_DESK}
  };
  var BUILTIN={trim:'Flare the candle',snuff:'Snuff or light the candle',doodle:'A doodle in the margin',flame:'Flame up',pump:'Pump'};
  var TABS=[{id:'looks',label:'Looks',looks:true,note:'A look sets every journal choice at once. The window keeps following the calendar.'},
    {id:'paper',label:'Paper',groups:[['stock','Paper'],['age','Wear']],note:'Grimoire pages take a pale version of the ink.'},
    {id:'ink',label:'Ink and hand',groups:[['ink','Ink'],['hand','Handwriting']]},
    {id:'seal',label:'Seal',groups:[['seal','Wax'],['emblem','Signet']],note:'Pressed into every recipe that is finished.'},
    {id:'binding',label:'Binding',groups:[['binding','Leather and ribbon']],note:'The cover shows round the pages, and the ribbon runs down the spine.'},
    {id:'light',label:'Light',groups:[['light','On the desk'],['window','Through the window']],note:'From the calendar follows tonight’s weather and hour. The others preview other weather.'},
    {id:'pieces',label:'Pieces',pieces:true}];
  function thumb(key,v){return opts.thumbs?opts.thumbs+encodeURIComponent(key+'-'+v)+'.jpg':'';}

  return {label:'A crafting journal open on a desk, with the bench drawn in ink',layout:L,defs:defs,background:background,art:art,book:bookW,fx:fx,ambient:ambient,info:INFO,act:act,itemSpot:itemSpot,
    onDone:onDone,onFeedback:onFeedback,onReset:onReset,mounted:mounted,down:down,move:move,up:up,turnTo:turnTo,sound:'paper',
    bind:function(a){host=a;},
    opts:O,setOpt:setOpt,looks:LOOKS,look:look,choices:CHOICES,kit:[],pieces:PIECES,glows:[],tabs:TABS,builtins:BUILTIN,thumb:thumb,
    tables:TABLES,emblemSVG:emblemSVG,
    destroy:function(){dead=true;urls.forEach(function(u){URL.revokeObjectURL(u);});urls.length=0;}};
  }
  return {create:create};
})();
