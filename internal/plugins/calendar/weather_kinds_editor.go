// Package calendar - weather_kinds_editor.go builds the Alpine state of the
// "Your own weather" editor in Calendar settings. The state lives in a nested
// x-data inside the Weather section and is written Go-side, not as a script
// helper, because the page can arrive by an htmx swap where a sibling script
// element is not reliably run.
package calendar

import (
	"encoding/json"
	"strconv"
	"strings"
)

// weatherKindsEditorJS is the editor's methods, spliced after the initial
// state by weatherKindsXData. Each change calls touch(), which re-serializes
// the list into the one hidden weather_kinds input. The markup pairs every
// change with the parent form's clearPreview(), like the climate select, so
// the methods never reach into the parent scope.
//
// A new kind's id is the slug of its name, tracked live while the kind is
// still pending and frozen when the owner presses Done, so a later rename
// never changes the id the generator and stored days refer to.
const weatherKindsEditorJS = `
init(){ var s=this; this.kinds=this.kinds.map(function(k){ return s.wrap(k,false); }); this.out=this.dump(); },
wrap(k,pending){
  var w=(k.words||[]).slice(0,8); while(w.length<8) w.push('');
  var se=k.seasons||{};
  return {key:++this.n,pending:pending,missing:false,id:k.id||'',name:k.name||'',icon:k.icon||'cloud',color:k.color||'#7a5cff',like:k.like||'clear',
    s:{winter:se.winter||'normal',spring:se.spring||'normal',summer:se.summer||'normal',autumn:se.autumn||'normal'},
    lasts:k.lasts||'normal',words:w,magic:!!k.magic,effect:(k.look&&k.look.effect)||''};
},
slug(n){ return String(n||'').toLowerCase().replace(/[^a-z0-9]+/g,'-').replace(/^-+|-+$/g,'').slice(0,36).replace(/-+$/,''); },
freeId(k){
  var base=this.slug(k.name); if(!base) return '';
  var used={}; this.kinds.forEach(function(o){ if(o!==k&&o.id) used[o.id]=1; });
  var id=base, i=2; while(used[id]){ id=base+'-'+i; i++; } return id;
},
dump(){
  var s=this;
  return JSON.stringify(this.kinds.map(function(k){
    if(k.pending) k.id=s.freeId(k);
    var o={id:k.id,name:k.name.trim(),icon:k.icon,color:k.color,like:k.like,seasons:{winter:k.s.winter,spring:k.s.spring,summer:k.s.summer,autumn:k.s.autumn}};
    if(k.lasts!=='normal') o.lasts=k.lasts;
    var w=k.words.map(function(x){ return x.trim(); }).filter(function(x){ return x; });
    if(w.length) o.words=w;
    if(k.magic) o.magic=true;
    if(k.effect) o.look={effect:k.effect};
    return o;
  }));
},
touch(){ this.out=this.dump(); },
add(){
  if(this.kinds.length>=this.max) return;
  var k=this.wrap({},true); this.kinds.push(k); this.editing=k.key; this.touch();
  this.$nextTick(function(){ var el=document.getElementById('calv5-wk-name-'+k.key); if(el) el.focus(); });
},
edit(k){
  this.editing=k.key;
  this.$nextTick(function(){ var el=document.getElementById('calv5-wk-name-'+k.key); if(el) el.focus(); });
},
done(k){
  if(!k.name.trim()){ k.missing=true; var el=document.getElementById('calv5-wk-name-'+k.key); if(el) el.focus(); return; }
  k.missing=false; k.pending=false; this.touch(); this.editing=null;
  this.$nextTick(function(){ var el=document.getElementById('calv5-wk-edit-'+k.key); if(el) el.focus(); });
},
remove(k){
  this.kinds=this.kinds.filter(function(o){ return o!==k; }); this.editing=null; this.touch();
  this.$nextTick(function(){ var el=document.getElementById('calv5-wk-add'); if(el) el.focus(); });
},
pick(k,icon){ k.icon=icon; this.touch(); },
summary(k){
  var s=this, parts=[], names={winter:'winter',spring:'spring',summer:'summer',autumn:'autumn'};
  [['often','often in '],['rare','rare in '],['off','never in ']].forEach(function(l){
    var m=Object.keys(names).filter(function(n){ return k.s[n]===l[0]; });
    if(m.length) parts.push(l[1]+m.join(' and '));
  });
  return ['like '+(s.likes[k.like]||k.like)].concat(parts).join(' · ');
},
dot(k){ return /^#[0-9a-fA-F]{6}$/.test(k.color)?k.color:'#888888'; }
`

// weatherKindIcon is one of the six glyphs a kind can use, with the Font
// Awesome class the calendar already draws it with (WEATHER_ICONS in
// calendar_view.js).
type weatherKindIcon struct {
	ID    string
	Label string
	Class string
}

var weatherKindIconOptions = []weatherKindIcon{
	{ID: "clear", Label: "Clear", Class: "fa-sun"},
	{ID: "cloud", Label: "Cloud", Class: "fa-cloud"},
	{ID: "rain", Label: "Rain", Class: "fa-cloud-rain"},
	{ID: "snow", Label: "Snow", Class: "fa-snowflake"},
	{ID: "storm", Label: "Storm", Class: "fa-cloud-bolt"},
	{ID: "fog", Label: "Fog", Class: "fa-smog"},
}

// weatherKindsXData returns the x-data expression for the editor: the stored
// kinds, the built-in weather labels (for the "like Rain" summaries) and the
// methods above.
func weatherKindsXData(kinds []WeatherKind) string {
	if kinds == nil {
		kinds = []WeatherKind{}
	}
	likes := make(map[string]string, len(weatherPresets))
	for _, p := range weatherPresets {
		likes[p.ID] = p.Label
	}
	icons := make(map[string]string, len(weatherKindIconOptions))
	for _, o := range weatherKindIconOptions {
		icons[o.ID] = o.Class
	}
	ij, _ := json.Marshal(icons)
	kj, _ := json.Marshal(kinds)
	lj, _ := json.Marshal(likes)
	var b strings.Builder
	b.WriteString("{kinds:")
	b.Write(kj)
	b.WriteString(",likes:")
	b.Write(lj)
	b.WriteString(",icons:")
	b.Write(ij)
	b.WriteString(",out:'',editing:null,n:0,max:")
	b.WriteString(strconv.Itoa(maxWeatherKinds))
	b.WriteString(",")
	b.WriteString(weatherKindsEditorJS)
	b.WriteString("}")
	return b.String()
}

// The labelled choices the editor renders, in display order.
var (
	weatherKindSeasons = []struct{ ID, Label string }{
		{"winter", "Winter"}, {"spring", "Spring"}, {"summer", "Summer"}, {"autumn", "Autumn"},
	}
	weatherKindLevelOptions = []struct{ ID, Label string }{
		{"off", "Off"}, {"rare", "Rare"}, {"normal", "Normal"}, {"often", "Often"},
	}
	weatherKindLastsOptions = []struct{ ID, Label string }{
		{"fleeting", "Fleeting"}, {"normal", "Normal"}, {"stable", "Stable"},
	}
)
