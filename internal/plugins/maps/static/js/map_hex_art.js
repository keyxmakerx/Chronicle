/**
 * map_hex_art.js -- the Detailed and Realistic terrain art of the hex layer.
 *
 * map_hexes.js draws the layer; this file only knows how one hex of a terrain
 * looks. The drawing code is the approved design's own, kept shape for shape
 * and colour for colour, so the hexes look the way they were signed off.
 *
 * Detailed is vector art: a shaded hex and a few trees, peaks, dunes or reeds
 * scattered from a seed (detailedArt returns SVG markup). Realistic turns each
 * hex into a small raised model: a height map of tree canopies, peaks, roofs and
 * water, lit by a low sun with cast shadows and soft occlusion, cut into a tile
 * whose front edges show earth (iso3 paints it on a canvas). Each terrain has
 * twelve pieces to choose from (PIECES, grouped into PFOLD folders for the
 * picker); a hex with no piece ("Mix") gets a stable one from its position
 * (pieceOf), so a painted area never repeats and never changes on a redraw.
 *
 * A Realistic tile is drawn from (terrain, piece, which lower edges show), not
 * from the hex's position, so the viewer can paint it once per zoom bucket and
 * stamp it on every hex that matches (see TileCache). The pure parts are exported
 * for test/js/map_hexes.test.mjs.
 */
(function () {
  'use strict';

  // The palette colours, the same as the Simple art's (map_hexes.js TERRAINS).
  var TER_COLOR = {
    plains: '#c9d58a', forest: '#5f8f4a', hills: '#b8a46a', mountain: '#8d8478', water: '#6f9fc0',
    swamp: '#6f7d55', desert: '#e3cc8c', snow: '#e8eef2', town: '#c4a27a', road: '#a0805a',
    jungle: '#3f8f6a', farmland: '#d6c463', scrub: '#b3a771', badlands: '#cf8f63', waste: '#7a6f6a',
    volcanic: '#8c4638', coast: '#bfe0cf', deep: '#3d5f92', ice: '#bfe1ee', cavern: '#5a5470'
  };

  // Each hex's art is seeded from a string: the same scatter every time it is
  // drawn, but no two seeds alike.
  function rng(s){var h=2166136261;for(var i=0;i<s.length;i++){h^=s.charCodeAt(i);h=Math.imul(h,16777619)}return function(){h^=h<<13;h^=h>>>17;h^=h<<5;return ((h>>>0)%10007)/10007}}
  function T(x,y,s){return ' transform="translate('+x.toFixed(1)+' '+y.toFixed(1)+') scale('+s.toFixed(3)+')"'}
  function hexPath(cx,cy,r){var d='';for(var i=0;i<6;i++){var a=Math.PI/3*i-Math.PI/2;d+=(i?'L':'M')+(cx+r*Math.cos(a)).toFixed(1)+' '+(cy+r*Math.sin(a)).toFixed(1)}return d+'Z'}

  // ---- Detailed: vector sprites ----

  var ART={
    conifer:function(x,y,s,snow){return '<g'+T(x,y,s)+'><ellipse cx="1.5" cy=".6" rx="6.5" ry="2" fill="#000" opacity=".2"/><rect x="-1" y="-3" width="2" height="3.6" fill="#5b3f24"/><path d="M0-17L-6-7h3L-7 0h14L3-7h3z" fill="#2c5527"/><path d="M0-17L-6-7h3L-7 0h7z" fill="#4c8739"/>'
      +(snow?'<path d="M0-17L-3-12l1.6-.4L0-10.2l1.4-1.4 1.6.4z" fill="#fff"/><path d="M-6.4-.4l2-1.6 2 1 2-1.2 2 1.2 2-1 2.4 1.6z" fill="#f4f8fb"/>':'')+'</g>'},
    oak:function(x,y,s){return '<g'+T(x,y,s)+'><ellipse cx="1.5" cy=".6" rx="7" ry="2.2" fill="#000" opacity=".2"/><rect x="-1" y="-5" width="2" height="5.6" fill="#5b3f24"/><circle cx="-3.2" cy="-8" r="4.6" fill="#3a6a2f"/><circle cx="3.2" cy="-8" r="4.6" fill="#2c5527"/><circle cx="0" cy="-12" r="5.2" fill="#4c8739"/><circle cx="-1.9" cy="-13.6" r="2.3" fill="#80b45f" opacity=".9"/></g>'},
    peak:function(x,y,s,snow){return '<g'+T(x,y,s)+'><ellipse cx="3" cy=".6" rx="14" ry="2.6" fill="#000" opacity=".16"/><path d="M-12 0L-2-20L2 0z" fill="#a69d8f"/><path d="M-2-20L12 0H2z" fill="#6b6359"/>'
      +(snow!==false?'<path d="M-2-20L-6.2-12.5-4-13.3-2.6-11.4-.6-13.2.4-12z" fill="#fff"/><path d="M-2-20L.4-12 2-13 4-11.3z" fill="#d6e0e6"/>':'')
      +'<path d="M-12 0L-2-20L12 0" fill="none" stroke="#3f3a33" stroke-width=".8" stroke-linejoin="round"/></g>'},
    hill:function(x,y,s){return '<g'+T(x,y,s)+'><ellipse cx="1" cy=".6" rx="12.5" ry="2.4" fill="#000" opacity=".15"/><path d="M-12 0Q-6-13 0-11Q7-9 12 0z" fill="#ad9758"/><path d="M-12 0Q-6-13 0-11Q-3-6-2 0z" fill="#cfba7a"/><path d="M-12 0Q-6-13 0-11Q7-9 12 0" fill="none" stroke="#6e5f35" stroke-width=".8"/><path d="M-6-4l-1-2M-3-6v-2M3-5l1-2M6-3l1-2" stroke="#5f7030" stroke-width=".8" stroke-linecap="round"/></g>'},
    wave:function(x,y,s){return '<g'+T(x,y,s)+'><path d="M-7 1.2q1.75-2.5 3.5 0t3.5 0t3.5 0t3.5 0" fill="none" stroke="#2f5f82" stroke-width="1.1" stroke-linecap="round" opacity=".45"/><path d="M-7 0q1.75-2.5 3.5 0t3.5 0t3.5 0t3.5 0" fill="none" stroke="#eef7fc" stroke-width="1.2" stroke-linecap="round" opacity=".9"/></g>'},
    reed:function(x,y,s){return '<g'+T(x,y,s)+'><path d="M0 0L-1-10M0 0L2-8M0 0L-3-6M0 0L3.5-4" stroke="#44532d" stroke-width=".9" stroke-linecap="round" fill="none"/><ellipse cx="-1.1" cy="-11" rx="1.1" ry="2.3" fill="#6b4a2a"/><ellipse cx="2.1" cy="-8.8" rx=".9" ry="1.9" fill="#7a5632"/></g>'},
    puddle:function(x,y,s){return '<g'+T(x,y,s)+'><ellipse rx="7.5" ry="3.1" fill="#58777e"/><ellipse rx="7.5" ry="3.1" fill="none" stroke="#3a5544" stroke-width=".7"/><path d="M-4.5-.8h3.4M1.5.6h2" stroke="#d4e7ea" stroke-width=".8" stroke-linecap="round"/></g>'},
    dune:function(x,y,s){return '<g'+T(x,y,s)+'><path d="M-12 0Q-4-9 4-6Q9-4 12 0z" fill="#ecd69b"/><path d="M4-6Q9-4 12 0H2Q4.5-3 4-6z" fill="#c8a862"/><path d="M-12 0Q-4-9 4-6" fill="none" stroke="#a5874a" stroke-width=".7"/><path d="M-7-2.5q3-1.6 6-1.2" stroke="#fff4d2" stroke-width=".7" fill="none" stroke-linecap="round"/></g>'},
    tuft:function(x,y,s){return '<path'+T(x,y,s)+' d="M-2 0L-3.2-4M0 0V-5.2M2 0L3.2-4" stroke="#5a7530" stroke-width="1" stroke-linecap="round" fill="none"/>'},
    flower:function(x,y,s,c){return '<g'+T(x,y,s)+'><circle r="1.3" fill="'+c+'"/><circle r=".5" fill="#a0702a"/></g>'},
    drift:function(x,y,s){return '<g'+T(x,y,s)+'><path d="M-10 0Q-4-6 2-3Q7-5 10 0z" fill="#fff"/><path d="M-10 0Q-4-6 2-3Q7-5 10 0" fill="none" stroke="#9db4c5" stroke-width=".8"/><path d="M-1-1.5q3-1.5 6-.6" stroke="#c6d6e1" stroke-width=".7" fill="none"/></g>'},
    rock:function(x,y,s){return '<g'+T(x,y,s)+'><path d="M-4.5 0L-3.2-3.4L0-4.4L3.4-2.4L4.5 0z" fill="#857b6d"/><path d="M-4.5 0L-3.2-3.4L0-4.4L-.8 0z" fill="#aea494"/></g>'},
    palm:function(x,y,s){return '<g'+T(x,y,s)+'><ellipse cx="1.5" cy=".6" rx="6" ry="1.9" fill="#000" opacity=".2"/><path d="M0 0Q1.6-7-.6-13.5" fill="none" stroke="#6b4a2a" stroke-width="1.8" stroke-linecap="round"/><path d="M-.6-13.5q-5-3.4-8.6.8M-.6-13.5q5-3.4 8.6.8M-.6-13.5q-3.4-5-7-4.4M-.6-13.5q3.4-5 7.4-4" fill="none" stroke="#1f6b3a" stroke-width="2.1" stroke-linecap="round"/><path d="M-.6-13.5q-4.4-2-7 1.4M-.6-13.5q.6-4.6 2.4-6" fill="none" stroke="#4ea85a" stroke-width="1.5" stroke-linecap="round"/></g>'},
    jtree:function(x,y,s){return '<g'+T(x,y,s)+'><ellipse cx="1.5" cy=".6" rx="8" ry="2.3" fill="#000" opacity=".22"/><rect x="-1.1" y="-6" width="2.2" height="6.6" fill="#4d3520"/><circle cx="-4.4" cy="-8.4" r="5" fill="#1f6b46"/><circle cx="4.2" cy="-8" r="5.2" fill="#175a3c"/><circle cx="0" cy="-13" r="6" fill="#2f8f55"/><circle cx="-2.2" cy="-15" r="2.6" fill="#6cc27a" opacity=".9"/><path d="M4.4-4.6v3.4M-5-4.6v2.6" stroke="#175a3c" stroke-width=".9" stroke-linecap="round"/></g>'},
    plot:function(x,y,s,c,d){return '<g'+T(x,y,s)+'><path d="M-10 2L-3-5L10-2L3 5z" fill="'+c+'" stroke="'+d+'" stroke-width=".8" stroke-linejoin="round"/><path d="M-6.5 3.2L.5-3.8M-3 4.1L4-2.9M.4 4.7L7.2-2.4" stroke="'+d+'" stroke-width=".7" opacity=".7"/></g>'},
    barn:function(x,y,s){return '<g'+T(x,y,s)+'><ellipse cx="1" cy=".6" rx="6.5" ry="1.8" fill="#000" opacity=".18"/><path d="M-5 0V-5l5-4 5 4V0z" fill="#b9523c" stroke="#5e241a" stroke-width=".8" stroke-linejoin="round"/><path d="M-6-4.6L0-9.6L6-4.6" fill="none" stroke="#5e241a" stroke-width="1.6" stroke-linejoin="round"/><rect x="-1.5" y="-3.4" width="3" height="3.4" fill="#5e241a"/></g>'},
    bush:function(x,y,s){return '<g'+T(x,y,s)+'><ellipse cx="1" cy=".5" rx="5.6" ry="1.6" fill="#000" opacity=".16"/><circle cx="-2.4" cy="-2.6" r="3.2" fill="#6c783b"/><circle cx="2.2" cy="-3" r="3.6" fill="#7f8a45"/><circle cx="-.2" cy="-4.6" r="2.3" fill="#a3ab63"/></g>'},
    acacia:function(x,y,s){return '<g'+T(x,y,s)+'><ellipse cx="1.5" cy=".6" rx="8" ry="2" fill="#000" opacity=".18"/><path d="M0 0V-6M0-5L-4.5-9M0-5.6L4.5-9.4" fill="none" stroke="#6b4a2a" stroke-width="1.4" stroke-linecap="round"/><ellipse cx="0" cy="-11" rx="9" ry="3.4" fill="#6f8140"/><ellipse cx="-2" cy="-12" rx="5.4" ry="1.7" fill="#9fb062"/></g>'},
    mesa:function(x,y,s){return '<g'+T(x,y,s)+'><ellipse cx="2" cy=".6" rx="13" ry="2.4" fill="#000" opacity=".16"/><path d="M-12 0L-8-11H8L12 0z" fill="#b06a47"/><path d="M-12 0L-8-11H-1L-3 0z" fill="#dc9a6c"/><path d="M-9.8-6H9.8M-10.9-3H10.9" stroke="#7a4129" stroke-width=".7" opacity=".7"/><path d="M-12 0L-8-11H8L12 0" fill="none" stroke="#5e3220" stroke-width=".8" stroke-linejoin="round"/></g>'},
    hoodoo:function(x,y,s){return '<g'+T(x,y,s)+'><ellipse cx="1" cy=".5" rx="4.6" ry="1.4" fill="#000" opacity=".18"/><path d="M-2.6 0L-1.8-9L-3-11.5Q0-15.5 3-11.5L1.8-9L2.6 0z" fill="#b06a47"/><path d="M-2.6 0L-1.8-9L-3-11.5Q-1.4-14 0-14.4L-.4 0z" fill="#dc9a6c"/><path d="M-2.2-4.6H2.2" stroke="#7a4129" stroke-width=".7"/></g>'},
    snag:function(x,y,s,c){return '<g'+T(x,y,s)+'><ellipse cx="1.5" cy=".5" rx="5" ry="1.5" fill="#000" opacity=".16"/><path d="M0 0V-13M0-6L-4.6-10.4M0-8.4L4.4-12.6M0-3.6L3.6-6.2" fill="none" stroke="'+(c||'#4a403a')+'" stroke-width="1.5" stroke-linecap="round"/></g>'},
    rubble:function(x,y,s){return '<g'+T(x,y,s)+'><path d="M-5 0V-3h4V0zM0 0V-4.6h3.4V0z" fill="#8f8880"/><path d="M-5-3h4L-.2-3.8H-4.2zM0-4.6h3.4l.8-.8H.8z" fill="#bdb6ac"/><path d="M3.8 0l1.6-2 1.8 2z" fill="#746d66"/></g>'},
    crack:function(x,y,s,c){return '<path'+T(x,y,s)+' d="M-8 1L-4-1.4L-1.6 1L2-1.8L4.6.4L8-1.2M-1.6 1L-2.6 3.6M2-1.8L3-4" fill="none" stroke="'+c+'" stroke-width="1" stroke-linecap="round" stroke-linejoin="round"/>'},
    cone:function(x,y,s){return '<g'+T(x,y,s)+'><ellipse cx="3" cy=".6" rx="14" ry="2.6" fill="#000" opacity=".2"/><path d="M-13 0L-4-16H4L13 0z" fill="#3c3131"/><path d="M-13 0L-4-16H0L-2 0z" fill="#6a5957"/><path d="M-4-16H4L2.6-12.6L1-14.2L-.6-11L-2.2-14z" fill="#ff8a2a"/><path d="M-.6-11Q-1.4-7-.2-3.6" fill="none" stroke="#ff8a2a" stroke-width="1.3" stroke-linecap="round"/><path d="M-13 0L-4-16H4L13 0" fill="none" stroke="#231a1a" stroke-width=".8" stroke-linejoin="round"/></g>'},
    lava:function(x,y,s){return '<g'+T(x,y,s)+'><path d="M-8 1L-4-1.4L-1.6 1L2-1.8L4.6.4L8-1.2" fill="none" stroke="#3a1a10" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"/><path d="M-8 1L-4-1.4L-1.6 1L2-1.8L4.6.4L8-1.2" fill="none" stroke="#ff9a3a" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/></g>'},
    sand:function(x,y,s){return '<g'+T(x,y,s)+'><path d="M-12 0Q-5-8 3-5.4Q9-3.4 12 0z" fill="#f3e4b6"/><path d="M3-5.4Q9-3.4 12 0H1Q3.6-2.8 3-5.4z" fill="#d9c48c"/><path d="M-12 0Q-5-8 3-5.4" fill="none" stroke="#a5874a" stroke-width=".7"/><path d="M-12 1.6q2-1.9 4 0t4 0t4 0t4 0t4 0t4 0" fill="none" stroke="#fff" stroke-width="1.2" stroke-linecap="round" opacity=".95"/></g>'},
    swell:function(x,y,s){return '<g'+T(x,y,s)+'><path d="M-9 1.4q3-4.4 6 0t6 0t6 0" fill="none" stroke="#12294a" stroke-width="1.5" stroke-linecap="round" opacity=".55"/><path d="M-9 0q3-4.4 6 0t6 0t6 0" fill="none" stroke="#a9c8ea" stroke-width="1.2" stroke-linecap="round" opacity=".85"/></g>'},
    fluke:function(x,y,s){return '<g'+T(x,y,s)+'><path d="M0 2Q0-3-5.4-7.4Q-1.2-6.4 0-3.4Q1.2-6.4 5.4-7.4Q0-3 0 2z" fill="#1d3558"/><path d="M-4 2.6q2-1.8 4 0t4 0" fill="none" stroke="#dbe9f7" stroke-width="1.1" stroke-linecap="round"/></g>'},
    shard:function(x,y,s,a,b){return '<g'+T(x,y,s)+'><ellipse cx="1.5" cy=".5" rx="5" ry="1.4" fill="#000" opacity=".13"/><path d="M-4 0L-1-14L3.6 0z" fill="'+(b||'#7fb6d6')+'"/><path d="M-4 0L-1-14L-.2 0z" fill="'+(a||'#d3eefa')+'"/><path d="M-4 0L-1-14L3.6 0" fill="none" stroke="#3f6f8c" stroke-width=".7" stroke-linejoin="round" opacity=".7"/></g>'},
    spike:function(x,y,s){return '<g'+T(x,y,s)+'><ellipse cx="1.5" cy=".5" rx="5" ry="1.4" fill="#000" opacity=".25"/><path d="M-3.6 0Q-1.6-6-.6-13Q1.2-6 3.6 0z" fill="#4d4660"/><path d="M-3.6 0Q-1.6-6-.6-13L-.2 0z" fill="#8a829c"/></g>'},
    shroom:function(x,y,s,c){return '<g'+T(x,y,s)+'><circle cx="0" cy="-5" r="6" fill="'+c+'" opacity=".22"/><rect x="-1" y="-5" width="2" height="5" fill="#d8d0e0"/><path d="M-4.6-4.4Q0-11 4.6-4.4z" fill="'+c+'"/><circle cx="-1.4" cy="-6.4" r=".8" fill="#fff" opacity=".8"/></g>'}
  };
  // A place in Detailed art is the Simple icon, a little larger.
  var TOWN_ICON='<path d="M-9 6V0l4-4 4 4v6z" fill="#d8c7a2" stroke="#6b5233" stroke-width="1"/><path d="M-10 0.5l5-5.5 5 5.5" fill="none" stroke="#9b4a32" stroke-width="2" stroke-linejoin="round"/><path d="M1 6V-2l4-4.5 4 4.5v8z" fill="#e6d8b8" stroke="#6b5233" stroke-width="1"/><path d="M0-1.5l5-5.5 5 5.5" fill="none" stroke="#7a3f2c" stroke-width="2" stroke-linejoin="round"/>';
  function mix(hex,w,amt){var a=parseInt(hex.slice(1),16),b=parseInt(w.slice(1),16);function c(sh){return Math.round(((a>>sh)&255)*(1-amt)+((b>>sh)&255)*amt)}return 'rgb('+c(16)+','+c(8)+','+c(0)+')'}

  // detailedDefs is the shading every Detailed hex is filled with, one
  // gradient per terrain; detailedArt refers to them by uid.
  function detailedDefs(uid){var d='';Object.keys(TER_COLOR).forEach(function(t){var c=TER_COLOR[t];d+='<radialGradient id="'+uid+'g'+t+'" cx="50%" cy="38%" r="70%"><stop offset="0" stop-color="'+mix(c,'#ffffff',.25)+'" stop-opacity=".5"/><stop offset="1" stop-color="'+mix(c,'#000000',.08)+'" stop-opacity=".66"/></radialGradient>'});return d}

  // DETAILED_VARIANTS is how many seeds a terrain's Detailed art is drawn from;
  // each hex takes one by its position, so neighbours differ and a seed can be
  // painted once and reused for every hex that has it.
  var DETAILED_VARIANTS = 12;
  function detailedSeed(t,v){return 'd'+t+v}

  /* Scatter points inside a hex, keeping them apart so trees and reeds don't pile on top of each other. */
  function scatter(R,cx,cy,r,n,gap){var out=[],tries=0;while(out.length<n&&tries<n*30){tries++;var a=R()*Math.PI*2,d=Math.sqrt(R())*r*.56;var x=cx+Math.cos(a)*d,y=cy+Math.sin(a)*d*.86+r*.14;
      if(out.every(function(p){return Math.hypot(p[0]-x,p[1]-y)>r*gap}))out.push([x,y])}return out.sort(function(a,b){return a[1]-b[1]})}
  function detailedArt(t,cx,cy,r,seed,uid){var s=r/26,R=rng(seed),o='',path=hexPath(cx,cy,r);
    o+='<path d="'+path+'" fill="url(#'+uid+'g'+t+')"/><path d="'+hexPath(cx,cy,r-1.2*s)+'" fill="none" stroke="'+mix(TER_COLOR[t],'#000000',.35)+'" stroke-opacity=".35" stroke-width="'+(1.4*s)+'"/>';
    var P;
    if(t==='forest'){P=scatter(R,cx,cy,r,7,.27);P.forEach(function(p){o+=R()<.6?ART.conifer(p[0],p[1],s*(.8+R()*.35)):ART.oak(p[0],p[1],s*(.78+R()*.3))})}
    else if(t==='mountain'){var j=(R()-.5)*.16*r;o+=ART.peak(cx-.4*r,cy+.2*r,s*.8,R()<.5)+ART.peak(cx+.4*r,cy+.28*r,s*.74,false)+ART.peak(cx+j,cy+.46*r,s*1.38)}
    else if(t==='hills'){P=scatter(R,cx,cy,r,3,.45);P.forEach(function(p){o+=ART.hill(p[0],p[1],s*(.75+R()*.3))})}
    else if(t==='water'){P=scatter(R,cx,cy,r,4,.3);P.forEach(function(p){o+=ART.wave(p[0],p[1]-r*.12,s*(.85+R()*.3))})}
    else if(t==='swamp'){scatter(R,cx,cy,r,2,.45).forEach(function(p){o+=ART.puddle(p[0],p[1]-r*.08,s*(.8+R()*.3))});scatter(R,cx,cy,r,5,.18).forEach(function(p){o+=ART.reed(p[0],p[1],s*(.8+R()*.35))})}
    else if(t==='desert'){scatter(R,cx,cy,r,3,.4).forEach(function(p,i){o+=i===2&&R()<.6?ART.rock(p[0],p[1],s):ART.dune(p[0],p[1],s*(.75+R()*.3))})}
    else if(t==='snow'){scatter(R,cx,cy,r,3,.36).forEach(function(p,i){o+=i===1?ART.conifer(p[0],p[1],s*.9,true):ART.drift(p[0],p[1],s*(.75+R()*.3))})}
    else if(t==='town')o+='<g'+T(cx,cy+r*.08,s*1.25)+'>'+TOWN_ICON+'</g>';
    else if(t==='plains'){scatter(R,cx,cy,r,7,.2).forEach(function(p,i){o+=i%3===2?ART.flower(p[0],p[1]-r*.05,s,R()<.5?'#f6e27a':'#fbfbf4'):ART.tuft(p[0],p[1],s*(.85+R()*.3))})}
    else if(t==='jungle'){scatter(R,cx,cy,r,7,.27).forEach(function(p){o+=R()<.62?ART.jtree(p[0],p[1],s*(.82+R()*.3)):ART.palm(p[0],p[1],s*(.85+R()*.3))})}
    else if(t==='farmland'){var FP=[['#d8c05a','#8a7526'],['#9db552','#566b26'],['#b98f5a','#6e4d2a'],['#e3d27c','#93802f']];scatter(R,cx,cy,r,4,.42).forEach(function(p,i){o+=i===3?ART.barn(p[0],p[1],s*.95):ART.plot(p[0],p[1]-r*.06,s*(1+R()*.15),FP[(i+Math.floor(R()*4))%4][0],FP[(i+Math.floor(R()*4))%4][1])})}
    else if(t==='scrub'){scatter(R,cx,cy,r,7,.21).forEach(function(p,i){o+=i===3||i===6?ART.acacia(p[0],p[1],s*(.8+R()*.25)):i%3===1?ART.tuft(p[0],p[1],s*(.85+R()*.3)):ART.bush(p[0],p[1],s*(.8+R()*.35))})}
    else if(t==='badlands'){o+=ART.mesa(cx-.12*r,cy+.02*r,s*1.05);scatter(R,cx,cy+.3*r,r*.9,3,.34).forEach(function(p,i){o+=i===2?ART.rock(p[0],p[1],s):ART.hoodoo(p[0],p[1],s*(.8+R()*.35))})}
    else if(t==='waste'){scatter(R,cx,cy,r,2,.5).forEach(function(p){o+=ART.crack(p[0],p[1]-r*.1,s*(1+R()*.3),'#3b3532')});scatter(R,cx,cy,r,5,.26).forEach(function(p,i){o+=i%2?ART.rubble(p[0],p[1],s*(.8+R()*.3)):ART.snag(p[0],p[1],s*(.8+R()*.35))})}
    else if(t==='volcanic'){o+=ART.cone(cx+(R()-.5)*.14*r,cy+.12*r,s*1.12);scatter(R,cx,cy+.42*r,r*.8,3,.3).forEach(function(p,i){o+=i===2?ART.snag(p[0],p[1],s*.7,'#2b2222'):ART.lava(p[0],p[1],s*(.8+R()*.3))})}
    else if(t==='coast'){var cs2=scatter(R,cx,cy-.2*r,r*.8,2,.5);cs2.forEach(function(p,i){o+=ART.sand(p[0],p[1],s*(1+R()*.2));if(i===0)o+=R()<.6?ART.palm(p[0]-r*.1,p[1]-r*.06,s*.8):ART.rock(p[0]+r*.1,p[1]-r*.08,s)});scatter(R,cx,cy+.42*r,r*.8,2,.4).forEach(function(p){o+=ART.wave(p[0],p[1],s*(.85+R()*.3))})}
    else if(t==='deep'){scatter(R,cx,cy,r,4,.34).forEach(function(p,i){o+=i===1&&R()<.4?ART.fluke(p[0],p[1],s):ART.swell(p[0],p[1]-r*.1,s*(.9+R()*.3))})}
    else if(t==='ice'){scatter(R,cx,cy,r,2,.5).forEach(function(p){o+=ART.crack(p[0],p[1]-r*.1,s*(1+R()*.3),'#6f9fbd')});scatter(R,cx,cy,r,4,.3).forEach(function(p){o+=ART.shard(p[0],p[1],s*(.7+R()*.5))})}
    else if(t==='cavern'){scatter(R,cx,cy,r,6,.24).forEach(function(p,i){o+=i%3===1?ART.shroom(p[0],p[1],s*(.8+R()*.4),R()<.5?'#7fe0d0':'#b58cf0'):i===5?ART.shard(p[0],p[1],s*.75,'#e0c8ff','#9a6fe0'):ART.spike(p[0],p[1],s*(.8+R()*.5))})}
    return o}

  // ---- Realistic: noise, height and colour per terrain ----

  function hsh(x,y,s){var h=(Math.imul(x,374761393)+Math.imul(y,668265263)+Math.imul(s,1442695041))|0;h=Math.imul(h^(h>>>13),1274126177);return ((h^(h>>>16))>>>0)/4294967296}
  function vnoise(x,y,s){var xi=Math.floor(x),yi=Math.floor(y),xf=x-xi,yf=y-yi,u=xf*xf*(3-2*xf),v=yf*yf*(3-2*yf),a=hsh(xi,yi,s),b=hsh(xi+1,yi,s),c=hsh(xi,yi+1,s),d=hsh(xi+1,yi+1,s);return a+(b-a)*u+(c-a)*v+(a-b-c+d)*u*v}
  function fbm(x,y,o,s){var t=0,a=.5,f=1,n=0;for(var i=0;i<o;i++){t+=a*vnoise(x*f,y*f,s+i*7);n+=a;a*=.5;f*=2.03}return t/n}
  function ridged(x,y,o,s){var t=0,a=.5,f=1,n=0;for(var i=0;i<o;i++){var v=1-Math.abs(vnoise(x*f,y*f,s+i*11)*2-1);t+=a*v*v;n+=a;a*=.5;f*=2.1}return t/n}
  function lerpC(a,b,t){return [a[0]+(b[0]-a[0])*t,a[1]+(b[1]-a[1])*t,a[2]+(b[2]-a[2])*t]}
  function ramp(st,t){t=Math.max(0,Math.min(1,t));for(var i=1;i<st.length;i++)if(t<=st[i][0]){var p=st[i-1],q=st[i];return lerpC(p[1],q[1],(t-p[0])/(q[0]-p[0]))}return st[st.length-1][1]}
  /* Worley (cell) noise: distance to the nearest scattered point. Used as packed domes for tree canopies and moss clumps. Returns [distance, cell id]. */
  function worley(x,y,s){var xi=Math.floor(x),yi=Math.floor(y),best=9,id=0;for(var j=-1;j<=1;j++)for(var i=-1;i<=1;i++){var cx=xi+i,cy=yi+j,px=cx+hsh(cx,cy,s),py=cy+hsh(cx,cy,s+1),d=(px-x)*(px-x)+(py-y)*(py-y);if(d<best){best=d;id=hsh(cx,cy,s+2)}}return [Math.sqrt(best),id]}
  function warp(x,y,s,a){return [x+(fbm(x/22,y/22,3,s)-.5)*a,y+(fbm(x/22+9,y/22+4,3,s+5)-.5)*a]}
  /* Each terrain gives a height (lit from the top left) and a colour. Heights use map coordinates, so the same terrain on neighbouring hexes is one continuous surface. */
  var RT={
    forest:{relief:9,h:function(x,y){var w=worley(x/2.6,y/2.6,81),c=Math.max(0,1-w[0]*1.25);return Math.sqrt(c)*.8+fbm(x/14,y/14,2,83)*.2},
      c:function(h,x,y){var w=worley(x/2.6,y/2.6,81),v=w[1],base=v<.33?[68,112,54]:v<.66?[90,126,58]:[58,100,64],gap=Math.min(1,w[0]*1.25);var c=lerpC(base,[30,50,28],gap*gap*.7);return lerpC(c,[98,118,58],fbm(x/30,y/30,2,85)*.25)}},
    mountain:{relief:15,h:function(x,y,d){var p=warp(x,y,11,10),c=Math.max(0,1-d*1.02);return ridged(p[0]/34,p[1]/34,5,11)*.34+fbm(x/8,y/8,3,5)*.06+c*c*(3-2*c)*.72},
      c:function(h,x,y){var sn=.8+fbm(x/6,y/6,2,3)*.08;if(h>sn)return lerpC([196,204,216],[226,230,236],Math.min(1,(h-sn)*6));return ramp([[0,[96,112,64]],[.18,[110,112,76]],[.36,[118,106,88]],[.58,[138,130,120]],[.72,[166,160,152]]],h)}},
    hills:{relief:20,h:function(x,y,d){var c=Math.max(0,1-d);return fbm(x/20,y/20,4,21)*.5+c*c*.45},
      c:function(h,x,y){var g=vnoise(x*1.2,y*.25,23)*.12;return ramp([[0,[92,118,52]],[.45,[126,140,64]],[.8,[160,152,84]],[1,[176,160,100]]],h+g-.06)}},
    plains:{relief:3,h:function(x,y){return fbm(x/12,y/12,4,61)*.7+vnoise(x*1.4,y*.3,62)*.3},
      c:function(h,x,y){var c=ramp([[0,[110,138,56]],[.55,[146,166,78]],[1,[182,186,104]]],h),sp=hsh(Math.floor(x*2),Math.floor(y*2),3);return sp>.985?[236,226,140]:sp>.975?[244,244,236]:c}},
    swamp:{relief:4,h:function(x,y){var p=warp(x,y,71,14),m=fbm(p[0]/16,p[1]/16,4,77);return m>.53?.2:.4+worley(x/1.8,y/1.8,79)[0]*-.25+fbm(x/6,y/6,2,73)*.2},
      c:function(h,x,y){var p=warp(x,y,71,14),m=fbm(p[0]/16,p[1]/16,4,77);if(m>.53){var c=ramp([[.53,[64,84,70]],[.62,[44,66,62]],[1,[36,56,58]]],m);var gl=fbm(x/5,y/9,2,75);return gl>.62?lerpC(c,[150,170,160],(gl-.62)*1.6):c}
        var e=Math.min(1,(.53-m)*10);return lerpC([58,66,40],ramp([[0,[76,86,46]],[1,[104,104,60]]],h),e)}},
    desert:{relief:2.5,h:function(x,y){var w=fbm(x/36,y/36,3,31),u=(x*.82+y*.5)/9+w*5,f=u-Math.floor(u);var prof=f<.72?f/.72:(1-f)/.28;return prof*prof*(3-2*prof)*.45+w*.55},
      c:function(h,x,y){return ramp([[0,[212,172,108]],[.6,[226,192,130]],[1,[236,208,152]]],h+fbm(x/2,y/2,2,13)*.05)}},
    snow:{relief:10,h:function(x,y,d){var p=warp(x,y,41,8);return fbm(p[0]/20,p[1]/20,4,41)*.8+Math.max(0,1-d)*.2},c:function(h){return ramp([[0,[176,196,216]],[.5,[218,230,240]],[1,[250,252,254]]],h)}},
    water:{relief:1.2,h:function(x,y){return fbm(x/16,y/16,4,51)},
      c:function(h,x,y){var c=ramp([[0,[30,70,108]],[1,[70,122,160]]],h),rip=Math.sin((y+fbm(x/12,y/12,2,57)*20)/2.2);return rip>.95?lerpC(c,[214,232,244],(rip-.95)*9):c}}
  };

  // ---- Realistic (diorama tiles) ----

  var SUN=(function(){var l=[-.62,-.5,.6],n=Math.hypot(l[0],l[1],l[2]);return [l[0]/n,l[1]/n,l[2]/n]})();
  function sstep(a,b,x){var t=Math.max(0,Math.min(1,(x-a)/(b-a)));return t*t*(3-2*t)}
  var PIECES={
    forest:['Mixed woods','Pine forest','Oak grove','Birch wood','Autumn wood','Clearing','Old growth','Woodcutter’s camp','Forest stream','Fey wood','Blighted wood','Jungle'],
    mountain:['Peaks','Great peak','Ridge','Volcano','Snowy massif','Red mesa','Mountain pass','Crags','Mountain lake','Mine','Glacier','Foothills'],
    hills:['Rolling hills','Rocky hills','Terraced farms','Wooded hills','Standing stones','Barrow','Heather moor','Lone knoll','Sheep pasture','Quarry','Watchtower','Cliffs'],
    plains:['Meadow','Wildflowers','Farmland','Wheat','Lone tree','Scattered trees','Boulders','Pond','Savanna','Crossroads','Farmstead','Camp'],
    swamp:['Fen','Mangroves','Reed bed','Dead marsh','Lily ponds','Peat bog','Cypress swamp','Stilt hut','Sunken ruins','Toadstool mire','Wisp marsh','Island'],
    desert:['Dunes','Great dunes','Rocky desert','Oasis','Mesas','Salt flat','Cactus field','Canyon','Bones','Pyramid','Ruins','Nomad camp'],
    snow:['Snowfield','Snowy pines','Ice spikes','Frozen lake','Crevasses','Snowy hills','Tundra','Frost rocks','Snowed ruins','Igloo camp','Ice cave','Hot spring'],
    water:['Open water','Calm lake','Rough sea','Shallows','Reef','Sea rocks','Islet','Kelp','Whirlpool','Ice floes','Shipwreck','Sea stacks'],
    town:['Hamlet','Village','Town','Walled city','Capital','Castle','Watchtower','Temple','Ruins','Port','Fort','Wizard’s tower'],
    jungle:['Rainforest','Palm jungle','Bamboo grove','Giant trees','Jungle river','Vine thicket','Jungle clearing','Waterfall pool','Overgrown temple','Tree village','Flowering jungle','Jungle hills'],
    farmland:['Patchwork fields','Wheat fields','Orchard','Vineyard','Pasture','Rice paddies','Ploughed fields','Mill','Farm village','Barns','Hedgerows','Harvest'],
    scrub:['Dry grass','Thornbrush','Savanna trees','Rocky scrub','Dry riverbed','Waterhole','Termite mounds','Tall grass','Red earth','Herders’ camp','Lone baobab','Burnt scrub'],
    badlands:['Broken ground','Hoodoos','Mesa','Deep canyon','Buttes','Striped hills','Dry gulch','Boulder field','Petrified forest','Cliff dwellings','Bone yard','Bandit hideout'],
    waste:['Ash plain','Cracked earth','Dead trees','Rubble field','Tar pits','Battlefield','Bone field','Poison pools','Fallen city','Crater','Cursed ground','Scavenger camp'],
    volcanic:['Lava field','Cinder cones','Lava river','Lava lake','Obsidian spires','Ash dunes','Steam vents','Caldera','Basalt columns','Sulphur springs','Burnt forest','Fire temple'],
    coast:['Sandy beach','Rocky shore','Dunes','Sea cliffs','Tidal flats','Cove','Palm beach','Lagoon','Fishing village','Lighthouse','Wreck on the sand','River mouth'],
    deep:['Deep water','Swells','Abyss','Maelstrom','Sea serpent','Icebergs','Kraken','Weed sea','Drowned spires','Ghost ship','Storm','Lonely rock'],
    ice:['Ice sheet','Pack ice','Ice ridges','Blue ice','Ice spires','Crevasses','Meltwater lakes','Ice cliffs','Frozen ship','Ice tower','Hunters’ camp','Giant’s bones'],
    cavern:['Cave floor','Stalagmites','Mushroom grove','Underground lake','Crystal cave','Chasm','Lava vein','Glowworm cave','Dwarven hall','Underground river','Bone pit','Dark city']};
  /* Mix: a stable piece per hex, so repainting the same hex keeps its look. */
  function pieceOf(t,seed,v){if(v!=null&&v>=0)return v;var h=2166136261;for(var i=0;i<seed.length;i++){h^=seed.charCodeAt(i);h=Math.imul(h,16777619)}return ((h>>>0)%997)%12}
  /* Scene for one piece: ground kind, landforms, water and roads, then objects. Everything is in hex units (centre 0,0, corner distance 1). */
  function scene(t,v,R){
    var G={gb:t==='town'?'plains':t,peaks:[],mounds:[],obs:[],water:[],roads:[],fields:[],wl:.012,wcol:[40,86,120],snowline:.52,rock:'grey',snowy:t==='snow',bank:t==='water'||t==='deep',up:.7,dirt:0,tint:null},O=G.obs;
    function P(n,gap,inset,ok){var o=[],tries=0;while(o.length<n&&tries<n*80){tries++;var a=R()*Math.PI*2,d=Math.sqrt(R())*inset,px=Math.cos(a)*d*.86,py=Math.sin(a)*d;if((!ok||ok(px,py))&&o.every(function(q){return Math.hypot(q[0]-px,q[1]-py)>gap}))o.push([px,py])}return o}
    function pick(a){return a[Math.floor(R()*a.length)]}
    var GREEN=[[80,122,50],[98,132,56],[66,108,44],[112,128,58]],PINE=[[38,78,46],[46,88,50],[34,70,52]];
    function con(p,s,col){O.push({k:'c',x:p[0],y:p[1],r:(.085+R()*.035)*s,b:.05*s,t:(.34+R()*.14)*s,c:col||pick(PINE)})}
    function oak(p,s,col,flat){var t2=(.26+R()*.08)*s;O.push({k:'o',x:p[0],y:p[1],r:(.11+R()*.045)*s,b:t2*(flat?.75:.42),t:t2,c:col||pick(GREEN)})}
    function rock(p,s,col){O.push({k:'r',x:p[0],y:p[1],r:(.045+R()*.03)*s,t:(.05+R()*.03)*s,c:col||[150,142,128]})}
    function stem(p,rad,top,col){O.push({k:'s',x:p[0],y:p[1],r:rad,t:top,c:col})}
    function house(p,w,d,hg,roof,ang,rc,wc){O.push({k:'h',x:p[0],y:p[1],w:w,d:d,t:hg,rf:roof,a:ang,ca:Math.cos(ang),sa:Math.sin(ang),r:Math.hypot(w,d)/2,c:rc,wc:wc||[214,200,170]})}
    function blk(p,w,d,hg,ang,col,wc){house(p,w,d,hg,0,ang,col,wc||col)}
    function tower(p,rad,hg,roof,rc,wc){O.push({k:'tw',x:p[0],y:p[1],r:rad,t:hg,rf:roof,c:roof?rc:wc,wc:wc})}
    function wall(a,b,th,hg,wc){O.push({k:'w',x:(a[0]+b[0])/2,y:(a[1]+b[1])/2,ax:a[0],ay:a[1],bx:b[0],by:b[1],th:th,t:hg,r:Math.hypot(b[0]-a[0],b[1]-a[1])/2+th,c:wc,wc:wc})}
    function ring(n,rad,th,hg,wc,tw,gap){var pts=[];for(var i=0;i<n;i++){var a=Math.PI*2*i/n+Math.PI/n;pts.push([Math.cos(a)*rad,Math.sin(a)*rad*.95])}for(i=0;i<n;i++){if(gap===i)continue;wall(pts[i],pts[(i+1)%n],th,hg,wc)}if(tw)pts.forEach(function(q){tower(q,th*1.5,hg*1.5,tw.rf||0,tw.rc,wc)})}
    function tent(p,rad,top,col){O.push({k:'tn',x:p[0],y:p[1],r:rad,t:top,c:col})}
    function pyr(p,s,hg,col){O.push({k:'py',x:p[0],y:p[1],w:s,t:hg,r:s*.72,c:col})}
    function mush(p,rad,top,col){O.push({k:'m',x:p[0],y:p[1],r:rad,t:top,c:col,wc:[224,214,190]})}
    function glow(p,rad,col){O.push({k:'g',x:p[0],y:p[1],r:rad,t:.02,c:col})}
    function pond(p,rad,wob){G.water.push({x:p[0],y:p[1],r:rad,wob:wob==null?.25:wob})}
    function river(pts,wd){G.water.push({line:pts,w:wd})}
    function road(pts,wd,col){G.roads.push({line:pts,w:wd,c:col||[150,124,88]})}
    function field(p,w,d,ang,cols,wet){G.fields.push({x:p[0],y:p[1],w:w,d:d,ca:Math.cos(ang),sa:Math.sin(ang),cols:cols,wet:wet})}
    function peak(x,y,rad,a){G.peaks.push({x:x,y:y,r:rad,a:a})}
    function mound(x,y,a,s,sy){G.mounds.push({x:x,y:y,a:a,s:s,sy:sy||s})}
    /* A smooth cone of rock or, glossy, of ice or crystal; a tree with a bare trunk; a low bush. */
    function spire(p,rad,top,col){O.push({k:'c',x:p[0],y:p[1],r:rad,b:0,t:top,c:col,sp:1})}
    function shard(p,rad,top,col){O.push({k:'c',x:p[0],y:p[1],r:rad,b:0,t:top,c:col||[150,200,230],ice:1})}
    function palm(p,s,col){O.push({k:'pm',x:p[0],y:p[1],r:.1*s,t:(.2+R()*.1)*s,c:col||pick([[70,150,60],[90,160,60],[50,130,70]]),wc:[44,92,48]})}
    function bush(p,s,col){O.push({k:'o',x:p[0],y:p[1],r:(.04+R()*.02)*s,b:.01,t:(.05+R()*.02)*s,c:col})}
    function ribs(x0,y0,n,gap,span,hi,col){for(var i=0;i<n;i++){var a=Math.sin(i/(n-1)*Math.PI);stem([x0+i*gap,y0-span],.022,hi*(.5+.5*a),col);stem([x0+i*gap,y0+span],.022,hi*(.5+.5*a),col)}}
    var ROOF=[[150,72,52],[120,64,48],[92,86,84],[164,96,58],[70,74,92]],PLASTER=[[222,208,178],[206,190,160],[196,176,146],[230,222,204]];
    function town(n,inset,gap,s,avoid){P(n,gap,inset,avoid).forEach(function(p){var ang=pick([0,Math.PI/2,.3,-.3,Math.PI/2+.3]);house(p,(.16+R()*.07)*s,(.11+R()*.04)*s,(.06+R()*.03)*s,(.05+R()*.02)*s,ang,pick(ROOF),pick(PLASTER))})}
    function inHex(x,y,m){var ax=Math.abs(x);return ax<.866-m&&Math.abs(y)+ax*.57735<1-m}
    if(t==='forest'){G.gb='forest';
      if(v===0)P(30,.13,.9).forEach(function(p){R()<.58?con(p,1):oak(p,1)});
      else if(v===1)P(40,.11,.92).forEach(function(p){con(p,.9+R()*.3)});
      else if(v===2){P(9,.27,.85).forEach(function(p){oak(p,1.5+R()*.3)});P(5,.1,.9).forEach(function(p){rock(p,.8)})}
      else if(v===3){G.gb='meadow';P(30,.12,.9).forEach(function(p){oak(p,.8,pick([[150,170,80],[170,180,90],[128,158,70]]))})}
      else if(v===4){G.gb='autumn';P(30,.13,.9).forEach(function(p){R()<.25?con(p,1):oak(p,1,pick([[196,96,40],[214,150,50],[170,60,40],[200,176,70],[150,110,50]]))})}
      else if(v===5){G.gb='meadow';P(26,.13,.92,function(x,y){return Math.hypot(x,y)>.45}).forEach(function(p){R()<.5?con(p,1):oak(p,1)});P(3,.12,.3).forEach(function(p){rock(p,.7)})}
      else if(v===6)P(7,.32,.82).forEach(function(p){R()<.5?con(p,1.9):oak(p,1.8)});
      else if(v===7){G.gb='meadow';P(14,.13,.92,function(x,y){return Math.hypot(x,y)>.55}).forEach(function(p){con(p,1)});house([.08,-.05],.26,.17,.09,.07,0,[110,76,46],[124,92,60]);
        P(7,.09,.45,function(x,y){return Math.hypot(x-.08,y+.05)>.18}).forEach(function(p){stem(p,.025,.03,[150,112,70])});blk([-.24,.2],.22,.07,.045,.4,[132,96,60]);road([[-.9,.35],[-.3,.15],[.08,.05]],.08)}
      else if(v===8){river([[-.9,-.4],[-.3,-.1],[.2,.15],[.9,.45]],.16);P(28,.13,.92,function(x,y){return Math.abs(y-(x*.47+.03))>.17}).forEach(function(p){R()<.55?con(p,1):oak(p,1)})}
      else if(v===9){P(16,.15,.9).forEach(function(p){oak(p,1,pick([[60,110,90],[80,130,100],[100,90,140]]))});P(6,.17,.8).forEach(function(p){mush(p,.07+R()*.04,.12+R()*.06,pick([[190,50,50],[150,80,190],[220,120,60]]))});P(12,.06,.9).forEach(function(p){glow(p,.012,[190,255,200])})}
      else if(v===10){G.gb='blight';P(18,.14,.9).forEach(function(p){stem(p,.026,.24+R()*.14,[78,70,62])});P(5,.2,.85).forEach(function(p){blk(p,.2,.04,.03,R()*3,[96,84,70])});P(6,.15,.9).forEach(function(p){rock(p,.9,[104,98,92])})}
      else{G.gb='jungle';P(36,.11,.93).forEach(function(p){oak(p,.8+R()*.8,pick([[40,130,60],[60,150,50],[30,110,70],[90,160,60]]))})}}
    else if(t==='mountain'){G.gb='mountain';G.up=1.4;
      if(v===0){peak(-.38+R()*.1,-.2+R()*.12,.62,.95+R()*.15);peak(.36+R()*.1,-.12+R()*.12,.58,.8+R()*.15);peak((R()-.5)*.2,.22+R()*.1,.72,1.2+R()*.15)}
      else if(v===1){peak(0,.05,1,1.6);G.up=1.8}
      else if(v===2){for(var i=0;i<5;i++)peak(-.62+i*.31,-.32+i*.16+(R()-.5)*.1,.42,.75+R()*.35)}
      else if(v===3){G.rock='dark';G.snowline=9;peak(0,.05,.95,1.15);G.crater={x:0,y:-.05,r:.24};O.push({k:'g',x:0,y:-.05,r:.17,t:0,c:[255,120,40],lava:1});river([[-.04,.12],[-.14,.3],[-.06,.45],[-.2,.62],[-.16,.78],[-.3,.95]],.075);G.lavaRiver=1}
      else if(v===4){G.snowline=.22;peak(-.3,-.15,.7,1.1);peak(.3,-.05,.7,1.0);peak(0,.3,.7,1.15)}
      else if(v===5){G.rock='red';G.gb='redrock';G.mesa=1;peak(-.3,-.15,.48,.6);peak(.32,.12,.5,.75);peak(-.1,.45,.3,.45);G.up=1}
      else if(v===6){peak(-.5,-.05,.62,1.1);peak(.5,.05,.62,1.1);road([[0,-1],[.02,0],[-.02,1]],.09,[140,124,96])}
      else if(v===7){for(var c7=0;c7<9;c7++)peak((R()-.5)*1.3,(R()-.5)*1.4,.25+R()*.12,.55+R()*.4)}
      else if(v===8){peak(-.45,-.35,.6,1.1);peak(.5,-.25,.55,.95);peak(.0,-.6,.5,.9);pond([.0,.35],.33,.15);G.wl=.02}
      else if(v===9){peak(0,-.1,.85,1.2);blk([.1,.6],.22,.07,.15,0,[30,26,24],[40,34,30]);road([[.1,.68],[.3,.85],[.7,.9]],.06,[120,100,80])}
      else if(v===10){G.snowline=.3;peak(-.5,-.2,.6,1.1);peak(.5,-.2,.6,1.05);G.glacier=1}
      else{G.snowline=9;peak(-.35,-.1,.6,.55);peak(.35,.05,.6,.5);peak(0,.4,.5,.6);G.up=1;P(8,.15,.9).forEach(function(p){con(p,.8)})}}
    else if(t==='hills'){
      if(v<=1||v===6){P(4,.38,.62).forEach(function(p){mound(p[0],p[1],.22+R()*.16,.2+R()*.08)});if(v===6)G.gb='heather';P(v===1?10:5,.15,.85).forEach(function(p,i){v===1||i<2?rock(p,v===1?1.3:1):oak(p,.5)})}
      else if(v===2){G.gb='hills';mound(-.25,-.2,.3,.32);mound(.3,.15,.26,.3);G.terrace=1}
      else if(v===3){P(4,.38,.62).forEach(function(p){mound(p[0],p[1],.22+R()*.16,.2+R()*.08)});P(18,.13,.9).forEach(function(p){R()<.5?con(p,.8):oak(p,.8)})}
      else if(v===4){mound(0,0,.2,.4);for(var s4=0;s4<9;s4++){var a4=s4/9*Math.PI*2;O.push({k:'h',x:Math.cos(a4)*.32,y:Math.sin(a4)*.3,w:.07,d:.045,t:.15+R()*.06,rf:0,a:a4,ca:Math.cos(a4),sa:Math.sin(a4),r:.07,c:[150,148,140],wc:[132,130,124]})}}
      else if(v===5){mound(0,-.05,.26,.42,.2);blk([0,.2],.16,.06,.1,0,[120,116,108]);blk([0,.23],.07,.05,.06,0,[30,28,26])}
      else if(v===7){mound(0,0,.42,.38);oak([0,-.02],1.2)}
      else if(v===8){mound(-.25,-.1,.24,.32);mound(.3,.2,.2,.3);P(10,.08,.85).forEach(function(p){O.push({k:'o',x:p[0],y:p[1],r:.028,b:.02,t:.045,c:[232,228,214]})});G.fence=1}
      else if(v===9){mound(-.3,-.2,.26,.3);mound(.35,0,.22,.3);G.quarry={x:.05,y:.25,r:.3};P(4,.08,.2).forEach(function(p){blk([p[0]+.05,p[1]+.25],.06,.05,.04,R(),[170,166,156])})}
      else if(v===10){mound(0,0,.38,.36);tower([0,-.02],.09,.42,.12,[110,70,50],[170,160,140])}
      else{mound(-.1,-.2,.45,.42,.3);G.cliff=1}}
    else if(t==='plains'){
      if(v===0)P(11,.14,.88).forEach(function(p,i){i<2&&R()<.7?rock(p,1):oak(p,.45)});
      else if(v===1){G.flowers=1;P(5,.2,.85).forEach(function(p){oak(p,.4)})}
      else if(v===2){var cols=[[[150,160,70],[130,146,60]],[[176,150,80],[160,134,70]],[[110,140,60],[96,128,52]],[[122,92,60],[110,82,52]]];var ang=R()*.6-.3;for(var f=0;f<4;f++)field([(f%2?.25:-.25),(f<2?-.25:.25)],.48,.48,ang,pick(cols));road([[-1,.02],[1,-.02]],.05);road([[0,-1],[0,1]],.04)}
      else if(v===3){G.gb='wheat';field([0,0],2,2,.4,[[214,180,90],[196,162,76]])}
      else if(v===4)oak([0,0],2.1);
      else if(v===5)P(9,.2,.88).forEach(function(p){R()<.6?oak(p,.9):con(p,.9)});
      else if(v===6)P(18,.12,.92).forEach(function(p){rock(p,.9+R()*.9)});
      else if(v===7){pond([0,.05],.38,.3);P(8,.1,.92,function(x,y){return Math.hypot(x,y-.05)>.45}).forEach(function(p){oak(p,.45)})}
      else if(v===8){G.gb='savanna';P(5,.32,.85).forEach(function(p){oak(p,1.2,[110,124,58],true)});P(4,.2,.9).forEach(function(p){rock(p,.8,[176,150,110])})}
      else if(v===9){road([[-1,.3],[1,-.3]],.08);road([[-.3,-1],[.3,1]],.08);P(4,.25,.9,function(x,y){return Math.abs(y+x*.3)>.15&&Math.abs(x-y*.3)>.15}).forEach(function(p){oak(p,.6)})}
      else if(v===10){house([-.2,-.18],.32,.2,.11,.08,0,[150,72,52],[222,208,178]);house([.28,-.18],.38,.22,.13,.1,Math.PI/2,[120,64,48],[150,110,70]);field([-.05,.42],.9,.36,0,[[176,150,80],[160,134,70]]);G.fence=1}
      else{G.gb='dirt';P(6,.3,.75).forEach(function(p){tent(p,.1,.17,pick([[200,186,150],[170,60,50],[180,170,140]]))});glow([0,0],.04,[255,170,60]);P(4,.1,.15).forEach(function(p){rock(p,.4,[90,86,80])})}}
    else if(t==='swamp'){G.wcol=[46,66,58];
      if(v===0){G.fen=1;P(2,.4,.7).forEach(function(p){stem(p,.022,.26+R()*.1,[62,50,38])});P(4,.3,.8).forEach(function(p){oak(p,.5,[74,92,50])})}
      else if(v===1){G.gb='swampwater';P(16,.18,.9).forEach(function(p){oak(p,1,pick([[60,96,50],[50,86,46]]),true)})}
      else if(v===2){G.fen=1;G.reeds=.35}
      else if(v===3){G.fen=1;G.dark=1;P(10,.16,.9).forEach(function(p){stem(p,.024,.22+R()*.15,[70,60,50])})}
      else if(v===4){G.gb='swampwater';G.lily=1;P(3,.3,.8).forEach(function(p){oak(p,.5,[74,92,50])})}
      else if(v===5){G.fen=1;G.wcol=[70,52,34];G.gb='peat'}
      else if(v===6){G.gb='swampwater';P(14,.17,.9).forEach(function(p){con(p,1.1,[50,70,48])})}
      else if(v===7){G.gb='swampwater';P(4,.2,.8).forEach(function(p){stem(p,.012,.06,[80,64,44])});P(4,.25,.25).forEach(function(p){stem(p,.016,.1,[80,64,44])});house([0,0],.32,.22,.18,.1,.2,[100,80,50],[120,96,66]);G.reeds=.15}
      else if(v===8){G.gb='swampwater';[[-.3,-.2],[.25,-.25],[.1,.3],[-.25,.3]].forEach(function(p){blk(p,.09,.09,.14+R()*.18,0,[150,146,132])});wall([-.45,.05],[.15,.12],.08,.1,[140,136,124])}
      else if(v===9){G.fen=1;P(8,.16,.85).forEach(function(p){mush(p,.05+R()*.04,.08+R()*.06,pick([[200,170,110],[170,80,60],[220,210,180]]))})}
      else if(v===10){G.fen=1;P(14,.08,.9).forEach(function(p){glow(p,.015,[170,255,170])})}
      else{G.gb='swampwater';G.island={x:0,y:0,r:.45};oak([0,-.02],1.2,[74,104,50]);P(4,.1,.35).forEach(function(p){rock(p,.6)})}}
    else if(t==='desert'){
      if(v===0)P(3,.3,.75).forEach(function(p,i){i===0&&R()<.65?stem(p,.028,.17+R()*.05,[86,128,64]):rock(p,1,[176,140,100])});
      else if(v===1)G.dunes=2;
      else if(v===2){G.gb='rockdesert';P(14,.12,.92).forEach(function(p){rock(p,.8+R()*.8,[170,130,96])})}
      else if(v===3){pond([0,.05],.3,.2);P(7,.13,.62,function(x,y){return Math.hypot(x,y-.05)>.33}).forEach(function(p){oak(p,.9,[70,130,60],true)});G.oasis=1}
      else if(v===4){G.rock='red';G.mesa=1;peak(-.3,-.2,.36,.5);peak(.35,.25,.32,.42)}
      else if(v===5){G.gb='salt'}
      else if(v===6)P(12,.14,.9).forEach(function(p){stem(p,.025,.13+R()*.08,[86,128,64])});
      else if(v===7){G.gb='rockdesert';G.canyon=1}
      else if(v===8){for(var b8=0;b8<7;b8++){var bx=-.4+b8*.12;stem([bx,-.13],.026,.16+Math.sin(b8/6*Math.PI)*.14,[226,218,196]);stem([bx,.13],.026,.16+Math.sin(b8/6*Math.PI)*.14,[226,218,196])}blk([.5,0],.2,.15,.09,0,[226,218,196])}
      else if(v===9){pyr([0,-.02],.82,.55,[214,186,132]);G.up=.9}
      else if(v===10){[[-.3,-.1],[.2,-.3],[.3,.2],[-.1,.35]].forEach(function(p){blk(p,.1,.1,.16+R()*.16,0,[204,176,126])});wall([-.5,.2],[.0,.08],.08,.12,[196,168,120]);wall([.1,-.45],[.5,-.1],.08,.09,[196,168,120])}
      else{P(5,.3,.75).forEach(function(p){tent(p,.1,.17,pick([[180,90,60],[200,180,140],[120,80,60]]))});glow([0,0],.035,[255,170,60])}}
    else if(t==='snow'){
      if(v===0)P(7,.2,.8).forEach(function(p){con(p,1,[34,66,50])});
      else if(v===1)P(30,.12,.92).forEach(function(p){con(p,1,[34,66,50])});
      else if(v===2)P(9,.2,.85).forEach(function(p){O.push({k:'c',x:p[0],y:p[1],r:.06+R()*.03,b:0,t:.35+R()*.22,c:[150,200,230],ice:1})});
      else if(v===3){pond([0,0],.62,.12);G.ice=1}
      else if(v===4){G.crev=1}
      else if(v===5){P(4,.38,.62).forEach(function(p){mound(p[0],p[1],.1+R()*.08,.24+R()*.08)});P(4,.2,.85).forEach(function(p){con(p,.9,[34,66,50])})}
      else if(v===6){G.gb='tundra';P(5,.2,.85).forEach(function(p){rock(p,.8,[120,116,108])})}
      else if(v===7)P(12,.15,.9).forEach(function(p){rock(p,1.2+R()*.6,[110,106,100])});
      else if(v===8){[[-.3,-.1],[.2,-.3],[.3,.2]].forEach(function(p){blk(p,.1,.1,.15+R()*.15,0,[160,156,150])});wall([-.5,.28],[.1,.22],.08,.1,[150,146,140]);G.snowy=1}
      else if(v===9){P(4,.34,.62).forEach(function(p){O.push({k:'o',x:p[0],y:p[1],r:.13,b:0,t:.11,c:[214,226,238]});blk([p[0],p[1]+.13],.06,.05,.05,0,[30,36,46])});glow([0,0],.04,[255,170,60])}
      else if(v===10){mound(0,-.12,.3,.36);blk([0,.24],.2,.06,.14,0,[24,30,40])}
      else{pond([0,.05],.35,.25);G.wcol=[60,150,170];G.steam=1;P(6,.12,.9,function(x,y){return Math.hypot(x,y-.05)>.42}).forEach(function(p){rock(p,.8,[110,106,100])})}}
    else if(t==='water'){
      if(v===1)G.calm=1;else if(v===2)G.rough=1;else if(v===3){G.wcol=[70,150,160];G.shallow=1}
      else if(v===4){G.wcol=[60,140,160];G.reef=1}
      else if(v===5)P(6,.18,.8).forEach(function(p){rock(p,1.5,[96,96,92])});
      else if(v===6){G.island={x:0,y:0,r:.35};oak([.02,-.02],1);rock([-.12,.08],.6)}
      else if(v===7)G.kelp=1;else if(v===8)G.whirl=1;
      else if(v===9)G.floes=1;
      else if(v===10){blk([-.05,0],.42,.13,.07,.5,[96,70,46],[80,58,38]);blk([.32,.25],.16,.12,.05,.9,[90,66,44]);stem([.0,-.03],.02,.3,[90,66,44]);stem([-.2,-.12],.016,.16,[90,66,44])}
      else if(v===11)P(5,.25,.8).forEach(function(p){stem(p,.06,.25+R()*.2,[130,120,104])})}
    else if(t==='town'){G.gb='townground';
      if(v===0){town(4,.62,.3,1);field([.3,.4],.5,.3,.2,[[176,150,80],[160,134,70]]);road([[-1,.1],[1,-.1]],.06)}
      else if(v===1){road([[-1,.15],[1,-.1]],.07);road([[.05,-1],[-.05,1]],.06);town(9,.8,.24,1,function(x,y){return Math.abs(y-.15+x*.12)>.1&&Math.abs(x+y*.05)>.09});stem([.12,.05],.025,.03,[120,116,110])}
      else if(v===2){road([[-1,.1],[1,-.05]],.07);road([[.0,-1],[.0,1]],.07);town(20,.86,.17,1,function(x,y){return Math.abs(y-.1+x*.07)>.08&&Math.abs(x)>.08});house([-.3,-.45],.3,.2,.16,.1,0,[80,82,100],[210,200,180])}
      else if(v===3){ring(8,.82,.05,.1,[160,152,140],{rf:.06,rc:[90,90,100]});road([[0,-1],[0,1]],.06);town(22,.7,.16,1,function(x,y){return Math.abs(x)>.07})}
      else if(v===4){ring(8,.84,.05,.11,[176,168,152],{rf:.07,rc:[60,70,110]});town(14,.74,.17,1,function(x,y){return Math.hypot(x,y)>.3});blk([0,0],.32,.27,.18,0,[170,162,146]);[[-.16,-.13],[.16,-.13],[-.16,.13],[.16,.13]].forEach(function(q){tower(q,.06,.3,.12,[60,70,110],[176,168,152])});tower([0,0],.09,.46,.16,[60,70,110],[186,178,162])}
      else if(v===5){mound(0,0,.2,.45);ring(6,.5,.06,.12,[150,144,132],{rf:0});blk([0,0],.3,.26,.22,0,[156,150,138]);tower([.08,-.06],.07,.4,0,null,[156,150,138]);house([-.25,.25],.2,.13,.07,.05,.3,[120,64,48],[200,186,160])}
      else if(v===6){G.gb='plains';tower([0,0],.11,.55,.14,[110,70,50],[170,160,140]);ring(6,.25,.03,.05,[150,140,120],null,2)}
      else if(v===7){house([0,-.04],.56,.38,.18,.1,0,[150,90,60],[226,220,204]);for(var c7b=0;c7b<7;c7b++)stem([-.25+c7b*.083,.22],.024,.18,[236,230,214]);road([[0,.26],[0,1]],.14,[200,190,170])}
      else if(v===8){G.gb='plains';[[-.35,-.2],[.25,-.3],[.3,.25],[-.1,.35],[0,0]].forEach(function(p){blk(p,.11,.1,.1+R()*.16,R(),[150,144,132])});wall([-.55,.1],[-.05,-.05],.08,.12,[140,134,124]);wall([.05,-.55],[.55,-.2],.08,.08,[140,134,124]);P(4,.15,.9).forEach(function(p){oak(p,.5)})}
      else if(v===9){G.port=1;town(9,.82,.2,1,function(x,y){return y<.05&&inHex(x,y,.08)});for(var d9=0;d9<3;d9++)blk([-.4+d9*.38,.36],.07,.34,.02,0,[120,92,60]);[[-.2,.42],[.2,.5],[.55,.35]].forEach(function(p){blk(p,.2,.07,.04,0,[110,80,50]);stem([p[0],p[1]],.012,.16,[100,76,50])})}
      else if(v===10){G.gb='dirt';for(var s10=0;s10<4;s10++){var a10=[[-.4,-.4],[.4,-.4],[.4,.4],[-.4,.4]],b10=a10[(s10+1)%4];for(var k10=0;k10<12;k10++){var f10=k10/12;stem([a10[s10][0]+(b10[0]-a10[s10][0])*f10,a10[s10][1]+(b10[1]-a10[s10][1])*f10],.022,.08+R()*.02,[110,82,52])}}P(4,.24,.28).forEach(function(p){tent(p,.11,.13,[200,186,150])});tower([.4,-.4],.06,.22,0,null,[110,82,52])}
      else{G.gb='plains';mound(0,0,.12,.4);tower([0,0],.1,.75,.22,[70,50,120],[140,130,150]);glow([0,-.02],.012,[200,180,255]);G.up=1.1;P(6,.15,.9).forEach(function(p){con(p,.8)})}}

    else if(t==='jungle'){G.gb='jungle';var JG=[[40,130,60],[60,150,50],[30,110,70],[90,160,60],[24,96,52]],JR=[[-.9,-.5],[-.35,-.25],[-.1,.1],[.4,.2],[.9,.5]];
      if(v===0)P(38,.11,.93).forEach(function(p){oak(p,.8+R()*.8,pick(JG))});
      else if(v===1){P(30,.11,.92).forEach(function(p){palm(p,.9+R()*.6)});P(16,.1,.92).forEach(function(p){bush(p,1.5,pick(JG))})}
      else if(v===2){G.gb='meadow';P(26,.13,.92).forEach(function(p){for(var b2=0;b2<7;b2++)stem([p[0]+(R()-.5)*.1,p[1]+(R()-.5)*.1],.014,.2+R()*.2,pick([[112,164,64],[134,180,76],[92,146,60]]))})}
      else if(v===3){P(6,.36,.78).forEach(function(p){oak(p,2.3+R()*.4,pick(JG))});P(16,.1,.92).forEach(function(p){bush(p,1.4,pick(JG))})}
      else if(v===4){G.wcol=[78,116,92];river(JR,.2);P(36,.11,.93,function(x,y){return lineD(x,y,JR)[0]>.17}).forEach(function(p){oak(p,.8+R()*.7,pick(JG))})}
      else if(v===5)P(70,.075,.94).forEach(function(p){bush(p,2+R()*.8,pick([[24,82,46],[30,98,52],[20,72,52],[44,104,48]]))});
      else if(v===6){G.gb='meadow';P(30,.11,.93,function(x,y){return Math.hypot(x,y)>.5}).forEach(function(p){oak(p,.8+R()*.7,pick(JG))});P(5,.12,.36).forEach(function(p){bush(p,1,[60,150,50])})}
      else if(v===7){G.wcol=[64,150,150];G.rock='grey';G.snowline=9;peak(-.1,-.55,.5,.6);river([[-.12,-.5],[-.06,-.2],[.02,.1]],.08);pond([.05,.28],.3,.2);P(26,.11,.93,function(x,y){return Math.hypot(x-.05,y-.28)>.36&&Math.hypot(x+.1,y+.5)>.4}).forEach(function(p){oak(p,.8+R()*.6,pick(JG))})}
      else if(v===8){pyr([0,-.02],.62,.32,[150,150,124]);blk([0,-.02],.12,.12,.4,0,[134,134,112]);P(30,.11,.93,function(x,y){return Math.max(Math.abs(x),Math.abs(y+.02))>.4}).forEach(function(p){oak(p,.8+R()*.6,pick(JG))});P(5,.1,.3).forEach(function(p){bush(p,1.2,pick(JG))})}
      else if(v===9){G.gb='meadow';[[-.25,-.2],[.2,-.28],[.28,.18],[-.15,.25],[0,0]].forEach(function(p){house(p,.2,.15,.09,.11,R()*.6,[206,176,100],[150,116,74])});glow([.06,.14],.03,[255,170,60]);P(24,.11,.93,function(x,y){return Math.hypot(x,y)>.55}).forEach(function(p){oak(p,.8+R()*.6,pick(JG))})}
      else if(v===10)P(36,.11,.93).forEach(function(p){oak(p,.8+R()*.7,R()<.34?pick([[232,112,152],[242,164,72],[204,124,224],[244,222,96]]):pick(JG))});
      else{P(4,.4,.6).forEach(function(p){mound(p[0],p[1],.2+R()*.14,.22+R()*.08)});P(32,.11,.93).forEach(function(p){oak(p,.7+R()*.6,pick(JG))})}}
    else if(t==='farmland'){G.gb='plains';var FC=[[[150,160,70],[130,146,60]],[[176,150,80],[160,134,70]],[[110,140,60],[96,128,52]],[[122,92,60],[110,82,52]],[[214,180,90],[196,162,76]],[[134,152,64],[150,168,76]]],fa=R()*.5-.25,
        grid=function(n,sz,gapf,each){for(var a=0;a<n;a++)for(var b=0;b<n;b++){var u=(a-(n-1)/2)*sz,w2=(b-(n-1)/2)*sz;each([u*Math.cos(fa)-w2*Math.sin(fa),u*Math.sin(fa)+w2*Math.cos(fa)],sz*gapf)}};
      if(v===0)grid(4,.5,.9,function(p,z){field(p,z,z,fa+(R()<.5?0:Math.PI/2),pick(FC))});
      else if(v===1){G.gb='wheat';field([0,0],2,2,fa+.4,[[214,180,90],[196,162,76]]);road([[-1,.25],[-.2,.05],[1,-.3]],.05);P(5,.3,.8).forEach(function(p){O.push({k:'o',x:p[0],y:p[1],r:.05,b:.01,t:.07,c:[226,196,104]})})}
      else if(v===2){G.gb='meadow';grid(7,.24,1,function(p){if(inHex(p[0],p[1],.06))oak(p,.62,R()<.2?[226,150,150]:pick([[84,140,56],[100,150,60]]))})}
      else if(v===3){field([0,0],2,2,fa+.5,[[72,112,52],[156,126,86]]);mound(-.2,-.15,.14,.5);house([.3,.3],.24,.16,.09,.07,fa,[150,72,52],[222,208,178]);road([[.3,.42],[.4,1]],.05)}
      else if(v===4){G.gb='meadow';var pen=[[-.5,-.35],[.45,-.42],[.55,.3],[-.42,.4]];for(var f4=0;f4<4;f4++)wall(pen[f4],pen[(f4+1)%4],.014,.035,[124,96,62]);P(9,.1,.42).forEach(function(p){O.push({k:'o',x:p[0],y:p[1],r:.03,b:.02,t:.048,c:R()<.7?[234,230,216]:[110,80,56]})});P(4,.3,.9,function(x,y){return Math.hypot(x,y)>.7}).forEach(function(p){oak(p,.7)});house([-.62,-.05],.16,.12,.07,.05,.2,[120,64,48],[150,110,70])}
      else if(v===5){G.wl=.02;G.calm=1;grid(5,.4,.86,function(p,z){field(p,z,z,fa,R()<.3?[[116,160,84],[98,146,80]]:[[96,150,138],[84,138,130]],1)})}
      else if(v===6)grid(3,.66,.92,function(p,z){field(p,z,z,fa+(R()<.5?0:Math.PI/2),pick([[[122,92,60],[104,78,50]],[[140,106,70],[120,90,58]],[[112,84,58],[96,72,48]]]))});
      else if(v===7){grid(3,.66,.9,function(p,z){if(Math.hypot(p[0],p[1])>.3)field(p,z,z,fa,pick(FC))});mound(0,-.05,.1,.26);tower([0,-.08],.085,.36,.12,[120,70,50],[228,218,198]);house([.2,.12],.2,.14,.07,.05,fa,[150,72,52],[222,208,178]);road([[0,.05],[-.1,.5],[-.2,1]],.05)}
      else if(v===8){grid(4,.5,.9,function(p,z){if(Math.hypot(p[0],p[1])>.45)field(p,z,z,fa,pick(FC))});town(8,.4,.17,1);road([[-1,.1],[0,0],[1,-.15]],.06)}
      else if(v===9){G.gb='dirt';house([-.22,-.15],.42,.24,.14,.11,fa,[150,60,48],[150,110,70]);house([.3,.12],.34,.22,.12,.1,fa+Math.PI/2,[132,54,44],[140,104,66]);tower([.42,-.3],.08,.3,.07,[92,86,84],[190,184,170]);tower([.24,-.36],.07,.24,.06,[92,86,84],[190,184,170]);field([-.2,.5],.9,.3,fa,[[176,150,80],[160,134,70]]);P(6,.1,.3,function(x,y){return y>.1&&x<0}).forEach(function(p){O.push({k:'o',x:p[0],y:p[1],r:.04,b:.01,t:.06,c:[226,196,104]})})}
      else if(v===10){grid(3,.66,.84,function(p,z){field(p,z,z,fa,pick(FC))});for(var h10=-2;h10<=2;h10++)for(var g10=-9;g10<=9;g10++){var a10=(h10+.5)*.66*(h10%1?1:1),b10=g10*.1;if(Math.abs(h10+.5)>1.6)continue;[[a10,b10],[b10,a10]].forEach(function(q){var px=q[0]*Math.cos(fa)-q[1]*Math.sin(fa),py=q[0]*Math.sin(fa)+q[1]*Math.cos(fa);if(inHex(px,py,.04)&&R()<.85)bush([px,py],1.5,pick([[60,104,46],[74,118,50],[52,92,48]]))})}}
      else{field([0,0],2,2,fa+.3,[[222,198,120],[204,178,104]]);grid(5,.34,1,function(p){if(inHex(p[0],p[1],.08)&&R()<.8)O.push({k:'o',x:p[0]+(R()-.5)*.08,y:p[1]+(R()-.5)*.08,r:.045,b:.012,t:.062,c:[232,204,112]})});house([.5,-.4],.2,.14,.08,.06,fa,[150,72,52],[222,208,178])}}
    else if(t==='scrub'){G.gb='scrub';var SB=[[122,130,68],[104,116,58],[138,134,76]],SD=[[84,96,50],[96,104,56],[70,84,48]],SW=[[-1,-.3],[-.3,.05],[.3,-.1],[1,.25]];
      if(v===0){P(12,.17,.9).forEach(function(p){bush(p,1,pick(SB))});P(3,.3,.8).forEach(function(p){rock(p,.8,[176,150,110])})}
      else if(v===1)P(50,.085,.93).forEach(function(p){bush(p,1.2+R()*.5,pick(SD))});
      else if(v===2){P(6,.3,.82).forEach(function(p){oak(p,1.15+R()*.2,[110,124,58],true)});P(10,.14,.92).forEach(function(p){bush(p,1,pick(SB))})}
      else if(v===3){P(14,.13,.92).forEach(function(p){rock(p,.9+R(),[176,150,110])});P(9,.14,.92).forEach(function(p){bush(p,1,pick(SB))})}
      else if(v===4){road(SW,.24,[214,198,158]);P(8,.12,.92,function(x,y){return lineD(x,y,SW)[0]<.12}).forEach(function(p){rock(p,.6,[170,156,130])});P(18,.12,.92,function(x,y){return lineD(x,y,SW)[0]>.16}).forEach(function(p){bush(p,1.1,pick(SB))})}
      else if(v===5){G.wcol=[96,134,124];pond([0,.05],.27,.3);P(6,.2,.6,function(x,y){return Math.hypot(x,y-.05)>.34}).forEach(function(p){oak(p,1,[100,124,58],true)});P(12,.13,.93,function(x,y){return Math.hypot(x,y-.05)>.34}).forEach(function(p){bush(p,1,pick(SB))})}
      else if(v===6){P(10,.2,.85).forEach(function(p){spire(p,.04+R()*.02,.12+R()*.12,[180,124,84])});P(10,.13,.93).forEach(function(p){bush(p,1,pick(SB))})}
      else if(v===7){P(170,.03,.96).forEach(function(p){stem(p,.011,.045+R()*.05,pick([[212,186,100],[196,170,88],[224,200,116]]))})}
      else if(v===8){G.gb='redearth';P(14,.14,.92).forEach(function(p){bush(p,1.1,pick(SD))});P(5,.2,.9).forEach(function(p){rock(p,.9,[160,96,66])})}
      else if(v===9){G.gb='redearth';P(5,.3,.6).forEach(function(p){tent(p,.1,.15,pick([[190,110,70],[206,186,150],[130,90,66]]))});glow([0,0],.035,[255,170,60]);var cp=[[.42,-.3],[.66,-.2],[.62,.06],[.4,.02]];for(var f9=0;f9<4;f9++)wall(cp[f9],cp[(f9+1)%4],.012,.03,[124,96,62]);P(5,.06,.1).forEach(function(p){O.push({k:'o',x:p[0]+.52,y:p[1]-.1,r:.026,b:.018,t:.044,c:[150,110,80]})});P(8,.14,.93,function(x,y){return Math.hypot(x,y)>.6}).forEach(function(p){bush(p,1,pick(SD))})}
      else if(v===10){O.push({k:'o',x:0,y:-.02,r:.2,b:.34,t:.44,c:[110,130,60],wc:[156,128,98]});O.push({k:'s',x:0,y:-.02,r:.1,t:.3,c:[156,128,98]});P(10,.15,.93,function(x,y){return Math.hypot(x,y)>.4}).forEach(function(p){bush(p,1,pick(SB))})}
      else{G.burnt=1;P(14,.15,.9).forEach(function(p){stem(p,.016,.08+R()*.1,[40,36,34])});P(6,.2,.9).forEach(function(p){bush(p,1,pick(SB))})}}
    else if(t==='badlands'){G.gb='badland';var BR=[[196,124,80],[214,160,110],[176,104,70]];
      if(v===0){G.gully=.15;P(5,.25,.9).forEach(function(p){rock(p,1,[186,126,88])})}
      else if(v===1)P(15,.16,.86).forEach(function(p){spire(p,.04+R()*.025,.2+R()*.26,pick(BR))});
      else if(v===2){G.rock='red';G.mesa=1;peak(0,-.02,.66,.52);G.up=1}
      else if(v===3){G.canyon=1;G.gully=.03}
      else if(v===4){G.rock='red';G.mesa=1;[[-.42,-.3,.26,.5],[.3,-.38,.22,.38],[.45,.2,.28,.58],[-.2,.35,.24,.44],[0,-.02,.18,.3]].forEach(function(b){peak(b[0],b[1],b[2],b[3])});G.up=1}
      else if(v===5){G.gully=.02;P(5,.36,.62).forEach(function(p){mound(p[0],p[1],.2+R()*.14,.2+R()*.07)})}
      else if(v===6){var BW=[[-1,.3],[-.35,.0],[.2,.12],[1,-.25]];road(BW,.2,[226,204,160]);G.gully=.1;P(8,.12,.92,function(x,y){return lineD(x,y,BW)[0]<.1}).forEach(function(p){rock(p,.6,[190,160,130])})}
      else if(v===7){G.gully=.03;P(20,.13,.92).forEach(function(p){rock(p,1.3+R()*1.4,pick(BR))})}
      else if(v===8){G.gully=.03;P(12,.17,.9).forEach(function(p){stem(p,.03,.08+R()*.2,pick([[150,130,116],[170,146,124],[132,112,100]]))});P(6,.2,.85).forEach(function(p){blk(p,.24,.05,.04,R()*3,[156,134,118])})}
      else if(v===9){G.rock='red';G.mesa=1;peak(0,-.3,.75,.6);G.up=1;for(var c9=0;c9<6;c9++){var cx9=-.38+c9*.15,cy9=.2+Math.abs(c9-2.5)*-.03;blk([cx9,cy9],.11,.09,.1+(c9%2)*.07,0,[224,190,146]);blk([cx9,cy9+.05],.035,.02,.05,0,[60,40,30])}road([[0,.4],[.1,.7],[0,1]],.05,[226,204,160])}
      else if(v===10){G.gully=.03;ribs(-.42,-.05,8,.11,.15,.34,[232,224,202]);blk([.58,-.05],.2,.16,.1,0,[232,224,202]);blk([.66,-.05],.05,.1,.12,0,[60,50,44]);P(4,.2,.9,function(x,y){return Math.abs(y+.05)>.3}).forEach(function(p){rock(p,.9,pick(BR))})}
      else{G.gully=.04;for(var k11=0;k11<16;k11++){var a11=k11/16*Math.PI*2;if(k11===4)continue;stem([Math.cos(a11)*.36,Math.sin(a11)*.34],.024,.1+R()*.02,[110,82,52])}P(3,.2,.2).forEach(function(p){tent(p,.09,.12,pick([[150,60,50],[200,186,150]]))});tower([-.3,-.24],.055,.24,0,null,[110,82,52]);glow([.02,.02],.03,[255,170,60]);P(9,.14,.93,function(x,y){return Math.hypot(x,y)>.55}).forEach(function(p){rock(p,1.4+R(),pick(BR))})}}
    else if(t==='waste'){G.gb='waste';var WG=[[70,62,56],[84,76,70],[58,52,50]];
      if(v===0){P(5,.3,.6).forEach(function(p){mound(p[0],p[1],.03+R()*.03,.2+R()*.1)});P(4,.3,.85).forEach(function(p){rock(p,.9,[104,98,92])});P(3,.3,.85).forEach(function(p){stem(p,.02,.05+R()*.04,[70,62,56])})}
      else if(v===1)G.cracked=1;
      else if(v===2){P(16,.15,.9).forEach(function(p){stem(p,.022,.18+R()*.18,pick(WG))});P(4,.25,.85).forEach(function(p){blk(p,.24,.04,.03,R()*3,[84,74,66])})}
      else if(v===3)P(18,.13,.9).forEach(function(p){blk(p,.06+R()*.07,.05+R()*.05,.03+R()*.07,R()*3,pick([[140,134,126],[124,118,112],[156,150,140]]))});
      else if(v===4){G.wcol=[22,20,24];G.calm=1;[[-.3,-.2,.24],[.3,.1,.28],[-.1,.45,.17]].forEach(function(b){pond([b[0],b[1]],b[2],.3)});P(5,.2,.9,function(x,y){return Math.hypot(x+.3,y+.2)>.3&&Math.hypot(x-.3,y-.1)>.34}).forEach(function(p){stem(p,.016,.1+R()*.08,[226,218,196])})}
      else if(v===5){P(30,.09,.9).forEach(function(p){stem(p,.011,.08+R()*.1,[110,96,80])});P(12,.14,.85).forEach(function(p){O.push({k:'o',x:p[0],y:p[1],r:.034,b:.006,t:.02,c:pick([[170,50,44],[66,84,140],[190,166,84]])})});[[-.32,.12,[170,50,44]],[.26,-.2,[66,84,140]],[.05,.4,[170,50,44]]].forEach(function(b){stem([b[0],b[1]],.016,.32,[110,96,80]);blk([b[0]+.07,b[1]],.12,.016,.32,0,b[2],b[2]);blk([b[0]+.07,b[1]+.012],.13,.02,.2,0,[110,96,80])});G.pit={x:.3,y:.25,r:.2,d:.05,c:[70,64,60]}}
      else if(v===6){ribs(-.5,-.3,5,.09,.1,.2,[226,218,196]);ribs(.05,.25,6,.09,.11,.24,[226,218,196]);blk([.62,.25],.14,.12,.08,0,[226,218,196]);P(8,.14,.9).forEach(function(p){stem(p,.014,.03+R()*.03,[226,218,196])})}
      else if(v===7){G.wcol=[104,150,66];G.calm=1;[[-.28,-.15,.22],[.3,-.05,.2],[0,.4,.24]].forEach(function(b){pond([b[0],b[1]],b[2],.35)});P(10,.1,.9).forEach(function(p){glow(p,.022,[196,244,120])});P(7,.2,.9).forEach(function(p){stem(p,.02,.14+R()*.12,pick(WG))})}
      else if(v===8){var WS=[150,144,136];ring(7,.7,.06,.11,WS,{rf:0},2);[[-.3,-.2],[.22,-.3],[.3,.22],[-.12,.3],[0,-.02],[-.4,.15]].forEach(function(p){blk(p,.12,.1,.08+R()*.2,R(),pick([[150,144,136],[132,126,120]]))});tower([.05,.05],.08,.4,0,null,[140,134,126]);P(8,.1,.6).forEach(function(p){rock(p,.7,[124,118,112])})}
      else if(v===9){G.pit={x:0,y:0,r:.6,d:.14,c:[60,54,52]};glow([0,.02],.05,[255,150,70]);rock([0,.02],1.6,[70,64,62]);P(6,.2,.92,function(x,y){return Math.hypot(x,y)>.72}).forEach(function(p){rock(p,.8,[104,98,92])})}
      else if(v===10){G.tint=[104,64,128];for(var s10=0;s10<7;s10++){var c10=s10/7*Math.PI*2;O.push({k:'h',x:Math.cos(c10)*.34,y:Math.sin(c10)*.32,w:.07,d:.045,t:.17+R()*.08,rf:0,a:c10,ca:Math.cos(c10),sa:Math.sin(c10),r:.07,c:[62,56,70],wc:[50,44,58]})}glow([0,0],.06,[190,120,255]);P(8,.12,.9,function(x,y){return Math.hypot(x,y)>.5}).forEach(function(p){glow(p,.012,[190,120,255])})}
      else{P(5,.3,.55).forEach(function(p){tent(p,.13,.17,pick([[150,124,96],[170,110,80],[134,120,104]]))});glow([0,0],.045,[255,170,60]);P(6,.15,.9,function(x,y){return Math.hypot(x,y)>.55}).forEach(function(p){blk(p,.07+R()*.05,.06,.04+R()*.05,R()*3,[132,126,120])});P(3,.3,.85).forEach(function(p){stem(p,.02,.15,pick(WG))})}}
    else if(t==='volcanic'){G.gb='basalt';G.rock='dark';G.snowline=9;var VB=[[60,54,54],[78,70,68],[46,42,44]],VL=[[-.8,-.6],[-.3,-.3],[-.1,.05],[.3,.2],[.5,.55],[.9,.7]];
      if(v===0)G.cracks=.07;
      else if(v===1){G.cracks=.02;[[-.4,-.25,.36,.5],[.35,-.3,.3,.4],[.38,.25,.34,.52],[-.2,.32,.3,.42]].forEach(function(b){peak(b[0],b[1],b[2],b[3]);O.push({k:'g',x:b[0],y:b[1]-.01,r:.075,t:0,c:[255,120,40],lava:1})});G.up=1}
      else if(v===2){G.cracks=.025;G.lavaRiver=1;river(VL,.15)}
      else if(v===3){G.lavaRiver=1;pond([0,.03],.52,.2);[[-.12,-.05],[.2,.14],[.02,.26]].forEach(function(p){rock(p,1.6,[50,44,44])})}
      else if(v===4){G.cracks=.02;P(13,.17,.86).forEach(function(p){shard(p,.045+R()*.03,.24+R()*.3,pick([[44,38,54],[36,32,44],[58,48,66]]))})}
      else if(v===5){G.gb='ash';P(4,.3,.85).forEach(function(p){stem(p,.016,.1+R()*.1,[40,36,34])})}
      else if(v===6){G.tint=[196,176,72];G.steams=[];P(5,.34,.7).forEach(function(p){spire(p,.09,.1,[84,74,68]);O.push({k:'g',x:p[0],y:p[1],r:.035,t:0,c:[255,170,80]});G.steams.push([p[0],p[1]-.2,.34])})}
      else if(v===7){peak(0,.05,1.05,.62);G.crater={x:0,y:0,r:.5};O.push({k:'g',x:0,y:0,r:.38,t:0,c:[255,120,40],lava:1});G.up=1}
      else if(v===8){G.cracks=.015;P(60,.062,.62).forEach(function(p){stem(p,.034,.06+(1-Math.hypot(p[0],p[1])/.62)*.3+R()*.06,pick([[104,96,94],[126,116,112],[88,80,80]]))})}
      else if(v===9){G.tint=[204,184,76];G.wcol=[70,176,164];G.calm=1;[[-.25,-.12,.24],[.28,.02,.2],[-.02,.4,.2]].forEach(function(b){pond([b[0],b[1]],b[2],.3)});G.steams=[[-.25,-.3,.3],[.28,-.15,.26]]}
      else if(v===10){G.gb='ash';P(22,.13,.9).forEach(function(p){stem(p,.018,.12+R()*.2,pick([[34,30,30],[48,40,38]]))});P(7,.2,.9).forEach(function(p){glow(p,.012,[255,140,50])})}
      else{G.cracks=.02;G.lavaRiver=1;river([[-.9,.45],[-.3,.5],[.3,.5],[.9,.4]],.12);blk([0,-.1],.62,.44,.2,0,[96,82,82]);blk([0,-.1],.36,.26,.34,0,[116,98,96]);[[-.31,-.32],[.31,-.32],[-.31,.12],[.31,.12]].forEach(function(q){tower(q,.075,.4,.14,[190,64,40],[96,82,82])});blk([0,.13],.1,.03,.13,0,[255,150,60],[255,150,60]);road([[0,.14],[0,.42]],.1,[120,104,100])}}
    else if(t==='coast'){G.gb='coast';G.wcol=[58,138,168];G.wl=.012;G.shore={k:'line',y:.2};var sea=function(y){return function(px,py){return py<y}};
      if(v===0)P(4,.3,.5,sea(0)).forEach(function(p){rock(p,.5,[170,160,140])});
      else if(v===1){G.pebble=1;G.shore.y=.12;P(22,.12,.94).forEach(function(p){rock(p,1+R()*1.3,pick([[110,106,100],[132,126,116],[92,90,88]]))})}
      else if(v===2){G.shore.y=.42;G.duneland=1;P(16,.13,.9,sea(.2)).forEach(function(p){bush(p,.8,pick([[128,150,84],[150,160,96]]))})}
      else if(v===3){G.shore.y=.1;G.cliffs=.24;G.grass=.02;G.up=.9;P(6,.2,.9,sea(-.1)).forEach(function(p){bush(p,1,[96,128,58])});P(4,.2,.9,function(x,y){return y>.3}).forEach(function(p){rock(p,1.3,[120,114,104])})}
      else if(v===4){G.shore.y=.72;G.tidal=1;P(5,.25,.85,sea(.5)).forEach(function(p){rock(p,.6,[120,114,104])})}
      else if(v===5){G.shore.k='cove';G.grass=.24;P(10,.14,.93,function(x,y){return Math.hypot(x,(y-.2)*1.15)>.7&&y<.5}).forEach(function(p){oak(p,.7)})}
      else if(v===6){G.shore.y=.25;P(14,.13,.92,sea(.12)).forEach(function(p){palm(p,1+R()*.5)})}
      else if(v===7){G.shore.k='lagoon';G.wcol=[64,170,176];P(7,.16,.93,function(x,y){return Math.hypot(x*.9,y+.08)>.52&&y<.6}).forEach(function(p){palm(p,.9+R()*.4)})}
      else if(v===8){G.shore.y=.24;G.grass=.4;P(7,.2,.86,sea(.06)).forEach(function(p){house(p,.17,.12,.06,.05,pick([0,.3,-.3]),pick([[150,72,52],[120,64,48],[164,96,58]]),[214,200,170])});for(var d8=0;d8<2;d8++)blk([-.3+d8*.5,.36],.06,.3,.03,0,[120,92,60]);[[-.05,.5],[.42,.45],[-.5,.52]].forEach(function(p){blk(p,.17,.06,.035,R()*.6-.3,[110,80,50])})}
      else if(v===9){G.shore.k='head';G.cliffs=.12;G.grass=.05;tower([0,-.26],.085,.52,.1,[170,50,44],[236,232,224]);O.push({k:'g',x:0,y:-.26,r:.03,t:0,c:[255,230,140]});house([.2,-.12],.18,.12,.07,.05,.2,[150,72,52],[222,208,178]);G.up=1}
      else if(v===10){G.shore.y=.3;blk([-.05,.24],.5,.15,.09,.35,[96,70,46],[80,58,38]);stem([-.12,.2],.02,.34,[90,66,44]);stem([.1,.28],.016,.2,[90,66,44]);blk([.4,.02],.2,.05,.03,1.2,[110,80,50]);P(4,.2,.6,sea(0)).forEach(function(p){rock(p,.5,[170,160,140])})}
      else{G.shore.y=.5;G.grass=.3;G.mouth=[[-.2,-1],[-.1,-.5],[.08,-.1],[.02,.5]];P(14,.13,.93,function(x,y){return y<.2&&lineD(x,y,G.mouth)[0]>.25}).forEach(function(p){bush(p,1,[96,128,58])})}}
    else if(t==='deep'){G.wcol=[26,54,96];
      if(v===1)G.rough=1;
      else if(v===2)G.abyss=1;
      else if(v===3){G.whirl=1;G.abyss=.5}
      else if(v===4){var SS=[40,110,84];[[-.5,.12,.1,.1],[-.26,-.02,.12,.17],[0,-.1,.12,.2],[.24,-.03,.11,.15],[.46,.1,.09,.08]].forEach(function(b){O.push({k:'o',x:b[0],y:b[1],r:b[2],b:0,t:b[3],c:SS})});O.push({k:'o',x:-.68,y:.26,r:.08,b:.06,t:.15,c:[54,128,96]});stem([.62,.22],.03,.12,SS)}
      else if(v===5)[[-.3,-.2,.2,.42],[.32,.05,.16,.3],[-.05,.42,.12,.2],[.4,-.42,.09,.14]].forEach(function(b){shard([b[0],b[1]],b[2],b[3],[214,234,246])});
      else if(v===6){G.abyss=.6;O.push({k:'o',x:0,y:0,r:.2,b:0,t:.16,c:[150,60,80]});for(var k6=0;k6<7;k6++){var a6=k6/7*Math.PI*2+.3,d6=.42+R()*.2;spire([Math.cos(a6)*d6,Math.sin(a6)*d6*.9],.045,.2+R()*.2,[170,74,92])}}
      else if(v===7)G.sarg=1;
      else if(v===8){[[-.3,-.2,.07,.34],[.2,-.3,.06,.26],[.32,.2,.07,.4],[-.1,.3,.055,.2]].forEach(function(b){tower([b[0],b[1]],b[2],b[3],.1,[70,110,110],[128,140,136])});blk([-.5,.1],.2,.07,.05,.3,[128,140,136]);blk([.05,0],.22,.2,.06,.2,[120,134,130])}
      else if(v===9){G.storm=1;var GH=[196,214,204];blk([0,0],.62,.19,.1,.3,GH,[160,180,172]);stem([-.15,-.06],.024,.46,GH);stem([.1,.03],.024,.4,GH);stem([.3,.09],.016,.22,GH);blk([-.15,-.06],.03,.26,.34,.3,[214,230,222]);blk([.1,.03],.03,.24,.3,.3,[214,230,222])}
      else if(v===10){G.rough=1;G.storm=1}
      else if(v===11){stem([0,0],.16,.2,[112,106,98]);P(5,.14,.3).forEach(function(p){rock(p,1.6,[96,92,88])});tower([0,-.02],.05,.5,.08,[150,60,50],[214,208,196]);O.push({k:'g',x:0,y:-.02,r:.02,t:0,c:[255,230,140]})}}
    else if(t==='ice'){G.gb='ice';var IC=[[170,214,238],[196,228,244],[140,196,230]];
      if(v===1)G.pack=1;
      else if(v===2){G.ridges=.42;G.up=.9}
      else if(v===3)G.blue=1;
      else if(v===4)P(16,.15,.88).forEach(function(p){shard(p,.045+R()*.035,.2+R()*.32,pick(IC))});
      else if(v===5)G.crev=1;
      else if(v===6){G.wcol=[60,176,206];G.calm=1;[[-.3,-.2,.24],[.28,.02,.2],[-.02,.42,.2],[.3,-.45,.12]].forEach(function(b){pond([b[0],b[1]],b[2],.3)})}
      else if(v===7){G.shelf=.26;G.up=.9;P(5,.2,.9,function(x,y){return y>.2}).forEach(function(p){shard(p,.05,.08+R()*.08,pick(IC))})}
      else if(v===8){blk([0,0],.5,.15,.1,.3,[96,70,46],[80,58,38]);stem([-.12,-.05],.02,.36,[90,66,44]);stem([.12,.03],.018,.26,[90,66,44]);P(8,.14,.6,function(x,y){return Math.hypot(x,y)>.3}).forEach(function(p){shard(p,.05,.1+R()*.1,pick(IC))})}
      else if(v===9){shard([0,-.02],.2,.7,[150,206,238]);for(var k9=0;k9<6;k9++){var a9=k9/6*Math.PI*2;shard([Math.cos(a9)*.36,Math.sin(a9)*.33],.07,.24+R()*.1,pick(IC))}glow([0,.22],.025,[150,220,255]);G.up=1}
      else if(v===10){P(4,.38,.55).forEach(function(p){O.push({k:'o',x:p[0],y:p[1],r:.15,b:0,t:.13,c:[236,242,250]});blk([p[0],p[1]+.15],.07,.05,.055,0,[30,36,46])});glow([0,0],.05,[255,170,60]);blk([.5,-.3],.26,.04,.05,.4,[110,80,50]);tent([-.5,.3],.1,.14,[150,110,80])}
      else if(v===11){ribs(-.45,-.02,8,.12,.17,.4,[232,236,236]);blk([.62,-.02],.22,.18,.12,0,[232,236,236]);blk([.71,-.02],.05,.11,.14,0,[60,70,84])}}
    else if(t==='cavern'){G.gb='cave';var CV=[[96,90,104],[112,104,116],[80,74,90]],CR=[[-.9,-.35],[-.3,-.1],[.15,.2],[.9,.4]];
      if(v===0){P(7,.22,.88).forEach(function(p){spire(p,.04+R()*.02,.08+R()*.1,pick(CV))});P(6,.18,.9).forEach(function(p){rock(p,1,pick(CV))})}
      else if(v===1)P(28,.115,.9).forEach(function(p){spire(p,.03+R()*.03,.12+R()*.32,pick(CV))});
      else if(v===2){P(13,.16,.86).forEach(function(p){mush(p,.055+R()*.05,.1+R()*.16,pick([[90,204,192],[150,112,224],[224,124,176]]))});P(14,.07,.9).forEach(function(p){glow(p,.02,[150,255,230])})}
      else if(v===3){G.wcol=[30,76,98];G.calm=1;pond([0,.06],.52,.22);P(12,.13,.93,function(x,y){return Math.hypot(x,y-.06)>.6}).forEach(function(p){spire(p,.035+R()*.02,.1+R()*.2,pick(CV))})}
      else if(v===4)P(15,.15,.88).forEach(function(p){shard(p,.055+R()*.04,.18+R()*.32,pick([[176,124,236],[112,204,236],[236,134,206]]))});
      else if(v===5){G.chasm=[[-.9,-.5],[-.3,-.15],[.05,.1],[.5,.2],[.9,.55]];P(8,.16,.9,function(x,y){return lineD(x,y,G.chasm)[0]>.2}).forEach(function(p){spire(p,.035+R()*.02,.1+R()*.14,pick(CV))})}
      else if(v===6){G.lavaRiver=1;river(CR,.11);P(9,.16,.9,function(x,y){return lineD(x,y,CR)[0]>.16}).forEach(function(p){spire(p,.035+R()*.02,.1+R()*.16,pick(CV))})}
      else if(v===7){P(50,.07,.92).forEach(function(p){glow(p,.018+R()*.012,[150,236,255])});P(7,.2,.88).forEach(function(p){spire(p,.04+R()*.02,.1+R()*.14,pick(CV))})}
      else if(v===8){road([[0,-1],[0,1]],.26,[122,116,124]);for(var k8=0;k8<5;k8++){stem([-.26,-.62+k8*.31],.045,.34,[140,134,142]);stem([.26,-.62+k8*.31],.045,.34,[140,134,142])}blk([0,-.5],.26,.14,.12,0,[150,144,150]);O.push({k:'g',x:0,y:-.5,r:.03,t:0,c:[255,170,60]});[[-.5,.0],[.5,.0]].forEach(function(p){blk(p,.16,.5,.2,0,[104,98,108])});G.up=.8}
      else if(v===9){G.wcol=[34,84,110];river(CR,.17);P(11,.15,.92,function(x,y){return lineD(x,y,CR)[0]>.18}).forEach(function(p){spire(p,.035+R()*.02,.1+R()*.2,pick(CV))})}
      else if(v===10){G.pit={x:0,y:0,r:.5,d:.12,c:[34,30,40]};ribs(-.22,0,5,.1,.1,.16,[226,218,196]);P(9,.1,.4).forEach(function(p){stem(p,.012,.02+R()*.03,[226,218,196])});P(7,.18,.93,function(x,y){return Math.hypot(x,y)>.66}).forEach(function(p){spire(p,.035,.1+R()*.12,pick(CV))})}
      else{var DC=[84,72,112];[[-.3,-.22,.07,.5],[.22,-.3,.06,.38],[.3,.2,.075,.6],[-.12,.3,.055,.3],[0,-.02,.09,.74],[-.45,.15,.05,.26]].forEach(function(b){tower([b[0],b[1]],b[2],b[3],.14,[84,60,120],DC);O.push({k:'g',x:b[0],y:b[1]+b[2]+.03,r:.03,t:0,c:[214,170,255]})});wall([-.6,.42],[.0,.55],.05,.09,DC);wall([.0,.55],[.6,.4],.05,.09,DC);G.up=1.1}}
    return G}
  /* Cell noise with the two nearest distances, for crack and plate edges. */
  function wor2(x,y,s){var xi=Math.floor(x),yi=Math.floor(y),b1=9,b2=9,id=0;for(var j=-1;j<=1;j++)for(var i=-1;i<=1;i++){var cx=xi+i,cy=yi+j,px=cx+hsh(cx,cy,s),py=cy+hsh(cx,cy,s+1),d=(px-x)*(px-x)+(py-y)*(py-y);if(d<b1){b2=b1;b1=d;id=hsh(cx,cy,s+2)}else if(d<b2)b2=d}return [Math.sqrt(b1),Math.sqrt(b2),id]}
  function segD(px,py,ax,ay,bx,by){var dx=bx-ax,dy=by-ay,l=dx*dx+dy*dy,t=l?Math.max(0,Math.min(1,((px-ax)*dx+(py-ay)*dy)/l)):0;return [Math.hypot(px-ax-dx*t,py-ay-dy*t),t]}
  function lineD(px,py,L){var best=9,al=0;for(var i=1;i<L.length;i++){var s=segD(px,py,L[i-1][0],L[i-1][1],L[i][0],L[i][1]);if(s[0]<best){best=s[0];al=i-1+s[1]}}return [best,al]}
  function iso3ModelJob(t,v,RQ){var cx=0,cy=0,r=TILE_R;
    var R=rng(t+'|'+v),G=scene(t,v,R);
    /* Rendered at 1.4x the display resolution and scaled down, which smooths every silhouette. */
    var Q=RQ*1.4,up=r*G.up,T=G.bank?0:r*.2,m=r*.04,W=Math.sqrt(3)*r+2*m,ox=cx-W/2,oy=cy-r-up,H=2*r+up+T+m,
      w=Math.ceil(W*Q),h=Math.ceil(H*Q),hw=Math.sqrt(3)/2*r,gx0=cx-hw,gy0=cy-r,gw=Math.ceil(2*hw*Q)+1,gh=Math.ceil(2*r*Q)+1,N=gw*gh,
      HP=new Float32Array(N),BC=new Float32Array(N*3),WC=new Float32Array(N*3),MT=new Uint8Array(N),IN=new Uint8Array(N),sc=30/r,k=r*Q,gb=G.gb;
    var LAVA=[255,128,40];
    function grassC(Xs,Ys,tf,n){var c=lerpC(RT.plains.c(n*.9+vnoise(Xs*1.4,Ys*.3,62)*.1,Xs,Ys),[96,120,58],.3+.25*sstep(.4,.7,fbm(Xs/6,Ys/6,3,64)));return lerpC(c,[c[0]*.8,c[1]*.85,c[2]*.7],1-tf)}
    /* Ground: height (hex units), colour and material before landforms, water, roads and objects. */
    function ground(p,q,X,Y,o){var Xs=X*sc,Ys=Y*sc,hh,c,mt=0,gw=null,n=fbm(Xs/12,Ys/12,4,61),tf=hsh(Math.floor(X*Q),Math.floor(Y*Q),3);
      if(gb==='plains'||gb==='meadow'||gb==='townground'||gb==='wheat'){hh=.02+n*.03+tf*.009;c=grassC(Xs,Ys,tf,n);if(gb==='meadow')c=lerpC(c,[150,170,84],.25);
        if(gb==='townground')c=lerpC(c,[164,146,114],sstep(.3,.6,fbm(Xs/5,Ys/5,3,66))*.85);
        if(G.flowers&&tf>.93)c=[[230,200,70],[220,110,150],[240,240,240],[160,120,220]][Math.floor(hsh(Math.floor(X*Q),Math.floor(Y*Q),8)*4)]}
      else if(gb==='savanna'){hh=.02+n*.025+tf*.008;c=lerpC([196,172,98],[168,150,80],fbm(Xs/6,Ys/6,3,64));c=lerpC(c,[c[0]*.82,c[1]*.82,c[2]*.75],1-tf)}
      else if(gb==='dirt'){hh=.02+n*.02;c=lerpC([150,124,88],[118,96,64],fbm(Xs/5,Ys/5,3,67));c=lerpC(c,[96,120,58],sstep(.6,.8,n)*.6)}
      else if(gb==='forest'||gb==='autumn'||gb==='blight'||gb==='jungle'){n=fbm(Xs/8,Ys/8,3,83);hh=.02+n*.02;c=gb==='autumn'?lerpC([110,74,40],[150,100,50],n):gb==='blight'?lerpC(lerpC([58,54,44],[92,84,64],fbm(Xs/3,Ys/3,3,84)),[44,52,40],sstep(.55,.7,n)*.7):gb==='jungle'?lerpC([40,70,30],[66,96,40],n):lerpC([44,58,30],[72,84,40],n)}
      else if(gb==='hills'||gb==='heather'){hh=.02+fbm(Xs/14,Ys/14,3,21)*.04+(tf-.5)*.008;c=null;mt=10}
      else if(gb==='mountain'||gb==='redrock'){hh=.02+fbm(Xs/10,Ys/10,3,5)*.03;c=gb==='redrock'?lerpC([190,130,84],[168,108,70],n):[104,116,70]}
      else if(gb==='desert'){var dh=RT.desert.h(Xs*.8,Ys*.8);hh=.015+dh*(G.dunes===2?.17:.07)+fbm(Xs/5,Ys/5,2,33)*.006;c=lerpC(RT.desert.c(dh,Xs,Ys),[196,150,96],sstep(.5,.8,fbm(Xs/10,Ys/10,3,34))*.35)}
      else if(gb==='rockdesert'){hh=.02+n*.02+tf*.006;c=lerpC([196,156,110],[170,128,90],fbm(Xs/4,Ys/4,3,35));if(tf>.9)c=[150,116,84]}
      else if(gb==='salt'){var wy=wor2(Xs/6,Ys/6,36);hh=.02+(wy[1]-wy[0]<.025?-.005:0);c=wy[1]-wy[0]<.025?[204,196,184]:lerpC([238,234,226],[222,216,206],n)}
      else if(gb==='snow'){hh=.02+RT.snow.h(Xs,Ys,9)*.16;c=[222,230,240];mt=4;
        if(G.crev){var cw=wor2(Xs/5,Ys/9,37);if(cw[1]-cw[0]<.028){hh-=.05;c=[60,96,130];mt=9}}}
      else if(gb==='tundra'){var sn=fbm(Xs/6,Ys/6,4,38);hh=.02+sn*.03;if(sn>.52){c=[222,230,240];mt=4}else c=lerpC([118,108,72],[140,128,86],tf)}
      else if(gb==='swamp'||gb==='peat'||gb==='swampwater'){var sp=warp(Xs,Ys,71,14),sm=fbm(sp[0]/16,sp[1]/16,4,77),th=gb==='swampwater'?.42:.53;
        if(sm>th){hh=G.wl;mt=3;c=lerpC(G.wcol,[G.wcol[0]*.8,G.wcol[1]*.8,G.wcol[2]*.85],Math.min(1,(sm-th)*6));
          if(G.lily){var lw=worley(Xs/1.2,Ys/1.2,39);if(lw[0]<.28&&hsh(Math.floor(Xs/1.2),Math.floor(Ys/1.2),40)>.4){hh=G.wl+.004;mt=1;c=lw[0]<.06?[230,200,220]:[70,120,50]}}}
        else{hh=.024+fbm(Xs/6,Ys/6,2,73)*.02;c=gb==='peat'?lerpC([70,60,40],[96,84,52],fbm(Xs/9,Ys/9,2,74)):lerpC([62,70,40],[96,98,56],fbm(Xs/9,Ys/9,2,74));if(G.dark)c=lerpC(c,[58,56,48],.6);
          /* Reeds: single-pixel spikes, densest at the water's edge. */
          var rd=hsh(Math.floor(X*Q),Math.floor(Y*Q),5),dens=G.reeds||.09;if(rd>1-dens*sstep(th-.09-(G.reeds?.2:0),th,sm)){hh+=.04+hsh(Math.floor(X*Q),Math.floor(Y*Q),6)*.06;c=[70,78,36];mt=1}}}

      else if(gb==='scrub'||gb==='redearth'){var sb2=fbm(Xs/5,Ys/5,3,92);hh=.02+n*.025+tf*.008;c=gb==='redearth'?lerpC([190,116,74],[162,98,64],sb2):lerpC([194,174,110],[160,150,90],sb2);if(sb2>.56)c=lerpC(c,[122,130,68],Math.min(1,(sb2-.56)*4));c=lerpC(c,[c[0]*.84,c[1]*.84,c[2]*.78],1-tf);
        if(G.burnt)c=lerpC(c,[42,38,36],sstep(.42,.56,fbm(Xs/7,Ys/7,3,93))*.92)}
      else if(gb==='badland'){hh=.02+ridged(Xs/11,Ys/11,4,94)*(G.gully||.06);c=null;mt=11}
      else if(gb==='waste'){var wz=wor2(Xs/5,Ys/5,95),ck=wz[1]-wz[0]<(G.cracked?.05:.012);hh=.02+n*.02-(ck?.007:0);c=lerpC([132,124,114],[100,94,90],fbm(Xs/6,Ys/6,3,96));if(G.tint)c=lerpC(c,G.tint,sstep(.38,.66,fbm(Xs/8,Ys/8,3,97))*.6);c=ck?[50,46,44]:lerpC(c,[c[0]*.88,c[1]*.88,c[2]*.88],1-tf)}
      else if(gb==='basalt'){var bz=wor2(Xs/5,Ys/5,98),lk=G.cracks&&bz[1]-bz[0]<G.cracks*(.5+fbm(Xs/9,Ys/9,2,99));hh=.025+n*.03+bz[2]*.014;c=lerpC([50,44,44],[86,76,72],fbm(Xs/4,Ys/4,3,100));if(G.tint)c=lerpC(c,G.tint,sstep(.45,.7,fbm(Xs/6,Ys/6,3,97))*.75);if(lk){c=LAVA;mt=6;hh-=.008}}
      else if(gb==='ash'){var ah=RT.desert.h(Xs*.8,Ys*.8);hh=.015+ah*.055;c=lerpC([104,100,98],[136,132,128],ah);c=lerpC(c,[68,64,62],sstep(.5,.8,fbm(Xs/10,Ys/10,3,101))*.4)}
      else if(gb==='ice'){var izs=G.pack?9:6,iz=wor2(Xs/izs,Ys/izs,102),ie=iz[1]-iz[0];hh=.03+fbm(Xs/10,Ys/10,3,103)*.03;mt=9;c=lerpC([170,208,234],[210,232,246],fbm(Xs/5,Ys/5,3,104));if(G.ridges)hh+=Math.pow(ridged(Xs/8,Ys/8,4,119),2)*G.ridges;if(G.shelf){var shf=sstep(.06,-.02,q+(vnoise(Xs/3,0,120)-.5)*.14);hh+=G.shelf*shf;gw=[120,184,224]}
        if(G.blue)c=lerpC(c,[104,172,212],.5+.4*fbm(Xs/8,Ys/8,2,107));else{var isn=fbm(Xs/6,Ys/6,3,108);if(isn>.56){c=lerpC(c,[244,248,252],Math.min(1,(isn-.56)*5));mt=4}}
        if(G.pack&&ie<.1){hh=0;mt=3;c=[26,58,92]}else if(ie<.022){c=lerpC(c,[140,186,214],.7);mt=9}
        if(G.crev){var cw2=wor2(Xs/5,Ys/9,37);if(cw2[1]-cw2[0]<.03){hh-=.05;c=[44,90,132];mt=9}}}
      else if(gb==='cave'){hh=.02+fbm(Xs/7,Ys/7,3,105)*.045+tf*.006;c=lerpC([74,70,90],[112,106,124],fbm(Xs/5,Ys/5,3,106));if(G.tint)c=lerpC(c,G.tint,sstep(.4,.7,fbm(Xs/6,Ys/6,3,97))*.5);mt=2}
      else if(gb==='coast'){var S=G.shore,wb2=(vnoise(Xs/4,Ys/9,110)-.5)*.14+(vnoise(Xs/1.4,Ys/3,111)-.5)*.03,sd;
        if(S.k==='cove')sd=Math.max(q-.62,.44-Math.hypot(p,(q-.2)*1.15))+wb2*.6;
        else if(S.k==='lagoon')sd=Math.max(q-.8,.4-Math.hypot(p*.9,q+.08),Math.abs(p-.12)<.06&&q>.2?.05:-9)+wb2*.5;
        else if(S.k==='head')sd=Math.hypot(p,(q+.22)*.9)-.56+wb2;
        else sd=q-S.y+wb2;
        if(G.mouth){var rm=(.05+Math.max(0,q+.3)*.2)-lineD(p,q,G.mouth)[0];if(rm>sd)sd=rm}
        if(sd>0){hh=G.wl;mt=3;c=lerpC([126,200,198],G.wcol,sstep(0,.24,sd));if(sd<.028+.012*Math.sin(Xs*1.3))c=[238,248,250]}
        else{var ld=-sd;hh=.016+Math.min(.05,ld*.12)+n*.008;c=lerpC([238,220,172],[216,196,146],fbm(Xs/5,Ys/5,3,112));
          if(G.pebble)c=lerpC(c,[146,140,132],.5+.35*tf);
          if(G.tidal){c=lerpC([170,150,114],[200,180,136],n);var tp=worley(Xs/3.2,Ys/3.2,113);if(tp[0]<.3&&tp[1]>.45&&ld<.75){hh=G.wl;mt=3;c=[116,176,180]}}
          c=lerpC([186,164,120],c,sstep(0,.07,ld));
          if(G.duneland){hh+=RT.desert.h(Xs*.9,Ys*.9)*.08*sstep(.05,.3,ld);c=lerpC(c,[150,164,100],sstep(.56,.76,fbm(Xs/4,Ys/4,3,114))*sstep(.1,.3,ld)*.8)}
          if(G.grass){var gf=sstep(G.grass,G.grass+.1,ld+(fbm(Xs/5,Ys/5,2,115)-.5)*.14);if(gf>0)c=lerpC(c,grassC(Xs,Ys,tf,n),gf)}
          if(G.cliffs){hh+=G.cliffs*sstep(0,.035,ld);gw=[158,148,132]}}}
      else{hh=0;mt=3;c=G.wcol.slice();if(G.floes){var fw=wor2(Xs/7,Ys/7,60);if(fw[2]>.45&&fw[1]-fw[0]>.07){hh=.025;c=[228,236,244];mt=9}}}
      o[0]=hh;o[1]=c;o[2]=mt;o[3]=gw}
    var gnd=[0,null,0,null];
    /* Height, top colour, wall colour and material of the whole surface at a point. */
    function surf(p,q,X,Y,o){var Xs=X*sc,Ys=Y*sc,i,d;ground(p,q,X,Y,gnd);var hh=gnd[0],c=gnd[1],mt=gnd[2],wc=gnd[3];
      if(G.island||G.port){var land=G.island?Math.hypot(p-G.island.x,q-G.island.y)<G.island.r*(1+.25*(vnoise(Xs/3,Ys/3,41)-.5)):q<.12+(vnoise(Xs/4,0,42)-.5)*.08;
        if(land){var n2=fbm(Xs/12,Ys/12,4,61),tf2=hsh(Math.floor(X*Q),Math.floor(Y*Q),3);hh=.03+n2*.02;c=G.port?lerpC(grassC(Xs,Ys,tf2,n2),[150,132,100],.6):grassC(Xs,Ys,tf2,n2);mt=0}
        else{hh=G.port?.0:G.wl;mt=3;c=G.wcol.slice()}}
      for(i=0;i<G.mounds.length;i++){var md=G.mounds[i];hh+=md.a*Math.exp(-((p-md.x)*(p-md.x)/(2*md.s*md.s)+(q-md.y)*(q-md.y)/(2*md.sy*md.sy)))}
      if(G.cliff&&q>-.1){var cf=sstep(.12,-.06,q+(vnoise(Xs/3,0,43)-.5)*.1);hh=.03+(hh-.03)*cf;if(cf<.98&&cf>.02)wc=G.cliffc||[132,124,112]}
      if(G.pit){d=Math.hypot(p-G.pit.x,q-G.pit.y)/G.pit.r+(vnoise(Xs/3,Ys/3,118)-.5)*.12;if(d<1){hh-=G.pit.d*(1-d*d);c=lerpC(G.pit.c,c||G.pit.c,d*d*.5);if(mt===11||mt===10)mt=0}else if(d<1.3)hh+=G.pit.d*.22*(1-(d-1)/.3)}
      if(G.chasm){d=lineD(p,q,G.chasm)[0]+(vnoise(Xs/3,Ys/3,117)-.5)*.07;if(d<.1){hh=-.34;c=[12,10,18];mt=6}else if(d<.14)wc=[40,36,48]}
      if(G.terrace&&hh>.06){var st=.055;hh=Math.floor(hh/st)*st+.01;var band=Math.floor(hh/st);c=band%2?[150,160,70]:[118,140,60]}
      if(G.quarry){d=Math.hypot(p-G.quarry.x,q-G.quarry.y);if(d<G.quarry.r){hh=Math.min(hh,-.02+d*.1);c=[160,156,146];mt=2;wc=[150,146,136]}}
      if(G.canyon){var cl=lineD(p,q,[[-1,-.3],[-.3,.05],[.3,-.1],[1,.25]])[0]+(vnoise(Xs/4,Ys/4,44)-.5)*.05;if(cl>.2){hh=.2+fbm(Xs/8,Ys/8,3,45)*.02;wc=[176,100,64]}else if(cl>.07){hh=.02+(cl-.07)/.13*.18;wc=[176,100,64];c=[190,120,76]}else{hh=.012;mt=3;c=[70,110,120]}}
      if(G.peaks.length){var b=0;for(i=0;i<G.peaks.length;i++){var e=G.peaks[i],dd=Math.hypot(p-e.x,(q-e.y)*1.1)/e.r;if(dd<1.15){var f=Math.max(0,1-dd);b=Math.max(b,G.mesa?e.a*sstep(0,.32,f+(vnoise(Xs/2,Ys/2,46)-.5)*.08):e.a*Math.pow(f,1.35))}}
        if(b>.01){var wp=warp(Xs,Ys,11,10);hh+=G.mesa?b+fbm(Xs/6,Ys/6,2,47)*.02*b:b*(.78+.4*ridged(wp[0]/12,wp[1]/12,5,11))+(ridged(wp[0]/5,wp[1]/5,3,12)-.5)*.1*Math.min(1,b*2);mt=7;c=c||[104,116,70]}
        if(G.crater){d=Math.hypot(p-G.crater.x,q-G.crater.y);if(d<G.crater.r){var fl=G.peaks[0].a*.78;if(hh>fl)hh=fl+(d/G.crater.r)*.03;if(d<G.crater.r*.72){c=LAVA;mt=6}}}
        var gwd=.2+q*.06+(vnoise(Ys/3,0,62)-.5)*.06;if(G.glacier&&Math.abs(p)<gwd&&q>-.7){var ge=Math.abs(p)/gwd,gl=.2-q*.13-ge*ge*.05+fbm(Xs/5,Ys/5,3,48)*.015;if(gl>hh){hh=gl;mt=9;c=lerpC([210,228,240],[170,200,222],sstep(.06,.18,Math.abs(p)*.2+fbm(Xs/2,Ys/8,2,49)*.15))}}}
      for(i=0;i<G.water.length;i++){var wb=G.water[i],inW=false;if(wb.line){inW=lineD(p,q,wb.line)[0]<wb.w/2*(1+.4*(vnoise(Xs/3,Ys/3,50)-.5))}else inW=Math.hypot(p-wb.x,q-wb.y)<wb.r*(1+wb.wob*(vnoise(Xs/4,Ys/4,51)-.5)*2);
        if(inW){if(G.lavaRiver){c=LAVA;mt=6;hh+=.004}else if(G.ice){hh=Math.min(hh,G.wl);mt=9;var iw=wor2(Xs/3,Ys/3,52);c=iw[1]-iw[0]<.04?[240,248,252]:lerpC([176,210,228],[200,224,238],iw[0])}else{hh=Math.min(hh,G.wl);mt=3;c=G.wcol.slice()}}}
      for(i=0;i<G.roads.length;i++){var rl=lineD(p,q,G.roads[i].line);if(rl[0]<G.roads[i].w/2*(1+.3*(vnoise(Xs/2,Ys/2,53)-.5))&&mt!==3&&mt!==6){var rc=G.roads[i].c;c=lerpC(rc,[rc[0]*.82,rc[1]*.82,rc[2]*.82],hsh(Math.floor(X*Q),Math.floor(Y*Q),4));hh-=.004;if(mt===10||mt===11)mt=0}}
      for(i=0;i<G.fields.length;i++){var fd=G.fields[i],dx0=p-fd.x,dy0=q-fd.y,fu=dx0*fd.ca+dy0*fd.sa,fv=-dx0*fd.sa+dy0*fd.ca;if(Math.abs(fu)<fd.w/2&&Math.abs(fv)<fd.d/2&&mt!==3&&mt!==7){var sb=Math.floor((fv+5)*28)%2;c=lerpC(fd.cols[sb],[fd.cols[sb][0]*.85,fd.cols[sb][1]*.85,fd.cols[sb][2]*.8],hsh(Math.floor(X*Q),Math.floor(Y*Q),9));hh+=sb*.004;if(fd.wet){mt=3;hh=G.wl}if(mt===10||mt===11)mt=0}}
      if(mt===10){c=lerpC(RT.hills.c(Math.min(1,hh/.42),Xs,Ys),[70,92,42],sstep(.45,.75,fbm(Xs/7,Ys/7,3,24))*.45);if(gb==='heather')c=lerpC(c,[124,86,112],sstep(.35,.6,fbm(Xs/5,Ys/5,3,25))*.7);mt=0}
      if(mt===11){var bb=Math.floor(hh*46+fbm(Xs/9,Ys/9,2,94)*2.2)%3,tb=.9+.1*hsh(Math.floor(X*Q),Math.floor(Y*Q),3);c=[[196,124,80],[218,166,114],[166,96,66]][bb];c=[c[0]*tb,c[1]*tb,c[2]*tb];mt=0}
      var gnd0=hh-.02;for(i=0;i<G.obs.length;i++){var ob=G.obs[i],dx=p-ob.x,dy=q-ob.y,d2=dx*dx+dy*dy;if(d2>=ob.r*ob.r*(ob.k==='w'?4:1))continue;var u=Math.sqrt(d2)/ob.r,th=-9,oc=ob.c,om=1,g0=gnd0;
        if(ob.k==='c'){var yy=1-u,tier=yy*3-Math.floor(yy*3);th=ob.b+(ob.t-ob.b)*(yy-(ob.ice||ob.sp?0:.08*tier))+(vnoise(X*Q*.9+ob.r*99,Y*Q*.9,8)-.5)*.012;if(ob.ice)om=9;else if(ob.sp)om=2}
        else if(ob.k==='o'){th=ob.b+(ob.t-ob.b)*Math.sqrt(1-u*u)*(.86+.14*worley(dx/ob.r*2.4+ob.x*9,dy/ob.r*2.4,9)[1]+.06*(vnoise(X*Q*.7+ob.x*9,Y*Q*.7,8)-.5))}
        else if(ob.k==='r'){th=ob.flat?ob.t:ob.t*Math.sqrt(Math.max(0,1-u*u))*(.8+.4*vnoise(dx/ob.r*2+ob.x*9,dy/ob.r*2,4));om=ob.flat?9:2}
        else if(ob.k==='s'){th=ob.t*(1-u*u*.3)}
        else if(ob.k==='pm'){if(u<.22||Math.abs(Math.sin(Math.atan2(dy,dx)*3+ob.x*40))>.42+u*.42)th=ob.t*(1-u*u*.55)}
        else if(ob.k==='h'){var lu=dx*ob.ca+dy*ob.sa,lv=-dx*ob.sa+dy*ob.ca;if(Math.abs(lu)<ob.w/2&&Math.abs(lv)<ob.d/2){th=ob.t+ob.rf*(1-Math.abs(lv)/(ob.d/2));om=8}}
        else if(ob.k==='tw'){th=ob.rf?ob.t+ob.rf*(1-u):ob.t+(u>.72&&Math.floor(Math.atan2(dy,dx)*4/Math.PI+8)%2?.018:0);om=8;oc=ob.rf?ob.c:ob.wc}
        else if(ob.k==='w'){var sg=segD(p,q,ob.ax,ob.ay,ob.bx,ob.by);if(sg[0]<ob.th/2){th=ob.t+(Math.floor(sg[1]*ob.r*2/(ob.th*.9))%2?.014:0);om=8}}
        else if(ob.k==='tn'){th=ob.t*(1-u);om=8}
        else if(ob.k==='py'){var mx=Math.max(Math.abs(dx),Math.abs(dy))/(ob.w/2);if(mx<1){th=ob.t*(1-mx);om=8}}
        else if(ob.k==='m'){th=ob.t*(.72+.28*Math.sqrt(1-u*u));om=8;if(worley(dx/ob.r*3,dy/ob.r*3,54)[0]<.18)oc=[240,236,226]}
        else if(ob.k==='g'){c=ob.c;mt=6;if(ob.lava)c=LAVA;continue}
        if(ob.k==='py')th=Math.floor(th/.035)*.035;th+=g0;
        if(th>hh){hh=th;mt=om;var vv=vnoise(X*Q*.35+ob.x*30,Y*Q*.35,7)*.6+vnoise(X*Q*1.2,Y*Q*1.2,17)*.4;c=om===8?[oc[0]*(.94+vv*.12),oc[1]*(.94+vv*.12),oc[2]*(.94+vv*.12)]:[oc[0]*(.88+vv*.24),oc[1]*(.88+vv*.24),oc[2]*(.88+vv*.24)];wc=ob.wc||null;if(G.snowy&&(om===1||(om===8&&ob.rf)))mt=5}}
      o[0]=hh;o[1]=c;o[2]=mt;o[3]=wc}
    var o=[0,null,0,null],maxH=0,j=0,i=0;
    function sample(until){for(var n=0;j<gh;j++){if(n++&&performance.now()>until)return false;for(i=0;i<gw;i++){var X=gx0+i/Q,Y=gy0+j/Q,p=(X-cx)/r,q=(Y-cy)/r,ii=j*gw+i,ap=Math.abs(p);if(ap>.8661||Math.abs(q)+ap*.57735>1.001)continue;
      IN[ii]=1;surf(p,q,X,Y,o);HP[ii]=o[0]*k;BC[ii*3]=o[1][0];BC[ii*3+1]=o[1][1];BC[ii*3+2]=o[1][2];MT[ii]=o[2];if(o[3]){WC[ii*3]=o[3][0];WC[ii*3+1]=o[3][1];WC[ii*3+2]=o[3][2]}else WC[ii*3]=-1;if(HP[ii]>maxH)maxH=HP[ii]}}return true}
    function hAt(i,j){i=Math.max(0,Math.min(gw-1,i));j=Math.max(0,Math.min(gh-1,j));var ii=j*gw+i;return IN[ii]?HP[ii]:-1}
    /* Ambient occlusion from a summed-area table: a point lower than its surroundings gets less sky. */
    var SA=new Float64Array((gw+1)*(gh+1)),SC=new Float64Array((gw+1)*(gh+1));
    function buildSat(){for(j=0;j<gh;j++)for(i=0;i<gw;i++){var a=(j+1)*(gw+1)+i+1,ii2=j*gw+i;SA[a]=(IN[ii2]?HP[ii2]:0)+SA[a-1]+SA[a-gw-1]-SA[a-gw-2];SC[a]=IN[ii2]+SC[a-1]+SC[a-gw-1]-SC[a-gw-2]}j=0}
    function boxAvg(i,j,rr){var x0=Math.max(0,i-rr),x1=Math.min(gw,i+rr+1),y0=Math.max(0,j-rr),y1=Math.min(gh,j+rr+1),A=function(T,x,y){return T[y*(gw+1)+x]},s=A(SA,x1,y1)-A(SA,x0,y1)-A(SA,x1,y0)+A(SA,x0,y0),n=A(SC,x1,y1)-A(SC,x0,y1)-A(SC,x1,y0)+A(SC,x0,y0);return n?s/n:0}
    var COL=new Uint8ClampedArray(N*3),LA=new Float32Array(N),AA=new Float32Array(N),WX=new Float32Array(N),sl=Math.hypot(SUN[0],SUN[1]),sdx=SUN[0]/sl,sdy=SUN[1]/sl,sdz=SUN[2]/sl,ar1=Math.round(Q*1.5),ar2=Math.round(Q*5);
    var wamp=G.calm?.3:G.rough?2.2:1,ROCK={grey:[[108,98,86],[164,152,136]],dark:[[56,50,48],[100,90,82]],red:[[160,86,52],[214,150,100]]}[G.rock];
    function light(until){for(var n=0;j<gh;j++){if(n++&&performance.now()>until)return false;for(i=0;i<gw;i++){var id=j*gw+i;if(!IN[id])continue;var hc=HP[id],hl=hAt(i-1,j),hr=hAt(i+1,j),hu=hAt(i,j-1),hd=hAt(i,j+1);if(hl<0)hl=hc;if(hr<0)hr=hc;if(hu<0)hu=hc;if(hd<0)hd=hc;
      var gx=(hr-hl)/2,gy=(hd-hu)/2,mt2=MT[id],X2=gx0+i/Q,Y2=gy0+j/Q,Xs2=X2*sc,Ys2=Y2*sc,c0=BC[id*3],c1=BC[id*3+1],c2=BC[id*3+2];
      if(mt2===3){var wv=Math.sin((Ys2+fbm(Xs2/9,Ys2/9,3,57)*14)/(G.rough?.9:1.3));
        if(G.whirl){var wpx=(X2-cx)/r,wpy=(Y2-cy)/r,wr=Math.hypot(wpx,wpy),wa=Math.atan2(wpy,wpx);wv=Math.sin(wa*3+wr*18);var dk2=sstep(.5,0,wr);c0*=1-.5*dk2;c1*=1-.45*dk2;c2*=1-.35*dk2}
        gx+=(fbm(Xs2/5,Ys2/7,3,57)-.5)*.35*wamp;gy+=((fbm(Xs2/5+5,Ys2/7,3,58)-.5)*.5+wv*.18)*wamp}
      var nl=1/Math.sqrt(gx*gx+gy*gy+1),nx=-gx*nl,ny=-gy*nl,nz=nl,dif=Math.max(0,nx*SUN[0]+ny*SUN[1]+nz*SUN[2]);
      /* Soft cast shadow: march toward the sun and keep the closest miss. */
      var lit=1,z=hc+.3,sx=i,sy=j;for(var st=1;st<90&&lit>0;st++){sx+=sdx*1.5;sy+=sdy*1.5;z+=sdz*1.5;if(z>maxH)break;var si=Math.round(sx),sj=Math.round(sy);if(si<0||sj<0||si>=gw||sj>=gh)break;var hs=HP[sj*gw+si];if(!IN[sj*gw+si])continue;lit=Math.min(lit,Math.max(0,1-(hs-z)/(Q*.6+st*.12)))}
      var occ=Math.max(0,boxAvg(i,j,ar1)-hc)*.7+Math.max(0,boxAvg(i,j,ar2)-hc)*.35,ao=Math.max(.35,1-occ/(Q*3.2));
      LA[id]=lit;AA[id]=ao;WX[id]=-Math.sign(hr-hl)*Math.min(1,Math.abs(hr-hl)/(Math.abs(hd-hu)+1));
      if(mt2===7){var hn=hc/maxH,slope=1-nz,nn=fbm(Xs2/5,Ys2/5,3,3);
        if(G.mesa){var bnd=Math.floor(hc/Q*.9+nn*1.5)%3,rc0=[[180,100,62],[204,138,90],[150,82,52]][bnd];if(nz>.8){rc0=[206,150,100]}c0=rc0[0];c1=rc0[1];c2=rc0[2]}
        else if(hn>G.snowline+nn*.16&&nz>.28){var sw=sstep(G.snowline+nn*.16,G.snowline+.08+nn*.16,hn)*sstep(.28,.42,nz+(ny<0?-ny*.3:0)),rk=ROCK[1];c0=rk[0]+(238-rk[0])*sw;c1=rk[1]+(242-rk[1])*sw;c2=rk[2]+(248-rk[2])*sw}
        else if(hn>.12||slope>.3){var band=Math.sin(hc/Q*1.3+nn*6)*.06,rc=lerpC(ROCK[0],ROCK[1],Math.min(1,hn*1.1+nn*.3));c0=rc[0]*(1+band);c1=rc[1]*(1+band);c2=rc[2]*(1+band);var gr2=G.rock==='grey'?sstep(.3,.08,hn)*sstep(.4,.15,slope):0;c0+=(96-c0)*gr2;c1+=(114-c1)*gr2;c2+=(64-c2)*gr2}}
      if(mt2===5){var snw=sstep(.15,.5,nz-ny*.6)*(.6+.4*vnoise(X2*Q*.6,Y2*Q*.6,12));c0+=(240-c0)*snw;c1+=(244-c1)*snw;c2+=(248-c2)*snw}
      if(mt2===3&&G.bank){var ap2=Math.abs((X2-cx)/r),q2=(Y2-cy)/r,dm=Math.min(.866-ap2,1-(-q2+ap2*.57735)),bk=.55+.45*sstep(0,.2,dm),dp=fbm(Xs2/14,Ys2/14,3,51);
        c0=(c0*.8+dp*30)*bk;c1=(c1*.85+dp*40)*bk;c2=(c2*.9+dp*44)*bk;
        if(G.shallow){var sa2=sstep(.4,.7,fbm(Xs2/6,Ys2/6,3,55));c0+=(200-c0)*sa2*.5;c1+=(190-c1)*sa2*.5;c2+=(150-c2)*sa2*.4}
        if(G.reef){var rw=worley(Xs2/6,Ys2/6,56);if(rw[0]<.3){var rcl=[[230,120,110],[240,170,90],[200,110,180],[120,200,170]][Math.floor(hsh(Math.floor(Xs2/6),Math.floor(Ys2/6),57)*4)];var rf2=sstep(.3,.12,rw[0])*.6;c0+=(rcl[0]-c0)*rf2;c1+=(rcl[1]-c1)*rf2;c2+=(rcl[2]-c2)*rf2}}
        if(G.kelp){var kf=sstep(.5,.65,fbm(Xs2/4,Ys2/8,3,58))*.7;c0+=(30-c0)*kf;c1+=(70-c1)*kf;c2+=(50-c2)*kf}
        if(G.sarg){var sf=sstep(.46,.58,fbm(Xs2/5,Ys2/5,3,116))*.85;c0+=(128-c0)*sf;c1+=(124-c1)*sf;c2+=(50-c2)*sf}
        if(G.abyss){var ab=sstep(.85,.1,Math.hypot(ap2,q2))*G.abyss;c0*=1-.8*ab;c1*=1-.75*ab;c2*=1-.6*ab}
        if(G.storm){c0*=.62;c1*=.64;c2*=.72}}
      BC[id*3]=c0;BC[id*3+1]=c1;BC[id*3+2]=c2;if(WC[id*3]<0){WC[id*3]=c0;WC[id*3+1]=c1;WC[id*3+2]=c2}
      if(mt2===6){var fl2=.85+.3*vnoise(X2*Q*.3,Y2*Q*.3,59);COL[id*3]=c0*fl2;COL[id*3+1]=c1*fl2;COL[id*3+2]=c2*fl2;continue}
      var amb=(mt2===4||mt2===9?.6:.48)*ao*(.55+.45*nz),sun=1.12*dif*lit,spec=0;
      if(mt2===3){var hx=SUN[0],hy=SUN[1],hz=SUN[2]+1,hl2=Math.hypot(hx,hy,hz);spec=Math.pow(Math.max(0,(nx*hx+ny*hy+nz*hz)/hl2),60)*lit*220;if(G.rough&&wv>.97)spec+=50;if(G.storm&&wv>.86)spec+=110}
      if(mt2===4||mt2===5)spec=Math.pow(dif,24)*lit*30;
      if(mt2===9)spec=Math.pow(dif,40)*lit*120;
      var gn=(hsh(i,j,7)-.5)*5;
      COL[id*3]=c0*(amb*.92+sun*1.06)+spec+gn;COL[id*3+1]=c1*(amb*.98+sun*1.0)+spec+gn;COL[id*3+2]=c2*(amb*1.12+sun*.86)+spec+gn}}return true}
    /* The model above is the costly part and does not depend on the neighbours; only the cut sides below do, so one model serves all four edge variants. */
    function make(nb){nb=nb||{};
    /* Draw front to back per column, each sample filling up to the last one drawn; the tile's cut sides are earth strata. */
    var cv=document.createElement('canvas');cv.width=w;cv.height=h;var x=cv.getContext('2d'),img=x.createImageData(w,h),D=img.data,offx=Math.round((gx0-ox)*Q),offy=(gy0-oy)*Q,Tp=T*Q;
    var SOIL={mountain:[108,100,90],redrock:[170,100,64],desert:[178,140,90],rockdesert:[170,130,92],salt:[200,190,176],snow:[186,198,212],tundra:[150,150,150],swamp:[78,66,44],peat:[70,56,40],swampwater:[78,66,44],scrub:[150,118,78],redearth:[150,92,60],badland:[166,98,64],waste:[82,76,72],basalt:[46,40,40],ash:[66,62,60],ice:[150,188,212],cave:[58,54,70],coast:[172,150,112]}[gb]||[104,80,52];
    function put(px,py,r0,g0,b0){var a=(py*w+px)*4;D[a]=r0;D[a+1]=g0;D[a+2]=b0;D[a+3]=255}
    for(i=0;i<gw;i++){var px=i+offx;if(px<0||px>=w)continue;var jf=-1;for(j=gh-1;j>=0;j--)if(IN[j*gw+i]){jf=j;break}if(jf<0)continue;
      var left=(gx0+i/Q)<cx,side=T&&!(left?nb.bl:nb.br),yb=Math.min(h,Math.floor(offy+jf+1+(side?Tp:0))),first=true;
      for(j=jf;j>=0;j--){var id3=j*gw+i;if(!IN[id3])break;var top=Math.floor(offy+j-HP[id3]);if(top<0)top=0;if(top>=yb)continue;
        if(first&&side){var lip=Math.floor(offy+j+1),fs=left?1:.7;for(var yy=Math.max(top,lip);yy<yb;yy++){var dpt=(yy-lip)/Math.max(1,Tp),nz2=hsh(px,yy,9)*10,band2=Math.sin(dpt*9+hsh(px>>2,0,3)*2)*6,dk=1-dpt*.45;
            if(yy-lip<Q*.6){put(px,yy,COL[id3*3]*.8,COL[id3*3+1]*.8,COL[id3*3+2]*.8)}else put(px,yy,(SOIL[0]+band2+nz2)*fs*dk,(SOIL[1]+band2+nz2)*fs*dk,(SOIL[2]+band2+nz2)*fs*dk)}
          yb=Math.min(yb,lip)}
        first=false;
        put(px,top,COL[id3*3],COL[id3*3+1],COL[id3*3+2]);
        var mt3=MT[id3];
        if(mt3===6){for(var y4=top+1;y4<yb;y4++)put(px,y4,COL[id3*3]*.8,COL[id3*3+1]*.8,COL[id3*3+2]*.8)}
        else if(yb-top>Q*.9){/* A wall facing the viewer: the sun is behind, so it gets sky light plus whatever side light its left or right lean catches. Buildings get lit windows' worth of variation from their plaster colour. */
          var b0=WC[id3*3],b1=WC[id3*3+1],b2=WC[id3*3+2],wd=Math.max(0,WX[id3]*SUN[0]*1.1+.08)*LA[id3],span=yb-top,base=offy+j,bld=mt3===8;
          for(var y2=top+1;y2<yb;y2++){var fr=(y2-top)/span,zz=(base-y2)/k,lw=bld?(.62*AA[id3]*(1-.3*fr)+wd*.8):(.44*AA[id3]*(1-.5*fr*fr)+wd*(1-.4*fr)),r1=b0,g1=b1,bl1=b2;
            if(mt3===5&&((zz*28)%1)<.35){r1=232;g1=238;bl1=246}
            if(bld&&y2===top+1){r1*=.6;g1*=.6;bl1*=.6}
            put(px,y2,r1*lw*.95,g1*lw,bl1*lw*1.12)}}
        else for(var y3=top+1;y3<yb;y3++)put(px,y3,COL[id3*3]*.92,COL[id3*3+1]*.92,COL[id3*3+2]*.92);
        yb=top}}
    x.putImageData(img,0,0);
    (G.steams||(G.steam?[[0,.05,.5]]:[])).forEach(function(sv){var sx2=(cx+sv[0]*r-ox)*Q,sy2=(cy+sv[1]*r-oy)*Q,sg=x.createRadialGradient(sx2,sy2,0,sx2,sy2,r*sv[2]*Q);sg.addColorStop(0,'rgba(255,255,255,.45)');sg.addColorStop(1,'rgba(255,255,255,0)');x.fillStyle=sg;x.fillRect(0,0,w,h)});
    var out=document.createElement('canvas');out.width=Math.ceil(W*RQ);out.height=Math.ceil(H*RQ);var ox2=out.getContext('2d');ox2.imageSmoothingQuality='high';ox2.drawImage(cv,0,0,out.width,out.height);
    return {canvas:out,x:ox/r,y:oy/r,w:W/r,h:H/r}}
    /* The model is built in row-sized steps so a caller can stop at a time limit and carry on in the next slice; step(until) always does at least one row and is true once the model is whole. */
    var stage=0;
    return {make:make,step:function(until){
      if(stage===0){if(!sample(until))return false;stage=1;buildSat()}
      if(stage===1){if(!light(until))return false;stage=2}
      return true}}}
  function iso3Model(t,v,RQ){var job=iso3ModelJob(t,v,RQ);job.step(Infinity);return job.make}
  function iso3(t,v,nb,RQ){return iso3Model(t,v,RQ)(nb)}
  /* Folders keep a long piece list scannable; each shows its first piece as the cover. */
  var PFOLD={forest:[['Woods',[0,1,2,3,4,11]],['Clearings & water',[5,8]],['Old & strange',[6,9,10]],['Camps',[7]]],
    mountain:[['Peaks',[0,1,2,7,11]],['Snow & ice',[4,10]],['Fire & stone',[3,5]],['Passes & places',[6,8,9]]],
    hills:[['Grassy',[0,7,3,6]],['Rocky',[1,9,11]],['Farms & herds',[2,8]],['Old places',[4,5,10]]],
    plains:[['Grass',[0,1,4,5,6]],['Farms',[2,3,10]],['Water & dry',[7,8]],['Roads & camps',[9,11]]],
    swamp:[['Wetlands',[0,2,4,5]],['Trees',[1,3,6]],['Strange',[9,10]],['Places',[7,8,11]]],
    desert:[['Sand',[0,1,6]],['Rock',[2,4,5,7]],['Oasis',[3]],['Places',[8,9,10,11]]],
    snow:[['Snow',[0,1,5,6]],['Ice',[2,3,4,10]],['Rocks & springs',[7,11]],['Places',[8,9]]],
    water:[['Open water',[0,1,2,8]],['Shallows',[3,4,7]],['Rocks & islands',[5,6,11]],['Ice & wrecks',[9,10]]],
    town:[['Villages',[0,1,2]],['Cities',[3,4,9]],['Strongholds',[5,6,10]],['Special',[7,8,11]]],
    jungle:[['Trees',[0,1,2,3,5,10]],['Water',[4,7]],['Open ground',[6,11]],['Places',[8,9]]],
    farmland:[['Fields',[0,1,6,11]],['Orchards & hedges',[2,3,10]],['Herds & water',[4,5]],['Farms',[7,8,9]]],
    scrub:[['Grass',[0,7,8]],['Bush & trees',[1,2,10]],['Rock & dust',[3,4,6,11]],['Water & camps',[5,9]]],
    badlands:[['Rock',[0,1,7]],['Mesas & canyons',[2,3,4,5]],['Dry & dead',[6,8,10]],['Places',[9,11]]],
    waste:[['Barren',[0,1,2]],['Ruin',[3,5,8]],['Pits & pools',[4,7,9]],['Strange',[6,10,11]]],
    volcanic:[['Lava',[0,2,3,7]],['Cones & stone',[1,4,8]],['Ash & steam',[5,6,9,10]],['Places',[11]]],
    coast:[['Beaches',[0,2,6]],['Rock & cliffs',[1,3]],['Bays & flats',[4,5,7,11]],['Places',[8,9,10]]],
    deep:[['Open sea',[0,1,2,10]],['Hazards',[3,5,7]],['Creatures',[4,6]],['Wrecks & ruins',[8,9,11]]],
    ice:[['Ice',[0,1,3]],['Broken ice',[2,4,5,7]],['Meltwater',[6]],['Places',[8,9,10,11]]],
    cavern:[['Stone',[0,1,5]],['Glow',[2,4,7]],['Water & fire',[3,6,9]],['Places',[8,10,11]]]};

  // ---- What the viewer needs around the drawing ----

  // TILE_R is the hex radius iso3 models in. The design rendered every tile at
  // this radius three pixels to the unit; iso3 keeps that model and changes only
  // the pixels per unit (RQ), so a tile reads the same at every zoom and only
  // gains or loses resolution.
  var TILE_R = 25;
  var DESIGN_RQ = 3;

  // terrainHasPieces says whether a terrain offers pieces (a road is a line).
  function terrainHasPieces(t){return !!PIECES[t]}

  // folderOf is the index of the folder in a terrain's picker that holds piece
  // v, or -1 (Mix, or no such piece).
  function folderOf(t,v){var F=PFOLD[t];if(!F||v==null)return -1;for(var i=0;i<F.length;i++)if(F[i][1].indexOf(v)>=0)return i;return -1}

  // detailedVariant is the seed index a hex's Detailed art is drawn from.
  function detailedVariant(key){var h=2166136261;for(var i=0;i<key.length;i++){h^=key.charCodeAt(i);h=Math.imul(h,16777619)}return ((h>>>0)%1009)%DETAILED_VARIANTS}

  // zoomBucket rounds a hex radius in device pixels up to the next half power
  // of two inside [min, max], so tiles are painted at a handful of sizes and a
  // zoom step reuses them; scaling a tile down a little keeps it sharp.
  function zoomBucket(px,min,max){if(!(px>0))return min;var b=Math.pow(2,Math.ceil(Math.log2(px)*2)/2);b=Math.round(b*100)/100;return Math.max(min,Math.min(max,b))}

  // TileCache keeps painted tiles, least recently used first out, within a cap
  // on how many it holds and on their total pixels, so a long session on a big
  // map cannot grow memory without bound.
  function TileCache(maxEntries,maxPixels){this.max=maxEntries;this.maxPx=maxPixels;this.map=new Map();this.px=0}
  TileCache.prototype.get=function(k){var e=this.map.get(k);if(!e)return undefined;this.map.delete(k);this.map.set(k,e);return e.v};
  TileCache.prototype.has=function(k){return this.map.has(k)};
  TileCache.prototype.set=function(k,v,px){px=px||0;var old=this.map.get(k);if(old){this.px-=old.px;this.map.delete(k)}this.map.set(k,{v:v,px:px});this.px+=px;
    while(this.map.size>1&&(this.map.size>this.max||this.px>this.maxPx)){var first=this.map.keys().next().value,e=this.map.get(first);this.map.delete(first);this.px-=e.px}};
  TileCache.prototype.clear=function(){this.map.clear();this.px=0};
  Object.defineProperty(TileCache.prototype,'size',{get:function(){return this.map.size}});

  // JobQueue runs queued work in time-limited slices. Jobs are deduplicated by
  // key. work(job, until) does as much of a job as fits before the deadline
  // `until` (a clock reading) and returns true when it is finished, false when
  // it stopped for time, or 'wait' when it cannot start yet; both of the latter
  // end the slice with the job still at the front. A slice always gives the
  // front job one go, so the queue cannot stall on a budget of zero.
  function JobQueue(work){this.work=work;this.list=[];this.keys={}}
  JobQueue.prototype.add=function(job,first){if(this.keys[job.key])return false;this.keys[job.key]=true;if(first)this.list.unshift(job);else this.list.push(job);return true};
  JobQueue.prototype.clear=function(){this.list=[];this.keys={}};
  Object.defineProperty(JobQueue.prototype,'length',{get:function(){return this.list.length}});
  JobQueue.prototype.run=function(budgetMs,now){
    var until=now()+budgetMs;
    while(this.list.length){
      var j=this.list[0];
      if(this.work(j,until)!==true)break;
      var at=this.list.indexOf(j);if(at>=0)this.list.splice(at,1);delete this.keys[j.key];
      if(now()>=until)break}
    return this.list.length};

  var api = {
    TER_COLOR: TER_COLOR, PIECES: PIECES, PFOLD: PFOLD, DETAILED_VARIANTS: DETAILED_VARIANTS, TILE_R: TILE_R, DESIGN_RQ: DESIGN_RQ,
    pieceOf: pieceOf, folderOf: folderOf, terrainHasPieces: terrainHasPieces, detailedVariant: detailedVariant, detailedSeed: detailedSeed,
    zoomBucket: zoomBucket, TileCache: TileCache, JobQueue: JobQueue, hexPath: hexPath,
    detailedDefs: detailedDefs, detailedArt: detailedArt, iso3: iso3, iso3Model: iso3Model, iso3ModelJob: iso3ModelJob
  };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (typeof window !== 'undefined') window.ChronicleHexArt = api;
})();
