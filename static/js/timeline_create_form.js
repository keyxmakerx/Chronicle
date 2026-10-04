/**
 * timeline_create_form.js -- Alpine component behind the Timelines page's
 * "New Timeline" form: loads the campaign's calendars into its picker.
 *
 * Loaded from the layout, before Alpine, rather than inline in the page: htmx
 * strips <script> tags from a boosted swap (allowScriptTags=false), so an
 * inline definition would be missing whenever the page is reached from the
 * sidebar.
 */
function timelineCreateForm() {
  return {
    calendarId: '',
    calendars: [],
    init() {
      var campaignID = window.location.pathname.split('/campaigns/')[1].split('/')[0];
      Chronicle.apiFetch('/campaigns/' + campaignID + '/timelines/calendars')
        .then(function (r) { return r.json(); })
        .then(function (data) { this.calendars = data || []; }.bind(this))
        .catch(function (err) { console.warn('[timeline] Failed to load calendars:', err); });
    }
  };
}
