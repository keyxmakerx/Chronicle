/**
 * craft_scene.js -- the crafting bench's play engine (window.CraftScene)
 *
 * A crafting scene is an engine and a skin. The engine owns play: the steps a
 * recipe is made of, how each step is graded, the project roll, hover cards,
 * the right-click menu, particles and sound. A skin (craft_desk.js is the
 * first) draws the art, the light and the ambiance, and says what each piece
 * is. So every skin plays the same rules, and a new look is a new skin, never
 * a new engine. craft_bench.ai.md is the guide for writing one.
 *
 * Recipes and items are data handed to mount(): nothing about a particular
 * potion lives here. Every text that came from data is escaped before it
 * reaches innerHTML.
 *
 * Usage:
 *   var scene = CraftScene.mount(el, CraftDesk.create(opts), {
 *     recipes: {...}, items: {...}, recipe: 'heal', settings: {...}
 *   });
 *   scene.destroy();
 */
window.CraftScene = (function () {
  var NS = 'http://www.w3.org/2000/svg', seq = 0;
  // Calm under the device's reduce-motion setting or Chronicle's own Calm/Off motion choices.
  function isCalm() {
    var h = document.documentElement;
    return !!((window.matchMedia && matchMedia('(prefers-reduced-motion: reduce)').matches) ||
      h.hasAttribute('data-cz-reduce') || h.getAttribute('data-view-motion') === 'calm');
  }
  var calm = isCalm();
  function clamp(v,a,b){return v<a?a:v>b?b:v;}
  function lerp(a,b,t){return a+(b-a)*t;}
  function hex(c){c=String(c||'#888888').replace('#','');if(c.length===3)c=c.replace(/./g,'$&$&');return [parseInt(c.slice(0,2),16)||0,parseInt(c.slice(2,4),16)||0,parseInt(c.slice(4,6),16)||0];}
  function mix(a,b,t){var x=hex(a),y=hex(b);return '#'+[0,1,2].map(function(i){var v=Math.round(lerp(x[i],y[i],clamp(t,0,1)));return (v<16?'0':'')+v.toString(16);}).join('');}
  function rgb(c){return hex(c).join(',');}
  function rnd(a,b){return a+Math.random()*(b-a);}
  function esc(s){return String(s==null?'':s).replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c];});}

  /* What each step asks of the player. A recipe is a list of these, so one
     recipe kind plays differently from another by its steps alone. */
  var SAY={
    add_mortar:'Drag the {item} into the mortar',
    add_flask:'Drag the {item} into the flask. It will react',
    grind:'Grind it: circle the pestle around the bowl',
    tip:'Drag the mortar over the flask to tip it in',
    pour:'Drag the {item} over the flask, then press and hold to pour to the gold line',
    simmer:'Press and hold to work the bellows. Keep the needle in the gold band',
    strike:'Catch the reaction: press when the mark crosses the gold'
  };
  var GRADED={pour:1,simmer:1,strike:1};
  var DIFF={
    easy:{band:.15,pour:[.09,.035],zone:.15,speed:.75},
    std:{band:.1,pour:[.06,.02],zone:.1,speed:1},
    hard:{band:.065,pour:[.04,.013],zone:.065,speed:1.3}
  };
  var DEFAULTS={difficulty:'std',bane:true,skip:true};

  /* A recipe's steps as the engine plays them: known kinds only, with sane
     counts, so data from a Director or their AI can never break the scene. */
  function buildSteps(r,items){
    return (r.steps||[]).filter(function(st){return st&&(st.t==='add'||SAY.hasOwnProperty(st.t))&&(!(st.t==='add'||st.t==='pour')||items[st.item]);}).map(function(st){
      var s={t:st.t};
      if(st.item)s.item=st.item;
      if(st.t==='add')s.to=st.to==='flask'?'flask':'mortar';
      if(st.t==='grind')s.turns=clamp(Math.round(+st.turns||3),1,8);
      if(st.t==='simmer')s.secs=clamp(+st.secs||5,2,20);
      if(st.t==='strike')s.n=clamp(Math.round(+st.n||3),1,6);
      if(GRADED[st.t])s.graded=true;
      return s;});
  }
  function itemName(items,k){return items[k]?String(items[k].name):'';}
  function stepSay(st,items){
    if(st.t==='add')return SAY['add_'+st.to].replace('{item}',itemName(items,st.item).toLowerCase());
    return SAY[st.t].replace('{item}',itemName(items,st.item).toLowerCase());
  }
  function stepName(st,items){
    return {add:'Add the '+itemName(items,st.item).toLowerCase(),grind:'Grind, '+st.turns+' turns',tip:'Tip it into the flask',
      pour:'Pour to the line',simmer:'Keep it steady',strike:'Catch the moment, '+st.n+' times'}[st.t];
  }
  /* Draw Steel: an edge is +2 on the project roll, a bane is -2. Clean work
     earns the edge; botched work earns a bane only if the Director allows it. */
  function verdict(res,set){
    var m=0,p=0;res.forEach(function(r){if(r.grade==='miss')m++;if(r.grade==='perfect')p++;});
    if(!res.length)return {k:'plain',v:0,label:'No edge',why:'Steps skipped'};
    if(m===0&&p>=1)return {k:'edge',v:2,label:'Edge (+2)',why:'Clean work'};
    if(set.bane&&m>=2)return {k:'bane',v:-2,label:'Bane (−2)',why:'Botched work'};
    return {k:'plain',v:0,label:'No edge',why:m?'One slip':'Steady work'};
  }
  /* Recipes and items as the engine trusts them: plain strings, numbers in
     range and colours that are really colours, whoever wrote them. */
  function hexOr(c,f){return /^#[0-9a-f]{6}$/i.test(String(c))?String(c):f;}
  function cleanItems(src){var o={};Object.keys(src||{}).forEach(function(k){var it=src[k]||{};
    o[k]={name:String(it.name||k),color:hexOr(it.color,'#8a8a8a'),desc:String(it.desc||'')};});return o;}
  function cleanRecipes(src){var o={};Object.keys(src||{}).forEach(function(k){var r=src[k]||{};
    o[k]={name:String(r.name||k),kind:String(r.kind||''),who:String(r.who||''),stat:String(r.stat||''),mod:clamp(Math.round(+r.mod||0),-10,10),
      goal:clamp(Math.round(+r.goal||10),1,999),start:clamp(Math.round(+r.start||0),0,999),
      base:hexOr(r.base,'#cfe3f2'),paste:hexOr(r.paste,'#7e1a2a'),mid:hexOr(r.mid,'#a63a4a'),final:hexOr(r.final,'#ff4d6d'),steps:Array.isArray(r.steps)?r.steps:[]};});return o;}
  /* Every id a skin draws is made unique to its scene, so two scenes on one
     page never borrow each other's gradients, filters or clip paths. */
  function scoper(p){return function(html){return String(html).replace(/(\s)id="/g,'$1id="'+p).replace(/url\(#/g,'url(#'+p).replace(/href="#/g,'href="#'+p);};}

  /* Particles on a canvas laid over the scene, in scene coordinates. */
  function Particles(canvas){
    var ctx=canvas.getContext('2d'),list=[],W=1200,H=700,scale=1,dpr=1;
    function resize(){var r=canvas.getBoundingClientRect();dpr=Math.min(2,window.devicePixelRatio||1);
      canvas.width=Math.max(1,Math.round(r.width*dpr));canvas.height=Math.max(1,Math.round(r.height*dpr));scale=canvas.width/W;}
    resize();
    var ro=window.ResizeObserver?new ResizeObserver(resize):null;if(ro)ro.observe(canvas);
    function emit(o){if(list.length>900)return;var p={x:0,y:0,vx:0,vy:0,ax:0,ay:0,drag:0,life:1,t:0,size:3,grow:0,color:'255,255,255',alpha:1,type:'glow',fade:'out',spin:0};
      for(var k in o)p[k]=o[k];list.push(p);return p;}
    function tick(dt){
      ctx.setTransform(1,0,0,1,0,0);ctx.clearRect(0,0,canvas.width,canvas.height);
      ctx.setTransform(scale,0,0,scale,0,0);
      for(var i=list.length-1;i>=0;i--){var p=list[i];p.t+=dt;if(p.t>=p.life){list.splice(i,1);continue;}
        p.vx+=p.ax*dt;p.vy+=p.ay*dt;if(p.drag){var d=Math.pow(1-p.drag,dt*60);p.vx*=d;p.vy*=d;}
        p.x+=p.vx*dt;p.y+=p.vy*dt;p.size=Math.max(.1,p.size+p.grow*dt);
        var k=p.t/p.life,a=p.fade==='inout'?Math.sin(Math.PI*k):p.fade==='in'?k:(1-k);a*=p.alpha;if(a<=0)continue;
        if(p.clip){ctx.save();ctx.beginPath();ctx.rect(p.clip[0],p.clip[1],p.clip[2],p.clip[3]);ctx.clip();}
        if(p.type==='glow'){ctx.globalCompositeOperation='lighter';var g=ctx.createRadialGradient(p.x,p.y,0,p.x,p.y,p.size);
          g.addColorStop(0,'rgba('+p.color+','+a+')');g.addColorStop(1,'rgba('+p.color+',0)');ctx.fillStyle=g;ctx.beginPath();ctx.arc(p.x,p.y,p.size,0,6.283);ctx.fill();}
        else if(p.type==='soft'){ctx.globalCompositeOperation='source-over';var g2=ctx.createRadialGradient(p.x,p.y,0,p.x,p.y,p.size);
          g2.addColorStop(0,'rgba('+p.color+','+a+')');g2.addColorStop(1,'rgba('+p.color+',0)');ctx.fillStyle=g2;ctx.beginPath();ctx.arc(p.x,p.y,p.size,0,6.283);ctx.fill();}
        else if(p.type==='streak'||p.type==='line'||p.type==='istreak'){ctx.globalCompositeOperation=p.type==='streak'?'lighter':'source-over';ctx.strokeStyle='rgba('+p.color+','+a+')';ctx.lineWidth=p.size;ctx.lineCap='round';
          var len=p.len||.035;ctx.beginPath();ctx.moveTo(p.x,p.y);ctx.lineTo(p.x-p.vx*len,p.y-p.vy*len);ctx.stroke();}
        else if(p.type==='blob'){ctx.globalCompositeOperation='source-over';ctx.fillStyle='rgba('+p.color+','+a+')';ctx.beginPath();ctx.arc(p.x,p.y,p.size,0,6.283);ctx.fill();}
        else if(p.type==='ring'){ctx.globalCompositeOperation=p.add?'lighter':'source-over';ctx.strokeStyle='rgba('+p.color+','+a+')';ctx.lineWidth=p.lw||2;ctx.beginPath();ctx.arc(p.x,p.y,p.size,0,6.283);ctx.stroke();}
        else if(p.type==='bubble'){ctx.globalCompositeOperation='source-over';ctx.strokeStyle='rgba('+p.color+','+a+')';ctx.lineWidth=1.2;ctx.beginPath();ctx.arc(p.x,p.y,p.size,0,6.283);ctx.stroke();
          ctx.fillStyle='rgba(255,255,255,'+(a*.5)+')';ctx.beginPath();ctx.arc(p.x-p.size*.35,p.y-p.size*.35,p.size*.3,0,6.283);ctx.fill();}
        if(p.clip)ctx.restore();
      }
      ctx.globalCompositeOperation='source-over';
    }
    return {emit:emit,tick:tick,clear:function(){list.length=0;},count:function(){return list.length;},destroy:function(){if(ro)ro.disconnect();list.length=0;}};
  }

  /* Sound, made on the spot with Web Audio so nothing is downloaded. Off
     until the player turns it on, which is also the gesture browsers need. */
  var Sound=(function(){
    var ac=null,master=null,noise=null,on=false,beds={},crackT=0,profile='desk',rainOn=false;
    function init(){if(ac)return;var A=window.AudioContext||window.webkitAudioContext;if(!A)return;ac=new A();master=ac.createGain();master.gain.value=.7;master.connect(ac.destination);
      var len=ac.sampleRate*2,b=ac.createBuffer(1,len,ac.sampleRate),d=b.getChannelData(0),last=0;
      for(var i=0;i<len;i++){var w=Math.random()*2-1;last=(last+.02*w)/1.02;d[i]=w*.6+last*2.4;}noise=b;}
    function bed(name,f,q,g,type){var s=ac.createBufferSource();s.buffer=noise;s.loop=true;var fl=ac.createBiquadFilter();fl.type=type||'bandpass';fl.frequency.value=f;fl.Q.value=q;
      var gn=ac.createGain();gn.gain.value=0;s.connect(fl);fl.connect(gn);gn.connect(master);s.start();beds[name]={g:gn,f:fl,base:g};gn.gain.setTargetAtTime(g,ac.currentTime,.6);}
    function burst(f,q,g,dur,type){if(!on)return;var s=ac.createBufferSource();s.buffer=noise;var fl=ac.createBiquadFilter();fl.type=type||'bandpass';fl.frequency.value=f;fl.Q.value=q;
      var gn=ac.createGain(),t=ac.currentTime;gn.gain.setValueAtTime(g,t);gn.gain.exponentialRampToValueAtTime(.0001,t+dur);s.connect(fl);fl.connect(gn);gn.connect(master);s.start(t,Math.random()*1.5);s.stop(t+dur+.05);}
    function tone(f,g,dur,type,f2){if(!on)return;var o=ac.createOscillator(),gn=ac.createGain(),t=ac.currentTime;o.type=type||'sine';o.frequency.setValueAtTime(f,t);if(f2)o.frequency.exponentialRampToValueAtTime(f2,t+dur);
      gn.gain.setValueAtTime(.0001,t);gn.gain.exponentialRampToValueAtTime(g,t+.012);gn.gain.exponentialRampToValueAtTime(.0001,t+dur);o.connect(gn);gn.connect(master);o.start(t);o.stop(t+dur+.05);}
    function set(v,prof){profile=prof||profile;if(v){init();if(!ac)return;ac.resume();on=true;
        // The desk always hears rain; another skin only while its own window shows it
        if(!beds.rain)bed('rain',1400,.4,.05);beds.rain.base=profile==='desk'||rainOn?.05:0;
        if(!beds.room)bed('room',220,.7,profile==='desk'?.05:.035,'lowpass');
        if(!beds.pour)bed('pour',900,1.2,0);
        Object.keys(beds).forEach(function(k){if(k!=='pour')beds[k].g.gain.setTargetAtTime(beds[k].base,ac.currentTime,.4);});}
      else{on=false;if(ac)Object.keys(beds).forEach(function(k){beds[k].g.gain.setTargetAtTime(0,ac.currentTime,.15);});}}
    function tick(dt,heat){if(!on)return;crackT-=dt;if(crackT<=0){crackT=rnd(.05,.5);burst(rnd(1800,4200),2,rnd(.03,.12)*(profile==='desk'?1:.6),rnd(.015,.04),'highpass');}
      if(heat>.35&&Math.random()<dt*heat*9)tone(rnd(260,420),.05,.08,'sine',rnd(600,900));}
    function pour(v){if(!on||!beds.pour)return;beds.pour.g.gain.setTargetAtTime(v?.16:0,ac.currentTime,.05);}
    function rain(v){rainOn=!!v;if(on&&beds.rain){beds.rain.base=profile==='desk'||rainOn?.05:0;beds.rain.g.gain.setTargetAtTime(beds.rain.base,ac.currentTime,.5);}}
    var fx={
      grind:function(){burst(rnd(500,800),3,.18,.09);},
      drop:function(){tone(180,.25,.18,'sine',90);burst(700,1,.08,.08);},
      clink:function(){tone(1800,.08,.25,'sine');tone(2700,.04,.2,'sine');},
      fizz:function(){burst(5000,.6,.12,.6,'highpass');},
      perfect:function(){tone(880,.18,.7);tone(1320,.12,.9);tone(1760,.06,1);},
      good:function(){tone(660,.14,.4);},
      miss:function(){tone(110,.3,.35,'triangle',55);burst(300,1,.2,.3);},
      pump:function(){burst(400,.8,.12,.25,'lowpass');},
      scratch:function(){burst(rnd(2500,3800),4,.07,.06);},
      seal:function(){tone(90,.35,.3,'sine',50);burst(500,1,.15,.12);},
      purr:function(){if(!on)return;var s=ac.createBufferSource();s.buffer=noise;var fl=ac.createBiquadFilter();fl.type='lowpass';fl.frequency.value=140;var g=ac.createGain(),t=ac.currentTime,lfo=ac.createOscillator(),lg=ac.createGain();lfo.frequency.value=24;lg.gain.value=.12;lfo.connect(lg);lg.connect(g.gain);g.gain.setValueAtTime(.14,t);g.gain.linearRampToValueAtTime(0,t+1.6);s.connect(fl);fl.connect(g);g.connect(master);s.start(t);lfo.start(t);s.stop(t+1.7);lfo.stop(t+1.7);},
      thunder:function(){burst(90,.6,.5,2.2,'lowpass');setTimeout(function(){burst(160,.5,.3,1.4,'lowpass');},180);},
      dice:function(){for(var i=0;i<5;i++)setTimeout(function(){tone(rnd(900,1500),.05,.05,'square');},i*70);},
      page:function(){burst(2200,.5,.16,.35,'bandpass');setTimeout(function(){burst(900,.6,.1,.25,'lowpass');},180);},
      wax:function(){burst(1200,.8,.06,.5,'lowpass');},
      done:function(){[523,659,784,1047].forEach(function(f,i){setTimeout(function(){tone(f,.12,.9);},i*110);});}
    };
    return {set:set,tick:tick,pour:pour,rain:rain,fx:function(n){if(on&&fx.hasOwnProperty(n))fx[n]();},isOn:function(){return on;},names:function(){return Object.keys(fx);}};
  })();

  function el(tag,attrs,html){var e=document.createElementNS(NS,tag);for(var k in attrs)e.setAttribute(k,attrs[k]);if(html)e.innerHTML=html;return e;}
  function arcPath(cx,cy,r,a0,a1){var x0=cx+r*Math.cos(a0),y0=cy+r*Math.sin(a0),x1=cx+r*Math.cos(a1),y1=cy+r*Math.sin(a1);
    return 'M'+x0.toFixed(1)+','+y0.toFixed(1)+' A'+r+','+r+' 0 '+(a1-a0>Math.PI?1:0)+' 1 '+x1.toFixed(1)+','+y1.toFixed(1);}
  function d10(x,y,v,cls){return '<g class="cr-die '+(cls||'')+'" transform="translate('+x+','+y+')"><path class="cr-die-face" d="M0,-30 L26,-6 L16,26 L-16,26 L-26,-6 Z"/><path class="cr-die-edge" d="M0,-30 L0,-12 M-26,-6 L-10,4 M26,-6 L10,4 M-16,26 L-10,4 L0,-12 L10,4 L16,26"/><text class="cr-die-n" x="0" y="0" text-anchor="middle" dominant-baseline="central">'+v+'</text></g>';}

  function mount(root,skin,opts){
    opts=opts||{};
    var set=Object.assign({},DEFAULTS,opts.settings||{}),L=skin.layout,RECIPES=cleanRecipes(opts.recipes),ITEMS=cleanItems(opts.items);
    var pfx='crs'+(++seq)+'-',scope=scoper(pfx),dead=false,raf=0,visible=true,timers=[],calm=isCalm();
    if(skin.bind)skin.bind({scope:scope,id:function(n){return pfx+n;},calm:calm});
    root.innerHTML='<div class="cr-stage" tabindex="0" aria-label="Crafting scene"><svg class="cr-svg" viewBox="0 0 1200 700" preserveAspectRatio="xMidYMid slice" role="img" aria-label="'+esc(skin.label)+'"><defs>'+scope(skin.defs())+'</defs>'+
      '<g class="cr-bg">'+scope(skin.background())+'</g><g class="cr-book"></g><g class="cr-app"></g><g class="cr-items"></g><g class="cr-shade" pointer-events="none">'+scope(skin.shade?skin.shade():'')+'</g><g class="cr-ui"></g><g class="cr-top" pointer-events="none">'+scope(skin.foreground?skin.foreground():'')+'</g></svg>'+
      '<canvas class="cr-fx" aria-hidden="true"></canvas><div class="cr-card" role="tooltip"></div><div class="cr-menu" role="menu" hidden></div><div class="cr-hint" role="status" aria-live="polite"><span class="cr-say"></span><span class="cr-acts"></span></div></div>';
    var stage=root.querySelector('.cr-stage'),svg=root.querySelector('.cr-svg'),P=Particles(root.querySelector('.cr-fx'));
    var gBook=svg.querySelector('.cr-book'),gApp=svg.querySelector('.cr-app'),gItems=svg.querySelector('.cr-items'),gUI=svg.querySelector('.cr-ui');
    var say=root.querySelector('.cr-say'),acts=root.querySelector('.cr-acts'),card=root.querySelector('.cr-card'),menu=root.querySelector('.cr-menu');
    var S,parts={},tweens=[],clock=0,listeners=[];
    // Timers die with the scene, so a step finishing after destroy() touches nothing.
    function later(fn,ms){var id=setTimeout(function(){drop(id);if(!dead)fn();},ms);timers.push(id);return id;}
    function drop(id){var i=timers.indexOf(id);if(i>=0)timers.splice(i,1);}
    function cancel(id){clearTimeout(id);drop(id);}

    function on(fn){listeners.push(fn);}
    function emitChange(){listeners.forEach(function(f){f(S);});}
    function tween(o,to,dur,done,ease){var from={};for(var k in to)from[k]=o[k];tweens.push({o:o,from:from,to:to,t:0,d:calm?.01:dur,done:done,ease:ease||'out'});}
    function runTweens(dt){for(var i=tweens.length-1;i>=0;i--){var w=tweens[i];w.t+=dt;var k=clamp(w.t/w.d,0,1),e=w.ease==='inout'?(k<.5?2*k*k:1-Math.pow(-2*k+2,2)/2):1-Math.pow(1-k,3);
      for(var p in w.to)w.o[p]=lerp(w.from[p],w.to[p],e);if(k>=1){tweens.splice(i,1);if(w.done)w.done();}}}

    function levelY(lv){var f=L.flask;return lerp(f.y+f.r*.9,f.y-f.r*.72,lv);}
    // Where an ingredient waits on the bench: the skin's own spot for it, or the next free one.
    function spot(k,i){if(skin.itemSpot)return skin.itemSpot(k,i);var sl=L.slots||[[1000,630]];return sl[i%sl.length];}
    function build(){
      var f=L.flask,m=L.mortar;
      gApp.innerHTML=scope(skin.art.burner(L)+'<g class="cr-flaskg">'+skin.art.flask(L)+'<g class="cr-marks"></g></g>'+
        '<path class="cr-stream" d="" fill="none"/>'+
        '<g class="cr-mortar cr-grab">'+skin.art.mortar(m.r)+'</g><g class="cr-pestle">'+skin.art.pestle()+'</g><g class="cr-ring"></g><g class="cr-result"></g>');
      var slot=gApp.querySelector('.cr-liqslot');
      slot.innerHTML=scope('<g clip-path="url(#cr-fclip)"><rect class="cr-liq" x="'+(f.x-f.r-4)+'" width="'+(f.r*2+8)+'" y="0" height="0"/><ellipse class="cr-liqtop" cx="'+f.x+'" cy="0" rx="0" ry="5"/></g>');
      parts.liq=slot.querySelector('.cr-liq');parts.top=slot.querySelector('.cr-liqtop');
      parts.flame=gApp.querySelector('.cr-flame');parts.flask=gApp.querySelector('.cr-flaskg');parts.marks=gApp.querySelector('.cr-marks');
      parts.mortar=gApp.querySelector('.cr-mortar');parts.paste=gApp.querySelector('.cr-paste');parts.pestle=gApp.querySelector('.cr-pestle');
      parts.stream=gApp.querySelector('.cr-stream');parts.ring=gApp.querySelector('.cr-ring');parts.result=gApp.querySelector('.cr-result');
      parts.bellows=gApp.querySelector('.cr-bellows');
      gItems.innerHTML='';parts.items={};
      S.items.forEach(function(k,i){
        var g=el('g',{'class':'cr-item cr-grab','data-item':k,tabindex:'-1'});g.innerHTML=scope('<rect x="-42" y="-52" width="84" height="104" fill="transparent"/>'+skin.art.item(k,ITEMS[k]));gItems.appendChild(g);
        var h=spot(k,i);parts.items[k]={g:g,x:h[0],y:h[1],rot:0,s:1,home:h,gone:false};});
    }

    /* A skin with turnTo animates a change of recipe (the journal turns the
       page) and calls go() at the moment the new recipe should appear. */
    var turning=false;
    function reset(rk){
      rk=rk&&RECIPES[rk]?rk:(S&&S.rk)||Object.keys(RECIPES)[0];
      if(skin.turnTo&&S&&rk!==S.rk){if(turning)return;turning=true;skin.turnTo(function(){turning=false;if(!dead)setup(rk);});return;}
      setup(rk);
    }
    function setup(rk){var r=RECIPES[rk];if(!r)return;var steps=buildSteps(r,ITEMS);
      var pre=!steps.some(function(s){return s.t==='pour';}),items=[];
      steps.forEach(function(s){if(s.item&&items.indexOf(s.item)<0)items.push(s.item);});
      S={rk:rk,r:r,steps:steps,items:items,i:0,res:[],level:pre?.6:.12,liq:r.base,paste:null,pasteK:0,heat:.12,holding:false,phase:'step',
        mortar:{x:L.mortar.x,y:L.mortar.y,rot:0},pestle:{x:0,y:0,rot:0},sel:null,drag:null,mini:null,verdict:null,points:+r.start||0,roll:null,shake:0,flash:0};
      restPestle();P.clear();tweens.length=0;
      build();gUI.innerHTML='';parts.result.innerHTML='';if(skin.onReset)skin.onReset(S);
      enterStep();emitChange();
    }
    function restPestle(){S.pestle.x=S.mortar.x+L.mortar.r*.32;S.pestle.y=S.mortar.y+2;S.pestle.rot=28;}
    function cur(){return S.steps[S.i];}

    function hot(){
      svg.querySelectorAll('.cr-hot').forEach(function(e){e.classList.remove('cr-hot');});
      var st=cur();if(S.phase!=='step'||!st)return;
      if(st.item&&parts.items[st.item])parts.items[st.item].g.classList.add('cr-hot');
      if(st.t==='grind'||st.t==='tip')parts.mortar.classList.add('cr-hot');
      if(st.t==='grind')parts.pestle.classList.add('cr-hot');
    }
    function setSay(t,btns){say.textContent=t;acts.innerHTML='';(btns||[]).forEach(function(b){var x=document.createElement('button');x.type='button';x.className='cr-btn'+(b.primary?' cr-btn-primary':'');x.textContent=b.label;x.addEventListener('click',b.fn);acts.appendChild(x);});}

    function enterStep(){
      var st=cur();S.mini=null;gUI.innerHTML='';parts.marks.innerHTML='';
      if(S.phase==='step'&&!st){S.phase='roll';S.verdict=verdict(S.res,set);}
      if(S.phase==='roll'){setSay('Work done: '+S.verdict.label+'. Roll the project: 2d10 + '+S.r.stat+(S.verdict.v?(S.verdict.v>0?' + 2':' − 2'):''),[{label:'Roll 2d10',primary:true,fn:roll}]);}
      else if(S.phase==='done'){}
      else{
        setSay(stepSay(st,ITEMS));
        if(st.t==='pour')drawMarks();
        if(st.t==='simmer'){S.mini={t:'simmer',started:false,time:0,inb:0,c:.6,w:DIFF[set.difficulty].band};drawGauge();}
        if(st.t==='strike'){S.mini={t:'strike',k:0,p:0,dir:1,c:rnd(.3,.7),z:DIFF[set.difficulty].zone,hits:[],cool:0};drawBar();}
        if(st.t==='pour')S.mini={t:'pour',posed:false,poured:false};
      }
      hot();renderBook();
    }
    function grade(g,extra){S.res.push({t:cur().t,grade:g,extra:extra});}
    function advance(){S.i++;enterStep();emitChange();if(skin.onStep)skin.onStep(S);}

    function renderBook(){gBook.innerHTML=scope(skin.book({S:S,r:S.r,steps:S.steps,i:S.i,res:S.res,phase:S.phase,verdict:S.verdict||verdict(S.res,set),name:function(st){return stepName(st,ITEMS);},esc:esc,set:set}));}

    function drawMarks(){var f=L.flask,d=DIFF[set.difficulty].pour,tg=.62,x0=f.x+f.r*.35,x1=f.x+f.r*.95;
      function ln(lv,c){var y=levelY(lv),hw=Math.sqrt(Math.max(0,f.r*f.r-(y-f.y)*(y-f.y)));return '<line class="'+c+'" x1="'+(f.x+hw*.45)+'" x2="'+(f.x+hw*.98)+'" y1="'+y+'" y2="'+y+'"/>';}
      parts.marks.innerHTML=ln(tg+d[0],'cr-mark')+ln(tg-d[0],'cr-mark')+ln(tg,'cr-mark cr-mark-gold');}
    function drawGauge(){var g=L.gauge;gUI.innerHTML=scope('<g class="cr-gauge" transform="translate('+g.x+','+g.y+')">'+(skin.art.gaugeFrame?skin.art.gaugeFrame():'')+
      '<path class="cr-g-track" d="'+arcPath(0,0,56,Math.PI,2*Math.PI)+'"/><path class="cr-g-band" d=""/><path class="cr-g-time" d=""/>'+
      '<line class="cr-g-needle" x1="0" y1="0" x2="-50" y2="0"/><circle class="cr-g-hub" r="6"/><text class="cr-g-lab" x="-62" y="18" text-anchor="start">cool</text><text class="cr-g-lab" x="62" y="18" text-anchor="end">boil</text></g>');
      parts.gBand=gUI.querySelector('.cr-g-band');parts.gNeedle=gUI.querySelector('.cr-g-needle');parts.gTime=gUI.querySelector('.cr-g-time');}
    function drawBar(){var b=L.bar;gUI.innerHTML=scope('<g class="cr-bar" transform="translate('+b.x+','+b.y+')">'+(skin.art.barFrame?skin.art.barFrame(b.w):'')+
      '<rect class="cr-b-track" x="0" y="-10" width="'+b.w+'" height="20" rx="4"/><rect class="cr-b-zone" y="-10" height="20"/><rect class="cr-b-perfect" y="-10" height="20"/>'+
      '<line class="cr-b-mark" y1="-18" y2="18"/><g class="cr-b-pips"></g></g>');
      parts.bZone=gUI.querySelector('.cr-b-zone');parts.bPerf=gUI.querySelector('.cr-b-perfect');parts.bMark=gUI.querySelector('.cr-b-mark');parts.bPips=gUI.querySelector('.cr-b-pips');placeZone();}
    function placeZone(){var b=L.bar,m=S.mini;parts.bZone.setAttribute('x',(m.c-m.z)*b.w);parts.bZone.setAttribute('width',2*m.z*b.w);
      parts.bPerf.setAttribute('x',(m.c-m.z*.35)*b.w);parts.bPerf.setAttribute('width',.7*m.z*b.w);
      var h='';for(var i=0;i<cur().n;i++){var r=m.hits[i];h+='<circle class="cr-b-pip'+(r?' is-'+r:'')+'" cx="'+(b.w/2+(i-(cur().n-1)/2)*20)+'" cy="30" r="5"/>';}parts.bPips.innerHTML=h;}

    /* Pointer play. Every drag also works as two clicks (pick, then place),
       and every hold or press also works with the space bar. */
    function pt(e){var p=svg.createSVGPoint();p.x=e.clientX;p.y=e.clientY;var m=svg.getScreenCTM();return m?p.matrixTransform(m.inverse()):{x:0,y:0};}
    function dist(a,b,c,d){return Math.hypot(a-c,b-d);}
    function targetOf(st){return st.t==='tip'||st.to==='flask'||st.t==='pour'?{x:L.flask.x,y:L.flask.neck+20,r:120,sel:'.cr-flaskg'}:{x:L.mortar.x,y:L.mortar.y,r:L.mortar.r*1.3,sel:'.cr-mortar'};}
    // The flask's target circle sits at its neck, so its round body counts too: the hint says "the flask", and players aim at the bulb.
    function onTarget(x,y,tg,n){if(dist(x,y,tg.x,tg.y)<tg.r)return true;if(n&&n.closest&&n.closest(tg.sel))return true;
      return tg.sel==='.cr-flaskg'&&dist(x,y,L.flask.x,L.flask.y)<L.flask.r*1.15;}

    // A skin's own drags (the journal's signet) get the pointer first and say whether they took it.
    function down(e){
      if(skin.down&&skin.down(e,pt(e))){e.preventDefault();return;}
      if(S.phase!=='step')return;var st=cur(),p=pt(e),hitItem=e.target.closest&&e.target.closest('[data-item]');
      if(S.sel){var tg=targetOf(st);if(onTarget(p.x,p.y,tg,e.target)){var s=S.sel;S.sel=null;svg.classList.remove('cr-picking');place(s);e.preventDefault();return;}}
      if((st.t==='add'||(st.t==='pour'&&!S.mini.posed))&&hitItem&&hitItem.getAttribute('data-item')===st.item){
        var it=parts.items[st.item];S.drag={k:'item',it:it,ox:p.x-it.x,oy:p.y-it.y,sx:p.x,sy:p.y,moved:0};}
      else if(st.t==='tip'&&(e.target.closest('.cr-mortar')||dist(p.x,p.y,S.mortar.x,S.mortar.y)<L.mortar.r*1.2)){
        S.drag={k:'mortar',ox:p.x-S.mortar.x,oy:p.y-S.mortar.y,sx:p.x,sy:p.y,moved:0};}
      else if(st.t==='grind'&&dist(p.x,p.y,S.mortar.x,S.mortar.y)<L.mortar.r*1.9){S.drag={k:'grind',last:null};grindMove(p);}
      else if(st.t==='pour'&&S.mini.posed||st.t==='simmer'){press();}
      else if(st.t==='strike'){strike();}
      else return;
      try{svg.setPointerCapture(e.pointerId);}catch(_){}
      e.preventDefault();
    }
    function move(e){if(skin.move&&skin.move(e,pt(e)))return;if(!S.drag){hover(e);return;}hideCard();var p=pt(e),d=S.drag;
      if(d.k==='grind'){grindMove(p);return;}
      d.moved=Math.max(d.moved,dist(p.x,p.y,d.sx,d.sy));
      if(d.k==='item'){d.it.x=p.x-d.ox;d.it.y=p.y-d.oy;var tg=targetOf(cur());d.it.over=onTarget(d.it.x,d.it.y,tg);}
      if(d.k==='mortar'){S.mortar.x=p.x-d.ox;S.mortar.y=p.y-d.oy;var near=dist(S.mortar.x,S.mortar.y,L.flask.x,L.flask.neck)<170;S.mortar.rot=lerp(S.mortar.rot,near?L.tip.rot:0,.25);restPestle();}
    }
    function up(e){if(skin.up&&skin.up(e,pt(e)))return;if(S.holding)release();var d=S.drag;S.drag=null;if(!d)return;
      if(d.k==='grind'){if(cur()&&cur().t==='grind')restPestle();return;}
      var st=cur(),tg=targetOf(st);
      if(d.k==='item'){if(d.moved<6){S.sel=d.it;svg.classList.add('cr-picking');setSay('Now click the '+(st.to==='mortar'?'mortar':'flask')+' to put it in');return;}
        if(onTarget(d.it.x,d.it.y,tg))place(d.it);else home(d.it);}
      if(d.k==='mortar'){if(d.moved<6){S.sel='mortar';svg.classList.add('cr-picking');setSay('Now click the flask to tip it in');return;}
        if(dist(S.mortar.x,S.mortar.y,L.flask.x,L.flask.neck)<190)place('mortar');else{tween(S.mortar,{x:L.mortar.x,y:L.mortar.y,rot:0},.35);}}
    }
    function home(it){tween(it,{x:it.home[0],y:it.home[1],rot:0},.35);}

    function place(what){var st=cur();
      if(what==='mortar'){var tp=L.tip;tween(S.mortar,{x:tp.x,y:tp.y,rot:tp.rot},.35,function(){
          var n=0;(function drip(){skin.fx.drop(P,tp.mx+rnd(-6,6),tp.my,S.paste||S.r.paste);if(++n<=18)later(drip,40);})();
          Sound.fx('drop');later(function(){Sound.fx('drop');},300);
          tween(S,{level:S.level+.06},.8);var from=S.liq;var o={k:0};tween(o,{k:1},.9,function(){S.paste=null;S.pasteK=0;tween(S.mortar,{x:L.mortar.x,y:L.mortar.y,rot:0},.45,function(){restPestle();advance();});});
          (function step(){if(o.k<1){S.liq=mix(from,S.r.mid,o.k);frame(step);}else S.liq=S.r.mid;})();
        });return;}
      var it=what;
      if(st.t==='pour'){var pp=L.pour;tween(it,{x:pp.x,y:pp.y,rot:pp.rot},.35,function(){S.mini.posed=true;setSay('Press and hold to pour. Let go on the gold line');});return;}
      var tg=st.to==='mortar'?{x:L.mortar.x,y:L.mortar.y-6}:{x:L.flask.x,y:L.flask.neck+10};
      tween(it,{x:tg.x,y:tg.y,s:.25,rot:it.rot+40},.4,function(){it.gone=true;it.g.style.display='none';
        if(st.to==='mortar'){S.paste=ITEMS[st.item].color;S.pasteK=0;Sound.fx('drop');for(var i=0;i<10;i++)skin.fx.dust(P,tg.x+rnd(-20,20),tg.y,S.paste);}
        else{Sound.fx('fizz');skin.fx.fizz(P,L.flask.x,levelY(S.level),ITEMS[st.item].color);var from=S.liq,o={k:0};tween(o,{k:1},1.2);(function s2(){S.liq=mix(from,ITEMS[st.item].color,o.k*.6);if(o.k<1)frame(s2);})();S.fizzing=true;}
        advance();});
    }

    function grindMove(p){var m=S.mortar,r=L.mortar.r,dx=p.x-m.x,dy=(p.y-m.y)*2.6,a=Math.atan2(dy,dx),R=Math.min(1,Math.hypot(dx,dy)/(r*.62));
      S.pestle.x=m.x+Math.cos(a)*r*.55*R;S.pestle.y=m.y+Math.sin(a)*r*.2*R+4;S.pestle.rot=18+Math.cos(a)*14;
      var d=S.drag;if(d.last!==null){var da=a-d.last;if(da>Math.PI)da-=2*Math.PI;if(da<-Math.PI)da+=2*Math.PI;d.acc=(d.acc||0)+Math.abs(da)*R;S.grindAcc=(S.grindAcc||0)+Math.abs(da)*R;
        if(Math.abs(da)>.05){if(Math.random()<.5)skin.fx.dust(P,S.pestle.x,S.pestle.y,S.paste||'#888888');if(Math.random()<.25)Sound.fx('grind');}}
      d.last=a;var need=cur().turns*2*Math.PI,k=clamp((S.grindAcc||0)/need,0,1);S.pasteK=k;
      parts.ring.innerHTML='<path class="cr-ringpath" d="'+arcPath(m.x,m.y+L.mortar.r*.5,L.mortar.r*1.25,-Math.PI/2,-Math.PI/2+Math.max(.001,k)*2*Math.PI*.999)+'"/>';
      setSay('Grinding: '+Math.min(cur().turns,Math.floor(k*cur().turns+.001))+' of '+cur().turns+' turns');
      if(k>=1){S.drag=null;S.grindAcc=0;parts.ring.innerHTML='';restPestle();Sound.fx('clink');advance();}
    }

    function press(){var st=cur();if(!st)return;S.holding=true;
      if(st.t==='simmer'&&!S.mini.started){S.mini.started=true;}
      if(st.t==='simmer'){Sound.fx('pump');if(parts.bellows)parts.bellows.classList.add('pump');}
    }
    function release(){S.holding=false;if(parts.bellows)parts.bellows.classList.remove('pump');Sound.pour(false);var st=cur();
      if(st&&st.t==='pour'&&S.mini.posed&&S.mini.poured){finishPour();}}
    function finishPour(){var d=DIFF[set.difficulty].pour,e=Math.abs(S.level-.62),g=e<=d[1]?'perfect':e<=d[0]?'good':'miss';
      parts.stream.setAttribute('d','');grade(g);feedback(g,L.flask.x,levelY(S.level));var it=parts.items[cur().item];if(it)home(it);S.mini.poured=false;later(advance,450);}
    function strike(){var m=S.mini;if(m.cool>0)return;var d=Math.abs(m.p-m.c),g=d<m.z*.35?'perfect':d<m.z?'good':'miss';m.hits.push(g);m.cool=.35;
      feedback(g,L.flask.x,levelY(S.level)-10);if(g!=='miss')S.flash=1;else S.shake=1;
      if(m.hits.length>=cur().n){var miss=m.hits.filter(function(h){return h==='miss';}).length,per=m.hits.filter(function(h){return h==='perfect';}).length;
        grade(miss>=2?'miss':miss===0&&per>=2?'perfect':'good');placeZone();later(function(){S.fizzing=false;tweenColor(S.r.final,.8);advance();},500);return;}
      m.c=rnd(.25,.75);m.z*=.82;placeZone();}
    function feedback(g,x,y){Sound.fx(g);if(skin.onFeedback)skin.onFeedback(g,x,y,S);skin.fx.spark(P,x,y,g);if(g==='miss')skin.fx.smoke(P,x,y);
      var t=el('text',{'class':'cr-pop is-'+g,x:x,y:y-40,'text-anchor':'middle'});t.textContent=g==='perfect'?'Perfect':g==='good'?'Good':'Miss';gUI.appendChild(t);later(function(){t.remove();},1100);}
    function tweenColor(c,dur){var from=S.liq,o={k:0};tween(o,{k:1},dur);(function s(){S.liq=mix(from,c,o.k);if(o.k<1)frame(s);})();}

    function roll(){if(S.phase!=='roll')return;S.phase='rolling';acts.innerHTML='';Sound.fx('dice');S.dice={t:0};renderBook();}
    function finishRoll(){var a=1+Math.floor(Math.random()*10),b=1+Math.floor(Math.random()*10),v=S.verdict.v,total=a+b+S.r.mod+v;
      S.roll={a:a,b:b,total:total};S.dice=null;drawDice(a,b,true);S.points=Math.min(S.r.goal,S.r.start+total);S.phase='done';renderBook();emitChange();
      var again=[{label:'Again',fn:function(){reset();}}];
      if(S.points>=S.r.goal){var R=L.result,o={s:0};parts.result.innerHTML=scope('<g class="cr-bottle">'+skin.art.bottle(S.r.final,S.r.name)+'</g>');var gB=parts.result.firstChild;
        tween(o,{s:1},.6);(function g(){gB.setAttribute('transform','translate('+R.x+','+R.y+') scale('+o.s+')');if(o.s<1)frame(g);})();
        // A skin's onDone may return true to finish in its own time (the journal's seal), calling fin when it has.
        var fin=function(){if(!dead)setSay('Finished. One '+S.r.name.toLowerCase()+' is in '+S.r.who+'’s pack.',again);};
        skin.fx.success(P,R.x,R.y-30,S.r.final);Sound.fx('done');if(!(skin.onDone&&skin.onDone(S,fin)===true))fin();}
      else setSay('+'+total+' project points. '+(S.r.goal-S.points)+' to go at the next respite.',again);}
    function drawDice(a,b,fin){var D=L.dice;gUI.innerHTML='<g class="cr-dice'+(fin?' is-final':'')+'">'+d10(D.x-34,D.y,a)+d10(D.x+34,D.y,b,'b')+'</g>';}

    function render(dt){
      var st=cur();
      for(var k in parts.items){var it=parts.items[k];if(!it.gone)it.g.setAttribute('transform','translate('+it.x.toFixed(1)+','+it.y.toFixed(1)+') rotate('+it.rot.toFixed(1)+') scale('+it.s+')');}
      parts.mortar.setAttribute('transform','translate('+S.mortar.x.toFixed(1)+','+S.mortar.y.toFixed(1)+') rotate('+S.mortar.rot.toFixed(1)+')');
      if(parts.paste){parts.paste.style.opacity=S.paste?1:0;if(S.paste)parts.paste.style.fill=mix(S.paste,S.r.paste,S.pasteK);}
      parts.pestle.setAttribute('transform','translate('+S.pestle.x.toFixed(1)+','+S.pestle.y.toFixed(1)+') rotate('+S.pestle.rot.toFixed(1)+')');
      var f=L.flask,y=levelY(S.level),hw=Math.sqrt(Math.max(0,f.r*f.r-(y-f.y)*(y-f.y)));if(y<f.y-f.r*.6)hw=Math.max(hw,f.neckW||14);
      var col=S.flash>0?mix(S.liq,'#ffffff',S.flash*.6):S.liq;
      parts.liq.setAttribute('y',y.toFixed(1));parts.liq.setAttribute('height',(f.y+f.r+8-y).toFixed(1));parts.liq.style.fill=col;
      parts.top.setAttribute('cy',y.toFixed(1));parts.top.setAttribute('rx',(hw*.96).toFixed(1));parts.top.setAttribute('ry',(4+S.heat*3*Math.abs(Math.sin(clock*9))).toFixed(1));parts.top.style.fill=mix(col,'#ffffff',.35);
      var B=L.burner,s=.55+S.heat*.95;if(parts.flame)parts.flame.setAttribute('transform','translate('+B.x+','+B.y+') scale('+(.85+S.heat*.3).toFixed(2)+','+s.toFixed(2)+') translate('+(-B.x)+','+(-B.y)+')');
      var sh=S.shake>0?Math.sin(clock*80)*4*S.shake:0;parts.flask.setAttribute('transform','translate('+sh.toFixed(1)+',0)');
      if(st&&st.t==='pour'&&S.mini&&S.mini.posed&&S.holding){var m=L.pour.mouth;parts.stream.setAttribute('d','M'+m[0]+','+m[1]+' Q'+(m[0]+12)+','+(m[1]+30)+' '+(f.x-4)+','+y.toFixed(1));parts.stream.style.stroke=ITEMS[st.item].color;}
      else parts.stream.setAttribute('d','');
      if(S.mini&&S.mini.t==='simmer'){var mm=S.mini,a=Math.PI+S.heat*Math.PI;parts.gNeedle.setAttribute('x2',(Math.cos(a)*48).toFixed(1));parts.gNeedle.setAttribute('y2',(Math.sin(a)*48).toFixed(1));
        parts.gBand.setAttribute('d',arcPath(0,0,56,Math.PI+(mm.c-mm.w)*Math.PI,Math.PI+(mm.c+mm.w)*Math.PI));
        parts.gTime.setAttribute('d',mm.time>0?arcPath(0,0,68,Math.PI,Math.PI+Math.min(.999,mm.time/cur().secs)*Math.PI):'');
        parts.gBand.classList.toggle('is-in',Math.abs(S.heat-mm.c)<mm.w);}
      if(S.mini&&S.mini.t==='strike'&&parts.bMark){var x=S.mini.p*L.bar.w;parts.bMark.setAttribute('x1',x.toFixed(1));parts.bMark.setAttribute('x2',x.toFixed(1));}
    }

    function update(dt){
      var st=cur(),m=S.mini;
      if(S.phase==='step'&&st){
        if(st.t==='pour'&&m.posed&&S.holding){S.level+=.2*dt;m.poured=true;Sound.pour(true);if(Math.random()<.5)skin.fx.drop(P,L.flask.x+rnd(-5,5),levelY(S.level),ITEMS[st.item].color);
          if(S.level>=.97){S.level=.97;S.holding=false;Sound.pour(false);for(var i=0;i<14;i++)skin.fx.drop(P,L.flask.x+rnd(-14,14),L.flask.neck,ITEMS[st.item].color,true);finishPour();}}
        if(st.t==='simmer'&&m.started){S.heat=clamp(S.heat+(S.holding?.62:-.36)*dt,0,1);m.time+=dt;m.c=.6+.13*Math.sin(m.time*1.4);
          if(Math.abs(S.heat-m.c)<m.w){m.inb+=dt;S.liq=mix(S.liq,S.r.final,dt*.35);}
          if(S.heat>m.c+m.w+.1&&Math.random()<dt*6){skin.fx.steam(P,L.flask.x,L.flask.neck,1.4);S.shake=Math.max(S.shake,.4);}
          if(m.time>=st.secs){var fr=m.inb/m.time,g=fr>=.75?'perfect':fr>=.5?'good':'miss';grade(g,fr);feedback(g,L.flask.x,levelY(S.level));if(g!=='miss')tweenColor(S.r.final,.6);S.holding=false;if(parts.bellows)parts.bellows.classList.remove('pump');advance();}}
        if(st.t==='strike'){m.p+=m.dir*dt*.9*DIFF[set.difficulty].speed*(1+m.hits.length*.22);if(m.p>1){m.p=1;m.dir=-1;}if(m.p<0){m.p=0;m.dir=1;}m.cool-=dt;}
      }
      if(!(st&&st.t==='simmer'&&m&&m.started))S.heat=lerp(S.heat,S.fizzing?.45:.22,dt*1.5);
      if(S.dice){S.dice.t+=dt;if(Math.floor(S.dice.t*14)!==Math.floor((S.dice.t-dt)*14))drawDice(1+Math.floor(Math.random()*10),1+Math.floor(Math.random()*10));if(S.dice.t>.9)finishRoll();}
      S.shake=Math.max(0,S.shake-dt*2.5);S.flash=Math.max(0,S.flash-dt*2.2);
      skin.fx.bubbles(P,L.flask,levelY(S.level),S.liq,S.fizzing?Math.max(S.heat,.8):S.heat,dt);
    }


    /* Hover cards and the right-click menu. Every object says what it is and
       what you can do with it now; the menu holds the same actions plus a few
       for flavour. A long press opens the menu on touch. */
    function targetAt(n){if(!n||!n.closest)return null;var it=n.closest('[data-item]');if(it)return {id:'item:'+it.getAttribute('data-item'),el:it};
      // The innermost wins, so a prop drawn inside the book (the journal's moons) is the one hovered
      var pr=n.closest('[data-prop]'),m=[['.cr-mortar','mortar'],['.cr-pestle','pestle'],['.cr-flaskg','flask'],['.cr-book','book']];
      for(var i=0;i<m.length;i++){var e=n.closest(m[i][0]);if(e&&!(pr&&e.contains(pr)))return {id:m[i][1],el:e};}
      return pr?{id:pr.getAttribute('data-prop'),el:pr}:null;}
    // The tools every recipe uses. A skin's info table renames or rewords them; their built-in actions stay.
    var BASE={mortar:{name:'Mortar',text:'Granite, heavy enough to stay put.'},pestle:{name:'Pestle',text:'Worn smooth by a hundred grindings.'},
      flask:{name:'Round flask',text:'Thin glass on a brass ring over the burner.',acts:[['Swirl it','swirl']]},book:{name:'Recipe book',text:'The recipe, copied out by hand.',acts:[['Read the recipe','read']]}};
    function infoFor(id){var st=S.phase==='step'?cur():null,inf={name:id,text:'',acts:[]},k;
      if(id.indexOf('item:')===0){k=id.slice(5);var I=ITEMS[k]||{name:k};inf={name:I.name,text:I.desc||'',meta:S.r.who?'From '+S.r.who+'’s pack':'',acts:[]};
        if(st&&st.item===k&&!(st.t==='pour'&&S.mini&&S.mini.posed)){inf.now=st.t==='pour'?'Drag it over the flask.':'Drag it into the '+st.to+'.';inf.acts.push([st.t==='pour'?'Hold it over the flask':'Put it in the '+st.to,'use']);}}
      else{var b=BASE[id]||{},sk=(skin.info&&skin.info[id])||{};inf={name:sk.name||b.name||id,text:sk.text||b.text||'',acts:(b.acts||[]).concat(sk.acts||[]).map(normAct).filter(Boolean)};
        if(id==='mortar'&&st&&st.t==='grind'){inf.now='Circle the pestle around the bowl.';}
        if(id==='mortar'&&st&&st.t==='tip'){inf.now='Drag it over the flask.';inf.acts.unshift(['Tip it into the flask','use']);}
        if(id==='flask'&&st&&(st.t==='simmer'||st.t==='strike'))inf.now=st.t==='simmer'?'Hold to pump the bellows.':'Press when the mark crosses the gold.';}
      inf.acts.push(['Examine','examine']);return inf;}
    var hovT=null,hovId=null,hovEl=null,pinned=false;
    function place2(x,y,el){var r=stage.getBoundingClientRect(),w=el.offsetWidth,h=el.offsetHeight,lx=x-r.left+14,ly=y-r.top+16;
      if(lx+w>r.width-8)lx=x-r.left-w-14;if(ly+h>r.height-8)ly=y-r.top-h-12;el.style.left=Math.max(8,lx)+'px';el.style.top=Math.max(8,ly)+'px';}
    function cardHTML(inf,more){return '<b>'+esc(inf.name)+'</b>'+(inf.meta?'<span class="cr-meta">'+esc(inf.meta)+'</span>':'')+(inf.text||more?'<p>'+esc(more||inf.text)+'</p>':'')+(inf.now?'<p class="cr-now">'+esc(inf.now)+'</p>':'')+(more?'':'<p class="cr-foot">Right-click for more</p>');}
    function hover(e){if(pinned)return;var t=targetAt(e.target);if(!t){hideCard();return;}
      if(hovEl!==t.el){if(hovEl)hovEl.classList.remove('cr-hov');hovEl=t.el;hovEl.classList.add('cr-hov');}
      if(t.id!==hovId){hovId=t.id;card.classList.remove('show');cancel(hovT);var cx=e.clientX,cy=e.clientY;hovT=later(function(){card.innerHTML=cardHTML(infoFor(t.id));place2(cx,cy,card);card.classList.add('show');},250);}
      else if(card.classList.contains('show'))place2(e.clientX,e.clientY,card);}
    function hideCard(){cancel(hovT);hovId=null;if(hovEl){hovEl.classList.remove('cr-hov');hovEl=null;}if(!pinned)card.classList.remove('show');}
    function pin(html,x,y){pinned=true;card.innerHTML=html+'<button class="cr-x" type="button" aria-label="Close">×</button>';card.classList.add('show','pinned');place2(x,y,card);
      card.querySelector('.cr-x').addEventListener('click',unpin);}
    function unpin(){pinned=false;card.classList.remove('show','pinned');}
    // An action is [label, name] for one the engine or skin knows, or a Director's own {label, fx, sound, say}.
    function normAct(a){if(Array.isArray(a))return [String(a[0]),String(a[1])];if(a&&typeof a==='object')return [String(a.label||'Untitled action'),'custom',a];return null;}
    function openMenu(t,x,y){hideCard();unpin();var inf=infoFor(t.id);menu.innerHTML='<div class="cr-mhead">'+esc(inf.name)+'</div>';
      inf.acts.forEach(function(a){var b=document.createElement('button');b.type='button';b.setAttribute('role','menuitem');b.textContent=a[0];b.addEventListener('click',function(){closeMenu();act(t,a[1],x,y,a[2]);});menu.appendChild(b);});
      menu.hidden=false;place2(x,y,menu);var f=menu.querySelector('button');if(f)f.focus({preventScroll:true});}
    function closeMenu(){menu.hidden=true;}
    function act(t,a,x,y,A){var st=cur(),inf=infoFor(t.id);
      if(a==='custom'){custom(t,A||{});return;}
      if(a==='examine'){pin(cardHTML(inf,(skin.info&&skin.info[t.id]&&skin.info[t.id].more)||inf.text),x,y);return;}
      if(a==='use'){if(t.id.indexOf('item:')===0){var k=t.id.slice(5);if(st&&st.item===k)place(parts.items[k]);}else if(t.id==='mortar'&&st&&st.t==='tip')place('mortar');return;}
      if(a==='swirl'){S.shake=.8;skin.fx.fizz(P,L.flask.x,levelY(S.level),S.liq);Sound.fx('fizz');return;}
      if(a==='read'){var h='<b>'+esc(S.r.name)+'</b><span class="cr-meta">'+esc(S.r.kind)+'</span><ol>'+S.steps.map(function(s){return '<li>'+esc(stepName(s,ITEMS))+'</li>';}).join('')+'</ol><p class="cr-now">Then roll 2d10 + '+esc(S.r.stat)+'. Goal '+S.r.goal+'.</p>';pin(h,x,y);return;}
      if(skin.act)skin.act(t.id,a,{P:P,S:S,svg:svg,el:t.el,sfx:Sound.fx,pop:popText,heat:function(v){S.heat=Math.min(1,S.heat+v);},pump:function(){if(parts.bellows){parts.bellows.classList.add('pump');later(function(){parts.bellows.classList.remove('pump');},220);}}});}
    /* A Director's own action plays one effect from a fixed list, then a
       sound and a line. Fixed lists keep a shared bench safe: an action is
       data, never code. "b:<name>" reuses one of the skin's own actions. */
    function boxOf(n){var r=n.getBoundingClientRect(),m=svg.getScreenCTM();if(!m||!r.width)return null;var iv=m.inverse(),p=svg.createSVGPoint();
      p.x=r.left;p.y=r.top;var a=p.matrixTransform(iv);p.x=r.right;p.y=r.bottom;var b=p.matrixTransform(iv);return {x:a.x,y:a.y,w:b.x-a.x,h:b.y-a.y,cx:(a.x+b.x)/2,cy:(a.y+b.y)/2};}
    function custom(t,A){var b=boxOf(t.el),fx=String(A.fx||''),i;
      if(fx.indexOf('b:')===0)act(t,fx.slice(2));
      else if(b&&fx==='shake'){if(!calm&&t.el.animate)t.el.animate([{translate:'0 0'},{translate:'3px 0'},{translate:'-3px 0'},{translate:'2px 0'},{translate:'0 0'}],{duration:420});}
      else if(b&&fx==='sparkle')for(i=0;i<18;i++)P.emit({type:'glow',x:b.cx+rnd(-b.w/3,b.w/3),y:b.cy+rnd(-b.h/3,b.h/4),vx:rnd(-40,40),vy:rnd(-90,-20),life:rnd(.5,1.1),size:rnd(2,3.6),color:'255,214,140'});
      else if(b&&fx==='smoke')for(i=0;i<10;i++)P.emit({type:'soft',x:b.cx+rnd(-10,10),y:b.y+b.h*.3,vx:rnd(-14,14),vy:rnd(-50,-24),life:rnd(1.4,2.4),size:rnd(6,10),grow:14,color:'190,185,180',alpha:.28,fade:'inout'});
      else if(b&&fx==='glow')P.emit({type:'glow',x:b.cx,y:b.cy,size:Math.max(30,Math.max(b.w,b.h)*.75),life:1.2,color:'255,206,140',alpha:.45,fade:'inout'});
      if(A.sound)Sound.fx(String(A.sound));
      if(A.say&&b)popText(String(A.say).slice(0,80),b.cx,Math.max(40,b.y-12),'is-say');}
    function popText(txt,x,y,cls){var t=el('text',{'class':'cr-pop '+(cls||'is-good'),x:x,y:y,'text-anchor':'middle'});t.textContent=txt;gUI.appendChild(t);later(function(){t.remove();},1100);}
    svg.addEventListener('contextmenu',function(e){var t=targetAt(e.target);if(!t)return;e.preventDefault();openMenu(t,e.clientX,e.clientY);});
    svg.addEventListener('pointerleave',hideCard);
    var lpT=null;svg.addEventListener('pointerdown',function(e){closeMenu();if(pinned&&!card.contains(e.target))unpin();if(e.pointerType!=='mouse'){var t=targetAt(e.target),x=e.clientX,y=e.clientY;cancel(lpT);if(t)lpT=later(function(){if(!S.drag||S.drag.moved<6){S.drag=null;openMenu(t,x,y);}},550);}});
    svg.addEventListener('pointerup',function(){cancel(lpT);});svg.addEventListener('pointermove',function(e){if(S.drag&&S.drag.moved>8)cancel(lpT);});
    function outside(e){if(!menu.hidden&&!menu.contains(e.target)&&!stage.contains(e.target))closeMenu();}
    document.addEventListener('pointerdown',outside);
    stage.addEventListener('keydown',function(e){if(e.key==='Escape'){closeMenu();unpin();}});

    /* One frame loop per scene. It stops while the scene is off screen or the
       tab is hidden, so a bench left open costs nothing, and picks up again
       without a jump in time. */
    var last=0;
    function frame(fn){if(!dead)requestAnimationFrame(fn);}
    function loop(now){raf=0;if(dead)return;var dt=last?Math.min(.05,(now-last)/1000):0;last=now;clock+=dt;
      runTweens(dt);update(dt);render(dt);if(skin.ambient)skin.ambient(P,dt,S,clock);if(skin.light)skin.light(clock,S,svg);P.tick(dt);Sound.tick(dt,S.heat);kick();}
    function kick(){if(dead)return;if(visible&&!document.hidden){if(!raf)raf=requestAnimationFrame(loop);}else{if(raf)cancelAnimationFrame(raf);raf=0;last=0;}}
    var io=window.IntersectionObserver?new IntersectionObserver(function(es){visible=es[es.length-1].isIntersecting;kick();}):null;if(io)io.observe(stage);
    document.addEventListener('visibilitychange',kick);

    /* A tap or click on something the step doesn't use opens its card, since a
       touch screen has no hover and a sketch can be hard to make out. */
    var tap=null;
    svg.addEventListener('pointerdown',function(e){stage.focus({preventScroll:true});down(e);
      var t=!e.defaultPrevented&&!S.sel&&targetAt(e.target);tap=t?{t:t,x:e.clientX,y:e.clientY,at:performance.now(),touch:e.pointerType!=='mouse'}:null;});
    svg.addEventListener('pointerup',function(e){var k=tap;tap=null;if(!k||!menu.hidden||performance.now()-k.at>500||Math.hypot(e.clientX-k.x,e.clientY-k.y)>8)return;
      var h=cardHTML(infoFor(k.t.id));if(k.touch)h=h.replace('Right-click for more','Press and hold for more');cancel(hovT);pin(h,e.clientX,e.clientY);});
    svg.addEventListener('pointermove',move);svg.addEventListener('pointerup',up);svg.addEventListener('pointercancel',up);
    stage.addEventListener('keydown',function(e){if(e.code!=='Space'&&e.key!==' ')return;e.preventDefault();if(e.repeat)return;var st=cur();if(S.phase!=='step'||!st)return;
      if(st.t==='strike')strike();else if((st.t==='pour'&&S.mini.posed)||st.t==='simmer')press();});
    stage.addEventListener('keyup',function(e){if(e.code==='Space'||e.key===' ')release();});

    var soundOn=false;
    function destroy(){if(dead)return;dead=true;if(raf)cancelAnimationFrame(raf);raf=0;timers.forEach(clearTimeout);timers.length=0;
      if(io)io.disconnect();document.removeEventListener('visibilitychange',kick);document.removeEventListener('pointerdown',outside);
      if(soundOn)Sound.set(false);P.destroy();if(skin.destroy)skin.destroy();root.innerHTML='';}

    /* What a skin with its own layers and drags needs from the scene, handed
       over once the scene's elements exist and before the first recipe. */
    if(skin.mounted)skin.mounted({P:P,svg:svg,stage:stage,root:root,calm:calm,state:function(){return S;},tween:tween,later:later,
      say:setSay,sfx:Sound.fx,rain:Sound.rain,rerender:function(){if(S)renderBook();}});
    reset(opts.recipe);kick();
    return {reset:reset,settings:set,apply:function(){reset(S.rk);},sound:function(v){soundOn=!!v;Sound.set(v,skin.sound);},
      skip:function(){if(!set.skip||S.phase!=='step')return;S.res=[];S.phase='roll';S.verdict=verdict([],set);S.verdict.why='Steps skipped';S.liq=S.r.final;enterStep();emitChange();},
      on:on,state:function(){return S;},stage:stage,svg:svg,skin:skin,rerender:function(){renderBook();},destroy:destroy};
  }

  return {sfx:function(n){Sound.fx(n);},mount:mount,
    // What a Director's own action may use, for editors to offer.
    effects:['shake','sparkle','smoke','glow'],sounds:Sound.names(),mix:mix,rgb:rgb,rnd:rnd,clamp:clamp,esc:esc,calm:calm,stepName:stepName,d10:d10,
    // The pure parts, for tests and for a skin that wants the same rules.
    rules:{buildSteps:buildSteps,verdict:verdict,scoper:scoper,cleanRecipes:cleanRecipes,cleanItems:cleanItems,DIFF:DIFF}};
})();
