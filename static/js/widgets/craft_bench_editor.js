/**
 * craft_bench_editor.js -- build mode for a crafting bench (window.CraftBenchEditor)
 *
 * Laid out like a game's build mode: the real scene at full width, a tray of
 * room choices under it shown as pictures of the real scene, and a small card
 * beside whichever piece is selected for its name, hover text and right-click
 * actions. Everything it knows about a room comes from the skin's tables
 * (pieces, tabs, choices, kit, thumb, glows, builtins), so it edits any skin
 * that fills them in, unchanged. Actions are made from the engine's fixed
 * effects and sounds, so nothing a Director writes is ever code.
 *
 * It stores nothing itself: state() hands back the bench as data, o.state
 * starts it from a saved one, and o.onChange hears every change, so the
 * crafting app saves it and the Design Lab doesn't. Usage:
 *   var ed = CraftBenchEditor.start(el, {skin: CraftDesk.create(o), recipes: {...}, items: {...},
 *     state: saved, onChange: fn, status: 'Changes save as you go.'});
 *   ed.state(); ed.status('Saved'); ed.destroy();
 */
window.CraftBenchEditor = (function () {
  var EFFECT_LABEL = {shake: 'Shake', sparkle: 'Sparkles', smoke: 'Puff of smoke', glow: 'Warm glow'};
  var SOUND_LABEL = {purr: 'Purr', clink: 'Clink', perfect: 'Chime', thunder: 'Thunder', scratch: 'Quill scratch', seal: 'Wax seal', dice: 'Dice', fizz: 'Fizz', pump: 'Whoosh', drop: 'Plop'};
  var I_LOCK = '<svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><rect x="3.5" y="7" width="9" height="6.5" rx="1.5"/><path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2"/></svg>',
      I_X = '<svg viewBox="0 0 16 16" width="14" height="14" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><path d="M4 4l8 8M12 4l-8 8"/></svg>';
  function esc(s){return String(s==null?'':s).replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c];});}
  function opt(v,l,on){return '<option value="'+esc(v)+'"'+(on?' selected':'')+'>'+esc(l)+'</option>';}

  function start(root, o) {
    o = o || {};
    var skin = o.skin, PIECES = skin.pieces || {}, CH = skin.choices || {}, TABS = skin.tabs || [], BUILTIN = skin.builtins || {};
    var EFFECTS = [['', 'No effect']].concat((CraftScene.effects || []).filter(function (k) { return EFFECT_LABEL[k]; }).map(function (k) { return [k, EFFECT_LABEL[k]]; }));
    var SOUNDS = [['', 'No sound']].concat((CraftScene.sounds || []).filter(function (k) { return SOUND_LABEL[k]; }).map(function (k) { return [k, SOUND_LABEL[k]]; }));
    var scene, ctrl, svg, stage, ov, mode = 'arrange', selId = null, hovId = null, drag = null, st = {}, pendSel = null, lightT = 0, tab = TABS.length ? TABS[0].id : '', openAct = -1, raf = 0, dead = false;
    function $(s){return root.querySelector(s);}
    function movable(id){return !!id&&!!PIECES[id]&&!PIECES[id].lock;}
    function decor(){return Object.keys(PIECES).filter(function(id){var P=PIECES[id];return P.kind!=='tool'&&(!P.kit||skin.opts.kit.indexOf(P.kit)>=0);});}
    function tools(){return Object.keys(PIECES).filter(function(id){return PIECES[id].kind==='tool';});}

    // auto: the Director hasn't touched its words or actions, so it follows the room (a lantern says Lantern)
    function fresh(src){return {name:src.name||'',text:src.text||'',more:src.more||'',dx:0,dy:0,hidden:false,auto:true,
      acts:(src.acts||[]).filter(Array.isArray).map(function(a){return {label:a[0],fx:'b:'+a[1],sound:'',say:''};})};}
    /* A saved piece keeps the Director's words, move and actions; the room's
       look came back through the skin's own opts. */
    function saved(id,src){var p=o.state&&o.state.pieces&&o.state.pieces[id];if(!p||typeof p!=='object')return null;
      var f=fresh(src);return {name:typeof p.name==='string'&&p.name?p.name:f.name,text:typeof p.text==='string'?p.text:f.text,more:typeof p.more==='string'?p.more:f.more,
        dx:+p.dx||0,dy:+p.dy||0,hidden:p.hidden===true,auto:false,
        acts:Array.isArray(p.acts)?p.acts.filter(function(a){return a&&typeof a==='object';}).map(function(a){return {label:String(a.label||''),fx:String(a.fx||''),sound:String(a.sound||''),say:String(a.say||'')};}):f.acts};}
    function initState(){Object.keys(PIECES).forEach(function(id){var src=skin.info[id]||{name:id};st[id]=saved(id,src)||fresh(src);sync(id);place(id);});}
    /* The scene reads hover cards and menus from the skin's info table on
       every hover, so an edit here shows on the next hover. */
    function sync(id){var s=st[id];skin.info[id]={name:s.name||'Unnamed piece',text:s.text,more:s.more||s.text,
      acts:s.acts.map(function(a){return {label:a.label||'Untitled action',fx:a.fx,sound:a.sound,say:a.say};})};}
    /* The skin redrew the room: put back every move and hide, take up new
       names for pieces the Director hasn't renamed, and pick up kit pieces. */
    function redrawn(){
      Object.keys(PIECES).forEach(function(id){var src=skin.info[id];if(!src)return;
        if(!st[id])st[id]=fresh(src);else if(st[id].auto){var was=st[id];st[id]=fresh(src);st[id].dx=was.dx;st[id].dy=was.dy;st[id].hidden=was.hidden;}
        sync(id);place(id);});
      renderTray();if(pendSel){var p=pendSel;pendSel=null;select(p);}else if(selId&&!els(selId).length)select(null);else renderInsp();}
    /* Moving the light moves where it shines from: the skin relights the
       room once the light settles. */
    function moveLight(id){clearTimeout(lightT);lightT=setTimeout(function(){if(dead)return;var s=st[id];skin.setOpt('ldx',s.hidden?0:Math.round(s.dx));skin.setOpt('ldy',s.hidden?0:Math.round(s.dy));},250);}

    function els(id){return PIECES[id]?Array.prototype.slice.call(svg.querySelectorAll(PIECES[id].sel)):[];}
    function toSvg(x,y){var p=svg.createSVGPoint(),m=svg.getScreenCTM();p.x=x;p.y=y;return m?p.matrixTransform(m.inverse()):{x:0,y:0};}
    function box(id){var l=1e9,t=1e9,r=-1e9,b=-1e9,any=false;
      els(id).forEach(function(e){if(e.style.display==='none')return;var q=e.getBoundingClientRect();if(!q.width)return;any=true;
        var a=toSvg(q.left,q.top),c=toSvg(q.right,q.bottom);l=Math.min(l,a.x);t=Math.min(t,a.y);r=Math.max(r,c.x);b=Math.max(b,c.y);});
      return any?{x:l,y:t,w:r-l,h:b-t}:null;}
    function place(id){var s=st[id];if(!s)return;
      els(id).forEach(function(e){if(e.__o==null)e.__o=e.getAttribute('transform')||'';e.setAttribute('transform',(s.dx||s.dy?'translate('+s.dx.toFixed(1)+','+s.dy.toFixed(1)+') ':'')+e.__o);e.style.display=s.hidden?'none':'';});
      // While the light is dragged its glows follow at once; the full relight comes when it settles
      if(PIECES[id].light)(skin.glows||[]).forEach(function(q){var g=svg.querySelector(q);if(!g)return;if(g.__cx==null){g.__cx=+g.getAttribute('cx');g.__cy=+g.getAttribute('cy');}
        g.setAttribute('cx',g.__cx+(s.hidden?0:s.dx)-skin.opts.ldx);g.setAttribute('cy',g.__cy+(s.hidden?0:s.dy)-skin.opts.ldy);});}
    function pieceAt(t){if(!t||!t.closest||t.closest('[data-item]'))return null;
      for(var id in PIECES){if(PIECES[id].kind==='tool'&&t.closest(PIECES[id].sel))return id;}
      var p=t.closest('[data-prop]');return p&&PIECES[p.getAttribute('data-prop')]?p.getAttribute('data-prop'):null;}

    /* The selection outline and name tag live in their own layer on top. */
    function tag(b,name,solid){var w=Math.max(60,name.length*8.2+22),x=Math.min(Math.max(b.x-6,4),1196-w),y=b.y-8>34?b.y-34:b.y+b.h+8;
      return '<rect x="'+(b.x-6)+'" y="'+(b.y-6)+'" width="'+(b.w+12)+'" height="'+(b.h+12)+'" rx="8" fill="'+(solid?'rgb(217 119 6 / .1)':'none')+'" stroke="#f59e0b" stroke-width="'+(solid?2.5:2)+'"'+(solid?'':' stroke-dasharray="7 5"')+'/>'+
        '<rect x="'+x+'" y="'+y+'" width="'+w+'" height="26" rx="6" fill="'+(solid?'#d97706':'rgb(17 24 39 / .82)')+'"/><text x="'+(x+11)+'" y="'+(y+17.5)+'" font-family="Inter,system-ui,sans-serif" font-size="14" font-weight="600" fill="#fff">'+esc(name)+'</text>';}
    function drawOv(){raf=0;if(dead)return;var h='';
      if(mode==='arrange'){if(hovId&&hovId!==selId){var b=box(hovId);if(b)h+=tag(b,st[hovId].name,false);}
        if(selId){var c=box(selId);if(c)h+=tag(c,st[selId].name+(movable(selId)?'':' · locked'),true);}}
      if(ov.__h!==h){ov.innerHTML=h;ov.__h=h;}
      if(mode==='arrange')raf=requestAnimationFrame(drawOv);}

    function select(id){if(id!==selId)openAct=-1;selId=id;renderList();renderInsp();}
    /* The piece card stands beside its piece, on whichever side has room, so
       the piece stays in view. On a phone it sits under the scene. */
    function placeCard(){var c=$('.crb-pcard');if(!c)return;c.hidden=!selId||mode!=='arrange';if(c.hidden)return;
      if(window.matchMedia('(max-width:720px)').matches){c.style.left=c.style.top='';return;}
      var b=box(selId),wr=$('.crb-wrap').getBoundingClientRect(),m=svg.getScreenCTM();if(!b||!m)return;
      var L=m.a*b.x+m.e-wr.left,Tp=m.d*b.y+m.f-wr.top,R=m.a*(b.x+b.w)+m.e-wr.left,cw=c.offsetWidth,ch=c.offsetHeight,g=16,x;
      if(wr.width-R-g>=cw+10)x=R+g;else if(L-g>=cw+10)x=L-g-cw;else x=Math.max(10,Math.min(wr.width-cw-10,(L+R)/2-cw/2));
      c.style.left=Math.round(x)+'px';c.style.top=Math.round(Math.max(10,Math.min(wr.height-ch-10,Tp)))+'px';}
    function setHover(id){if(hovId===id)return;hovId=id;root.querySelectorAll('.crb-prow').forEach(function(r){r.classList.toggle('is-hov',r.getAttribute('data-id')===id);});}
    function nudge(id,dx,dy){var s=st[id];s.dx=Math.max(-900,Math.min(900,s.dx+dx));s.dy=Math.max(-500,Math.min(500,s.dy+dy));place(id);}
    function moved(id){if(PIECES[id].light)moveLight(id);renderPos();renderList();placeCard();changed();}
    function changed(){if(o.onChange)o.onChange(state());}

    /* Arrange mode catches pointer events before the scene does, so the
       scene's own play (dragging ingredients, hover cards) is paused. */
    function capture(e){
      if(mode!=='arrange')return;
      var ty=e.type;
      if(ty==='keydown'){var k=e.key,n=e.shiftKey?16:4;
        if(selId&&movable(selId)&&/^Arrow/.test(k)){e.preventDefault();e.stopPropagation();nudge(selId,k==='ArrowLeft'?-n:k==='ArrowRight'?n:0,k==='ArrowUp'?-n:k==='ArrowDown'?n:0);moved(selId);return;}
        if(k==='Escape'&&selId){e.stopPropagation();select(null);return;}
        if(k===' '||k==='Spacebar')e.stopPropagation();return;}
      e.stopPropagation();
      if(ty==='contextmenu'){e.preventDefault();var cid=pieceAt(e.target);if(cid)select(cid);return;}
      if(ty==='pointerdown'){e.preventDefault();stage.focus({preventScroll:true});var id=pieceAt(e.target);select(id);
        if(movable(id)){var p=toSvg(e.clientX,e.clientY);drag={id:id,x:p.x,y:p.y,dx:st[id].dx,dy:st[id].dy,moved:false};try{stage.setPointerCapture(e.pointerId);}catch(_){}}return;}
      if(ty==='pointermove'){
        if(drag){var q=toSvg(e.clientX,e.clientY),s=st[drag.id];s.dx=drag.dx+q.x-drag.x;s.dy=drag.dy+q.y-drag.y;drag.moved=true;place(drag.id);stage.style.cursor='grabbing';$('.crb-wrap').classList.add('is-dragging');return;}
        var h=pieceAt(e.target);setHover(h);stage.style.cursor=h?(movable(h)?'grab':'pointer'):'default';return;}
      if(ty==='pointerup'||ty==='pointercancel'){if(drag){$('.crb-wrap').classList.remove('is-dragging');if(drag.moved)moved(drag.id);drag=null;stage.style.cursor='grab';}return;}
      if(ty==='pointerleave')setHover(null);
    }
    function setMode(m){mode=m;scene.classList.toggle('is-arrange',m==='arrange');root.querySelector('.crb').classList.toggle('is-playing',m!=='arrange');
      root.querySelectorAll('[data-mode]').forEach(function(b){b.setAttribute('aria-pressed',String(b.getAttribute('data-mode')===m));});
      var who=ctrl.state().r.who;
      $('.crb-hint').textContent=m==='arrange'?'Click a piece to edit it, drag to move it. The room itself is in the tray below.':(who?'Playing as '+who+'. ':'')+'Hover a piece for its card; right-click it for its actions.';
      $('.crb-try').hidden=m==='arrange';placeCard();
      var card=scene.querySelector('.cr-card'),menu=scene.querySelector('.cr-menu');if(card)card.classList.remove('show','pinned');if(menu)menu.hidden=true;
      stage.style.cursor='';setHover(null);if(m==='arrange'&&!raf)raf=requestAnimationFrame(drawOv);else if(m!=='arrange')drawOv();}

    function row(id){var s=st[id],P=PIECES[id],note=s.hidden?'off the bench':P.lock?'':(s.dx||s.dy)?'moved':'';
      return '<li><button type="button" class="crb-prow" data-id="'+esc(id)+'" aria-current="'+(selId===id)+'"><span>'+esc(s.name)+'</span>'+(P.lock?'<span class="crb-lk" title="Stays put">'+I_LOCK+'</span>':'')+(note?'<small>'+note+'</small>':'')+'</button></li>';}
    function renderList(){var a=$('.crb-ptools'),b=$('.crb-ppieces');if(a)a.innerHTML=tools().map(row).join('');if(b)b.innerHTML=decor().map(row).join('');}
    /* The tray: one tab of pictures at a time, the scene above it untouched. */
    function th(key,v,label,on,attr){var src=skin.thumb?skin.thumb(key,v):'';return '<button type="button" class="crb-th" '+(attr||'data-room="'+esc(key)+'" data-v="'+esc(v)+'"')+' aria-pressed="'+!!on+'">'+(src?'<img src="'+esc(src)+'" alt="" loading="lazy">':'<i class="crb-thnone"></i>')+'<span>'+esc(label)+'</span></button>';}
    function grp(key,title){var cur=skin.opts[key];return '<div class="crb-tgrp"><p class="crb-grp">'+esc(title)+'</p><div class="crb-tths">'+(CH[key]||[]).map(function(c){return th(key,c[0],c[1],cur===c[0]);}).join('')+'</div></div>';}
    function segs(key){var cur=skin.opts[key],label={time:'Time',weather:'Weather'}[key]||key;return '<div class="crb-ctl"><span>'+esc(label)+'</span><div class="crb-seg" role="group" aria-label="'+esc(label)+'">'+(CH[key]||[]).map(function(c){return '<button type="button" data-room="'+esc(key)+'" data-v="'+esc(c[0])+'" aria-pressed="'+(cur===c[0])+'">'+esc(c[1])+'</button>';}).join('')+'</div></div>';}
    function isLook(Lk){var O=skin.opts;return Object.keys(Lk.o).every(function(k){var a=Lk.o[k],b=O[k];return Array.isArray(a)?a.slice().sort().join()===b.slice().sort().join():a===b;});}
    function pane(t){var h='',O=skin.opts,Lks=skin.looks||{};if(!t)return '';
      if(t.looks)h+='<div class="crb-tgrp"><div class="crb-tths">'+Object.keys(Lks).map(function(k){return th('look',k,Lks[k].name,isLook(Lks[k]),'data-look="'+esc(k)+'"');}).join('')+'</div></div>';
      (t.groups||[]).forEach(function(g){h+=grp(g[0],g[1]);});
      if(t.segs)h+='<div class="crb-tgrp crb-tside">'+t.segs.map(segs).join('')+'</div>';
      if(t.kit)h+='<div class="crb-tgrp"><div class="crb-tths">'+(skin.kit||[]).map(function(q){var on=O.kit.indexOf(q[0])>=0;return th('kit',q[0],on?q[1]+' · on the bench':q[1],on,'data-kit="'+esc(q[0])+'"');}).join('')+'</div></div>';
      if(t.pieces)h+='<div class="crb-tgrp"><p class="crb-grp">Tools <small>steps find these</small></p><ul class="crb-ptools"></ul></div><div class="crb-tgrp"><p class="crb-grp">Pieces</p><ul class="crb-ppieces"></ul></div>';
      return h+(t.note?'<p class="crb-tnote">'+esc(t.note)+'</p>':'');}
    function renderTray(){var b=$('.crb-tbody');if(!b)return;var sc=b.scrollLeft,t=TABS.filter(function(x){return x.id===tab;})[0];b.innerHTML=pane(t);b.scrollLeft=sc;renderList();
      root.querySelectorAll('[data-tab]').forEach(function(x){x.setAttribute('aria-selected',String(x.getAttribute('data-tab')===tab));});}
    function posText(id){var s=st[id];if(!movable(id)||(!s.dx&&!s.dy))return '';
      var h=Math.round(Math.abs(s.dx)),v=Math.round(Math.abs(s.dy));
      return 'Moved '+[h?h+(s.dx>0?' right':' left'):'',v?v+(s.dy>0?' down':' up'):''].filter(Boolean).join(' and ')+'.';}
    function renderPos(){var p=$('.crb-postxt');if(p&&selId)p.textContent=posText(selId);var b=$('[data-act="reset"]');if(b&&selId)b.disabled=!(st[selId].dx||st[selId].dy);}
    function renderInsp(){
      var el=$('.crb-pcard'),h='';
      if(!selId){el.innerHTML='';placeCard();return;}
      var s=st[selId],P=PIECES[selId];
      // The name is the card's title, edited in place
      h+='<div class="crb-ihead"><input class="crb-in crb-title" data-f="name" value="'+esc(s.name)+'" aria-label="Name" maxlength="60"><span class="crb-pill">'+(P.kind==='tool'?'Tool':'Piece')+'</span><button type="button" class="crb-ib" data-act="close" aria-label="Close">'+I_X+'</button></div>';
      if(P.lock)h+='<p class="crb-lock">'+I_LOCK+'<span>'+esc(P.lock)+'</span></p>';
      if(P.uses)h+='<p class="crb-fine">Steps look for this by kind. '+esc(P.uses)+'</p>';
      h+='<label class="crb-fl"><span>Hover line</span><textarea class="crb-txt" rows="2" data-f="text" maxlength="200">'+esc(s.text)+'</textarea></label>'+
        '<details class="crb-more"><summary>Examine text'+(s.more?'':' <small>same as the hover line</small>')+'</summary><textarea class="crb-txt" rows="3" data-f="more" maxlength="600" placeholder="Leave empty to repeat the hover line">'+esc(s.more)+'</textarea></details>';
      h+='<div class="crb-sec"><h4>Right-click actions</h4>'+(s.acts.length?'':'<p class="crb-fine">None yet. Players still get Examine.</p>');
      // One action open for editing at a time; the rest are one line each
      s.acts.forEach(function(A,i){var bi=A.fx.indexOf('b:')===0?A.fx.slice(2):null;
        if(i!==openAct){var fl=bi?(BUILTIN[bi]||bi):(EFFECTS.filter(function(x){return x[0]===A.fx;})[0]||['',''])[1],sl=(SOUNDS.filter(function(x){return x[0]===A.sound&&x[0];})[0]||[])[1];
          h+='<button type="button" class="crb-arow" data-act="edit-act" data-i="'+i+'"><b>'+esc(A.label||'Untitled action')+'</b><small>'+esc([fl,sl,A.say?'“'+A.say+'”':''].filter(Boolean).join(' · '))+'</small></button>';return;}
        h+='<div class="crb-actc" data-i="'+i+'"><div class="crb-acth"><input class="crb-in" data-a="label" value="'+esc(A.label)+'" aria-label="Action name" placeholder="What the menu says" maxlength="40"><button type="button" class="crb-ib" data-act="rm-act" aria-label="Remove action">'+I_X+'</button></div>'+
          '<div class="crb-grid2"><label class="crb-fl"><span>Effect</span><select class="crb-in crb-sm" data-a="fx">'+(bi?'<optgroup label="Built in">'+opt('b:'+bi,BUILTIN[bi]||bi,true)+'</optgroup><optgroup label="Any piece">':'')+EFFECTS.map(function(x){return opt(x[0],x[1],A.fx===x[0]);}).join('')+(bi?'</optgroup>':'')+'</select></label>'+
          '<label class="crb-fl"><span>Sound</span><select class="crb-in crb-sm" data-a="sound">'+SOUNDS.map(function(x){return opt(x[0],x[1],A.sound===x[0]);}).join('')+'</select></label></div>'+
          '<label class="crb-fl"><span>Line it shows</span><input class="crb-in" data-a="say" value="'+esc(A.say)+'" placeholder="Optional, a few words" maxlength="80"></label></div>';});
      h+='<div class="crb-rowb"><button type="button" class="crb-btn crb-sm" data-act="add-act">Add an action</button></div><p class="crb-fine">'+(P.fixed?'Always there too: '+esc(P.fixed.join(', '))+' and Examine.':'Examine is always there too.')+'</p></div>';
      if(movable(selId))h+='<div class="crb-rowb crb-pos"><button type="button" class="crb-btn crb-sm" data-act="reset"'+(s.dx||s.dy?'':' disabled')+'>Put it back</button><button type="button" class="crb-btn crb-sm" data-act="hide">'+(P.kit?'Back in the kit':s.hidden?'Put it on the bench':'Take it off')+'</button></div><p class="crb-fine crb-postxt">'+esc(posText(selId))+'</p>';
      el.innerHTML=h;placeCard();
    }
    function onInput(e){var t=e.target,f=t.getAttribute('data-f'),a=t.getAttribute('data-a');if(!selId)return;var s=st[selId];
      if(f){s[f]=t.value;s.auto=false;sync(selId);if(f==='name')renderList();changed();}
      else if(a==='label'||a==='say'){var i=+t.closest('.crb-actc').getAttribute('data-i');s.acts[i][a]=t.value;s.auto=false;sync(selId);changed();}}
    function onChange(e){var t=e.target,a=t.getAttribute('data-a');if(!selId||(a!=='fx'&&a!=='sound'))return;var i=+t.closest('.crb-actc').getAttribute('data-i');st[selId].acts[i][a]=t.value;st[selId].auto=false;sync(selId);changed();}
    function onTray(e){var b=e.target.closest('button');if(!b)return;
      if(b.hasAttribute('data-tab')){tab=b.getAttribute('data-tab');renderTray();return;}
      var pr=b.closest('.crb-prow');if(pr){if(mode!=='arrange')setMode('arrange');select(pr.getAttribute('data-id'));return;}
      if(b.hasAttribute('data-room')){var k=b.getAttribute('data-room'),v=b.getAttribute('data-v');skin.setOpt(k,v);scene.setAttribute('data-'+k,skin.opts[k]);
        b.parentNode.querySelectorAll('button').forEach(function(x){x.setAttribute('aria-pressed',String(x===b));});changed();return;}
      if(b.hasAttribute('data-look')){skin.look(b.getAttribute('data-look'));['time','weather','light'].forEach(function(k){scene.setAttribute('data-'+k,skin.opts[k]);});changed();return;}
      if(b.hasAttribute('data-kit')){var q=b.getAttribute('data-kit'),id='kit-'+q;if(mode!=='arrange')setMode('arrange');
        if(skin.opts.kit.indexOf(q)<0){skin.setOpt('kit',skin.opts.kit.concat([q]));pendSel=PIECES[id]?id:null;changed();}else select(id);}}
    function onCard(e){var b=e.target.closest('button');if(!b)return;
      var act=b.getAttribute('data-act'),s=selId&&st[selId],el=$('.crb-pcard');
      if(act==='close'){select(null);return;}
      if(!s)return;
      if(act==='edit-act'){openAct=+b.getAttribute('data-i');renderInsp();var f=el.querySelector('.crb-actc [data-a="label"]');if(f)f.focus();return;}
      if(act==='add-act'){s.auto=false;s.acts.push({label:'',fx:'sparkle',sound:'',say:''});openAct=s.acts.length-1;sync(selId);renderInsp();el.scrollTop=el.scrollHeight;var ins=el.querySelectorAll('[data-a="label"]');ins[ins.length-1].focus();changed();}
      else if(act==='rm-act'){s.auto=false;s.acts.splice(+b.closest('.crb-actc').getAttribute('data-i'),1);openAct=-1;sync(selId);renderInsp();changed();}
      else if(act==='reset'){s.dx=0;s.dy=0;place(selId);renderInsp();moved(selId);}
      else if(act==='hide'){var P=PIECES[selId];if(P.kit){skin.setOpt('kit',skin.opts.kit.filter(function(x){return x!==P.kit;}));select(null);changed();return;}
        s.hidden=!s.hidden;place(selId);renderInsp();moved(selId);}}
    /* The bench as data, for whatever stores it: the room's look, and each
       piece's moves, words and actions. */
    function state(){var pieces={};Object.keys(st).forEach(function(id){var s=st[id];pieces[id]={name:s.name,text:s.text,more:s.more,dx:Math.round(s.dx),dy:Math.round(s.dy),hidden:s.hidden,
      acts:s.acts.map(function(a){return {label:a.label,fx:a.fx,sound:a.sound,say:a.say};})};});
      return {look:JSON.parse(JSON.stringify(skin.opts)),pieces:pieces};}

    var recipes=o.recipes||{},rkeys=Object.keys(recipes);
    root.innerHTML='<div class="crb"><div class="crb-bar"><div class="crb-seg" role="group" aria-label="Mode"><button type="button" data-mode="arrange" aria-pressed="true">Arrange</button><button type="button" data-mode="try" aria-pressed="false">Try it as a player</button></div>'+
      '<span class="crb-hint"></span><span class="crb-try" hidden>'+(rkeys.length>1?'<select class="crb-in crb-sm crb-recipe" aria-label="Recipe">'+rkeys.map(function(k){return opt(k,recipes[k].name||k,k===o.recipe);}).join('')+'</select>':'')+
      '<button type="button" class="crb-btn crb-sm crb-skip">Skip and roll</button></span>'+
      '<button type="button" class="crb-btn crb-sm crb-sound" aria-pressed="false">Sound off</button><span class="crb-status" role="status">'+esc(o.status||'Preview only. Nothing is saved.')+'</span></div>'+
      '<div class="crb-wrap"><div class="crb-scene"></div><div class="crb-pcard" role="dialog" aria-label="Piece" hidden></div></div>'+
      '<div class="crb-tray"><div class="crb-ttabs" role="tablist" aria-label="Room">'+TABS.map(function(t){return '<button type="button" role="tab" data-tab="'+esc(t.id)+'" aria-selected="'+(t.id===tab)+'">'+esc(t.label)+'</button>';}).join('')+'</div>'+
      '<div class="crb-tbody" role="tabpanel"></div></div></div>';
    scene=$('.crb-scene');
    ctrl=CraftScene.mount(scene,skin,{recipes:recipes,items:o.items,recipe:o.recipe,settings:o.settings});
    ['time','weather','light'].forEach(function(k){scene.setAttribute('data-'+k,skin.opts[k]);});
    svg=ctrl.svg;stage=ctrl.stage;
    ov=document.createElementNS('http://www.w3.org/2000/svg','g');ov.setAttribute('pointer-events','none');svg.appendChild(ov);
    initState();
    var CAP=['pointerdown','pointermove','pointerup','pointercancel','pointerleave','pointerover','pointerout','contextmenu','click','dblclick','keydown','keyup'];
    CAP.forEach(function(t){scene.addEventListener(t,capture,true);});
    root.querySelectorAll('[data-mode]').forEach(function(b){b.addEventListener('click',function(){setMode(b.getAttribute('data-mode'));});});
    var snd=$('.crb-sound');snd.addEventListener('click',function(){var v=snd.getAttribute('aria-pressed')!=='true';snd.setAttribute('aria-pressed',String(v));snd.textContent=v?'Sound on':'Sound off';ctrl.sound(v);});
    $('.crb-skip').addEventListener('click',function(){ctrl.skip();});
    var rsel=$('.crb-recipe');if(rsel)rsel.addEventListener('change',function(){ctrl.reset(rsel.value);setMode('try');});
    var card=$('.crb-pcard');card.addEventListener('input',onInput);card.addEventListener('change',onChange);card.addEventListener('click',onCard);
    card.addEventListener('keydown',function(e){if(e.key==='Escape'){select(null);stage.focus({preventScroll:true});}});
    var tray=$('.crb-tray');tray.addEventListener('click',onTray);tray.addEventListener('mouseover',function(e){var b=e.target.closest('.crb-prow');setHover(b?b.getAttribute('data-id'):null);});
    window.addEventListener('resize',placeCard);
    svg.addEventListener('craft:redrawn',redrawn);
    renderTray();renderInsp();setMode('arrange');

    function destroy(){if(dead)return;dead=true;if(raf)cancelAnimationFrame(raf);raf=0;clearTimeout(lightT);window.removeEventListener('resize',placeCard);ctrl.destroy();root.innerHTML='';}
    function status(t){var el=$('.crb-status');if(el)el.textContent=t;}
    return {state:state,select:select,setMode:setMode,box:box,scene:ctrl,status:status,destroy:destroy};
  }
  return {start:start};
})();
