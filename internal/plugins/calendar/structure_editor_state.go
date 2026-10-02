// Package calendar - structure_editor_state.go builds the Alpine state of
// the structure editor that both the new-calendar wizard's "Build your own"
// step and an existing calendar's "Edit structure" page render: the
// wizard starts it from defaults, the edit page from the calendar itself.
package calendar

import (
	"encoding/json"
	"fmt"
)

// editorMonth is one month row. Key is stable for the row's life in the
// browser (reordering keeps it), so a season can point at a month by key
// and still land on the right one after months move.
type editorMonth struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Days  int    `json:"days"`
	Leap  int    `json:"leap"`
	Inter bool   `json:"inter"`
}

type editorWeekday struct {
	Name string `json:"name"`
	Rest bool   `json:"rest"`
}

// editorMoon carries an existing moon's ID so a save updates that moon in
// place (MoonInput's upsert) instead of replacing it.
type editorMoon struct {
	ID    *int    `json:"id"`
	Name  string  `json:"name"`
	Cycle float64 `json:"cycle"`
}

// editorSeason carries an existing season's ID for the same reason, and its
// start and end months by month key.
type editorSeason struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	StartKey string `json:"startKey"`
	StartDay int    `json:"startDay"`
	EndKey   string `json:"endKey"`
	EndDay   int    `json:"endDay"`
}

// editorState is the structure editor's starting state.
type editorState struct {
	Months    []editorMonth
	Weekdays  []editorWeekday
	Moons     []editorMoon
	Seasons   []editorSeason
	LeapOn    bool
	LeapEvery int
	EraName   string
	EraYear   int
}

// newCalendarEditorState is the wizard's starting point: twelve months,
// seven weekdays, one moon and one era for the owner to reshape.
func newCalendarEditorState() editorState {
	st := editorState{LeapEvery: 4, EraName: "Age 1", EraYear: 1}
	for i := 0; i < 12; i++ {
		st.Months = append(st.Months, editorMonth{Key: fmt.Sprintf("m%d", i), Name: fmt.Sprintf("Month %d", i+1), Days: 30})
	}
	for i := 0; i < 7; i++ {
		st.Weekdays = append(st.Weekdays, editorWeekday{Name: fmt.Sprintf("Day %d", i+1)})
	}
	st.Moons = []editorMoon{{Name: "Moon", Cycle: 29.5}}
	return st
}

// calendarEditorState prefills the editor from an existing calendar, which
// must have Months, Weekdays, Moons and Seasons loaded.
func calendarEditorState(cal *Calendar) editorState {
	st := editorState{LeapOn: cal.LeapYearEvery > 0, LeapEvery: cal.LeapYearEvery}
	if st.LeapEvery <= 0 {
		st.LeapEvery = 4
	}
	keys := make([]string, len(cal.Months))
	for i, m := range cal.Months {
		keys[i] = fmt.Sprintf("m%d", i)
		st.Months = append(st.Months, editorMonth{Key: keys[i], Name: m.Name, Days: m.Days, Leap: m.LeapYearDays, Inter: m.IsIntercalary})
	}
	keyFor := func(month int) string {
		if month >= 1 && month <= len(keys) {
			return keys[month-1]
		}
		return ""
	}
	for _, w := range cal.Weekdays {
		st.Weekdays = append(st.Weekdays, editorWeekday{Name: w.Name, Rest: w.IsRestDay})
	}
	for _, m := range cal.Moons {
		id := m.ID
		st.Moons = append(st.Moons, editorMoon{ID: &id, Name: m.Name, Cycle: m.CycleDays})
	}
	for _, s := range cal.Seasons {
		st.Seasons = append(st.Seasons, editorSeason{
			ID: s.ID, Name: s.Name,
			StartKey: keyFor(s.StartMonth), StartDay: s.StartDay,
			EndKey: keyFor(s.EndMonth), EndDay: s.EndDay,
		})
	}
	return st
}

// structureEditorXData is the editor's entire Alpine x-data object
// literal: the starting state plus the add/remove/reorder/serialize
// methods its buttons call. The state is JSON-marshaled (so names are
// quoted and escaped) and spliced into a static JS template; JSON is a
// valid subset of JS literal syntax. buildJSON() produces the same
// ImportResult shape an uploaded file does, and the server re-validates it
// (parseWizardImportJSON) exactly as it would an upload. Numbers are parsed
// defensively so a blank or non-numeric field still serializes to
// something the server can clamp, never NaN.
//
// previewSlot, when not empty, is the id of an element holding a preview of
// this structure; it is emptied whenever the structure changes, so a
// preview on screen always describes what is in the editor.
func structureEditorXData(st editorState, previewSlot string) string {
	if st.Months == nil {
		st.Months = []editorMonth{}
	}
	if st.Weekdays == nil {
		st.Weekdays = []editorWeekday{}
	}
	if st.Moons == nil {
		st.Moons = []editorMoon{}
	}
	if st.Seasons == nil {
		st.Seasons = []editorSeason{}
	}
	monthsJSON, _ := json.Marshal(st.Months)
	weekdaysJSON, _ := json.Marshal(st.Weekdays)
	moonsJSON, _ := json.Marshal(st.Moons)
	seasonsJSON, _ := json.Marshal(st.Seasons)
	eraJSON, _ := json.Marshal(st.EraName)
	slotJSON, _ := json.Marshal(previewSlot)

	return fmt.Sprintf(`{
months: %s,
weekdays: %s,
moons: %s,
seasons: %s,
leapOn: %t,
leapEvery: %d,
eraName: %s,
eraYear: %d,
nextKey: %d,
previewSlot: %s,
addMonth(){ this.months.push({key:'n'+(this.nextKey++), name:'Month '+(this.months.length+1), days:30, leap:0, inter:false}); },
removeMonth(i){ if(this.months.length>1){ this.months.splice(i,1); } },
moveMonth(i,dir){ var j=i+dir; if(j<0||j>=this.months.length) return; var t=this.months.splice(i,1)[0]; this.months.splice(j,0,t); },
addWeekday(){ this.weekdays.push({name:'Day '+(this.weekdays.length+1), rest:false}); },
removeWeekday(i){ if(this.weekdays.length>1){ this.weekdays.splice(i,1); } },
moveWeekday(i,dir){ var j=i+dir; if(j<0||j>=this.weekdays.length) return; var t=this.weekdays.splice(i,1)[0]; this.weekdays.splice(j,0,t); },
addMoon(){ this.moons.push({id:null, name:'Moon '+(this.moons.length+1), cycle:29.5}); },
removeMoon(i){ this.moons.splice(i,1); },
addSeason(){ var k=this.months.length?this.months[0].key:''; this.seasons.push({id:0, name:'Season '+(this.seasons.length+1), startKey:k, startDay:1, endKey:k, endDay:1}); },
removeSeason(i){ this.seasons.splice(i,1); },
monthPos(key){ for(var i=0;i<this.months.length;i++){ if(this.months[i].key===key) return i+1; } return 1; },
clearPreview(){ if(!this.previewSlot) return; var el=document.getElementById(this.previewSlot); if(el) el.innerHTML=''; },
buildJSON(){
  var self=this;
  var months=this.months.map(function(mo,i){ return {name:(mo.name||('Month '+(i+1))), days:Math.max(1,parseInt(mo.days,10)||1), sort_order:i, is_intercalary:!!mo.inter, leap_year_days:Math.max(0,parseInt(mo.leap,10)||0)}; });
  var weekdays=this.weekdays.map(function(w,i){ return {name:(w.name||('Day '+(i+1))), sort_order:i, is_rest_day:!!w.rest}; });
  var moons=this.moons.map(function(mo){ var o={name:(mo.name||'Moon'), cycle_days:(parseFloat(mo.cycle)||29.5), phase_offset:0, color:'#c0c0c0'}; if(mo.id){ o.id=mo.id; } return o; });
  var seasons=this.seasons.map(function(se,i){ var o={name:(se.name||('Season '+(i+1))), start_month:self.monthPos(se.startKey), start_day:Math.max(1,parseInt(se.startDay,10)||1), end_month:self.monthPos(se.endKey), end_day:Math.max(1,parseInt(se.endDay,10)||1), color:'#808080'}; if(se.id){ o.id=se.id; } return o; });
  var eras=this.eraName?[{name:this.eraName, start_year:(parseInt(this.eraYear,10)||1), start_month:1, start_day:1, color:'#6366f1'}]:[];
  var year=parseInt(this.eraYear,10)||1;
  return JSON.stringify({format:'built', calendar_name:'My calendar', months:months, weekdays:weekdays, moons:moons, seasons:seasons, eras:eras,
    settings:{mode:'fantasy', current_year:year, hours_per_day:24, minutes_per_hour:60, seconds_per_minute:60, leap_year_every:(this.leapOn?(parseInt(this.leapEvery,10)||0):0), leap_year_offset:0},
    today:{year:year}});
}
}`, monthsJSON, weekdaysJSON, moonsJSON, seasonsJSON, st.LeapOn, st.LeapEvery, eraJSON, st.EraYear, len(st.Months), slotJSON)
}
