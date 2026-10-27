/**
 * craft_desk.js -- the desk skin for the crafting bench (window.CraftDesk)
 *
 * A lit apothecary bench at night. Static art is SVG; the living parts (rain,
 * dust in the candlelight, steam, bubbles, sparks) are particles; the light
 * flickers over everything. Realism comes from one light: every highlight
 * faces the flame, every shadow runs away from it, and its light falls off
 * with distance into cool moonlit dark. Plaster and wood are lit per pixel by
 * SVG lighting filters, baked once into images so the frame stays cheap.
 *
 * The window holds the campaign's own sky, painted by the calendar's sky code
 * (SkyPane.Scene) from the campaign's calendar, so it shows the same date,
 * hour, moons and weather as the calendar page.
 *
 * CraftDesk.create(opts) makes one skin for one scene:
 *   opts.sky     {campaignId, calendarId} for the live calendar, or
 *                {calendar: {...}} for a fixed one (the Design Lab's sample)
 *   opts.info    hover names, lines and actions per piece, over the defaults
 *   opts.look    the room's settings (wall, bench, light, view...) to start from
 *   opts.thumbs  the folder holding the build tray's pictures
 * craft_bench.ai.md says what a skin must provide.
 */
window.CraftDesk = (function () {
  function create(opts) {
  opts = opts || {};
  var host = {scope: function (h) { return h; }, id: function (n) { return n; }, calm: false}, svgRef = null, dead = false;
  /* The Director's look settings. Every one of them redraws the room and
     bakes its light again, so a choice reads as a different room rather than
     a recoloured one. ldx and ldy are how far the light has been moved. */
  var O={time:'calendar',weather:'calendar',light:'candle',wall:'plaster',bench:'oak',view:'harbour',mood:'moody',cat:'black',cover:'oxblood',ldx:0,ldy:0,kit:[]};
  var LOOK={light:1,wall:1,bench:1,view:1,mood:1,cat:1,cover:1,ldx:1,ldy:1,kit:1};
  /* Every choice the build tray offers, with the words it shows. A saved look
     is checked against these, so an unknown value falls back to the default. */
  var CHOICES={wall:[['plaster','Plaster'],['stone','Stone'],['timber','Boards'],['brick','Brick']],bench:[['oak','Oak'],['walnut','Walnut'],['pine','Pine'],['slate','Slate']],
    light:[['candle','Candle'],['lantern','Lantern'],['witch','Witchlight']],mood:[['cosy','Warm'],['moody','Moody'],['grim','Grim']],
    view:[['harbour','Harbour'],['forest','Forest'],['mountains','Peaks'],['city','City'],['shutters','Shutters']],
    cat:[['black','Black'],['ginger','Ginger'],['grey','Grey'],['cream','Cream']],cover:[['oxblood','Oxblood'],['green','Green'],['navy','Navy'],['black','Black']],
    time:[['calendar','From the calendar'],['night','Night'],['dusk','Dusk']],weather:[['calendar','From the calendar'],['rain','Rain'],['snow','Snow']]};
  var KITDRAWN=[['skull','Skull'],['ball','Crystal ball'],['scales','Brass scales'],['mandrake','Mandrake']];
  function setLook(o){Object.keys(o||{}).forEach(function(k){var v=o[k];
    if(CHOICES[k]){if(CHOICES[k].some(function(c){return c[0]===v;}))O[k]=v;}
    else if(k==='kit'&&Array.isArray(v)){O.kit=v.filter(function(q,i){return KIT[q]&&v.indexOf(q)===i;});}
    else if(k==='ldx'||k==='ldy'){var n=Math.round(+v);if(isFinite(n))O[k]=Math.max(k==='ldx'?-1000:-420,Math.min(k==='ldx'?100:240,n));}});}
  // fy: how far above the stand the flame burns, which sets where every highlight and shadow points
  var LIGHT={candle:{c:'255,196,120',hex:'#ffc478',key:'#ffd2a0',fy:152,near:'#fff6e6'},lantern:{c:'255,226,170',hex:'#ffe2aa',key:'#ffe6c0',fy:50,near:'#fffaee'},witch:{c:'176,150,255',hex:'#b096ff',key:'#c8b8ff',fy:112,near:'#f3eeff'}};
  // The far room: how dark it gets away from the light, and the colour of that dark
  var MOOD={cosy:{vig:.38,amb:['#5a5466','#57505a','#665e70'],k3:.62,far:['#efe6dc','#c2b8b4','#8a8296','#4a4658']},
    moody:{vig:.55,amb:['#3e4a74','#3c4560','#4a5478'],k3:.5,far:['#e6ded4','#a6a6b6','#525874','#20243a']},
    grim:{vig:.7,amb:['#262c48','#252a3c','#2e3450'],k3:.36,far:['#cfc8c4','#7a7c92','#30344c','#0e1020']}};
  var BENCH={oak:[[.17,.09,.04],[.6,.38,.2]],walnut:[[.09,.045,.025],[.4,.22,.13]],pine:[[.32,.2,.1],[.86,.64,.38]]};
  var CAT={black:['#0d0a08','#0d0a08','#1c1814'],ginger:['#3a1c0a','#8a4a1c','#d08a48'],grey:['#1e1e22','#4e4e56','#8a8a94'],cream:['#5a5048','#b8ab98','#efe4d2']};
  var COVER={oxblood:['#3a1612','#4c1f1a','#8a4a32','#5e1212','#d06048'],green:['#142a1a','#1d3a24','#4a7a4e','#6a5a1a','#d8b860'],navy:['#121a30','#1a2644','#4a5e8a','#7a1a1a','#d86a5a'],black:['#141210','#1e1b18','#5a5048','#3a3a3a','#a8a8a8']};
  var KIT={skull:[1012,456],ball:[1166,466],scales:[645,446],mandrake:[150,378]};
  // A live calendar sets the hour; without one the room starts at night.
  if(!(opts.sky&&opts.sky.campaignId))O.time='night';
  setLook(opts.look);
  var L={
    mortar:{x:520,y:520,r:62},
    flask:{x:790,y:418,r:78,neck:262,neckW:17},
    burner:{x:790,y:566},
    // Ingredients wait on the tray: drawn ones where they sit best, any other in the next free spot
    items:{bloodmoss:[968,636],water:[1086,622],saltglass:[968,640],bile:[1086,630]},slots:[[968,636],[1086,624],[1030,668]],
    gauge:{x:612,y:322},bar:{x:620,y:196,w:340},dice:{x:604,y:424},result:{x:992,y:574},
    pour:{x:738,y:222,rot:125,mouth:[779,254]},
    tip:{x:742,y:206,rot:65,mx:784,my:262}
  };
  var CBX=1092,CBY=452,FX=1092,FY=300;   // where the light stands, and its flame
  function relight(){CBX=1092+O.ldx;CBY=452+O.ldy;FX=CBX;FY=CBY-LIGHT[O.light].fy;}
  /* Which way a shadow falls from (x,y): straight away from the flame. */
  function lightPt(){return [FX,FY];}
  function away(x,y,k){var dx=x-FX,dy=y-FY,d=Math.sqrt(dx*dx+dy*dy)||1;return [dx/d*k,dy/d*k];}
  function aff(tl,tr,bl,w,h){return 'matrix('+((tr[0]-tl[0])/w).toFixed(4)+','+((tr[1]-tl[1])/w).toFixed(4)+','+((bl[0]-tl[0])/h).toFixed(4)+','+((bl[1]-tl[1])/h).toFixed(4)+','+tl[0]+','+tl[1]+')';}
  var BOOK={l:aff([78,388],[261,397],[62,642],200,260),r:aff([266,397],[447,388],[258,646],200,260)};
  function stops(a){return a.map(function(s){return '<stop offset="'+s[0]+'" stop-color="'+s[1]+'"'+(s[2]!=null?' stop-opacity="'+s[2]+'"':'')+'/>';}).join('');}
  function lin(id,v,a){return '<linearGradient id="'+id+'" x1="0" y1="0" x2="'+(v?0:1)+'" y2="'+(v?1:0)+'">'+stops(a)+'</linearGradient>';}
  function rad(id,a,o){return '<radialGradient id="'+id+'"'+(o||'')+'>'+stops(a)+'</radialGradient>';}

  /* Filters for the baked layers. They use literal colours only, because
     a baked layer is drawn as a standalone image that cannot see the
     page's CSS. The key light's colour follows the Light setting. */
  function woodF(id,seed,fx,fy,c0,c1){var d=[c1[0]-c0[0],c1[1]-c0[1],c1[2]-c0[2]];
    return '<filter id="'+id+'" x="0" y="0" width="1" height="1" color-interpolation-filters="sRGB">'+
      '<feTurbulence type="fractalNoise" baseFrequency="'+fx+' '+fy+'" numOctaves="3" seed="'+seed+'" result="t"/>'+
      '<feComponentTransfer in="t" result="r"><feFuncR type="table" tableValues="0 .5 .28 .7 .42 .88 .5 .8 .34 .62 1"/></feComponentTransfer>'+
      '<feColorMatrix in="r" type="matrix" values="'+d[0]+' 0 0 0 '+c0[0]+'  '+d[1]+' 0 0 0 '+c0[1]+'  '+d[2]+' 0 0 0 '+c0[2]+'  0 0 0 0 1" result="c"/>'+
      '<feTurbulence type="fractalNoise" baseFrequency="'+(fx*14).toFixed(3)+' '+(fy*6).toFixed(3)+'" numOctaves="2" seed="'+(seed+5)+'" result="p"/>'+
      '<feColorMatrix in="p" type="matrix" values="0 0 0 0 .05  0 0 0 0 .025  0 0 0 0 .01  0 0 0 -6 2.25" result="pores"/>'+
      '<feComposite in="pores" in2="c" operator="over" result="w"/><feComposite in="w" in2="SourceAlpha" operator="in"/></filter>';}
  /* No filter uses a point light. Some phone GPUs work its distances out in
     half precision, which overflows across a scene this size, and the lit
     surface comes out black. So the bump is lit by a distant light from the
     flame's side, normalised so flat ground reads as it would under the
     flame, and the falloff with distance is a gradient laid over it. The
     book needs no gradient: the flame is far enough off that its light
     barely changes across the pages, so the distant light alone matches. */
  function sun(cx,cy,lx,ly,z){var d=Math.max(1,Math.hypot(lx-cx,ly-cy)),el=Math.atan2(z,d);
    return {s:Math.sin(el),light:'<feDistantLight azimuth="'+(Math.atan2(ly-cy,lx-cx)*180/Math.PI).toFixed(1)+'" elevation="'+(el*180/Math.PI).toFixed(1)+'"/>'};}
  /* The falloff a point light at height z gives flat ground, z/√(r²+z²),
     as a darkening gradient; amb is the room's ambient share, which stays. */
  function fall(id,lx,ly,z,amb){var R=1500,s='<radialGradient id="'+id+'" gradientUnits="userSpaceOnUse" cx="'+lx+'" cy="'+ly+'" r="'+R+'">';
    for(var i=0;i<=12;i++){var r=i/12*R,f=z/Math.sqrt(r*r+z*z);s+='<stop offset="'+(i/12).toFixed(3)+'" stop-color="#000" stop-opacity="'+((1-f)/(1+amb)).toFixed(3)+'"/>';}
    return s+'</radialGradient>';}
  // Light at half strength, doubled again in the last step, so no stage clips before the end.
  function half(hx){var c=parseInt(hx.slice(1),16);return 'rgb('+[c>>16&255,c>>8&255,c&255].map(function(v){return Math.round(v/2);}).join(',')+')';}
  /* The pool where the flame's highlight shows in the varnish, measured from
     the point light it replaces: an oval, taller than wide, fading out a
     few hands from the flame. */
  function sheen(cx,cy){var g='<radialGradient id="b-sheeng" gradientUnits="userSpaceOnUse" cx="'+cx+'" cy="'+cy+'" r="400" gradientTransform="translate('+cx+' '+cy+') scale(1 1.5) translate('+(-cx)+' '+(-cy)+')">'+
      stops([[0,'#fff',1],[.15,'#fff',.8],[.25,'#fff',.5],[.4,'#fff',.2],[.55,'#fff',.07],[.75,'#fff',.02],[1,'#fff',0]]);
    return g+'</radialGradient><mask id="b-sheenm" maskUnits="userSpaceOnUse" x="0" y="330" width="1200" height="370"><rect y="330" width="1200" height="370" fill="url(#b-sheeng)"/></mask>';}
  function lum(hx,k){var c=parseInt(hx.slice(1),16);return ((c>>16&255)*.2126+(c>>8&255)*.7152+(c&255)*.0722)/255*k;}
  /* Lighting for a wall drawn flat: a fine bump from noise, lit per pixel by
     the flame, plus the room's ambient. fq is the bump's grain; sc its depth. */
  function wallLit(id,fq,sc,seed){var M=MOOD[O.mood],k=LIGHT[O.light].key,W=sun(600,170,FX,FY,330);
    return '<filter id="'+id+'" filterUnits="userSpaceOnUse" x="0" y="0" width="1200" height="345" color-interpolation-filters="sRGB">'+
      '<feTurbulence type="fractalNoise" baseFrequency="'+fq+'" numOctaves="4" seed="'+seed+'" result="n"/>'+
      '<feTurbulence type="fractalNoise" baseFrequency=".0045" numOctaves="3" seed="8" result="m"/>'+
      '<feComposite in="n" in2="m" operator="arithmetic" k2=".3" k3=".9" k4="-.1" result="h"/>'+
      '<feDiffuseLighting in="h" surfaceScale="'+sc+'" diffuseConstant="'+(1.05/W.s).toFixed(3)+'" lighting-color="'+half(k)+'" result="d">'+W.light+'</feDiffuseLighting>'+
      '<feFlood flood-color="'+M.amb[0]+'" result="amb"/><feComposite in="d" in2="amb" operator="arithmetic" k2="1" k3="'+(M.k3/2)+'" result="L"/>'+
      '<feComposite in="SourceGraphic" in2="L" operator="arithmetic" k1="2.3" result="lit"/><feComposite in="lit" in2="SourceAlpha" operator="in"/></filter>';}
  function bakeDefs(){var k=LIGHT[O.light].key,M=MOOD[O.mood],W=BENCH[O.bench]||BENCH.oak,dz=308+LIGHT[O.light].fy,SX=O.bench==='slate'?30:24,SK=.6,D=sun(600,520,FX,CBY,dz),B=sun(255,517,FX,CBY-32,900);
    function wd(i,seed,fx,fy,a,b){return woodF('b-wood'+i,seed,fx,fy,W[0].map(function(v){return v*a;}),W[1].map(function(v){return v*b;}));}
    return wallLit('b-plaster',.03,3.2,4)+wallLit('b-stonelit',.05,4.5,6)+wallLit('b-bricklit',.09,2.4,7)+wallLit('b-timberlit',.02,1.6,9)+
    '<filter id="b-stain" x="0" y="0" width="1" height="1"><feTurbulence type="fractalNoise" baseFrequency=".006 .014" numOctaves="3" seed="15"/>'+
      '<feColorMatrix type="matrix" values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 -2.6 1.45"/><feComposite in="SourceGraphic" operator="in"/></filter>'+
    // Ragged mortar: the joints wander a little, as laid by hand
    '<filter id="b-rag" x="-2%" y="-2%" width="104%" height="104%"><feTurbulence type="fractalNoise" baseFrequency=".06" numOctaves="2" seed="3" result="t"/><feDisplacementMap in="SourceGraphic" in2="t" scale="5" xChannelSelector="R" yChannelSelector="G"/></filter>'+
    woodF('b-beam',3,.004,.16,[.12,.07,.035],[.42,.27,.15])+woodF('b-post',9,.16,.004,[.12,.07,.035],[.4,.26,.14])+
    woodF('b-board',13,.11,.0035,[.14,.08,.04],[.5,.33,.18])+
    wd(0,21,.0024,.052,1,1)+wd(1,33,.0022,.048,1.1,1.06)+wd(2,47,.0026,.055,.95,.97)+wd(3,58,.002,.045,1.05,1.03)+
    '<filter id="b-slate" x="0" y="0" width="1" height="1" color-interpolation-filters="sRGB"><feTurbulence type="fractalNoise" baseFrequency=".004 .012" numOctaves="4" seed="27" result="t"/>'+
      '<feColorMatrix in="t" type="matrix" values=".16 0 0 0 .12  .16 0 0 0 .135  .18 0 0 0 .16  0 0 0 0 1" result="c"/>'+
      '<feTurbulence type="fractalNoise" baseFrequency=".6" numOctaves="2" seed="28" result="p"/><feColorMatrix in="p" type="matrix" values="0 0 0 0 .3  0 0 0 0 .32  0 0 0 0 .35  0 0 0 -10 4.6" result="fl"/>'+
      '<feComposite in="fl" in2="c" operator="over" result="w"/><feComposite in="w" in2="SourceAlpha" operator="in"/></filter>'+
    '<filter id="b-desklit" filterUnits="userSpaceOnUse" x="0" y="330" width="1200" height="370" color-interpolation-filters="sRGB">'+
      // Wood is bumped along its grain; honed slate only faintly, all over
      '<feTurbulence type="fractalNoise" baseFrequency="'+(O.bench==='slate'?'.006 .012':'.003 .07')+'" numOctaves="'+(O.bench==='slate'?1:3)+'" seed="31" result="b"/>'+
      '<feDiffuseLighting in="b" surfaceScale="'+(O.bench==='slate'?.8:2)+'" diffuseConstant="'+(1/D.s).toFixed(3)+'" lighting-color="'+half(k)+'" result="d">'+D.light+'</feDiffuseLighting>'+
      '<feFlood flood-color="'+M.amb[1]+'" result="amb"/><feComposite in="d" in2="amb" operator="arithmetic" k2="1" k3="'+(M.k3*.45).toFixed(3)+'" result="L"/>'+
      '<feComposite in="SourceGraphic" in2="L" operator="arithmetic" k1="2.5" result="lit"/>'+
      '</filter>'+
    // The varnish sheen: the same grain's highlights, kept to the pool in front of the flame by a mask
    '<filter id="b-sheen" filterUnits="userSpaceOnUse" x="0" y="330" width="1200" height="370" color-interpolation-filters="sRGB">'+
      '<feTurbulence type="fractalNoise" baseFrequency="'+(O.bench==='slate'?'.006 .012':'.003 .07')+'" numOctaves="'+(O.bench==='slate'?1:3)+'" seed="31" result="b"/>'+
      '<feSpecularLighting in="b" surfaceScale="3" specularConstant="'+(O.bench==='slate'?2.2:5.3)+'" specularExponent="'+SX+'" lighting-color="'+k+'" result="s"><feDistantLight azimuth="50" elevation="38"/></feSpecularLighting>'+
      '<feComponentTransfer><feFuncA type="linear" slope="'+SK+'"/></feComponentTransfer></filter>'+
    sheen(FX,CBY+108)+
    '<filter id="b-booklit" filterUnits="userSpaceOnUse" x="0" y="350" width="500" height="340" color-interpolation-filters="sRGB">'+
      '<feTurbulence type="fractalNoise" baseFrequency=".04" numOctaves="3" seed="12" result="b"/>'+
      '<feDiffuseLighting in="b" surfaceScale="1.2" diffuseConstant="1" lighting-color="'+k+'" result="d">'+B.light+'</feDiffuseLighting>'+
      '<feFlood flood-color="'+M.amb[2]+'" result="amb"/><feComposite in="d" in2="amb" operator="arithmetic" k2="1" k3="'+(M.k3*1.1).toFixed(2)+'" result="L"/>'+
      '<feComposite in="SourceGraphic" in2="L" operator="arithmetic" k1="1.32"/></filter>'+
    '<filter id="b-paper" x="0" y="0" width="1" height="1"><feTurbulence type="fractalNoise" baseFrequency=".09 .5" numOctaves="3" seed="5"/>'+
      '<feColorMatrix type="matrix" values="0 0 0 0 .35  0 0 0 0 .25  0 0 0 0 .12  0 0 0 -3 1.75"/><feComposite in2="SourceGraphic" operator="in"/></filter>'+
    '<filter id="b-leather" x="0" y="0" width="1" height="1"><feTurbulence type="fractalNoise" baseFrequency=".09" numOctaves="4" seed="19"/>'+
      '<feColorMatrix type="matrix" values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 -4 2.3"/><feComposite in2="SourceGraphic" operator="in"/></filter>'+
    fall('b-fallw',FX,FY,330,lum(M.amb[0],M.k3))+fall('b-falld',FX,CBY,dz,lum(M.amb[1],M.k3*.9))+
    rad('b-soot',[[0,'#080503',.32],[.6,'#080503',.1],[1,'#080503',0]])+
    lin('b-run',1,[[0,'#140c06',.9],[1,'#140c06',0]])+
    lin('b-ao',1,[[0,'#000',0],[.55,'#000',.25],[1,'#000',.7]])+
    lin('b-beamsh',1,[[0,'#000',.55],[.7,'#000',.1],[.85,LIGHT[O.light].key,.25],[1,LIGHT[O.light].key,.05]])+
    lin('b-postL',0,[[0,'#000',.55],[.8,'#000',.15],[1,'#000',.35]])+lin('b-postR',0,[[0,LIGHT[O.light].key,.18],[.25,'#000',0],[1,'#000',.45]])+
    lin('b-gutL',0,[[0,'#2a1a0c',0],[.7,'#2a1a0c',.06],[1,'#2a1a0c',.34]])+lin('b-gutR',0,[[0,'#2a1a0c',.3],[.3,'#2a1a0c',.05],[1,'#2a1a0c',0]])+
    lin('b-pagel',0,[[0,'#cdb88c'],[.08,'#e6d6b0'],[.9,'#f1e5c6'],[1,'#e2d0a8']])+lin('b-pager',0,[[0,'#e2d0a8'],[.1,'#f3e8cb'],[.92,'#eadbb7'],[1,'#d2be94']]);}

  function defs(){relight();return ''+
    '<linearGradient id="d-sky" x1="0" y1="0" x2="0" y2="1"><stop offset="0" style="stop-color:var(--sky1)"/><stop offset="1" style="stop-color:var(--sky2)"/></linearGradient>'+
    lin('d-brass',0,[[0,'#24180a'],[.08,'#6a675f'],[.18,'#3b2a11'],[.46,'#8e6a2b'],[.68,'#e6c178'],[.76,'#fff0c8'],[.84,'#bd8b3a'],[1,'#2e200c']])+
    lin('d-brassv',1,[[0,'#f6dc9c'],[.3,'#b88b3e'],[.75,'#5e4318'],[1,'#2e200c']])+
    lin('d-stone',0,[[0,'#2c2f34'],[.07,'#6c7079'],[.18,'#3e4147'],[.58,'#8a8d93'],[.78,'#c1c3c7'],[.88,'#878a90'],[1,'#303338']])+
    lin('d-woodh',1,[[0,'#2e1c0e'],[.2,'#6e4a2a'],[.55,'#4e321b'],[1,'#24160a']])+
    lin('d-frame',0,[[0,'#24170c'],[.5,'#3a2716'],[.85,'#5a3d22'],[1,'#3a2716']])+
    lin('d-glassh',0,[[0,'#04070a',.6],[.12,'#04070a',.2],[.32,'#fff',0],[.66,'#fff',.05],[.8,'#ffe6c4',.26],[.9,'#04070a',.18],[1,'#04070a',.62]])+
    lin('d-cyl',0,[[0,'#000',.6],[.25,'#000',.25],[.62,'#fff',0],[.8,'#ffd9a8',.16],[1,'#000',.55]])+
    rad('d-glassball',[[0,'#d8ecff',.02],[.7,'#bcd8f0',.05],[.87,'#081018',.3],[.955,'#e8f4ff',.5],[1,'#e8f4ff',.1]],' cx=".5" cy=".5" r=".5"')+
    rad('d-sphere',[[0,'#fff',.5],[.2,'#fff',.1],[.55,'#000',0],[1,'#000',.6]],' cx=".66" cy=".32" r=".75"')+
    lin('d-wax',0,[[0,'#7f735a'],[.16,'#c4b897'],[.5,'#eadfc2'],[.74,'#fff6df'],[.88,'#dccfae'],[1,'#7c6e52']])+
    lin('d-waxglow',1,[[0,'#ffc56a',.8],[.28,'#ffc56a',.22],[1,'#ffc56a',0]])+
    lin('d-page',0,[[0,'#d9c9a3'],[.85,'#efe3c4'],[1,'#c8b48a']])+
    lin('d-linen',1,[[0,'#cfc2a4'],[.5,'#e2d7bd'],[1,'#b9ab8c']])+
    '<radialGradient id="d-flame" cx=".5" cy=".75" r=".7"><stop offset="0" stop-color="#fffbe6"/><stop offset=".35" stop-color="#ffd27a"/><stop offset=".8" stop-color="#ff8a2a"/><stop offset="1" stop-color="#ff5a1a" stop-opacity="0"/></radialGradient>'+
    '<radialGradient id="d-bflame" cx=".5" cy=".8" r=".75"><stop offset="0" stop-color="#e8f4ff"/><stop offset=".35" stop-color="#7ab8ff"/><stop offset=".75" stop-color="#2f5bd8"/><stop offset="1" stop-color="#1a2a8a" stop-opacity="0"/></radialGradient>'+
    '<radialGradient id="d-glow"><stop offset="0" style="stop-color:var(--lit)" stop-opacity=".9"/><stop offset=".4" style="stop-color:var(--lit)" stop-opacity=".25"/><stop offset="1" style="stop-color:var(--lit)" stop-opacity="0"/></radialGradient>'+
    '<radialGradient id="d-bloom"><stop offset="0" stop-color="#fffbea" stop-opacity=".95"/><stop offset=".14" style="stop-color:var(--lit)" stop-opacity=".55"/><stop offset=".45" style="stop-color:var(--lit)" stop-opacity=".1"/><stop offset="1" style="stop-color:var(--lit)" stop-opacity="0"/></radialGradient>'+
    '<radialGradient id="d-bglow"><stop offset="0" stop-color="#6aa8ff" stop-opacity=".55"/><stop offset="1" stop-color="#6aa8ff" stop-opacity="0"/></radialGradient>'+
    '<radialGradient id="d-moon"><stop offset="0" stop-color="#fff8e0"/><stop offset=".25" stop-color="#f2ecd6" stop-opacity=".9"/><stop offset=".26" stop-color="#cfd8ff" stop-opacity=".25"/><stop offset="1" stop-color="#cfd8ff" stop-opacity="0"/></radialGradient>'+
    '<radialGradient id="d-vig" cx=".62" cy=".45" r=".85"><stop offset=".5" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" style="stop-opacity:var(--vig)"/></radialGradient>'+
    // The candle's falloff, multiplied over the whole room: warm and full near the flame, cool and dim far off
    rad('d-shade',(function(){var F=MOOD[O.mood].far,n=LIGHT[O.light].near;return [[0,n],[.2,n],[.42,F[0]],[.62,F[1]],[.82,F[2]],[1,F[3]]];})(),' cx=".5" cy=".5" r=".5"')+
    lin('d-catf',0,[[0,CAT[O.cat][0]],[.55,CAT[O.cat][1]],[1,CAT[O.cat][2]]])+
    rad('d-orb',[[0,'#ffffff'],[.25,'#e8e0ff'],[.6,'#a890ff',.85],[1,'#6a4aff',0]])+
    rad('d-lglass',[[0,'#fff6dc',.95],[.35,'#ffd890',.7],[1,'#c88a3a',.4]],' cx=".5" cy=".55" r=".6"')+
    lin('d-bone',0,[[0,'#4a3e30'],[.4,'#a8987a'],[.8,'#eadcc0'],[1,'#c8b898']])+lin('d-pot',0,[[0,'#3a1a0e'],[.5,'#8a4a2a'],[.85,'#c8784a'],[1,'#6a3018']])+
    lin('d-iron',0,[[0,'#0e0d0c'],[.4,'#2a2724'],[.7,'#4a4540'],[.85,'#6a6258'],[1,'#1a1816']])+
    rad('d-shadow',[[0,'#000',.55],[1,'#000',0]])+rad('d-ao',[[0,'#000',.85],[.55,'#000',.4],[1,'#000',0]])+
    lin('d-cast',0,[[0,'#000',.6],[.45,'#000',.28],[1,'#000',0]])+
    rad('d-liqglow',[[0,'#fff',.5],[1,'#fff',0]])+rad('d-warm',[[0,'#ffd9a0',.9],[.3,'#ffb460',.3],[1,'#ff9a40',0]])+
    rad('d-caus2',[[0,'#fff',.95],[.45,'#fff',.4],[1,'#fff',0]])+rad('d-caustic',[[0,'#fff',.95],[.45,'#fff',.4],[1,'#fff',0]])+lin('d-fog',1,[[0,'#dfe8ff',0],[1,'#dfe8ff',.16]])+lin('d-under',0,[[0,'#000',.55],[1,'#ffcf96',.35]])+lin('d-bowl',1,[[0,'#5a5d63'],[.45,'#2a2c31'],[1,'#121316']])+lin('d-quill',0,[[0,'#8f8572'],[.55,'#ece4d2'],[1,'#fffaf0']])+lin('d-tray',1,[[0,'#7a5230'],[.12,'#4e321b'],[1,'#2e1c0e']])+
    '<filter id="d-grain" x="0" y="0" width="100%" height="100%"><feTurbulence type="fractalNoise" baseFrequency=".005 .16" numOctaves="3" seed="7"/><feColorMatrix values="0 0 0 0 .1  0 0 0 0 .05  0 0 0 0 .02  0 0 0 1.1 -.45"/><feComposite in2="SourceGraphic" operator="in"/></filter>'+
    '<filter id="d-speck" x="0" y="0" width="1" height="1"><feTurbulence type="fractalNoise" baseFrequency=".55" numOctaves="2" seed="2"/><feColorMatrix type="matrix" values="0 0 0 0 .1  0 0 0 0 .1  0 0 0 0 .11  0 0 0 -9 4.4"/><feComposite in2="SourceGraphic" operator="in"/></filter>'+
    '<filter id="d-blur14" x="-20%" y="-20%" width="140%" height="140%"><feGaussianBlur stdDeviation="14"/></filter><filter id="d-blur8" x="-30%" y="-30%" width="160%" height="160%"><feGaussianBlur stdDeviation="8"/></filter>'+
    '<filter id="d-blur5" x="-30%" y="-60%" width="160%" height="220%"><feGaussianBlur stdDeviation="5"/></filter><filter id="d-blur3" x="-30%" y="-30%" width="160%" height="160%"><feGaussianBlur stdDeviation="3"/></filter>'+
    '<filter id="d-blur1"><feGaussianBlur stdDeviation="1.2"/></filter>'+
    // The back wall sits beyond the focus of the bench, so it is a touch soft
    '<filter id="d-dof" x="-2%" y="-2%" width="104%" height="104%"><feGaussianBlur stdDeviation=".75"/></filter>'+
    '<linearGradient id="d-shaft" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#a9bcff" stop-opacity=".24"/><stop offset="1" stop-color="#a9bcff" stop-opacity="0"/></linearGradient>'+
    '<clipPath id="d-winclip"><rect x="706" y="42" width="248" height="200"/></clipPath>'+bakeDefs();
  }

  /* A soft shadow on the desk from (x,y), running straight away from the
     candle and foreshortened like the desk top. */
  function cast(x,y,w,len,op){var dx=x-CBX,dy=(y-CBY)*2,a=Math.atan2(dy,dx)*180/Math.PI;
    return '<g transform="translate('+x+','+y+') scale(1,.5) rotate('+a.toFixed(1)+')" opacity="'+(op||1)+'"><path d="M0,'+(-w/2)+' L'+len+','+(-w*.85)+' L'+len+','+(w*.85)+' L0,'+(w/2)+' Z" fill="url(#d-cast)" filter="url(#d-blur5)"/></g>';}
  function ao(x,y,rx,ry,op){return '<ellipse cx="'+x+'" cy="'+y+'" rx="'+rx+'" ry="'+ry+'" fill="url(#d-ao)" opacity="'+(op||1)+'"/>';}
  /* Seeded randomness, so the scratches and crumbs are the same every load. */
  function rng(s){return function(){s=(s*16807)%2147483647;return (s-1)/2147483646;};}

  /* The back wall, the ceiling beam and the posts. Baked. Each material is
     drawn flat, then lit per pixel by the flame like the plaster. */
  function wall(){var s='',R=rng(41),i,x,y,w;
    if(O.wall==='stone'){
      // Rough-dressed blocks in uneven courses, ragged mortar between
      var j='',t='',e='';s+='<g filter="url(#b-stonelit)"><rect width="1200" height="345" fill="#77706a"/>';
      for(y=26;y<345;y+=h){var h=44+R()*16|0;for(x=-R()*60;x<1200;x+=w){w=70+R()*90|0;var v=R();
        t+='<rect x="'+x.toFixed(0)+'" y="'+y+'" width="'+w+'" height="'+h+'" rx="6" fill="'+(v<.5?'#000':'#e8d6bc')+'" fill-opacity="'+(v<.5?.05+v*.2:.03+(v-.5)*.12).toFixed(3)+'"/>';
        j+='M'+x.toFixed(0)+','+y+' h'+w+' M'+x.toFixed(0)+','+y+' v'+h+' ';
        e+='M'+(x+5).toFixed(0)+','+(y+h-3)+' h'+(w-10)+' ';
        t+='<rect x="'+(x+2).toFixed(0)+'" y="'+(y+2)+'" width="'+(w-4)+'" height="9" rx="4" fill="#000" opacity=".16"/>';}}
      s+=t+'<path d="'+j+'" stroke="#2a2420" stroke-width="5" fill="none" filter="url(#b-rag)" opacity=".85"/>'+
        '<path d="'+j+'" stroke="#120e0b" stroke-width="2" fill="none" filter="url(#b-rag)"/>'+
        '<path d="'+e+'" stroke="#ffdcb0" stroke-opacity=".12" stroke-width="2" fill="none" filter="url(#b-rag)"/></g>'+
        '<rect width="1200" height="345" fill="#1a140e" filter="url(#b-stain)" opacity=".26"/>';}
    else if(O.wall==='brick'){
      // Running bond, a few bricks burnt darker or paler than the rest
      var bt='',mj='';s+='<g filter="url(#b-bricklit)"><rect width="1200" height="345" fill="#6e6456"/>';
      for(y=28,i=0;y<345;y+=24,i++){for(x=i%2?-34:0;x<1200;x+=68){var c=R(),col=c<.15?'#3e1c14':c<.3?'#7e4e38':c<.55?'#663024':'#70382a';
        bt+='<rect x="'+(x+2)+'" y="'+(y+2)+'" width="64" height="20" rx="1.5" fill="'+col+'"/>';
        if(R()<.25)bt+='<rect x="'+(x+2+R()*40).toFixed(0)+'" y="'+(y+3)+'" width="'+(8+R()*14).toFixed(0)+'" height="'+(4+R()*8).toFixed(0)+'" fill="#000" opacity=".12"/>';
        mj+='M'+(x+4)+','+(y+22)+' h60 ';}}
      s+='<g filter="url(#b-rag)">'+bt+'</g><path d="'+mj+'" stroke="#ffd9b0" stroke-opacity=".14" stroke-width="1.4" fill="none"/></g>'+
        '<rect width="1200" height="345" fill="#140c08" filter="url(#b-stain)" opacity=".3"/>';}
    else if(O.wall==='timber'){
      // Upright boards of different widths, the gaps between them dark
      s+='<g filter="url(#b-timberlit)"><rect width="1200" height="345" fill="#000" filter="url(#b-board)"/>';var g='';
      for(x=36;x<1170;x+=w){w=58+R()*40|0;var q=R();s+='<rect x="'+x+'" width="'+w+'" height="345" fill="'+(q<.5?'#000':'#f0c890')+'" opacity="'+(q<.5?.06+q*.24:.02+(q-.5)*.1).toFixed(3)+'"/>';
        g+='M'+x+',0 V345 ';if(R()<.5){var ky=60+R()*220;s+='<ellipse cx="'+(x+w*(.3+R()*.4)).toFixed(0)+'" cy="'+ky.toFixed(0)+'" rx="5" ry="9" fill="#1a0e06" opacity=".5"/>';}}
      s+='<path d="'+g+'" stroke="#0a0604" stroke-width="3.5" fill="none"/><path d="'+g.replace(/M(\d+)/g,function(m,n){return 'M'+(+n+2.5);})+'" stroke="#ffd2a0" stroke-opacity=".12" stroke-width="1.2" fill="none"/></g>';}
    else s+='<rect width="1200" height="345" fill="#9a7f62" filter="url(#b-plaster)"/>'+
      '<rect width="1200" height="345" fill="#1e140b" filter="url(#b-stain)" opacity=".32"/>';
    s+='<rect width="1200" height="345" fill="url(#b-fallw)"/><ellipse cx="'+(FX-2)+'" cy="'+Math.min(170,FY-130)+'" rx="46" ry="130" fill="url(#b-soot)"/>'+
      '<rect y="296" width="1200" height="50" fill="url(#b-ao)"/>';
    // Beam and posts: rough oak, lit from below by the flame
    s+='<rect width="1200" height="28" fill="#000" filter="url(#b-beam)"/><rect width="1200" height="28" fill="url(#b-beamsh)"/><rect y="28" width="1200" height="10" fill="url(#b-ao)" transform="rotate(180 600 33)" opacity=".6"/>'+
      '<rect x="0" width="36" height="345" fill="#000" filter="url(#b-post)"/><rect x="0" width="36" height="345" fill="url(#b-postL)"/><rect x="36" width="10" height="345" fill="#000" opacity=".28"/>'+
      '<rect x="1164" width="36" height="345" fill="#000" filter="url(#b-post)"/><rect x="1164" width="36" height="345" fill="url(#b-postR)"/><rect x="1154" width="10" height="345" fill="#000" opacity=".18"/>';
    return s;}

  /* The bench top: four oak planks whose grain tightens toward the wall,
     lit by the candle with a varnish sheen in front of it. Baked. */
  function desk(){var P=[[338,404,.55],[404,492,.72],[492,602,.9],[602,700,1.1]],R=rng(11),s='<g filter="url(#b-desklit)">',slate=O.bench==='slate';
    if(slate){
      // Two slate slabs, honed smooth, with one joint and a pale bevel at the front
      s+='<g transform="translate(0,338) scale(1,.7)"><rect width="1200" height="520" fill="#000" filter="url(#b-slate)"/></g>'+
        '<rect x="612" y="338" width="3" height="362" fill="#06080a" opacity=".8"/><rect x="615" y="338" width="1.2" height="362" fill="#cfd8e0" opacity=".16"/>'+
        '<rect y="336" width="1200" height="3" fill="#cfd8e0" opacity=".28"/>';}
    else{
      P.forEach(function(p,i){var h=(p[1]-p[0])/p[2];s+='<g transform="translate(0,'+p[0]+') scale(1,'+p[2]+')"><rect width="1200" height="'+h.toFixed(1)+'" fill="#000" filter="url(#b-wood'+i+')"/></g>';
        s+='<rect y="'+p[0]+'" width="1200" height="'+(p[1]-p[0])+'" fill="'+['#000','#3a1c08','#000','#2a1406'][i]+'" opacity="'+[.12,.1,0,.08][i]+'"/>';});
      // seams between planks
      P.slice(1).forEach(function(p){var y=p[0];s+='<rect y="'+(y-4)+'" width="1200" height="3" fill="#000" opacity=".2"/><rect y="'+(y-1.5)+'" width="1200" height="3" fill="#0b0502" opacity=".85"/><rect y="'+(y+1.5)+'" width="1200" height="1.2" fill="#ffd8a8" opacity=".14"/>';});
      // Pine shows its knots
      if(O.bench==='pine')[[180,372,.55],[860,450,.72],[300,560,.9],[1010,650,1.1],[640,520,.9]].forEach(function(k){s+='<ellipse cx="'+k[0]+'" cy="'+k[1]+'" rx="'+(14/k[2]*.6).toFixed(1)+'" ry="'+(5*k[2]).toFixed(1)+'" fill="#3a1e0a" opacity=".55"/><ellipse cx="'+k[0]+'" cy="'+k[1]+'" rx="'+(26/k[2]*.6).toFixed(1)+'" ry="'+(9*k[2]).toFixed(1)+'" fill="none" stroke="#5a3010" stroke-opacity=".3" stroke-width="2"/>';});
      s+='<rect y="336" width="1200" height="3" fill="#e6b27a" opacity=".3"/>';}
    // years of use: a cup ring, scratches, ink, scorch under the burner, crumbs by the mortar
    s+='<ellipse cx="626" cy="440" rx="30" ry="9" fill="none" stroke="#1a0d05" stroke-opacity=".38" stroke-width="2.4"/><ellipse cx="629" cy="441" rx="25" ry="7" fill="none" stroke="#1a0d05" stroke-opacity=".14" stroke-width="1.2"/>';
    for(var i=0;i<26;i++){var x=R()*1200,y=345+R()*350,l=8+R()*36,a=(R()-.5)*.5;s+='<path d="M'+x.toFixed(0)+','+y.toFixed(0)+' l'+(Math.cos(a)*l).toFixed(1)+','+(Math.sin(a)*l*.5).toFixed(1)+'" stroke="'+(R()<.6?'#f2c28a':'#0e0703')+'" stroke-opacity="'+(.04+R()*.07).toFixed(2)+'" stroke-width="1"/>';}
    for(i=0;i<9;i++)s+='<ellipse cx="'+(424+R()*90).toFixed(0)+'" cy="'+(418+R()*20).toFixed(0)+'" rx="'+(1+R()*3.5).toFixed(1)+'" ry="'+(.6+R()*1.4).toFixed(1)+'" fill="#06040a" opacity=".55"/>';
    s+='<ellipse cx="790" cy="652" rx="76" ry="15" fill="url(#d-ao)" opacity=".35"/></g><rect y="336" width="1200" height="364" fill="url(#b-falld)"/><rect y="336" width="1200" height="364" filter="url(#b-sheen)" mask="url(#b-sheenm)" style="mix-blend-mode:screen;mix-blend-mode:plus-lighter"/>';
    for(i=0;i<34;i++)s+='<circle cx="'+(440+R()*190).toFixed(0)+'" cy="'+(588+R()*22).toFixed(0)+'" r="'+(.6+R()*1.3).toFixed(1)+'" fill="'+(R()<.5?'#6b6e75':'#7d1828')+'" opacity=".7"/>';
    // shadows of the apparatus that never moves
    s+=cast(520,586,104,170,.85)+ao(520,585,58,9)+
      cast(790,640,80,120,.8)+ao(790,640,48,9)+
      cast(712,646,6,110,.6)+cast(868,646,6,110,.5);
    return s;}
  /* Where a shelf object's shadow lands on the wall behind it: away from the
     flame, mostly upward because the light sits below the shelf and close
     to the wall. */
  function wsh(x,y){return [Math.max(-16,Math.min(16,(x-FX)*.012)),Math.max(-26,Math.min(14,(y-FY)*.09))];}
  /* A glass jar on the shelf, its shadow climbing the wall away from the light. */
  function jar(x,w,h,col,lid,glow){var y=186,x0=x-w/2,r=(w*.22).toFixed(1),d=wsh(x,y-h/2);
    return '<rect x="'+(x0+d[0]).toFixed(1)+'" y="'+(y-h+d[1]).toFixed(1)+'" width="'+w+'" height="'+(h+4)+'" rx="'+r+'" fill="#000" opacity=".32" filter="url(#d-blur3)"/>'+
      (glow?'<ellipse cx="'+x+'" cy="'+(y-h/2)+'" rx="'+(w*1.3)+'" ry="'+(h*.8)+'" fill="'+col+'" opacity=".1" filter="url(#d-blur8)" class="d-jarglow"/>':'')+
      '<rect x="'+(x0+2.5)+'" y="'+(y-h*.62)+'" width="'+(w-5)+'" height="'+(h*.62-2)+'" rx="'+(w*.18).toFixed(1)+'" fill="'+col+'"/>'+
      '<rect x="'+x0+'" y="'+(y-h)+'" width="'+w+'" height="'+h+'" rx="'+r+'" fill="'+col+'" fill-opacity=".16"/>'+
      '<rect x="'+x0+'" y="'+(y-h)+'" width="'+w+'" height="'+h+'" rx="'+r+'" fill="url(#d-glassh)"/>'+
      '<rect x="'+(x0+3)+'" y="'+(y-h*.62)+'" width="'+(w-6)+'" height="1.4" fill="#fff" opacity=".28"/>'+
      '<rect x="'+(x0+w*.72).toFixed(1)+'" y="'+(y-h+6)+'" width="2" height="'+(h-13)+'" rx="1" fill="#fff4dc" opacity=".75"/>'+
      '<rect x="'+(x0+w*.14).toFixed(1)+'" y="'+(y-h+8)+'" width="2.5" height="'+(h-18)+'" rx="1.2" fill="#cfe0ff" opacity=".2"/>'+
      '<rect x="'+(x0-2)+'" y="'+(y-h-7)+'" width="'+(w+4)+'" height="8" rx="2" fill="'+(lid||'#7a5530')+'"/><rect x="'+(x0-2)+'" y="'+(y-h-7)+'" width="'+(w+4)+'" height="8" rx="2" fill="url(#d-cyl)"/>'+
      '<rect x="'+(x0-2)+'" y="'+(y-h+.5)+'" width="'+(w+4)+'" height="1.5" fill="#000" opacity=".35"/>';}
  function spine(x,y,w,h,c){var d=wsh(x+w/2,y+h/2);return '<rect x="'+(x+d[0]).toFixed(1)+'" y="'+(y+d[1]).toFixed(1)+'" width="'+w+'" height="'+h+'" fill="#000" opacity=".3" filter="url(#d-blur3)"/>'+
    '<rect x="'+x+'" y="'+y+'" width="'+w+'" height="'+h+'" fill="'+c+'"/><rect x="'+x+'" y="'+y+'" width="'+w+'" height="'+h+'" fill="url(#d-cyl)"/>'+
    '<rect x="'+x+'" y="'+(y+6)+'" width="'+w+'" height="1.4" fill="#d8b46a" opacity=".55"/><rect x="'+x+'" y="'+(y+h-9)+'" width="'+w+'" height="1.4" fill="#d8b46a" opacity=".55"/>';}
  function herbs(x,len,col,sh){var st=sh?'#000':'#8a7350',s='<line x1="'+x+'" y1="24" x2="'+x+'" y2="'+(40+len*.2)+'" stroke="'+st+'" stroke-width="1.5"/><g transform="translate('+x+','+(44+len*.2)+')">';
    for(var i=0;i<9;i++){var a=-40+i*10,ly=8+(i%3)*10+len*.4,ex=(Math.sin(a*Math.PI/180)*ly).toFixed(1),ey=ly.toFixed(1);
      s+='<path d="M0,0 L'+ex+','+ey+'" stroke="'+(sh?'#000':col)+'" stroke-width="1.6"/><ellipse cx="'+ex+'" cy="'+ey+'" rx="3.4" ry="7" transform="rotate('+(-a)+' '+ex+' '+ey+')" fill="'+(sh?'#000':col)+'"'+(!sh&&i%2?' fill-opacity=".72"':'')+'/>';
      if(!sh&&a>-10)s+='<path d="M'+(+ex+2.6).toFixed(1)+','+(ey-5)+' q1.6,5 0,10" transform="rotate('+(-a)+' '+ex+' '+ey+')" stroke="#ffd9a0" stroke-opacity=".3" stroke-width="1" fill="none"/>';}
    return s+'<rect x="-5" y="-3" width="10" height="7" rx="2" fill="'+(sh?'#000':'#9b7b4c')+'"/></g>';}
  function herb(x,len,col){var d=wsh(x,60);return '<g transform="translate('+(d[0]*1.1).toFixed(1)+','+(d[1]*.25).toFixed(1)+')" opacity=".3" filter="url(#d-blur3)">'+herbs(x,len,col,1)+'</g>'+herbs(x,len,col);}

  /* The window. Behind the glass is the campaign's own sky (Chronicle's sky
     code paints it into the canvas), then whatever stands outside: the
     harbour, a forest, mountains or a city. The drawn moon and stars are only
     a fallback for when that sky can't load. Shutters close it off. */
  function pine(x,b,h,w){var s='M'+x+','+(b-h),t=4,i;for(i=1;i<=t;i++){var y=b-h+h*i/t,ww=w*(.35+.65*i/t);s+=' L'+(x+ww/2).toFixed(1)+','+y.toFixed(1)+' L'+(x+ww*.18).toFixed(1)+','+(y-h*.06).toFixed(1);}
    for(i=t;i>=1;i--){var y2=b-h+h*i/t,w2=w*(.35+.65*i/t);s+=' L'+(x-w2*.18).toFixed(1)+','+(y2-h*.06).toFixed(1)+' L'+(x-w2/2).toFixed(1)+','+y2.toFixed(1);}return s+' Z';}
  function outside(){var v=O.view,R=rng(17),s='',i,x;
    if(v==='forest'){var far='',near='';for(x=700;x<960;x+=10+R()*8)far+=pine(x,236,30+R()*26,16+R()*8);for(x=704;x<960;x+=18+R()*16)near+=pine(x,246,48+R()*44,24+R()*12);
      return '<path d="'+far+'" fill="#121a28"/><path d="'+near+'" fill="#06090f"/>'+
        '<g class="d-eyes"><circle cx="884" cy="228" r="1.4" fill="#d8f07a"/><circle cx="890" cy="228" r="1.4" fill="#d8f07a"/></g>';}
    if(v==='mountains'){return '<path d="M706,200 L736,150 L752,166 L788,104 L812,138 L826,124 L866,176 L898,118 L930,160 L954,140 L954,242 L706,242 Z" fill="#33405e"/>'+
        '<path d="M788,104 L812,138 L804,134 L797,140 L790,128 L781,136 L776,127 Z M898,118 L930,160 L920,150 L912,156 L903,141 L893,149 L886,138 Z M826,124 L840,142 L832,139 L824,145 L818,135 Z M736,150 L752,166 L744,162 L738,167 L731,159 Z" fill="#d6def0" opacity=".55"/>'+
        '<path d="M788,104 L776,127 L760,182 L772,160 Z M898,118 L886,138 L872,190 L884,166 Z" fill="#0e1424" opacity=".45"/>'+
        '<path d="M706,226 Q760,200 820,214 T954,206 L954,242 L706,242 Z" fill="#0a0e1a"/>'+
        '<circle cx="768" cy="214" r="16" fill="url(#d-warm)" opacity=".3"/><rect x="767" y="212" width="2.4" height="3.4" fill="#ffe2a0"/>';}
    if(v==='city'){var b='',w='',far2='';for(x=704;x<960;x+=14+R()*10)far2+='<rect x="'+x.toFixed(0)+'" y="'+(170+R()*30).toFixed(0)+'" width="16" height="80" fill="#141c30"/>';
      var B=[[706,30,62],[734,22,84,'s'],[754,34,50],[786,18,104,'t'],[802,30,70],[830,40,46,'d'],[868,24,78],[890,28,96,'s'],[916,22,60],[936,20,82]];
      B.forEach(function(q){var top=242-q[2];b+='M'+q[0]+',242 V'+top+' h'+q[1]+' V242 Z ';
        if(q[3]==='s')b+='M'+q[0]+','+top+' L'+(q[0]+q[1]/2)+','+(top-26)+' L'+(q[0]+q[1])+','+top+' Z ';
        if(q[3]==='t')b+='M'+(q[0]+4)+','+top+' V'+(top-14)+' h'+(q[1]-8)+' V'+top+' Z M'+(q[0]+q[1]/2-3)+','+(top-14)+' L'+(q[0]+q[1]/2)+','+(top-30)+' L'+(q[0]+q[1]/2+3)+','+(top-14)+' Z ';
        if(q[3]==='d')b+='M'+q[0]+','+top+' A'+q[1]/2+','+q[1]/2+' 0 0 1 '+(q[0]+q[1])+','+top+' Z ';
        for(var r=top+8;r<236;r+=10)for(var c=q[0]+4;c<q[0]+q[1]-4;c+=7)if(R()<.22)w+='<rect x="'+c+'" y="'+r+'" width="2.6" height="3.6" opacity="'+(.35+R()*.55).toFixed(2)+'"/>';});
      return far2+'<path d="'+b+'" fill="#080c18"/><g fill="#f0c070">'+w+'</g><circle cx="'+(786+9)+'" cy="152" r="3" fill="#ffe6a8" opacity=".8"/>';}
    // the harbour: Saltreach's roofs and the lighthouse
    return '<path d="M706,242 L706,176 L722,176 L722,166 L740,154 L758,166 L758,182 L776,182 L776,170 L792,160 L808,170 L808,190 L830,190 L830,242 Z" fill="#090d1a"/>'+
      '<g fill="#e8b860" opacity=".55"><rect x="728" y="184" width="3" height="4"/><rect x="784" y="176" width="3" height="4"/><rect x="814" y="198" width="3" height="3"/></g>'+
      '<path d="M928,242 L928,200 L940,200 L944,124 L952,124 L956,200 L954,242 Z" fill="#0a0f1f"/><rect x="940" y="116" width="16" height="10" fill="#0a0f1f"/>'+
      '<circle class="d-beacon" cx="948" cy="120" r="5" fill="#ffe7a8"/><path class="d-beam" d="M948,120 L1040,96 L1040,146 Z" fill="#ffe7a8" opacity=".18"/>'+
      '<circle cx="934" cy="214" r="30" fill="url(#d-warm)" opacity=".26"/><ellipse cx="934" cy="214" rx="2.2" ry="4" fill="#fff3d6" opacity=".75"/>';}
  function shutters(){var s='',x,R=rng(29);
    // Two leaves of boards on the inside, a Z brace each, iron straps; a thread of moonlight where they meet
    [[706,124],[830,124]].forEach(function(l){s+='<rect x="'+l[0]+'" y="42" width="'+l[1]+'" height="200" fill="#000" filter="url(#b-post)" opacity=".95"/>';
      for(x=l[0];x<l[0]+l[1];x+=31)s+='<rect x="'+x+'" y="42" width="2.5" height="200" fill="#050302" opacity=".8"/><rect x="'+(x+2.5)+'" y="42" width="1" height="200" fill="'+LIGHT[O.light].key+'" opacity=".12"/>';
      s+='<path d="M'+(l[0]+8)+',70 H'+(l[0]+l[1]-8)+' L'+(l[0]+8)+',204 H'+(l[0]+l[1]-8)+'" stroke="#2a180c" stroke-width="13" fill="none"/><path d="M'+(l[0]+8)+',76 H'+(l[0]+l[1]-8)+' M'+(l[0]+8)+',210 H'+(l[0]+l[1]-8)+'" stroke="'+LIGHT[O.light].key+'" stroke-opacity=".14" stroke-width="1.2"/>';
      var hx=l[0]<800?l[0]:l[0]+l[1]-46;s+='<rect x="'+hx+'" y="62" width="46" height="7" rx="2" fill="url(#d-iron)"/><rect x="'+hx+'" y="196" width="46" height="7" rx="2" fill="url(#d-iron)"/>';});
    return s+'<rect x="828" y="42" width="4" height="200" fill="#000"/><rect x="829.4" y="60" width="1.2" height="170" fill="#c8d6ff" opacity=".5"/>'+
      '<rect x="814" y="132" width="32" height="10" rx="2" fill="url(#d-iron)"/><rect x="826" y="128" width="8" height="18" rx="2" fill="#1a1612"/>';}
  function windowArt(){var R=rng(5),drops='',shut=O.view==='shutters';for(var i=0;i<22;i++){var dx=(710+R()*240).toFixed(0),dy=(48+R()*188).toFixed(0),dr=(1+R()*1.6).toFixed(1);drops+='<circle cx="'+dx+'" cy="'+dy+'" r="'+dr+'" fill="#cfe0ff" opacity=".22"/><circle cx="'+(+dx+.5)+'" cy="'+(dy-.6)+'" r=".55" fill="#fff" opacity=".6"/>';}
    return '<g data-prop="window"><rect x="680" y="20" width="300" height="250" rx="6" fill="#000" opacity=".45" filter="url(#d-blur8)"/>'+
      '<rect x="692" y="30" width="276" height="226" rx="3" fill="url(#d-frame)"/><path d="M967,33 V253" stroke="#ffcf96" stroke-opacity=".22" stroke-width="2"/>'+
      '<g clip-path="url(#d-winclip)">'+(shut?shutters():'<rect x="706" y="42" width="248" height="200" fill="url(#d-sky)"/><g class="d-fallsky">'+
      '<g class="d-stars" fill="#fff"><circle cx="730" cy="60" r="1"/><circle cx="782" cy="78" r="1.2"/><circle cx="944" cy="58" r="1"/><circle cx="846" cy="52" r=".8"/><circle cx="760" cy="104" r=".9"/></g>'+
      '<circle class="d-moon" cx="902" cy="92" r="64" fill="url(#d-moon)"/>'+
      (O.view==='harbour'?'<rect x="706" y="196" width="248" height="46" fill="#0b1226" class="d-sea"/><path d="M880,200 h44 M874,208 h56 M886,216 h34 M878,226 h48" stroke="#f2ecd6" stroke-opacity=".35" stroke-width="2" class="d-glint"/>':'')+'</g>'+
      '<foreignObject x="706" y="42" width="248" height="200"><canvas class="d-skycv" aria-hidden="true" style="display:block;width:100%;height:100%"></canvas></foreignObject>'+
      outside()+
      '<rect x="706" y="160" width="248" height="82" fill="url(#d-fog)"/><g class="d-drops">'+drops+'</g>'+
      '<rect class="d-flash" x="706" y="42" width="248" height="200" fill="#dfe8ff" opacity="0"/>'+
      '<path d="M706,42 L800,42 L706,170 Z" fill="#fff" opacity=".05"/>')+'</g>'+
      '<path d="M706,242 V42 H954" stroke="#000" stroke-opacity=".5" stroke-width="3" fill="none"/><path d="M706,242 H954 V42" stroke="#e0a868" stroke-opacity=".28" stroke-width="2" fill="none"/>'+
      (shut?'':'<rect x="826" y="42" width="8" height="200" fill="#2e1e10"/><rect x="832" y="42" width="2" height="200" fill="#e0a868" opacity=".4"/>'+
      '<rect x="706" y="138" width="248" height="7" fill="#2e1e10"/><rect x="706" y="143" width="248" height="2" fill="#e0a868" opacity=".3"/>')+
      '<rect x="684" y="262" width="292" height="10" fill="#000" opacity=".4" filter="url(#d-blur3)"/>'+
      '<rect x="684" y="250" width="292" height="12" rx="2" fill="#4a3019"/><rect x="684" y="250" width="292" height="3.5" rx="1.5" fill="#8a6038"/>'+(shut?'':'<rect x="706" y="250" width="248" height="2" fill="#b9c8ff" opacity=".3"/>')+'</g>';}
  /* The cat on the sill, seen from behind. Its fur takes the candle on the
     right side; the moon rims its head and back. */
  function catArt(){var c=CAT[O.cat]||CAT.black;
    return '<g data-prop="cat"><ellipse cx="886" cy="251" rx="42" ry="3" fill="#000" opacity=".55"/><g class="d-cat"><path class="d-cat-tail" d="M914,248 C924,262 922,280 912,290" stroke="'+c[1]+'" stroke-width="7" stroke-linecap="round" fill="none"/>'+
      (O.cat!=='black'?'<path class="d-cat-tail" d="M916,250 C925,263 923,279 914,288" stroke="'+c[2]+'" stroke-opacity=".5" stroke-width="2" stroke-linecap="round" fill="none"/>':'')+
      '<g class="d-cat-body"><path d="M848,250 C844,232 860,218 884,218 C908,218 924,230 924,250 Z" fill="url(#d-catf)"/><path d="M852,236 C850,224 856,212 866,212 C876,212 880,222 878,236 Z" fill="url(#d-catf)"/><path d="M855,216 L857,203 L863,212 Z M869,212 L875,202 L877,216 Z" fill="'+c[1]+'"/>'+
      (O.cat==='ginger'?'<path d="M868,222 C878,221 894,222 906,226 M872,230 C884,229 900,231 912,236 M876,238 C890,238 904,240 916,244" stroke="'+c[0]+'" stroke-opacity=".45" stroke-width="3" fill="none" stroke-linecap="round"/>':'')+
      '<path d="M858,214 C862,210 870,210 874,213 M862,219 C874,216 900,216 918,232" stroke="#a9bcff" stroke-opacity="'+(O.view==='shutters'?.12:.45)+'" stroke-width="1.5" fill="none"/>'+
      '</g></g></g>';}
  function background(){
    var s='<g data-bake="wall">'+wall()+'</g>'+
      '<circle class="d-wallglow" cx="'+FX+'" cy="'+FY+'" r="420" fill="url(#d-glow)" opacity=".42"/>';
    s+='<g class="d-far">';
    s+=windowArt()+catArt();
    s+='<g class="d-dofg" filter="url(#d-dof)"><g data-prop="herbs">'+herb(110,30,'#5f6b3a')+'</g><g data-prop="herbs">'+herb(178,10,'#7d6a8f')+'</g><g data-prop="herbs">'+herb(612,24,'#6f7a44')+'</g>';
    // Shelf of jars, with a jar of glowing caps for a little living light.
    // Lit from below, the shelf throws its own shadow up the wall.
    var ss=wsh(334,192),bs=wsh(276,168);
    s+='<g data-prop="shelf"><rect x="'+(49+ss[0]).toFixed(1)+'" y="'+(164.6+ss[1]*1.5).toFixed(1)+'" width="566" height="26" fill="#000" opacity=".26" filter="url(#d-blur8)"/>'+
      '<path d="M84,198 L84,226 L108,198 Z M572,198 L572,226 L548,198 Z" fill="#2e1d0f"/><path d="M84,198 L84,226 L88,222 L88,198 Z M572,198 L572,226 L576,221 L576,198 Z" fill="#8a5a30" opacity=".6"/>'+
      jar(78,34,48,'#c98a2e')+jar(118,24,72,'#3f7a4a','#4a3520')+jar(160,40,40,'#7a4a9a')+
      '<g>'+spine(196,136,14,50,'#5a2a2a')+spine(211,128,12,58,'#2f4a5a')+spine(224,140,16,46,'#5a4a2a')+
      '<rect x="'+(244+bs[0]).toFixed(1)+'" y="'+(150+bs[1]).toFixed(1)+'" width="72" height="44" fill="#000" opacity=".3" filter="url(#d-blur3)"/>'+
      '<rect x="244" y="172" width="64" height="14" fill="#4a2f2f"/><rect x="248" y="160" width="58" height="12" fill="#2f3a4a"/><rect x="246" y="150" width="60" height="10" fill="#5a3a22"/>'+
      '<rect x="302" y="172" width="6" height="14" fill="#d8c8a0" opacity=".7"/><rect x="300" y="160" width="6" height="12" fill="#d8c8a0" opacity=".7"/><rect x="300" y="150" width="6" height="10" fill="#d8c8a0" opacity=".7"/>'+
      '<path d="M244,172 H308 M248,160 H306 M246,150 H306" stroke="#ffd9a0" stroke-opacity=".18"/></g>'+
      jar(340,36,58,'#b0b86a')+jar(384,22,44,'#a83a3a','#2a1a10')+jar(412,22,52,'#3a6aa8','#2a1a10')+
      jar(470,46,64,'#3a8fb0','#4a3520',true)+
      '<g class="d-caps"><circle cx="460" cy="168" r="6" fill="#9ff0ff"/><circle cx="474" cy="160" r="7" fill="#c8f8ff"/><circle cx="483" cy="172" r="5" fill="#9ff0ff"/></g>'+
      jar(530,30,40,'#d9c27a')+'<path d="M566,186 L590,186 L582,166 L590,146 L566,146 L574,166 Z" fill="#c9a96a" opacity=".5"/><path d="M566,186 L590,186 L582,166 L590,146 L566,146 L574,166 Z" fill="url(#d-glassh)"/><rect x="562" y="142" width="32" height="5" fill="#5a4119"/><rect x="562" y="185" width="32" height="5" fill="#5a4119"/>'+
      '<rect x="50" y="186" width="568" height="12" fill="url(#d-woodh)"/><rect x="50" y="186" width="568" height="1.5" fill="#000" opacity=".5"/><rect x="50" y="198" width="568" height="3" fill="#6a4426"/><rect x="50" y="198" width="568" height="3" fill="url(#d-under)"/></g></g></g>';
    // The bench, and the light that pools on it
    s+='<g data-bake="desk">'+desk()+'</g>'+
      '<circle class="d-deskglow" cx="'+CBX+'" cy="'+CBY+'" r="460" fill="url(#d-glow)" opacity=".28"/>'+
      (O.view==='shutters'?'':'<path class="d-moonshaft" d="M712,252 L952,252 L800,600 L430,600 Z" fill="url(#d-shaft)" filter="url(#d-blur14)"/>')+
      '<circle class="d-bglow" cx="790" cy="560" r="170" fill="url(#d-bglow)" opacity=".5"/>';
    s+=pieces()+kitArt();
    return s;
  }
  /* A hooded iron lantern: the flame burns low inside a glass chimney, so the
     glass itself glows and its light sits close to the bench. */
  function lantern(){var k=LIGHT.lantern.key;
    return '<g transform="translate(1092,452)"><ellipse cx="0" cy="2" rx="30" ry="8" fill="#0e0c0a"/><path d="M-28,-6 L-28,0 Q0,8 28,0 L28,-6 Z" fill="url(#d-iron)"/><ellipse cx="0" cy="-6" rx="28" ry="7" fill="url(#d-iron)"/>'+
      '<path d="M6,-12.5 A28,7 0 0 1 27,-8" stroke="'+k+'" stroke-opacity=".55" stroke-width="1.2" fill="none"/>'+
      '<rect x="-19" y="-76" width="38" height="68" rx="3" fill="url(#d-lglass)"/>'+
      '<rect x="-6" y="-40" width="12" height="8" rx="1.5" fill="url(#d-brass)"/>'+
      '<g class="d-cflame"><path d="M0,-70 C7,-58 8,-50 0,-41 C-8,-50 -7,-58 0,-70 Z" fill="url(#d-flame)"/><path d="M0,-60 C3,-55 3,-50 0,-45 C-3,-50 -3,-55 0,-60 Z" fill="#fffef5"/></g>'+
      '<path d="M-12,-76 V-8 M0,-76 V-8 M12,-76 V-8" stroke="#1a1612" stroke-width="2.2"/><path d="M-19,-42 Q0,-37 19,-42" stroke="#1a1612" stroke-width="2.4" fill="none"/>'+
      '<path d="M14,-72 V-12" stroke="#fff6e0" stroke-opacity=".5" stroke-width="1.4"/>'+
      '<rect x="-23" y="-80" width="4.5" height="74" fill="url(#d-iron)"/><rect x="18.5" y="-80" width="4.5" height="74" fill="url(#d-iron)"/><rect x="21" y="-78" width="1.2" height="70" fill="'+k+'" opacity=".45"/>'+
      '<path d="M-26,-78 L-13,-97 L13,-97 L26,-78 Z" fill="url(#d-iron)"/><ellipse cx="0" cy="-78" rx="26" ry="6" fill="#1a1714"/><path d="M-23,-79 Q0,-73 23,-79" stroke="'+k+'" stroke-opacity=".6" stroke-width="2" fill="none"/>'+
      '<g fill="#050404"><circle cx="-8" cy="-88" r="1.6"/><circle cx="0" cy="-89" r="1.6"/><circle cx="8" cy="-88" r="1.6"/></g>'+
      '<rect x="-13" y="-103" width="26" height="6" rx="1.5" fill="#1a1612"/><path d="M8,-96 L22,-79" stroke="#8a8070" stroke-opacity=".5" stroke-width="1.2"/>'+
      '<ellipse cx="0" cy="-114" rx="9" ry="11" fill="none" stroke="#24201c" stroke-width="3.2"/><path d="M6,-122 A9,11 0 0 1 8,-108" stroke="#8a8070" stroke-opacity=".6" stroke-width="1.2" fill="none"/></g>';}
  /* A witchlight: a cold violet orb that hangs over a stone dish carved with
     marks, bobbing a little. */
  function witchlight(){
    return '<g transform="translate(1092,452)"><path d="M-26,-10 C-26,2 -14,4 0,4 C14,4 26,2 26,-10 Z" fill="url(#d-stone)"/><path d="M-26,-10 C-26,2 -14,4 0,4 C14,4 26,2 26,-10 Z" fill="#000" filter="url(#d-speck)" opacity=".4"/>'+
      '<ellipse cx="0" cy="-10" rx="26" ry="6" fill="url(#d-stone)"/><ellipse cx="0" cy="-9.5" rx="21" ry="4.4" fill="#141018"/><ellipse cx="0" cy="-9.5" rx="16" ry="3" fill="#b8a0ff" opacity=".55"/>'+
      '<path d="M-24,-12 A26,6 0 0 1 24,-12" stroke="#d8ccff" stroke-opacity=".55" stroke-width="1.2" fill="none"/>'+
      '<path d="M-15,-3 l3,-4 l3,4 M-3,-1 v-5 l4,2 M8,-2 l2,-5 l3,3" stroke="#c8b8ff" stroke-opacity=".6" stroke-width="1" fill="none"/>'+
      '<ellipse class="d-wisp" cx="0" cy="-60" rx="10" ry="48" fill="#b8a0ff" opacity=".08" filter="url(#d-blur8)"/>'+
      '<g class="d-orbf"><circle cx="0" cy="-112" r="44" fill="url(#d-orb)" opacity=".42"/><circle cx="0" cy="-112" r="17" fill="url(#d-orb)"/><circle cx="0" cy="-112" r="8" fill="#f4f0ff"/><circle cx="-2" cy="-114" r="3.4" fill="#fff"/></g></g>';}
  /* Pieces from the shared kit, drawn only once a Director adds them. Each
     stands where the kit puts it and can be moved like any other piece. */
  function kitArt(){var s='';
    O.kit.forEach(function(k){var p=KIT[k];if(!p)return;var x=p[0],y=p[1],t='<g transform="translate('+x+','+y+')">';
      if(k==='skull')s+='<g data-prop="kit-skull"><g data-bake="sh-skull">'+cast(x,y,40,70,.8)+ao(x,y,24,5)+'</g>'+t+
        '<path d="M-20,-14 C-24,-34 -12,-46 2,-46 C18,-46 26,-34 22,-16 C21,-10 18,-6 16,-2 L-14,-2 C-17,-6 -19,-9 -20,-14 Z" fill="url(#d-bone)"/>'+
        '<path d="M-12,-4 L-12,2 Q0,6 12,2 L12,-4 Z" fill="url(#d-bone)"/><path d="M-8,-3 v4 M-4,-3 v5 M0,-3 v5 M4,-3 v5 M8,-3 v4" stroke="#3a3024" stroke-width="1"/>'+
        '<ellipse cx="-8" cy="-20" rx="6" ry="6.5" fill="#1a140e"/><ellipse cx="9" cy="-20" rx="6.5" ry="6.5" fill="#1a140e"/><path d="M0,-14 L-3,-8 L3,-8 Z" fill="#241a12"/>'+
        '<path d="M13,-40 Q23,-31 20,-17" stroke="#fff8e6" stroke-opacity=".6" stroke-width="2" fill="none" stroke-linecap="round"/><path d="M13,-14 Q16,-20 15,-25" stroke="#fff8e6" stroke-opacity=".4" stroke-width="1.4" fill="none"/>'+
        '<path d="M-6,-44 l3,6 l-2,5" stroke="#5a4a36" stroke-width=".8" fill="none"/></g></g>';
      if(k==='ball')s+='<g data-prop="kit-ball"><g data-bake="sh-ball">'+cast(x,y,34,44,.6)+ao(x,y,20,5)+'</g>'+t+
        '<ellipse cx="'+(-18)+'" cy="2" rx="9" ry="2.6" fill="#ffe2b0" opacity=".3"/>'+
        '<path d="M-16,0 L-12,-10 L12,-10 L16,0 Z" fill="#2a1a0e"/><ellipse cx="0" cy="0" rx="16" ry="4" fill="#1a0f08"/><ellipse cx="0" cy="-10" rx="12" ry="3" fill="url(#d-brass)"/>'+
        '<circle cx="0" cy="-30" r="20" fill="#1a2440" fill-opacity=".55"/><path class="d-swirl" d="M-11,-27 C-5,-41 7,-21 12,-35" stroke="#b8d0ff" stroke-opacity=".4" stroke-width="3" fill="none" stroke-linecap="round"/>'+
        '<circle cx="0" cy="-30" r="20" fill="url(#d-glassball)"/><circle cx="9" cy="-37" r="2.4" fill="#fff4d8"/><path d="M-14,-38 A17,17 0 0 1 -4,-46" stroke="#cfe0ff" stroke-opacity=".3" stroke-width="1.5" fill="none"/></g></g>';
      if(k==='scales')s+='<g data-prop="kit-scales"><g data-bake="sh-scales">'+cast(x,y,40,120,.7)+ao(x,y,30,6)+'</g>'+t+
        '<ellipse cx="0" cy="0" rx="26" ry="7" fill="#3a2a10"/><ellipse cx="0" cy="-3" rx="24" ry="6" fill="url(#d-brass)"/><rect x="-3" y="-100" width="6" height="97" fill="url(#d-brass)"/><circle cx="0" cy="-104" r="4" fill="url(#d-brass)"/>'+
        '<path d="M-46,-94 L46,-98" stroke="#3a2a10" stroke-width="4.5" stroke-linecap="round"/><path d="M-46,-94 L46,-98" stroke="url(#d-brassv)" stroke-width="3" stroke-linecap="round"/>'+
        '<path d="M-44,-94 L-57,-48 M-44,-94 L-31,-48 M44,-98 L31,-52 M44,-98 L57,-52" stroke="#a8843c" stroke-width="1" stroke-dasharray="2 1.4"/>'+
        '<path d="M-60,-48 Q-44,-36 -28,-48 Z" fill="url(#d-brass)"/><ellipse cx="-44" cy="-48" rx="16" ry="3.4" fill="#5a4418"/><ellipse cx="-46" cy="-49" rx="5" ry="1.6" fill="#e8c060"/>'+
        '<path d="M28,-52 Q44,-40 60,-52 Z" fill="url(#d-brass)"/><ellipse cx="44" cy="-52" rx="16" ry="3.4" fill="#5a4418"/>'+
        '<path d="M1.5,-98 V-6" stroke="#fff0c8" stroke-opacity=".6" stroke-width="1"/></g></g>';
      if(k==='mandrake')s+='<g data-prop="kit-mandrake"><g data-bake="sh-mand">'+cast(x,y-2,36,50,.6)+'</g>'+t+
        [[-14,-56,-38],[11,-60,32],[0,-68,4],[-22,-46,-64],[20,-48,62]].map(function(l){return '<ellipse cx="'+l[0]+'" cy="'+l[1]+'" rx="6.5" ry="16" transform="rotate('+l[2]+' '+l[0]+' '+l[1]+')" fill="#3a5a2a"/><path d="M0,-38 L'+l[0]+','+l[1]+'" stroke="#6a8a3a" stroke-width="1.4"/>';}).join('')+
        '<path d="M8,-64 Q14,-58 12,-48" stroke="#a8c870" stroke-opacity=".45" stroke-width="1.2" fill="none"/>'+
        '<path d="M-17,-30 L17,-30 L13,0 L-13,0 Z" fill="url(#d-pot)"/><rect x="-20" y="-37" width="40" height="8" rx="2" fill="url(#d-pot)"/><path d="M-20,-29 h40" stroke="#000" stroke-opacity=".3"/>'+
        '<ellipse cx="0" cy="-36" rx="17" ry="3" fill="#2a1a10"/><path d="M-6,-36 Q0,-46 6,-36 Z" fill="#8a6a48"/><circle cx="-2" cy="-39.5" r=".9" fill="#1a1008"/><circle cx="2.4" cy="-39.5" r=".9" fill="#1a1008"/></g></g>';});
    return s;}
  /* Pieces on the bench. Each keeps its own shadow so the bench editor can
     move them; shadows are baked as images like the wall and desk. */
  var PL='M78,388 L261,397 L254,646 L62,642 Z',PR='M266,397 L447,388 L444,642 L258,646 Z';
  function pieces(){var R=rng(23),s='',i;
    function fox(a,b){var o='';for(var i=0;i<6;i++)o+='<ellipse cx="'+(a+R()*(b-a)).toFixed(0)+'" cy="'+(390+R()*250).toFixed(0)+'" rx="'+(2+R()*6).toFixed(1)+'" ry="'+(2+R()*5).toFixed(1)+'" fill="#8a6430" opacity="'+(.05+R()*.08).toFixed(2)+'"/>';return o;}
    // Book: leather boards, a thick block of pages, the gutter in shadow
    var C=COVER[O.cover]||COVER.oxblood;
    s+='<g data-prop="book"><g data-bake="book">'+ao(250,656,240,16)+'<path d="M48,656 L66,378 L50,378 L28,660 Z" fill="#000" opacity=".35" filter="url(#d-blur5)"/>'+
      '<g filter="url(#b-booklit)"><path d="M66,378 L462,378 L460,656 L48,656 Z" fill="'+C[0]+'"/><path d="M66,378 L462,378 L460,656 L48,656 Z" fill="#000" filter="url(#b-leather)" opacity=".5"/>'+
      '<path d="M66,378 L462,378 L460,656 L48,656 Z" fill="none" stroke="'+C[2]+'" stroke-opacity=".4" stroke-width="2.5"/><path d="M70,382 L456,382 L454,650 L54,650 Z" fill="'+C[1]+'"/>'+
      '<path d="M62,642 L256,648 L444,642 L448,650 L256,656 L58,650 Z" fill="#c9b58c"/><path d="M61,645 L256,651 L446,645 M60,648 L256,654 L447,648" stroke="#9a8560" stroke-opacity=".7" stroke-width=".8" fill="none"/>'+
      '<path d="'+PL+'" fill="url(#b-pagel)"/><path d="'+PR+'" fill="url(#b-pager)"/>'+
      '<clipPath id="d-pgl"><path d="'+PL+'"/></clipPath><clipPath id="d-pgr"><path d="'+PR+'"/></clipPath>'+
      '<path d="'+PL+'" fill="#000" filter="url(#b-paper)" opacity=".22"/><path d="'+PR+'" fill="#000" filter="url(#b-paper)" opacity=".22"/>'+
      '<g clip-path="url(#d-pgl)">'+fox(60,210)+'<path d="M213,394 L261,397 L254,646 L206,645 Z" fill="url(#b-gutL)"/></g>'+
      '<g clip-path="url(#d-pgr)">'+fox(270,440)+'<path d="M266,397 L316,393 L310,645 L258,646 Z" fill="url(#b-gutR)"/></g>'+
      '<path d="M261,397 L266,397 L258,646 L254,646 Z" fill="#000" opacity=".3"/><path d="M78,388 Q170,391 261,397 M266,397 Q356,391 447,388" stroke="#fff6e0" stroke-opacity=".5" stroke-width="1.2" fill="none"/></g>'+
      '<path d="M428,456 Q440,524 434,604" stroke="'+C[3]+'" stroke-width="5" fill="none"/><path d="M430,456 Q442,524 436,604" stroke="'+C[4]+'" stroke-opacity=".35" stroke-width="1.2" fill="none"/></g></g>';
    // Ink and quill
    s+='<g data-prop="inkwell"><g data-bake="sh-ink">'+cast(452,414,40,130,.85)+ao(456,414,26,6)+'</g><g transform="translate(456,402)">'+
      '<path d="M-20,12 Q-22,-6 -10,-10 L10,-10 Q22,-6 20,12 Q0,16 -20,12 Z" fill="#120e16"/><path d="M-20,12 Q-22,-6 -10,-10 L10,-10 Q22,-6 20,12 Q0,16 -20,12 Z" fill="url(#d-cyl)" opacity=".8"/>'+
      '<rect x="-8" y="-16" width="16" height="7" rx="2" fill="#1d1820"/><rect x="-8" y="-16" width="16" height="7" rx="2" fill="url(#d-cyl)"/><ellipse cx="0" cy="-16" rx="7.5" ry="2.2" fill="#040306"/>'+
      '<path d="M12,-4 Q16,2 15,9" stroke="#ffe2b8" stroke-opacity=".6" stroke-width="1.6" fill="none" stroke-linecap="round"/><path d="M-15,0 Q-17,-4 -12,-7" stroke="#b8c8ff" stroke-opacity=".25" stroke-width="1.5" fill="none"/>'+
      '<path d="M2,-12 C-10,-60 -40,-100 -70,-118 C-48,-90 -24,-56 2,-12 Z" fill="url(#d-quill)"/>';
    for(i=0;i<12;i++){var k=i/12,qx=2-72*k-6*k*k,qy=-12-106*k+4*k*k;s+='<path d="M'+qx.toFixed(1)+','+qy.toFixed(1)+' l'+(8+6*Math.sin(k*3)).toFixed(1)+','+(-2-k*4).toFixed(1)+'" stroke="#bfb49c" stroke-opacity=".55" stroke-width=".7"/>';}
    s+='<path d="M2,-12 C-12,-50 -36,-88 -66,-114" stroke="#8a7f68" stroke-width="1.2" fill="none"/></g></g>';
    // Hourglass, close to the candle, so its shadow runs long
    s+='<g data-prop="hourglass"><g data-bake="sh-hg">'+cast(944,420,40,170,.9)+ao(948,420,26,6)+'</g><g class="d-hg" transform="translate(948,416)">'+
      '<path d="M-14,-68 Q-14,-44 0,-36 Q14,-44 14,-68 Z M-14,-4 Q-14,-28 0,-36 Q14,-28 14,-4 Z" fill="#cfe4f2" fill-opacity=".1"/>'+
      '<path d="M-10,-58 Q0,-48 10,-58 Q6,-44 0,-38 Q-6,-44 -10,-58 Z" fill="#d9b46c"/><path d="M-12,-6 Q0,-20 12,-6 Z" fill="#d9b46c"/><path d="M-12,-6 Q0,-20 12,-6 Z" fill="url(#d-cyl)" opacity=".6"/>'+
      '<line class="d-sand" x1="0" y1="-38" x2="0" y2="-12" stroke="#e2c07a" stroke-width="1.4"/>'+
      '<path d="M-14,-68 Q-14,-44 0,-36 Q14,-44 14,-68 Z M-14,-4 Q-14,-28 0,-36 Q14,-28 14,-4 Z" fill="url(#d-glassh)" stroke="#e8f2ff" stroke-opacity=".3"/>'+
      '<path d="M9,-64 Q10,-50 4,-42 M9,-8 Q10,-22 4,-30" stroke="#fff4dc" stroke-opacity=".7" stroke-width="1.3" fill="none" stroke-linecap="round"/>'+
      '<rect x="-21" y="-5" width="42" height="7" rx="1.5" fill="#4a3018"/><rect x="-21" y="-5" width="42" height="7" rx="1.5" fill="url(#d-cyl)"/>'+
      '<rect x="-21" y="-75" width="42" height="7" rx="1.5" fill="#4a3018"/><rect x="-21" y="-75" width="42" height="7" rx="1.5" fill="url(#d-cyl)"/><rect x="-21" y="-68" width="42" height="1.5" fill="#000" opacity=".4"/>'+
      '<rect x="-19" y="-70" width="3" height="66" fill="#3a2410"/><rect x="16" y="-70" width="3" height="66" fill="#6a4524"/><rect x="18" y="-70" width="1" height="66" fill="#ffd9a0" opacity=".45"/></g></g>';
    // The room's light at the back right: a candle, a lantern or a witchlight.
    // Wax or glass glows near the flame, and the varnish shows its reflection.
    var lt=O.light,by=452-LIGHT[lt].fy,rk=LIGHT[lt].fy/152;
    s+='<g data-prop="candle"><g data-bake="sh-candle">'+ao(1092,457,lt==='candle'?52:34,11)+(lt==='candle'?cast(1056,459,60,46,.5):'')+'</g>'+
      '<ellipse class="d-refl" cx="1092" cy="'+(452+LIGHT[lt].fy*.47).toFixed(0)+'" rx="'+(30*Math.max(.6,rk)).toFixed(0)+'" ry="'+(60*Math.max(.45,rk)).toFixed(0)+'" fill="url(#d-warm)" opacity=".32"/>'+
      (lt==='lantern'?lantern():lt==='witch'?witchlight():'<g transform="translate(1092,452)"><path d="M-36,0 Q-36,9 0,9 Q36,9 36,0" fill="#3e2c10"/><ellipse cx="0" cy="0" rx="36" ry="10" fill="url(#d-brass)"/><ellipse cx="0" cy=".5" rx="28" ry="7" fill="#5e4418"/>'+
      '<path d="M-22,1 Q-24,-4 -12,-5 Q8,-7 20,-3 Q26,1 14,4 Q-4,6 -22,1 Z" fill="#ece0c2"/><path d="M12,-9 Q30,-7 35.5,-1" stroke="#fff3cf" stroke-opacity=".8" stroke-width="1.5" fill="none"/>'+
      '<path d="M30,-2 q16,-2 16,10 q0,8 -10,6" stroke="url(#d-brass)" stroke-width="4" fill="none"/><path d="M38,-2.5 q8,1 8,9" stroke="#fff0c8" stroke-opacity=".6" stroke-width="1.2" fill="none"/>'+
      '<rect x="-15" y="-124" width="30" height="122" rx="2.5" fill="url(#d-wax)"/><rect x="-15" y="-124" width="30" height="122" rx="2.5" fill="url(#d-waxglow)"/>'+
      '<path d="M-15,-118 q-4,18 1,30 q4,-8 3,-30 Z M11,-121 q8,14 3,44 q-6,-10 -6,-42 Z M-4,-123 q-2,10 1,16 q2,-6 1,-16 Z" fill="#f4ead2"/>'+
      '<path d="M13,-112 q3,9 1,26" stroke="#fffaf0" stroke-opacity=".7" stroke-width="1" fill="none"/>'+
      '<ellipse cx="0" cy="-124" rx="15" ry="4.6" fill="#efe3c6"/><ellipse cx="0" cy="-124" rx="11" ry="3.2" fill="#ffe2a0"/><ellipse cx="1" cy="-124.5" rx="6" ry="1.6" fill="#fff4d0"/>'+
      '<path d="M0,-124 q1.5,-5 -1,-10" stroke="#1a120c" stroke-width="2" fill="none" stroke-linecap="round"/><circle cx="-1" cy="-134" r="1.3" fill="#ff8a3a"/>'+
      '<g class="d-cflame"><path d="M0,-170 C10,-150 11,-138 0,-128 C-11,-138 -10,-150 0,-170 Z" fill="url(#d-flame)"/><path d="M-5,-131 Q0,-137 5,-131 Q2,-127.5 0,-127.5 Q-2,-127.5 -5,-131 Z" fill="#5a86ff" opacity=".55"/>'+
      '<path d="M0,-152 C4,-144 4.5,-138 0,-133 C-4.5,-138 -4,-144 0,-152 Z" fill="#fffef5"/></g></g>')+
      '<circle class="d-bloom2" cx="1092" cy="'+(by+2)+'" r="96" fill="url(#d-bloom)" opacity=".45"/><circle class="d-bloom1" cx="1092" cy="'+(by+4)+'" r="24" fill="url(#d-bloom)" opacity=".8"/></g>';
    // The tray holding the pack's ingredients: an oak tray lined with linen
    s+='<g data-prop="tray"><g data-bake="sh-tray">'+ao(1030,702,210,16,.9)+cast(878,690,36,80,.6)+'</g>'+
      '<path d="M886,596 L1184,596 L1200,700 L866,700 Z" fill="url(#d-tray)"/><path d="M886,596 L1184,596" stroke="#d19a64" stroke-opacity=".55" stroke-width="2"/>'+
      '<path d="M896,604 L1176,604 L1190,700 L878,700 Z" fill="url(#d-linen)"/>'+
      '<path d="M900,620 Q980,640 1060,616 T1186,640 L1188,650 Q1110,628 1062,628 Q980,652 899,632 Z" fill="#fff8e8" opacity=".22"/>'+
      '<path d="M899,633 Q980,653 1062,629 T1188,651 M884,670 Q980,652 1080,674 T1196,664" stroke="#6e5e40" stroke-opacity=".35" stroke-width="3" fill="none" filter="url(#d-blur1)"/>'+
      '<path d="M896,604 L1176,604 L1177,612 L895,612 Z" fill="#000" opacity=".28" filter="url(#d-blur1)"/></g>';
    return s;}
  var art={
    burner:function(L){var f=L.flask,b=L.burner,ry=f.y+f.r*.78,legs='M736,'+ry+' L712,646 M844,'+ry+' L868,646 M790,'+(ry+10)+' L790,616';
      return '<g class="cr-bellows" data-prop="bellows" transform="translate(662,604) scale(-1.15,1.15)"><path d="M-62,6 L-20,2 L-20,10 Z" fill="url(#d-brass)"/>'+
        '<path d="M-22,4 L40,-6 Q62,-4 62,8 Q62,18 40,20 L-22,10 Z" fill="#5e3c22"/><path class="cr-bel-mid" d="M-20,6 L44,0 L44,16 L-20,10 Z" fill="#7a4e30"/><path d="M-6,4 L-4,10 M8,3 L10,11 M22,2 L24,13 M36,1 L38,15" stroke="#3e2414" stroke-width="2"/>'+
        '<path class="cr-bel-top" d="M-22,4 L40,-10 Q64,-10 64,0 Q62,6 40,4 Z" fill="#6e4528" stroke="#2e1a0c"/><path d="M-20,3.5 L40,-9" stroke="#2e1a0c" stroke-opacity=".6" stroke-width="1.2"/><path d="M64,-2 L84,-8" stroke="#3d2614" stroke-width="5" stroke-linecap="round"/></g>'+
        '<ellipse class="d-caustic" cx="730" cy="664" rx="44" ry="9" fill="url(#d-caus2)" opacity="0"/>'+
        '<g class="d-stand"><path d="'+legs+'" stroke="#2e200c" stroke-width="7" stroke-linecap="round"/><path d="'+legs+'" stroke="url(#d-brass)" stroke-width="5" stroke-linecap="round"/>'+
        '<path d="M738,'+(ry+3)+' L715,644 M846,'+(ry+3)+' L870,644" stroke="#fff0c8" stroke-opacity=".4" stroke-width="1.2"/>'+
        '<ellipse cx="790" cy="'+ry+'" rx="58" ry="12" fill="none" stroke="#2e200c" stroke-width="7"/><ellipse cx="790" cy="'+ry+'" rx="58" ry="12" fill="none" stroke="url(#d-brass)" stroke-width="4.5"/>'+
        '<path d="M806,'+(ry+11.5)+' A58,12 0 0 0 847,'+(ry+2)+'" stroke="#fff0c8" stroke-opacity=".7" stroke-width="1.4" fill="none"/></g>'+
        '<ellipse cx="790" cy="640" rx="54" ry="9" fill="url(#d-ao)"/><g class="d-burner" data-prop="burner"><ellipse cx="790" cy="637" rx="45" ry="11" fill="#2e200c"/><ellipse cx="790" cy="634" rx="44" ry="10" fill="url(#d-brass)"/>'+
        '<path d="M748,634 Q744,600 772,592 L808,592 Q836,600 832,634 Z" fill="url(#d-brass)"/><path d="M748,634 Q790,642 832,634" stroke="#1e1508" stroke-opacity=".5" stroke-width="2" fill="none"/>'+
        '<ellipse cx="806" cy="617" rx="5.5" ry="7.5" fill="#5a3a10" stroke="#3a2608" stroke-width="1.2"/><path d="M800.5,616 A5.5,7.5 0 0 0 811.5,616 Z" fill="#d89a3a"/><path d="M809,611.5 q1.6,2.5 .8,5.5" stroke="#fff3d0" stroke-opacity=".75" stroke-width="1" fill="none"/>'+
        '<rect x="776" y="582" width="28" height="12" rx="2" fill="url(#d-brass)"/><rect x="784" y="'+(b.y+4)+'" width="12" height="'+(582-b.y-4)+'" fill="url(#d-brass)"/>'+
        '<path d="M822,626 Q826,608 814,598" stroke="#fff3d0" stroke-opacity=".75" stroke-width="2.4" fill="none" stroke-linecap="round"/><path d="M755,626 Q752,610 764,600" stroke="#b8c8ff" stroke-opacity=".2" stroke-width="2" fill="none"/></g>'+
        '<g class="cr-flame"><circle cx="'+b.x+'" cy="'+(b.y-20)+'" r="34" fill="url(#d-bglow)"/><path d="M'+b.x+','+(b.y-56)+' C'+(b.x+16)+','+(b.y-30)+' '+(b.x+14)+','+(b.y-8)+' '+b.x+','+b.y+' C'+(b.x-14)+','+(b.y-8)+' '+(b.x-16)+','+(b.y-30)+' '+b.x+','+(b.y-56)+' Z" fill="url(#d-bflame)" class="d-bf"/>'+
        '<path d="M'+b.x+','+(b.y-26)+' C'+(b.x+6)+','+(b.y-16)+' '+(b.x+5)+','+(b.y-6)+' '+b.x+','+(b.y-2)+' C'+(b.x-5)+','+(b.y-6)+' '+(b.x-6)+','+(b.y-16)+' '+b.x+','+(b.y-26)+' Z" fill="#eaf6ff" opacity=".85"/></g>';},
    flask:function(L){var f=L.flask,nw=f.neckW,top=f.neck,r=f.r,x=f.x,y=f.y,nb=y-r*.82,
        out='M'+(x-nw)+','+top+' L'+(x-nw)+','+nb+' A'+r+','+r+' 0 1 0 '+(x+nw)+','+nb+' L'+(x+nw)+','+top;
      function arc(rr,a0,a1){a0*=Math.PI/180;a1*=Math.PI/180;return 'M'+(x+rr*Math.cos(a0)).toFixed(1)+','+(y+rr*Math.sin(a0)).toFixed(1)+' A'+rr+','+rr+' 0 0 1 '+(x+rr*Math.cos(a1)).toFixed(1)+','+(y+rr*Math.sin(a1)).toFixed(1);}
      return '<clipPath id="cr-fclip"><circle cx="'+x+'" cy="'+y+'" r="'+(r-3)+'"/><rect x="'+(x-nw+3)+'" y="'+(top+4)+'" width="'+(nw*2-6)+'" height="'+(y-top)+'"/></clipPath>'+
        '<circle cx="'+x+'" cy="'+y+'" r="'+r+'" fill="#0a1118" fill-opacity=".16"/><rect x="'+(x-nw)+'" y="'+top+'" width="'+nw*2+'" height="'+(y-top-r*.8)+'" fill="#0a1118" fill-opacity=".16"/>'+
        '<ellipse class="d-liqglow" cx="'+x+'" cy="'+(y+r*.3)+'" rx="'+(r*1.5)+'" ry="'+(r*.9)+'" fill="url(#d-liqglow)" opacity="0"/>'+
        '<g class="cr-liqslot"></g>'+
        // Thick glass reads dark at the rim, with a bright line where it turns away
        '<circle cx="'+x+'" cy="'+y+'" r="'+r+'" fill="url(#d-glassball)"/><rect x="'+(x-nw)+'" y="'+top+'" width="'+nw*2+'" height="'+(y-top-r*.8)+'" fill="url(#d-glassh)"/>'+
        '<path d="'+out+'" fill="none" stroke="#dfeeff" stroke-opacity=".3" stroke-width="1.6"/>'+
        '<path d="'+arc(r-1.5,-62,58)+'" stroke="#fff1d8" stroke-opacity=".7" stroke-width="2" fill="none" stroke-linecap="round"/>'+
        '<ellipse cx="'+x+'" cy="'+top+'" rx="'+(nw+4)+'" ry="5" fill="none" stroke="#0a1118" stroke-opacity=".5" stroke-width="4"/><ellipse cx="'+x+'" cy="'+top+'" rx="'+(nw+4)+'" ry="5" fill="none" stroke="#eef6ff" stroke-opacity=".75" stroke-width="2"/>'+
        // The window, mirrored small in the glass; the candle as a hot point
        '<g transform="translate('+(x-r*.46).toFixed(1)+','+(y-r*.48).toFixed(1)+') rotate(-38)" opacity=".4"><path d="M-10,-16 Q0,-19 10,-16 L11,13 Q0,16 -11,13 Z" fill="#dce8ff" opacity=".5"/><path d="M0,-18 V15 M-11,-2 Q0,0 11,-2" stroke="#0a1118" stroke-opacity=".45" stroke-width="1.6" fill="none"/></g>'+
        '<circle cx="'+(x+r*.6).toFixed(1)+'" cy="'+(y-r*.3).toFixed(1)+'" r="13" fill="url(#d-warm)" opacity=".55"/><ellipse cx="'+(x+r*.6).toFixed(1)+'" cy="'+(y-r*.3).toFixed(1)+'" rx="3" ry="5.5" fill="#fffaf0"/>'+
        // Light focused through the glass gathers on the far inside wall
        '<path d="'+arc(r*.84,118,196)+'" stroke="#fff4e0" stroke-opacity=".28" stroke-width="3.5" fill="none" stroke-linecap="round"/>'+
        '<rect x="'+(x+nw-6)+'" y="'+(top+8)+'" width="2" height="'+(y-top-r*.95)+'" fill="#fff1d8" opacity=".6"/><rect x="'+(x-nw+4)+'" y="'+(top+10)+'" width="2" height="'+(y-top-r*1.05)+'" fill="#cfe0ff" opacity=".2"/>';},
    mortar:function(r){var body='M'+(-r)+',0 C'+(-r)+','+(r*.9)+' '+(-r*.45)+','+(r*1.05)+' 0,'+(r*1.05)+' C'+(r*.45)+','+(r*1.05)+' '+r+','+(r*.9)+' '+r+',0 Z';
      return '<path d="'+body+'" fill="url(#d-stone)"/><path d="'+body+'" fill="#000" filter="url(#d-speck)" opacity=".5"/>'+
        '<ellipse cx="0" cy="'+(r*1.04)+'" rx="'+(r*.52)+'" ry="'+(r*.12)+'" fill="#26282c"/>'+
        '<path d="M'+(r*.62)+','+(r*.18)+' Q'+(r*.6)+','+(r*.7)+' '+(r*.28)+','+(r*.92)+'" stroke="#fff0d8" stroke-opacity=".35" stroke-width="5" fill="none" stroke-linecap="round"/>'+
        '<path d="M'+(-r*.9)+','+(r*.15)+' Q'+(-r*.82)+','+(r*.7)+' '+(-r*.45)+','+(r*.95)+'" stroke="#a9bcff" stroke-opacity=".14" stroke-width="3" fill="none"/>'+
        '<ellipse cx="0" cy="0" rx="'+r+'" ry="'+(r*.28)+'" fill="url(#d-stone)"/><ellipse cx="0" cy="0" rx="'+r+'" ry="'+(r*.28)+'" fill="none" stroke="#202226" stroke-opacity=".6" stroke-width="1.2"/>'+
        '<ellipse cx="0" cy="2" rx="'+(r*.84)+'" ry="'+(r*.21)+'" fill="url(#d-bowl)"/>'+
        '<ellipse class="cr-paste" cx="0" cy="6" rx="'+(r*.66)+'" ry="'+(r*.14)+'" style="opacity:0"/>'+
        '<path d="M'+(-r*.1)+','+(-r*.278)+' A'+r+','+(r*.28)+' 0 0 1 '+(r*.98)+','+(-r*.05)+'" stroke="#f4ead8" stroke-opacity=".55" stroke-width="1.6" fill="none"/>'+
        '<g fill="#1e2024" opacity=".5"><circle cx="-24" cy="30" r="1.6"/><circle cx="14" cy="44" r="1.2"/><circle cx="34" cy="22" r="1.4"/><circle cx="-6" cy="52" r="1.1"/></g>';},
    pestle:function(){return '<rect x="-7" y="-104" width="14" height="100" rx="7" fill="url(#d-stone)"/><rect x="-7" y="-104" width="14" height="100" rx="7" fill="#000" filter="url(#d-speck)" opacity=".4"/>'+
      '<ellipse cx="0" cy="0" rx="11" ry="13" fill="url(#d-stone)"/><path d="M3,-96 L3,-12" stroke="#fff0d8" stroke-opacity=".45" stroke-width="2" stroke-linecap="round"/>';},
    item:function(k,it){
      if(k==='bloodmoss'){var b=[[-18,8,15,'#6e1322'],[18,8,15,'#7a1626'],[0,-2,19,'#9e2236'],[-8,14,14,'#8a1c2e'],[10,15,13,'#b3263a']],s='<ellipse cx="-12" cy="26" rx="42" ry="7" fill="url(#d-shadow)"/>';
        b.forEach(function(c){s+='<circle cx="'+c[0]+'" cy="'+c[1]+'" r="'+c[2]+'" fill="'+c[3]+'"/><circle cx="'+c[0]+'" cy="'+c[1]+'" r="'+c[2]+'" fill="url(#d-sphere)"/>';});
        return s+'<g fill="#e86a7a"><circle cx="-6" cy="-10" r="2"/><circle cx="6" cy="-6" r="1.6"/><circle cx="-14" cy="2" r="1.5"/><circle cx="14" cy="4" r="1.8"/><circle cx="2" cy="8" r="1.4"/></g>'+
          '<g fill="#fff6ea" opacity=".85"><circle cx="8" cy="-12" r="1.6"/><circle cx="24" cy="2" r="1.2"/><circle cx="16" cy="9" r="1.1"/></g>'+
          '<path d="M-4,-18 q-3,-12 -10,-16 M4,-18 q2,-10 9,-14 M0,-20 v-14" stroke="#4a6a2a" stroke-width="2" fill="none"/><path d="M-26,10 Q0,20 26,10" stroke="#c8a870" stroke-width="3" fill="none"/><path d="M-26,10 Q0,20 26,10" stroke="#000" stroke-opacity=".3" stroke-width="1" fill="none" transform="translate(0,1.5)"/>';}
      if(k==='water')return '<ellipse cx="-10" cy="46" rx="32" ry="6" fill="url(#d-shadow)"/>'+
        '<path d="M-22,4 L22,4 L22,38 Q22,44 16,44 L-16,44 Q-22,44 -22,38 Z" fill="#7fb4dc" fill-opacity=".55"/>'+
        '<path d="M-24,40 Q-24,46 -18,46 L18,46 Q24,46 24,40 L24,-4 Q24,-16 10,-20 L10,-42 L-10,-42 L-10,-20 Q-24,-16 -24,-4 Z" fill="#cfe6ff" fill-opacity=".1"/>'+
        '<path d="M-24,40 Q-24,46 -18,46 L18,46 Q24,46 24,40 L24,-4 Q24,-16 10,-20 L10,-42 L-10,-42 L-10,-20 Q-24,-16 -24,-4 Z" fill="url(#d-glassh)" stroke="#e6f4ff" stroke-opacity=".35" stroke-width="1.4"/>'+
        '<rect x="-9" y="-54" width="18" height="14" rx="3" fill="#a8784a"/><rect x="-9" y="-54" width="18" height="14" rx="3" fill="url(#d-cyl)"/><rect x="-15" y="10" width="30" height="18" rx="2" fill="#e9dcc0"/><rect x="-15" y="10" width="30" height="18" rx="2" fill="url(#d-cyl)" opacity=".5"/><text x="0" y="23" text-anchor="middle" class="d-label">Spring</text>'+
        '<line x1="17" y1="-6" x2="17" y2="38" stroke="#fff4dc" stroke-opacity=".75" stroke-width="2" stroke-linecap="round"/><line x1="-17" y1="-2" x2="-17" y2="36" stroke="#cfe0ff" stroke-opacity=".25" stroke-width="2" stroke-linecap="round"/>';
      if(k==='saltglass')return '<ellipse cx="-10" cy="24" rx="38" ry="7" fill="url(#d-shadow)"/><ellipse cx="0" cy="-4" rx="34" ry="30" fill="#9fe3e0" opacity=".22" filter="url(#d-blur8)"/>'+
        '<path d="M-26,22 L-30,-6 L-20,-16 L-14,22 Z" fill="#3f8482"/><path d="M-20,-16 L-12,-8 L-14,22 Z" fill="#a8e8e6"/>'+
        '<path d="M-10,24 L-8,-30 L4,-40 L12,-28 L10,24 Z" fill="#6cc0bd"/><path d="M4,-40 L12,-28 L10,24 L2,24 Z" fill="#c8f6f4"/><path d="M-8,-30 L4,-40 L2,24 L-10,24 Z" fill="#2f7472" opacity=".6"/>'+
        '<path d="M12,24 L18,-6 L28,-12 L30,24 Z" fill="#5ab0ad"/><path d="M28,-12 L30,24 L24,24 Z" fill="#d8fbfa"/>'+
        '<path d="M4,-40 L12,-28 M28,-12 L30,24 M-20,-16 L-12,-8" stroke="#fff" stroke-opacity=".8" stroke-width="1"/>';
      if(k==='bile')return '<ellipse cx="-8" cy="30" rx="28" ry="6" fill="url(#d-shadow)"/><ellipse cx="0" cy="0" rx="26" ry="34" fill="#5fbf3a" opacity=".22" filter="url(#d-blur8)"/>'+
        '<rect x="-12" y="-28" width="24" height="56" rx="9" fill="#3f8a26" fill-opacity=".85"/><rect x="-12" y="-28" width="24" height="56" rx="9" fill="url(#d-glassh)" stroke="#d8f5c8" stroke-opacity=".35" stroke-width="1.4"/>'+
        '<path d="M-8,0 q8,-8 16,0 q-8,10 -16,0" fill="#1f4a12" opacity=".7"/><rect x="-10" y="-40" width="20" height="13" rx="3" fill="#8a1a1a"/><rect x="-10" y="-40" width="20" height="13" rx="3" fill="url(#d-cyl)"/><path d="M-6,-28 q0,8 3,10 q2,-4 1,-10 Z" fill="#8a1a1a"/>'+
        '<line x1="7" y1="-20" x2="7" y2="20" stroke="#fff4dc" stroke-opacity=".7" stroke-width="2" stroke-linecap="round"/>';
      // Any other ingredient: a corked vial of its own colour, until the shared kit draws it
      var vc=(it&&it.color)||'#8a8a8a';
      return '<ellipse cx="-8" cy="32" rx="28" ry="6" fill="url(#d-shadow)"/><ellipse cx="0" cy="4" rx="24" ry="30" fill="'+vc+'" opacity=".2" filter="url(#d-blur8)"/>'+
        '<path d="M-14,-18 L14,-18 L16,26 Q16,32 10,32 L-10,32 Q-16,32 -16,26 Z" fill="'+vc+'" fill-opacity=".8"/>'+
        '<path d="M-14,-18 L14,-18 L16,26 Q16,32 10,32 L-10,32 Q-16,32 -16,26 Z" fill="url(#d-glassh)" stroke="#e6f4ff" stroke-opacity=".35" stroke-width="1.4"/>'+
        '<rect x="-9" y="-31" width="18" height="14" rx="3" fill="#a8784a"/><rect x="-9" y="-31" width="18" height="14" rx="3" fill="url(#d-cyl)"/>'+
        '<line x1="9" y1="-12" x2="10" y2="24" stroke="#fff4dc" stroke-opacity=".7" stroke-width="2" stroke-linecap="round"/>';},
    bottle:function(col,name){return '<ellipse cx="-8" cy="0" rx="38" ry="7" fill="url(#d-shadow)"/><circle cx="0" cy="-34" r="50" fill="'+col+'" opacity=".35" filter="url(#d-blur8)"/>'+
      '<circle cx="0" cy="-30" r="27" fill="'+col+'" fill-opacity=".9"/><circle cx="0" cy="-30" r="27" fill="url(#d-sphere)" stroke="#fff" stroke-opacity=".35" stroke-width="1.4"/>'+
      '<rect x="-8" y="-70" width="16" height="16" fill="'+col+'" fill-opacity=".6"/><rect x="-8" y="-70" width="16" height="16" fill="url(#d-glassh)" stroke="#fff" stroke-opacity=".35" stroke-width="1.4"/>'+
      '<rect x="-9" y="-80" width="18" height="13" rx="3" fill="#a8784a"/><rect x="-9" y="-80" width="18" height="13" rx="3" fill="url(#d-cyl)"/><path d="M6,-52 A20,20 0 0 1 19,-38" stroke="#fff6e4" stroke-opacity=".8" stroke-width="3" fill="none" stroke-linecap="round"/>'+
      '<rect x="-17" y="-28" width="34" height="14" rx="2" fill="#efe3c4" transform="rotate(-4 0 -21)"/><text x="0" y="-18" text-anchor="middle" class="d-label" transform="rotate(-4 0 -21)">'+esc(String(name).split(' ')[0])+'</text>';},
    gaugeFrame:function(){return '<path d="M-84,10 A84,84 0 0 1 84,10 Z" fill="#1d140c" fill-opacity=".82" stroke="url(#d-brass)" stroke-width="4"/><g fill="#e4c27a"><circle cx="-74" cy="4" r="3"/><circle cx="74" cy="4" r="3"/></g>';},
    barFrame:function(w){return '<rect x="-18" y="-30" width="'+(w+36)+'" height="76" rx="10" fill="#1d140c" fill-opacity=".82" stroke="url(#d-brass)" stroke-width="4"/>';}
  };
  function esc(t){return CraftScene.esc(t);}
  var TICK='<path d="M-5,0 L-1,5 L7,-6" class="bk-tick"/>',CROSS='<path d="M-5,-5 L5,5 M5,-5 L-5,5" class="bk-cross"/>';
  function book(c){var r=c.r,S=c.S,s='<g transform="'+BOOK.l+'"><text class="bk-h" x="14" y="36">'+esc(r.name)+'</text>'+
      '<text class="bk-k" x="14" y="56">'+esc([r.kind,r.who].filter(Boolean).join(' · '))+'</text><path d="M14,66 Q100,62 186,66" class="bk-rule"/>';
    var gi=0;c.steps.forEach(function(st,i){var y=92+i*32,done=i<c.i||c.phase!=='step',g=null;if(st.graded){g=done?c.res[gi]:null;gi++;}
      s+='<g transform="translate(24,'+(y-4)+')">'+(done?(g&&g.grade==='miss'?CROSS:TICK):(i===c.i?'<circle r="5" class="bk-now"/>':'<circle r="3.5" class="bk-dot"/>'))+'</g>'+
        '<text class="bk-s'+(i===c.i&&c.phase==='step'?' is-now':'')+(done?' is-done':'')+'" x="40" y="'+y+'">'+esc(c.name(st))+'</text>'+
        (g?'<text class="bk-g is-'+g.grade+'" x="40" y="'+(y+13)+'">'+(g.grade==='perfect'?'perfect':g.grade==='good'?'good':'a slip')+'</text>':'');});
    s+='</g><g transform="'+BOOK.r+'"><text class="bk-h2" x="14" y="36">The project</text>'+
      '<text class="bk-t" x="14" y="60">Goal '+r.goal+' points</text><text class="bk-t" x="14" y="78">Roll 2d10 + '+esc(r.stat)+'</text>';
    var k=S.points/r.goal;s+='<rect x="14" y="94" width="172" height="12" class="bk-bar"/><rect x="15" y="95" width="'+(170*k).toFixed(1)+'" height="10" class="bk-fill"/>'+
      '<text class="bk-t" x="14" y="124">'+S.points+' of '+r.goal+'</text>';
    var v=c.verdict;s+='<text class="bk-t" x="14" y="154">Your work: <tspan class="bk-v is-'+v.k+'">'+esc(v.label)+'</tspan></text>';
    if(S.roll)s+='<text class="bk-t" x="14" y="180">Rolled '+S.roll.a+' + '+S.roll.b+(r.mod?' + '+r.mod:'')+(v.v?(v.v>0?' + 2':' − 2'):'')+' = '+S.roll.total+'</text>';
    if(c.phase==='done'&&S.points>=r.goal)s+='<g transform="translate(100,222) rotate(-8)"><rect x="-62" y="-20" width="124" height="34" rx="4" class="bk-stamp"/><text class="bk-stampt" x="0" y="4" text-anchor="middle">Finished</text></g>';
    else s+='<text class="bk-n" x="14" y="236">Edge +2, bane −2.</text>';
    return s+'</g>';}

  var fx={
    dust:function(P,x,y,col){P.emit({type:'blob',x:x,y:y,vx:CraftScene.rnd(-40,40),vy:CraftScene.rnd(-60,-10),ay:260,life:CraftScene.rnd(.4,.8),size:CraftScene.rnd(1.2,2.6),color:CraftScene.rgb(col),alpha:.9});},
    drop:function(P,x,y,col,spill){P.emit({type:'blob',x:x,y:y,vx:spill?CraftScene.rnd(-90,90):CraftScene.rnd(-10,10),vy:spill?CraftScene.rnd(-60,20):CraftScene.rnd(30,80),ay:900,life:spill?.7:.35,size:CraftScene.rnd(2,4),color:CraftScene.rgb(col),alpha:.85});},
    fizz:function(P,x,y,col){for(var i=0;i<40;i++)P.emit({type:i%3?'bubble':'glow',x:x+CraftScene.rnd(-30,30),y:y+CraftScene.rnd(-5,20),vx:CraftScene.rnd(-20,20),vy:CraftScene.rnd(-160,-40),life:CraftScene.rnd(.5,1.1),size:CraftScene.rnd(2,6),color:CraftScene.rgb(col),alpha:.9});},
    spark:function(P,x,y,g){var n=g==='perfect'?40:g==='good'?18:0,c=g==='perfect'?'255,214,120':'255,240,210';
      for(var i=0;i<n;i++){var a=CraftScene.rnd(0,6.283),sp=CraftScene.rnd(120,420);P.emit({type:'streak',x:x,y:y,vx:Math.cos(a)*sp,vy:Math.sin(a)*sp-80,ay:500,drag:.03,life:CraftScene.rnd(.35,.8),size:CraftScene.rnd(1.2,2.4),color:c,len:.04});}
      if(g!=='miss'){P.emit({type:'glow',x:x,y:y,life:.45,size:g==='perfect'?160:90,grow:120,color:c,alpha:.7});P.emit({type:'ring',x:x,y:y,life:.5,size:20,grow:300,color:c,alpha:.8,add:true,lw:3});}},
    smoke:function(P,x,y){for(var i=0;i<16;i++)P.emit({type:'soft',x:x+CraftScene.rnd(-20,20),y:y+CraftScene.rnd(-10,10),vx:CraftScene.rnd(-30,30),vy:CraftScene.rnd(-90,-30),life:CraftScene.rnd(1,1.8),size:CraftScene.rnd(14,26),grow:30,color:'40,34,30',alpha:.55,fade:'out'});},
    steam:function(P,x,y,k){P.emit({type:'soft',x:x+CraftScene.rnd(-6,6),y:y,vx:CraftScene.rnd(-14,14),vy:CraftScene.rnd(-70,-40),life:CraftScene.rnd(1.6,2.6),size:CraftScene.rnd(8,14),grow:22,color:'235,235,240',alpha:.32*(k||1),fade:'inout'});},
    success:function(P,x,y,col){var c=CraftScene.rgb(col);P.emit({type:'glow',x:x,y:y,life:1.2,size:120,grow:200,color:c,alpha:.9});P.emit({type:'ring',x:x,y:y,life:.9,size:10,grow:420,color:'255,230,170',alpha:.9,add:true,lw:3});
      for(var i=0;i<70;i++){var a=CraftScene.rnd(0,6.283),sp=CraftScene.rnd(40,260);P.emit({type:i%2?'glow':'streak',x:x,y:y,vx:Math.cos(a)*sp,vy:Math.sin(a)*sp-60,ay:i%2?-30:300,drag:.02,life:CraftScene.rnd(.8,2),size:i%2?CraftScene.rnd(3,7):1.6,color:i%3?c:'255,230,170',fade:'out'});}},
    bubbles:function(P,F,ly,col,heat,dt){var rate=4+heat*heat*70;if(Math.random()<rate*dt){var bot=F.y+F.r*.85,y=CraftScene.rnd(Math.max(ly+6,bot-30),bot),hw=Math.sqrt(Math.max(0,F.r*F.r-(y-F.y)*(y-F.y)))*.8,sp=CraftScene.rnd(40,90)*(.6+heat);
        P.emit({type:'bubble',x:F.x+CraftScene.rnd(-hw,hw),y:y,vx:CraftScene.rnd(-6,6),vy:-sp,life:Math.max(.1,(y-ly)/sp),size:CraftScene.rnd(1.4,3.6)*(.7+heat*.6),color:'255,255,255',alpha:.75,fade:'in'});}
      if(heat>.4&&Math.random()<heat*4*dt)fx.steam(P,F.x,F.neck-4,heat);}
  };

  /* What each piece says on hover and offers on right-click, before a
     Director rewrites it. The window and the light describe whatever the
     room's settings make them. opts.info lays a bench's own words on top. */
  var INFO={
    window:null,
    cat:{name:'Cat',text:'Asleep on the sill. Sleeps through anything.',acts:[['Pet the cat','pet']]},
    herbs:{name:'Drying herbs',text:'Sage, lavender and nettle, hung to dry.'},
    shelf:{name:'Shelf of jars',text:'Spares and oddments. The blue jar glows on its own.'},
    inkwell:{name:'Ink and quill',text:'For writing down what worked.'},
    hourglass:{name:'Hourglass',text:'One turn is about a minute.',acts:[['Turn it over','turn']]},
    candle:null,
    tray:{name:'Tray',text:'What was brought from the pack.'},
    burner:{name:'Oil burner',text:'Lamp oil and a cotton wick.',acts:[['Turn the flame up','flame']]},
    bellows:{name:'Bellows',text:'Pump them to feed the flame.',acts:[['Give it a pump','pump']]},
    'kit-skull':{name:'Skull',text:'Nobody remembers whose. It watches the door.'},
    'kit-ball':{name:'Crystal ball',text:'Clouds turn inside it when nobody is looking.',acts:[['Gaze into it','gaze']]},
    'kit-scales':{name:'Brass scales',text:'For weighing reagents to the grain.'},
    'kit-mandrake':{name:'Mandrake',text:'Potted and asleep. Do not pull it up.',acts:[['Water it','water']]}
  };
  // The window and the light say what they are; the editor can still rename them
  var VIEWINFO={harbour:{name:'Window',text:'The harbour. The lighthouse turns all night.',acts:[['Watch the storm','storm']]},
    forest:{name:'Window',text:'Pines right up to the wall. Something out there blinks back.',acts:[['Watch the storm','storm']]},
    mountains:{name:'Window',text:'The peaks, white most of the year.',acts:[['Watch the storm','storm']]},
    city:{name:'Window',text:'Roofs and spires. The bell tower keeps the hours.',acts:[['Watch the storm','storm']]},
    shutters:{name:'Shutters',text:'Barred for the night. The wind still finds the gaps.'}};
  var LIGHTINFO={candle:{name:'Candle',text:'Tallow, half burned. The room\u2019s only real light.',acts:[['Trim the wick','trim']]},
    lantern:{name:'Lantern',text:'Hooded iron, with a night\u2019s oil in the font.',acts:[['Turn up the wick','trim']]},
    witch:{name:'Witchlight',text:'A cold violet light. It hums if you lean close.',acts:[['Stir the light','trim']]}};
  function infoNow(){INFO.window=VIEWINFO[O.view]||VIEWINFO.harbour;INFO.candle=LIGHTINFO[O.light]||LIGHTINFO.candle;}
  infoNow();
  Object.keys(opts.info||{}).forEach(function(k){var v=opts.info[k];if(v&&typeof v==='object')INFO[k]=v;});
  // The built-in actions a Director can give a piece again, by the name the editor shows
  var BUILTIN={pet:'Purr and tail',storm:'Lightning',turn:'Flip it over',trim:'Flare the flame',flame:'Flame up',pump:'Pump',gaze:'Clouds swirl',water:'A tiny yawn'};
  var flare=0;
  function lightning(sfx){var w=svgRef&&svgRef.querySelector('.d-flash');if(!w)return;w.style.transition='none';w.style.opacity='.6';setTimeout(function(){w.style.transition='opacity .6s';w.style.opacity='0';},90);if(sfx)setTimeout(function(){sfx('thunder');},400);}
  function act(id,a,api){
    if(a==='pet'){api.sfx('purr');api.el.classList.add('petted');setTimeout(function(){api.el.classList.remove('petted');},1800);api.pop('purr',884,196);}
    if(a==='storm'){lightning(api.sfx);}
    if(a==='turn'){var h=api.el.querySelector('.d-hg');h.classList.remove('flip');void h.getBBox();h.classList.add('flip');}
    if(a==='trim'){flare=1;var lp=lightPt();for(var i=0;i<14;i++)api.P.emit({type:'glow',x:lp[0],y:lp[1]-10,vx:CraftScene.rnd(-30,30),vy:CraftScene.rnd(-90,-30),life:CraftScene.rnd(.4,.9),size:CraftScene.rnd(2,4),color:LIGHT[O.light].c});}
    if(a==='gaze'){var b=api.el.getBBox();for(var j=0;j<16;j++)api.P.emit({type:'glow',x:b.x+b.width/2+CraftScene.rnd(-10,10),y:b.y+14+CraftScene.rnd(-8,8),vx:CraftScene.rnd(-20,20),vy:CraftScene.rnd(-40,-10),life:CraftScene.rnd(.6,1.2),size:CraftScene.rnd(1.6,3),color:'184,208,255'});api.sfx('perfect');}
    if(a==='water'){var c=api.el.getBBox();api.pop('a tiny yawn',c.x+c.width/2,c.y-6);}
    if(a==='flame'){api.heat(.4);api.sfx('pump');}
    if(a==='pump'){api.pump();api.heat(.3);api.sfx('pump');}
  }
  var flashT=6;
  function ambient(P,dt,S,t){
    var shut=O.view==='shutters',lp=lightPt();
    if(O.weather==='rain'&&!sky.on&&!shut){for(var i=0;i<3;i++)if(Math.random()<dt*40)P.emit({type:'line',x:CraftScene.rnd(690,980)-ox*9,y:CraftScene.rnd(20,60),vx:-120,vy:900,life:.3,size:1,color:'200,215,255',alpha:.45,len:.025,clip:[706-ox*9,42-oy*5,248,200]});}
    if(O.weather==='snow'&&!sky.on&&!shut&&Math.random()<dt*14)P.emit({type:'blob',x:CraftScene.rnd(700,980),y:40,vx:CraftScene.rnd(-20,10),vy:CraftScene.rnd(30,60),life:4,size:CraftScene.rnd(1,2.4),color:'240,245,255',alpha:.9,fade:'inout',clip:[706-ox*9,42-oy*5,248,200]});
    if(Math.random()<dt*3)P.emit({type:'glow',x:lp[0]+CraftScene.rnd(-330,90),y:lp[1]+CraftScene.rnd(-160,220),vx:CraftScene.rnd(-6,6),vy:CraftScene.rnd(-8,4),life:CraftScene.rnd(5,9),size:CraftScene.rnd(2,3.6),color:LIGHT[O.light].c,alpha:.45,fade:'inout'});
    // A thread of smoke off the wick or out of the lantern's cap; a witchlight sheds sparks instead
    if(O.light==='witch'){if(Math.random()<dt*4)P.emit({type:'glow',x:lp[0]+CraftScene.rnd(-8,8),y:lp[1]+CraftScene.rnd(-6,6),vx:CraftScene.rnd(-8,8),vy:CraftScene.rnd(-26,-10),life:CraftScene.rnd(.8,1.6),size:CraftScene.rnd(1.2,2.4),color:'200,184,255',alpha:.8,fade:'out'});}
    else if(Math.random()<dt*2.5)P.emit({type:'soft',x:lp[0]+CraftScene.rnd(-1,1),y:O.light==='lantern'?lp[1]-56:lp[1]-22,vx:CraftScene.rnd(-6,6),vy:-30,life:2.2,size:3,grow:6,color:'200,190,180',alpha:.12,fade:'inout'});
    if(O.weather==='rain'){flashT-=dt;if(flashT<0){flashT=CraftScene.rnd(7,16);lightning();}}
  }
  /* The window shows the campaign's sky, drawn by Chronicle's own sky code
     (the calendar's sky pane) from the calendar as this viewer may see it,
     read the same way the header's Sky background reads it. "From the
     calendar" follows the calendar's hour and the day's weather; Night, Dusk,
     Rain and Snow override them for this bench. With no calendar the drawn
     moon and stars stay. */
  var HOUR={night:[0,40],dusk:[18,40]};
  var WX={rain:{preset_id:'rain',preset_label:'Rain'},snow:{preset_id:'snow',preset_label:'Snow'}};
  var REFRESH_MS=5*60*1000;
  var sky={key:null,last:-1,on:false,src:null,gen:0};
  function loadSky(){var Sc=window.SkyPane&&window.SkyPane.Scene,o=opts.sky||{};if(!Sc||!window.SkyWorld)return;
    if(o.calendar){sky.src={calendar:o.calendar,events:[],weather:o.calendar.weather||null,seed:o.calendar.name||'sky'};sky.gen++;return;}
    if(!o.campaignId||!o.calendarId||sky.loading)return;
    var base='/campaigns/'+encodeURIComponent(o.campaignId)+'/calendars/'+encodeURIComponent(o.calendarId);sky.loading=true;sky.loadedAt=Date.now();
    Sc.getJSON(base).then(function(cal){
      return Promise.all([Sc.getJSON(base+'/events?year='+cal.current_year+'&month='+cal.current_month).catch(function(){return [];}),Sc.dayWeather(o.campaignId,o.calendarId,cal)]).then(function(got){
        if(dead)return;var ev=Array.isArray(got[0])?got[0]:(got[0]&&got[0].data)||[];
        sky.src={calendar:cal,events:ev.filter(function(e){return e.year===cal.current_year&&e.month===cal.current_month&&e.day===cal.current_day;}),weather:got[1],seed:o.calendarId};
        sky.gen++;sky.force=1;});
    }).catch(function(err){if(window.console)console.warn('[craft-desk] the calendar could not be read',err);}).then(function(){sky.loading=false;});}
  function skyFrame(svg,t){var S=window.SkyPane&&window.SkyPane.Scene;if(!S||!window.SkyWorld||!window.Sky2D)return;
    if(!sky.asked){sky.asked=1;loadSky();}
    if(opts.sky&&opts.sky.campaignId&&!document.hidden&&Date.now()-(sky.loadedAt||0)>REFRESH_MS)loadSky();
    if(!sky.src)return;
    var cv=sky.cv||(sky.cv=svg.querySelector('.d-skycv'));if(!cv)return;
    var r=cv.getBoundingClientRect();if(!r.width)return;
    var dpr=Math.min(2,window.devicePixelRatio||1),W=Math.round(r.width),H=Math.round(r.height),key=O.time+'|'+O.weather+'|'+sky.gen;
    var resized=cv.width!==Math.round(W*dpr)||cv.height!==Math.round(H*dpr);if(resized){cv.width=Math.round(W*dpr);cv.height=Math.round(H*dpr);}
    if(!sky.gl&&window.SkyGL){sky.gl=function(){sky.force=1;};window.SkyGL.onChange(sky.gl);}
    if(key!==sky.key){var src=sky.src,cal=Object.assign({},src.calendar),h=HOUR[O.time],wx=WX[O.weather]||src.weather;
      if(h){cal.current_hour=h[0];cal.current_minute=h[1];}
      sky.model={calendar:cal,todayEvents:src.events,dayWeather:wx,skym:window.SkyWorld.makeSkym(cal),
        landSeed:S.seed(String(src.seed)),width:W,height:H,reduced:!!host.calm,glide:sky.model?sky.model.glide:null};
      sky.key=key;sky.force=1;}
    // The window is small and its sky drifts slowly, so 30 frames a second is plenty
    var m=sky.model,pace=sky.pace==null?2:sky.pace,gap=pace===2?1/30:pace===1?1/12:1e9;m.width=W;m.height=H;
    if(sky.lite)gap=Math.max(gap,1/6);
    if(!sky.force&&!resized&&t-sky.last<gap)return;
    var dt=sky.last<0?0:Math.min(1,t-sky.last);sky.last=t;sky.force=0;
    try{var g=sky.ctx||(sky.ctx=cv.getContext('2d'));g.setTransform(1,0,0,1,0,0);g.clearRect(0,0,cv.width,cv.height);
      var st=S.state(m,host.calm?0:t,dt),t0=performance.now();S.paint(g,sky.hold||(sky.hold={}),st,dpr,0,pace===2);skyCost(performance.now()-t0);sky.pace=host.calm?0:S.pace(st);
      if(!sky.on){sky.on=true;svg.classList.add('d-calsky');}}
    catch(e){sky.on=false;svg.classList.remove('d-calsky');}}
  /* The painted sky is cheap on a graphics card and very slow where a browser
     paints it in software, and the sky's own check times only part of the
     work. So the whole paint is timed here: if it keeps eating the frame, the
     window switches to the calendar's basic sky, the same fallback the
     calendar uses, rather than make the bench stutter. SkyGLAllowSoftware,
     Chronicle's switch for browser checks, keeps the painted sky. */
  function skyCost(ms){var G=window.SkyGL;if(sky.basic||window.SkyGLAllowSoftware||!G||!G._painter||!G.available())return;
    sky.n=(sky.n||0)+1;sky.avg=sky.avg==null?ms:sky.avg*.8+ms*.2;sky.stall=ms>60?(sky.stall||0)+1:0;
    if(sky.stall>=2||(sky.n>=8&&sky.avg>14)){sky.basic=1;G._painter.slow=true;sky.force=1;}}
  /* Depth: the back wall drifts a little against the pointer, so the bench
     reads as nearer than the window. The candle's flicker moves every light
     in the room together, and the dark corners breathe with it. Only the
     falloff layer uses a blend mode; glows are plain translucent paint,
     which looks the same over a dark room and repaints far cheaper. */
  var n1=0,n2=0,ox=0,oy=0,tx=0,ty=0,far=null,dirty=0,gen=0,liqCol=null,perf={t:0,n:0,last:0};
  /* A slow device drops the depth blur and the grain after a few seconds,
     and repaints the window's sky less often, rather than stutter; the
     lighting and textures stay. */
  function watch(svg){var now=performance.now();if(perf.done)return;if(!perf.last){perf.last=now;return;}
    if(now-perf.last>250){perf.last=now;return;}perf.t+=now-perf.last;perf.n++;perf.last=now;
    if(perf.t>4000){perf.done=1;if(perf.n/(perf.t/1000)<40&&svg.parentNode){svg.parentNode.classList.add('d-lite');sky.lite=1;}}}
  function light(t,S,svg){
    if(!svg.__par){svg.__par=1;svgRef=svg;grain(svg);svg.addEventListener('pointermove',function(e){var r=svg.getBoundingClientRect();tx=((e.clientX-r.left)/r.width-.5)*2;ty=((e.clientY-r.top)/r.height-.5)*2;});svg.addEventListener('pointerleave',function(){tx=0;ty=0;});}
    if(!svg.__baked){svg.__baked=1;svg.style.setProperty('--vig',MOOD[O.mood].vig);bake(svg);}
    else if(dirty){dirty=0;redraw(svg);}
    else watch(svg);
    skyFrame(svg,t);
    if(!host.calm){ox+=(tx-ox)*.05;oy+=(ty-oy)*.05;far=far||svg.querySelector('.d-far');if(far)far.setAttribute('transform','translate('+(-ox*9).toFixed(2)+','+(-oy*5).toFixed(2)+')');}
    flare=Math.max(0,flare-.012);n1+=(Math.random()-.5)*.25;n1*=.92;n2=Math.sin(t*7.3)*.03+Math.sin(t*13.1)*.02;var f=(.9+n1*.25+n2)*(1+flare*.9);
    function op(q,v){var e=svg.querySelector(q);if(e)e.setAttribute('opacity',v.toFixed(3));}
    op('.d-wallglow',.3*f);op('.d-deskglow',.2*f);op('.d-bloom1',Math.min(1,.8*f));op('.d-bloom2',.45*f);op('.d-refl',.32*f);
    op('.d-shade',Math.max(.55,Math.min(1,1.02-(f-.9)*.6)));
    op('.d-bglow',.25+S.heat*.6);op('.d-liqglow',S.phase==='done'?.7:S.heat*.35+(S.flash||0)*.6);
    // Light through the flask throws a patch of the liquid's colour on the desk
    var lq=svg.querySelector('.cr-liq'),c=lq&&lq.style.fill;
    if(c&&c!==liqCol){liqCol=c;svg.querySelectorAll('#'+host.id('d-caus2')+' stop').forEach(function(s){s.setAttribute('stop-color',c);});}
    op('.d-caustic',c?Math.min(.75,S.level*1.1)*f:0);}
  function foreground(){return '<rect width="1200" height="700" fill="url(#d-vig)"/>';}
  function shade(){return '<ellipse class="d-shade" cx="'+(FX-92)+'" cy="'+(FY+100)+'" rx="1300" ry="980" fill="url(#d-shade)" style="mix-blend-mode:multiply"/>';}
  /* A look setting changed: draw the room again from the settings, swap in
     the new drawing, and bake its light. The editor re-applies its own moves
     when it hears craft:redrawn. */
  function redraw(svg){relight();infoNow();gen++;
    var d=svg.querySelector('defs');if(d)d.innerHTML=host.scope(defs());
    svg.querySelector('.cr-bg').innerHTML=host.scope(background());
    var sh=svg.querySelector('.cr-shade'),tp=svg.querySelector('.cr-top');if(sh)sh.innerHTML=host.scope(shade());if(tp)tp.innerHTML=host.scope(foreground());
    far=null;liqCol=null;sky.cv=null;sky.ctx=null;sky.key=null;sky.force=1;
    svg.style.setProperty('--vig',MOOD[O.mood].vig);bake(svg);
    try{svg.dispatchEvent(new CustomEvent('craft:redrawn'));}catch(e){}}

  /* Bake: each [data-bake] group is drawn once, filters and all, into an
     image at twice the scene's resolution, then swapped in. If a browser
     refuses, the live drawing stays, just slower to repaint. */
  var urls=[];
  function bake(svg){var gg=gen,d=host.scope(defs());
    svg.querySelectorAll('[data-bake]').forEach(function(g){
      if(g.__src==null)g.__src=g.innerHTML;var bb;try{bb=g.getBBox();}catch(e){return;}if(!bb||!bb.width)return;
      var p=24,x=Math.floor(bb.x-p),y=Math.floor(bb.y-p),w=Math.ceil(bb.width+2*p),h=Math.ceil(bb.height+2*p),k=2;
      var src='<svg xmlns="http://www.w3.org/2000/svg" viewBox="'+x+' '+y+' '+w+' '+h+'" width="'+w*k+'" height="'+h*k+'"><defs>'+d+'</defs>'+g.__src+'</svg>',img=new Image();
      // The new picture goes on top of what is there and the old drawing is
      // removed only once it has loaded, so the layer never blinks out.
      function swap(u){if(gen!==gg||!g.isConnected)return;var im=document.createElementNS('http://www.w3.org/2000/svg','image');
        [['href',u],['x',x],['y',y],['width',w],['height',h],['preserveAspectRatio','none']].forEach(function(a){im.setAttribute(a[0],a[1]);});
        im.addEventListener('load',function(){Array.prototype.slice.call(g.childNodes).forEach(function(n){if(n!==im)g.removeChild(n);});});g.appendChild(im);}
      img.onload=function(){try{var cv=document.createElement('canvas');cv.width=w*k;cv.height=h*k;cv.getContext('2d').drawImage(img,0,0);
        cv.toBlob(function(b){if(!b||dead){if(!dead)swap(img.src);return;}var u=URL.createObjectURL(b);if(g.__url)URL.revokeObjectURL(g.__url);g.__url=u;urls.push(u);swap(u);});}catch(e){swap(img.src);}};
      img.src='data:image/svg+xml;charset=utf-8,'+encodeURIComponent(src);});}
  /* Film grain over the whole frame, particles included, so the drawn
     shapes and the canvas effects sit in the same photograph. */
  function grain(svg){var st=svg.parentNode;if(!st||st.querySelector('.d-grainov'))return;
    try{var c=document.createElement('canvas'),n=160;c.width=c.height=n;var x=c.getContext('2d'),im=x.createImageData(n,n);
      for(var i=0;i<im.data.length;i+=4){var v=128+(Math.random()-.5)*120|0;im.data[i]=im.data[i+1]=im.data[i+2]=v;im.data[i+3]=255;}x.putImageData(im,0,0);
      var dv=document.createElement('div');dv.className='d-grainov';dv.setAttribute('aria-hidden','true');dv.style.backgroundImage='url('+c.toDataURL()+')';
      var fx=st.querySelector('.cr-fx');st.insertBefore(dv,fx?fx.nextSibling:null);}catch(e){}}

  /* Ready-made looks: each sets every room choice at once. They leave the
     window's time and weather alone, so the sky keeps following the calendar. */
  var LOOKS={
    apothecary:{name:'Harbour apothecary',note:'Plaster, oak, the harbour, a candle',o:{wall:'plaster',bench:'oak',view:'harbour',light:'candle',mood:'moody',cat:'black',cover:'oxblood',kit:[]}},
    hold:{name:'Dwarven hold',note:'Stone, slate, the peaks, a lantern',o:{wall:'stone',bench:'slate',view:'mountains',light:'lantern',mood:'cosy',cat:'grey',cover:'navy',kit:['scales']}},
    hut:{name:'Witch\u2019s cottage',note:'Boards, pine, the forest, a witchlight',o:{wall:'timber',bench:'pine',view:'forest',light:'witch',mood:'grim',cat:'ginger',cover:'green',kit:['skull','mandrake']}},
    city:{name:'City alchemist',note:'Brick, walnut, the rooftops, a candle',o:{wall:'brick',bench:'walnut',view:'city',light:'candle',mood:'cosy',cat:'cream',cover:'black',kit:['ball','scales']}}};

  /* What the build-mode editor needs to know about this room: which pieces
     there are and what draws each, the tray's tabs, and its pictures. A new
     skin fills in the same tables and the editor works on it unchanged. */
  var LOCK_TOOL='Tools stay where the bench puts them, so every step can find its tool.';
  var PIECES={
    mortar:{kind:'tool',sel:'.cr-mortar, .cr-pestle',lock:LOCK_TOOL,uses:'Add, grind and tip use the mortar.'},
    flask:{kind:'tool',sel:'.cr-flaskg',lock:LOCK_TOOL,fixed:['Swirl it'],uses:'Add, pour and every finale use the flask.'},
    burner:{kind:'tool',sel:'[data-prop="burner"]',lock:LOCK_TOOL,uses:'Keep it steady uses the burner and bellows.'},
    bellows:{kind:'tool',sel:'[data-prop="bellows"]',lock:LOCK_TOOL,uses:'Keep it steady uses the burner and bellows.'},
    candle:{kind:'piece',sel:'[data-prop="candle"]',light:true},
    cat:{kind:'piece',sel:'[data-prop="cat"]'},
    shelf:{kind:'piece',sel:'[data-prop="shelf"]'},
    herbs:{kind:'piece',sel:'[data-prop="herbs"]'},
    hourglass:{kind:'piece',sel:'[data-prop="hourglass"]'},
    inkwell:{kind:'piece',sel:'[data-prop="inkwell"]'},
    book:{kind:'piece',sel:'[data-prop="book"]',lock:'The recipe is written on it, so it stays where players can read it.',fixed:['Read the recipe']},
    tray:{kind:'piece',sel:'[data-prop="tray"]',lock:'The ingredients wait on it, so it stays put.'},
    window:{kind:'piece',sel:'[data-prop="window"]',lock:'Part of the room. Change the view, weather and time in the Window tab.'},
    'kit-skull':{kind:'piece',sel:'[data-prop="kit-skull"]',kit:'skull'},
    'kit-ball':{kind:'piece',sel:'[data-prop="kit-ball"]',kit:'ball'},
    'kit-scales':{kind:'piece',sel:'[data-prop="kit-scales"]',kit:'scales'},
    'kit-mandrake':{kind:'piece',sel:'[data-prop="kit-mandrake"]',kit:'mandrake'}
  };
  // Glows that follow the light at once while it is dragged; the full relight comes when it settles
  var GLOWS=['.d-wallglow','.d-deskglow','.d-shade'];
  var TABS=[{id:'looks',label:'Looks',looks:true,note:'A look sets every room choice at once. Your moves, names and actions stay.'},
    {id:'wall',label:'Walls',groups:[['wall','Walls']]},
    {id:'bench',label:'Bench top',groups:[['bench','Bench top']]},
    {id:'light',label:'Light',groups:[['light','Source'],['mood','Mood']],note:'Drag the light in the scene and the whole room relights from where you put it.'},
    {id:'window',label:'Window',groups:[['view','View']],segs:['time','weather']},
    {id:'details',label:'Details',groups:[['cat','Cat'],['cover','Book cover']]},
    {id:'kit',label:'Kit',kit:true,note:'Click one to put it on the bench, then drag it where you like.'},
    {id:'pieces',label:'Pieces',pieces:true}];
  function thumb(key,v){return opts.thumbs?opts.thumbs+encodeURIComponent(key+'-'+v)+'.jpg':'';}

  var skin={label:'An apothecary bench at night, lit by a candle',layout:L,defs:defs,background:background,foreground:foreground,shade:shade,art:art,book:book,fx:fx,ambient:ambient,light:light,sound:'desk',info:INFO,act:act,
    itemSpot:function(k,i){return L.items[k]||L.slots[i%L.slots.length];},
    bind:function(a){host=a;},
    opts:O,setOpt:function(k,v){if(k==='ldx'||k==='ldy'||k==='kit'||CHOICES[k]){var o={};o[k]=v;setLook(o);if(LOOK[k])dirty=1;}},
    looks:LOOKS,look:function(id){var Lk=LOOKS[id];if(!Lk)return;setLook(Lk.o);dirty=1;},
    choices:CHOICES,kit:KITDRAWN,pieces:PIECES,glows:GLOWS,tabs:TABS,builtins:BUILTIN,thumb:thumb,
    destroy:function(){dead=true;if(sky.gl&&window.SkyGL&&window.SkyGL.offChange)window.SkyGL.offChange(sky.gl);urls.forEach(function(u){URL.revokeObjectURL(u);});urls.length=0;}};
  return skin;
  }
  return {create:create};
})();
