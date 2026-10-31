/**
 * shop_storefront.js -- draws one shop's storefront as an SVG string.
 *
 * A pure function of the storefront's settings, so the Shops street and the
 * GM's storefront card draw exactly the same thing. Every setting is checked
 * against the same lists the server keeps (ShopFront in
 * internal/plugins/armory/shop_front_model.go; test/js/shop_storefront.test.mjs
 * fails when they drift) and colours must be #rrggbb, so a stored value can
 * never reach the SVG as markup. Text is escaped.
 *
 * Classes the street animates: .glass/.lampglow/.flame (lit when open),
 * .closer.c-* (the closed look), .c-bar (the barred door, for a shop that
 * won't serve the viewer).
 */
(function(){
var uidN=0;
function hx(c){c=c.replace('#','');return[parseInt(c.slice(0,2),16),parseInt(c.slice(2,4),16),parseInt(c.slice(4,6),16)]}
function mix(a,b,t){var A=hx(a),B=hx(b);return'#'+A.map(function(v,i){var c=Math.round(v+(B[i]-v)*t);return(c<16?'0':'')+c.toString(16)}).join('')}
function esc(s){return String(s==null?'':s).replace(/[&<>"]/g,function(c){return{'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]})}
var WARE={
 bottle:function(x,y){return '<rect x="'+(x-3)+'" y="'+(y-22)+'" width="6" height="8" rx="1"/><path d="M'+(x-9)+' '+(y-10)+'q9 -6 18 0v12a3 3 0 0 1 -3 3h-12a3 3 0 0 1 -3 -3z"/>'},
 sword:function(x,y){return '<rect x="'+(x-1.5)+'" y="'+(y-32)+'" width="3" height="26"/><rect x="'+(x-8)+'" y="'+(y-7)+'" width="16" height="3" rx="1"/><rect x="'+(x-1.8)+'" y="'+(y-4)+'" width="3.6" height="8"/>'},
 scroll:function(x,y){return '<rect x="'+(x-11)+'" y="'+(y-18)+'" width="22" height="16" rx="2"/><rect x="'+(x-13)+'" y="'+(y-20)+'" width="4" height="20" rx="2"/><rect x="'+(x+9)+'" y="'+(y-20)+'" width="4" height="20" rx="2"/>'},
 gem:function(x,y){return '<path d="M'+(x-10)+' '+(y-16)+'h20l5 6-15 14-15-14z"/>'},
 bread:function(x,y){return '<ellipse cx="'+x+'" cy="'+(y-7)+'" rx="14" ry="7"/>'},
 shield:function(x,y){return '<path d="M'+(x-10)+' '+(y-26)+'h20v10c0 10-6 15-10 18-4-3-10-8-10-18z"/>'},
 hammer:function(x,y){return '<rect x="'+(x-1.5)+'" y="'+(y-24)+'" width="3" height="24"/><rect x="'+(x-9)+'" y="'+(y-28)+'" width="18" height="8" rx="1"/>'},
 herb:function(x,y){return '<path d="M'+x+' '+y+'v-20M'+x+' '+(y-8)+'q-9-2-10-12q9 1 10 12M'+x+' '+(y-14)+'q9-2 10-12q-9 1-10 12" stroke-width="2" stroke="currentColor" fill="none"/>'}
};
function front(c,o){o=o||{};var u='sf'+(++uidN),base=c.colour||'#b98a5e',dark=mix(base,'#000000',.45),lite=mix(base,'#ffffff',.25),beam=mix(base,'#1a0f07',.7);
 var s='<svg viewBox="0 0 240 236" role="img" aria-label="'+esc(c.name)+' storefront"><defs>'+
 '<linearGradient id="'+u+'g" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#fff3c4"/><stop offset=".55" stop-color="#ffcf7a"/><stop offset="1" stop-color="#e0913a"/></linearGradient>'+
 '<radialGradient id="'+u+'l"><stop offset="0" stop-color="#ffd27a" stop-opacity=".9"/><stop offset="1" stop-color="#ffb347" stop-opacity="0"/></radialGradient>'+
 '<linearGradient id="'+u+'f" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="'+lite+'"/><stop offset="1" stop-color="'+base+'"/></linearGradient>'+
 '<pattern id="'+u+'s" width="28" height="14" patternUnits="userSpaceOnUse"><rect width="28" height="14" fill="'+base+'"/><path d="M0 13.5h28M14 0v7M0 7h28M0 0v7" stroke="'+dark+'" stroke-opacity=".35" stroke-width="1"/><path d="M7 7v7M21 7v7" stroke="'+dark+'" stroke-opacity=".35"/></pattern>'+
 '<linearGradient id="'+u+'d" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#000" stop-opacity=".34"/><stop offset=".16" stop-color="#000" stop-opacity="0"/><stop offset=".82" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".28"/></linearGradient>'+'<linearGradient id="'+u+'e" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#000" stop-opacity=".22"/><stop offset=".1" stop-color="#000" stop-opacity="0"/><stop offset=".9" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".22"/></linearGradient>'+'<linearGradient id="'+u+'i" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#1a0d04" stop-opacity=".55"/><stop offset="1" stop-color="#1a0d04" stop-opacity="0"/></linearGradient>'+'<linearGradient id="'+u+'h" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#fff" stop-opacity=".32"/><stop offset=".45" stop-color="#fff" stop-opacity=".06"/><stop offset="1" stop-color="#fff" stop-opacity="0"/></linearGradient>'+'<radialGradient id="'+u+'w"><stop offset="0" stop-color="#ffc066" stop-opacity=".55"/><stop offset="1" stop-color="#ffc066" stop-opacity="0"/></radialGradient>'+'<linearGradient id="'+u+'r" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#fff" stop-opacity=".12"/><stop offset="1" stop-color="#000" stop-opacity=".25"/></linearGradient>'+'</defs>';
 // ground shadow and body
 s+='<ellipse cx="120" cy="234" rx="118" ry="5" fill="rgb(0 0 0/.25)"/>';
 var fill=c.style==='stone'?'url(#'+u+'s)':c.style==='painted'?base:'url(#'+u+'f)';
 s+='<rect x="10" y="30" width="220" height="204" fill="'+fill+'"/>';
 if(c.style==='timber'){s+='<g fill="'+beam+'">'+[10,94,142,222].map(function(x){return '<rect x="'+x+'" y="30" width="8" height="204"/>'}).join('')+'<rect x="10" y="30" width="220" height="7"/><rect x="10" y="178" width="220" height="6"/><path d="M18 37l76 0-76 40z" opacity=".55"/><path d="M222 37l-72 0 72 40z" opacity=".55"/></g>'}
 if(c.style==='painted'){s+='<rect x="10" y="30" width="220" height="10" fill="'+dark+'"/><rect x="10" y="226" width="220" height="8" fill="'+dark+'"/>'}
 s+='<rect x="10" y="30" width="220" height="204" fill="url(#'+u+'d)"/><rect x="10" y="30" width="220" height="204" fill="url(#'+u+'e)"/>';
 // roof line
 s+='<path d="M2 32h236l-10-20H12z" fill="'+mix(base,'#2b1a10',.75)+'"/><path d="M2 32h236l-10-20H12z" fill="url(#'+u+'r)"/>'+[16,20,24,28].map(function(y){return '<path d="M'+(12+(y-12)*0.5)+' '+y+'H'+(228-(y-12)*0.5)+'" stroke="rgb(0 0 0/.22)" stroke-width=".8"/>'}).join('')+'<rect x="2" y="30" width="236" height="4" fill="rgb(0 0 0/.3)"/><rect x="10" y="34" width="220" height="8" fill="rgb(0 0 0/.16)"/>';
 // sign
 var words=esc(c.words||c.name);
 if(c.sign==='board'){s+='<rect x="34" y="42" width="172" height="26" rx="3" fill="#2a1a0e"/><rect x="37" y="45" width="166" height="20" rx="2" fill="none" stroke="#c9a15a" stroke-width="1"/><text x="120" y="60" text-anchor="middle" font-family="Spectral,Georgia,serif" font-weight="700" font-size="'+(words.length>18?11:14)+'" fill="#f3d79b">'+words+'</text>'}
 else if(c.sign==='banner'){s+='<path d="M28 44h184l-8 12 8 12H28l8-12z" fill="'+(c.trim||'#7a1f2b')+'"/><text x="120" y="60" text-anchor="middle" font-family="Spectral,Georgia,serif" font-weight="700" font-size="'+(words.length>18?11:13)+'" fill="#fff4dc">'+words+'</text>'}
 else{s+='<rect x="14" y="44" width="40" height="3" fill="#2a1a0e"/><path d="M20 47v6M48 47v6" stroke="#2a1a0e" stroke-width="1.5"/><rect x="14" y="53" width="40" height="28" rx="3" fill="#2a1a0e"/><rect x="16.5" y="55.5" width="35" height="23" rx="2" fill="'+(c.trim||'#7a5a2a')+'"/>'+signIcon(c.icon||'bottle')+'<text x="120" y="62" text-anchor="middle" font-family="Spectral,Georgia,serif" font-weight="700" font-size="'+(words.length>18?11:13)+'" fill="'+(c.style==='painted'?'#fff6e0':'#2a1a0e')+'" opacity=".85">'+words+'</text>'}
 // windows
 var wins=[[24,96],[148,96]],ww=68,wh=74;
 wins.forEach(function(w,wi){var x=w[0],y=w[1];
  s+='<rect x="'+(x-4)+'" y="'+(y-4)+'" width="'+(ww+8)+'" height="'+(wh+8)+'" rx="2" fill="'+mix(base,'#140a04',.75)+'"/>';
  s+='<rect x="'+x+'" y="'+y+'" width="'+ww+'" height="'+wh+'" fill="#20242e"/>';
  s+='<g class="glass"><rect x="'+x+'" y="'+y+'" width="'+ww+'" height="'+wh+'" fill="url(#'+u+'g)"/><g fill="rgb(70 38 14/.78)" color="rgb(70 38 14/.78)">';
  var ws=(c.wares||[]).slice(wi*2,wi*2+2);if(!ws.length&&c.wares&&c.wares.length)ws=[c.wares[0]];
  ws.forEach(function(k,ki){var f=WARE[k]||WARE.bottle;s+=f(x+(ws.length===1?ww/2:ww*(ki?0.72:0.3)),y+wh-8)});
  s+='</g><rect x="'+x+'" y="'+(y+wh-7)+'" width="'+ww+'" height="7" fill="rgb(70 38 14/.45)"/></g>';
  s+='<path d="M'+(x+ww/2)+' '+y+'v'+wh+'M'+x+' '+(y+wh/2)+'h'+ww+'" stroke="'+mix(base,'#140a04',.75)+'" stroke-width="2.5"/>';
  s+='<rect x="'+x+'" y="'+y+'" width="'+ww+'" height="14" fill="url(#'+u+'i)"/><path d="M'+x+' '+y+'h'+(ww*0.55)+'l-'+(ww*0.55)+' '+(wh*0.7)+'z" fill="url(#'+u+'h)"/>';
  s+='<rect x="'+(x-6)+'" y="'+(y+wh+4)+'" width="'+(ww+12)+'" height="5" rx="1" fill="'+mix(base,'#140a04',.6)+'"/><ellipse class="glass" cx="'+(x+ww/2)+'" cy="'+(y+wh+16)+'" rx="'+(ww*0.62)+'" ry="14" fill="url(#'+u+'w)"/>';
 });
 // door: the room thread's door look, dark planks and a brass knob
 s+='<rect x="96" y="100" width="48" height="134" fill="'+mix(base,'#140a04',.75)+'"/>';
 s+='<g><rect x="100" y="104" width="20" height="130" fill="#2c1a0d"/><rect x="120" y="104" width="20" height="130" fill="#2c1a0d"/>'+[104,110,124,130].map(function(x){return '<rect x="'+x+'" y="104" width="1" height="130" fill="#3b2412"/>'}).join('')+'<rect x="119.5" y="104" width="1" height="130" fill="#140a04"/><circle cx="115" cy="172" r="2.4" fill="#e4c477"/><circle cx="125" cy="172" r="2.4" fill="#e4c477"/>'+
  '<rect class="glass" x="106" y="112" width="28" height="18" rx="9" fill="url(#'+u+'g)" opacity=".9"/></g>';
 s+='<rect x="90" y="229" width="60" height="6" rx="1" fill="#6f6a62"/><rect x="90" y="229" width="60" height="1.5" fill="#9a948a"/>';
 // awning
 if(c.awning&&c.awning!=='none'){var ac=c.awningColour||'#7a1f2b',n=11,wdt=220/n,p='';
  s+='<path d="M10 76h220l-6 16H16z" fill="'+ac+'"/>';
  if(c.awning==='stripe'){for(var i=0;i<n;i+=2){p+='<path d="M'+(10+i*wdt)+' 76h'+wdt+'l'+(-0.4)+' 16h'+(-wdt+0.8)+'z" fill="#f4ead6"/>'}s+=p}
  var sc='';for(var j=0;j<n;j++){sc+='<path d="M'+(16+j*(208/n))+' 92q'+(104/n)+' 9 '+(208/n)+' 0z" fill="'+(c.awning==='stripe'&&j%2===0?'#f4ead6':ac)+'"/>'}s+=sc;
  s+='<rect x="10" y="76" width="220" height="3" fill="rgb(0 0 0/.18)"/>';}
 // lamp
 if(c.lamp!==false){s+='<circle class="lampglow" cx="228" cy="108" r="30" fill="url(#'+u+'l)"/><path d="M230 92h-10v4" stroke="#1d1209" stroke-width="2" fill="none"/><rect x="223" y="96" width="10" height="16" rx="2" fill="#1d1209"/><rect x="225" y="99" width="6" height="10" fill="#3a2a1a"/><rect class="flame" x="226" y="100" width="4" height="8" rx="2" fill="#ffd27a"/>'}
 // closed looks
 var cl=c.closed||'shutters';
 if(cl==='shutters'){wins.forEach(function(w){s+='<g class="closer c-shutters"><rect x="'+(w[0]-2)+'" y="'+(w[1]-2)+'" width="'+(ww+4)+'" height="'+(wh+4)+'" fill="'+(c.shutterColour||'#5b6b5a')+'"/>'+[1,2,3,4,5,6,7,8].map(function(k){return '<rect x="'+(w[0]-2)+'" y="'+(w[1]-2+k*(wh+4)/9)+'" width="'+(ww+4)+'" height="1.6" fill="rgb(0 0 0/.28)"/>'}).join('')+'</g>'})}
 if(cl==='curtains'){wins.forEach(function(w){var cc=c.curtainColour||'#6d2338';s+='<g class="closer c-curtL"><path d="M'+w[0]+' '+w[1]+'h'+(ww/2)+'v'+wh+'h-'+(ww/2)+'z" fill="'+cc+'"/><path d="M'+(w[0]+8)+' '+w[1]+'v'+wh+'M'+(w[0]+18)+' '+w[1]+'v'+wh+'M'+(w[0]+27)+' '+w[1]+'v'+wh+'" stroke="rgb(0 0 0/.22)" stroke-width="2"/></g><g class="closer c-curtR"><path d="M'+(w[0]+ww/2)+' '+w[1]+'h'+(ww/2)+'v'+wh+'h-'+(ww/2)+'z" fill="'+cc+'"/><path d="M'+(w[0]+ww-8)+' '+w[1]+'v'+wh+'M'+(w[0]+ww-18)+' '+w[1]+'v'+wh+'M'+(w[0]+ww-27)+' '+w[1]+'v'+wh+'" stroke="rgb(0 0 0/.22)" stroke-width="2"/></g>'})}
 if(cl==='boards'){s+='<g class="closer c-boards" fill="#8b6a45" stroke="#4a3220" stroke-width="1">'+wins.map(function(w){return '<rect x="'+(w[0]-8)+'" y="'+(w[1]+18)+'" width="'+(ww+16)+'" height="10" transform="rotate(-12 '+(w[0]+ww/2)+' '+(w[1]+23)+')"/><rect x="'+(w[0]-8)+'" y="'+(w[1]+46)+'" width="'+(ww+16)+'" height="10" transform="rotate(9 '+(w[0]+ww/2)+' '+(w[1]+51)+')"/>'}).join('')+'<rect x="92" y="150" width="56" height="10" transform="rotate(-8 120 155)"/></g>'}
 if(cl==='board'){var nt=esc((c.closedNote||'Closed').slice(0,40)),l1=nt,l2='';if(nt.length>14){var cut=nt.lastIndexOf(' ',15);if(cut<5)cut=14;l1=nt.slice(0,cut);l2=nt.slice(cut).trim()}
  s+='<g class="closer c-board"><path d="M120 104l-26 24M120 104l26 24" stroke="#2a1a0e" stroke-width="1.2"/><circle cx="120" cy="104" r="2" fill="#e4c477"/><rect x="84" y="126" width="72" height="'+(l2?34:24)+'" rx="2" fill="#efe2c4" stroke="#6b4a2a"/><text x="120" y="141" text-anchor="middle" font-family="Spectral,Georgia,serif" font-weight="700" font-size="9" fill="#3a2414">'+l1+'</text>'+(l2?'<text x="120" y="153" text-anchor="middle" font-family="Spectral,Georgia,serif" font-weight="600" font-size="9" fill="#3a2414">'+l2+'</text>':'')+'</g>'}
 // refusing: lit windows, barred door; shown only when the front carries .barred
 var rn=esc((c.refuseNote||'No trade').slice(0,18));
 s+='<g class="c-bar"><rect x="96" y="100" width="48" height="134" fill="rgb(0 0 0/.28)"/><rect x="86" y="148" width="68" height="13" rx="2" fill="#4a2e17" stroke="#1e1209" stroke-width="1.2"/><rect x="86" y="150" width="68" height="2.5" fill="rgb(255 255 255/.12)"/><rect x="91" y="145" width="6" height="19" rx="1" fill="#2b2b2e"/><rect x="143" y="145" width="6" height="19" rx="1" fill="#2b2b2e"/><circle cx="94" cy="154.5" r="1.3" fill="#9a9aa0"/><circle cx="146" cy="154.5" r="1.3" fill="#9a9aa0"/><path d="M116 161v-4a4 4 0 0 1 8 0v4" fill="none" stroke="#7d7d84" stroke-width="2"/><rect x="113" y="160" width="14" height="11" rx="2" fill="#b08a3a" stroke="#5b4416"/><circle cx="120" cy="165" r="1.6" fill="#3a2a0e"/>'+
  '<rect x="92" y="112" width="56" height="24" rx="2.5" fill="#f4e7cc" stroke="#8a2424" stroke-width="1.8"/><text x="120" y="128" text-anchor="middle" font-family="Spectral,Georgia,serif" font-weight="700" font-size="'+(rn.length>10?7.5:9.5)+'" fill="#8a2424">'+rn+'</text></g>';
 s+='</svg>';return s}
function signIcon(k){var f=WARE[k]||WARE.bottle;return '<g fill="#f3d79b" color="#f3d79b" transform="translate(34 79) scale(.62) translate(-0 -0)">'+f(0,0)+'</g>'}
var LISTS={
 style:['timber','stone','plaster','painted'],
 awning:['none','plain','stripe'],
 sign:['hang','board','banner'],
 icon:['','bottle','sword','scroll','gem','bread','shield','hammer','herb'],
 ware:['bottle','sword','scroll','gem','bread','shield','hammer','herb'],
 closed:['shutters','curtains','boards','board','dark']
};
var HEX=/^#[0-9a-f]{6}$/;
function pick(v,list,d){return list.indexOf(v)>=0?v:d}
function col(v,d){v=String(v||'').toLowerCase();return HEX.test(v)?v:d}
/* clean returns a copy of c holding only listed values and real colours. */
function clean(c){c=c||{};return {
 name:String(c.name||''),words:String(c.words||''),
 style:pick(c.style,LISTS.style,'timber'),colour:col(c.colour,'#b98a5e'),trim:col(c.trim,''),
 awning:pick(c.awning,LISTS.awning,'none'),awningColour:col(c.awningColour,''),
 sign:pick(c.sign,LISTS.sign,'hang'),icon:pick(c.icon,LISTS.icon,''),
 wares:(Array.isArray(c.wares)?c.wares:[]).filter(function(w){return LISTS.ware.indexOf(w)>=0}).slice(0,3),
 lamp:!!c.lamp,closed:pick(c.closed,LISTS.closed,'shutters'),
 shutterColour:col(c.shutterColour,''),curtainColour:col(c.curtainColour,''),
 closedNote:String(c.closedNote||''),refuseNote:String(c.refuseNote||'')}}
window.ChronicleStorefront={draw:function(c,o){return front(clean(c),o)},clean:clean,mix:mix,esc:esc,LISTS:LISTS,WARES:Object.keys(WARE)};
})();
