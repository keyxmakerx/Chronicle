/**
 * timeline_page.js -- the Alpine components of a time line's own page: the
 * owner's visibility and entity-group panels, the standalone event forms,
 * the per-event visibility modal and the calendar event picker.
 *
 * Mount: <div data-widget="timeline-page" x-data x-ignore> around the page in
 * timelineShowContent (timeline.templ). A page <script> would be stripped by
 * htmx on a boosted swap, so these load on sight instead; x-ignore holds
 * Alpine off until they exist, and init hands the page to Alpine.
 */
(function () {
  function visibilityManager(campaignID, timelineID, initVisibility, initRules) {
    return {
      visibility: initVisibility,
      ruleMode: 'none',
      selectedUsers: [],
      members: [],
      saving: false,
      saved: false,
      apiBase: '/campaigns/' + campaignID + '/timelines/' + timelineID,
      init() {
        // Parse initial rules.
        if (initRules && initRules.allowed_users && initRules.allowed_users.length > 0) {
          this.ruleMode = 'allow';
          this.selectedUsers = initRules.allowed_users.slice();
        } else if (initRules && initRules.denied_users && initRules.denied_users.length > 0) {
          this.ruleMode = 'deny';
          this.selectedUsers = initRules.denied_users.slice();
        }
        // Load campaign members.
        var self = this;
        Chronicle.apiFetch('/campaigns/' + campaignID + '/timelines/members')
          .then(function(r) { return r.json(); })
          .then(function(data) {
            // Show non-owner members only (owners always see everything).
            self.members = (data || []).filter(function(m) { return m.role !== 'owner'; });
          })
          .catch(function(err) { console.warn('[timeline] Failed to load members:', err); });
      },
      toggleUser(uid) {
        var idx = this.selectedUsers.indexOf(uid);
        if (idx === -1) {
          this.selectedUsers.push(uid);
        } else {
          this.selectedUsers.splice(idx, 1);
        }
        this.updateRules();
      },
      updateRules() {
        this.saveTimeline();
      },
      saveTimeline() {
        var self = this;
        self.saving = true;
        self.saved = false;
        var rules = null;
        if (self.ruleMode === 'allow' && self.selectedUsers.length > 0) {
          rules = JSON.stringify({allowed_users: self.selectedUsers});
        } else if (self.ruleMode === 'deny' && self.selectedUsers.length > 0) {
          rules = JSON.stringify({denied_users: self.selectedUsers});
        }
        Chronicle.apiFetch(self.apiBase + '/visibility', {
          method: 'PUT',
          body: {
            visibility: self.visibility,
            visibility_rules: rules
          }
        })
        .then(function(r) {
          self.saving = false;
          if (r.ok) {
            self.saved = true;
            setTimeout(function() { self.saved = false; }, 2000);
          }
        })
        .catch(function(err) {
          self.saving = false;
          console.warn('[timeline] Failed to save visibility:', err);
        });
      }
    };
  }

  function entityGroupManager(campaignID, timelineID) {
    return {
      groups: [],
      newGroupName: '',
      newGroupColor: '#6b7280',
      entityResults: {},
      apiBase: '/campaigns/' + campaignID + '/timelines/' + timelineID,
      init() {
        this.loadGroups();
      },
      loadGroups() {
        var self = this;
        Chronicle.apiFetch(self.apiBase + '/groups')
          .then(function(r) { return r.json(); })
          .then(function(data) { self.groups = data || []; })
          .catch(function(err) { console.warn('[timeline] Failed to load groups:', err); });
      },
      createGroup() {
        var name = this.newGroupName.trim();
        if (!name) return;
        var self = this;
        Chronicle.apiFetch(self.apiBase + '/groups', {
          method: 'POST',
          body: {name: name, color: self.newGroupColor}
        })
        .then(function(r) { return r.json(); })
        .then(function(g) {
          g.members = [];
          self.groups.push(g);
          self.newGroupName = '';
          self.newGroupColor = '#6b7280';
        })
        .catch(function(err) { console.warn('[timeline] Failed to create group:', err); });
      },
      deleteGroup(groupID) {
        if (!confirm('Delete this group?')) return;
        var self = this;
        Chronicle.apiFetch(self.apiBase + '/groups/' + groupID, {
          method: 'DELETE'
        })
        .then(function(r) {
          if (r.ok) self.groups = self.groups.filter(function(g) { return g.id !== groupID; });
        })
        .catch(function(err) { console.warn('[timeline] Failed to delete group:', err); });
      },
      addMember(groupID, entityID, entityName) {
        var self = this;
        Chronicle.apiFetch(self.apiBase + '/groups/' + groupID + '/members', {
          method: 'POST',
          body: {entity_id: entityID}
        })
        .then(function(r) {
          if (r.ok) {
            var group = self.groups.find(function(g) { return g.id === groupID; });
            if (group) {
              if (!group.members) group.members = [];
              group.members.push({entity_id: entityID, entity_name: entityName});
            }
            self.entityResults[groupID] = [];
            var input = document.getElementById('group-entity-input-' + groupID);
            if (input) input.value = '';
          }
        })
        .catch(function(err) { console.warn('[timeline] Failed to add member:', err); });
      },
      removeMember(groupID, entityID) {
        var self = this;
        Chronicle.apiFetch(self.apiBase + '/groups/' + groupID + '/members/' + entityID, {
          method: 'DELETE'
        })
        .then(function(r) {
          if (r.ok) {
            var group = self.groups.find(function(g) { return g.id === groupID; });
            if (group) {
              group.members = (group.members||[]).filter(function(m) { return m.entity_id !== entityID; });
            }
          }
        })
        .catch(function(err) { console.warn('[timeline] Failed to remove member:', err); });
      },
      searchEntities(groupID, query) {
        var self = this;
        if (!query || query.length < 2) {
          self.entityResults[groupID] = [];
          return;
        }
        Chronicle.apiFetch('/campaigns/' + campaignID + '/entities/search?q=' + encodeURIComponent(query))
        .then(function(r) { return r.json(); })
        .then(function(data) {
          var results = (data.entities || data || []).slice(0, 8);
          // Filter out already-added members.
          var group = self.groups.find(function(g) { return g.id === groupID; });
          var memberIDs = {};
          if (group && group.members) {
            for (var i = 0; i < group.members.length; i++) {
              memberIDs[group.members[i].entity_id] = true;
            }
          }
          self.entityResults[groupID] = results.filter(function(e) { return !memberIDs[e.id]; });
        })
        .catch(function(err) { console.warn('[timeline] Entity search failed:', err); });
      }
    };
  }

  function standaloneEventForm() {
    var parts = window.location.pathname.split('/');
    var campaignID = parts[2];
    var timelineID = parts[4];
    return {
      name: '',
      year: 1,
      month: 1,
      day: 1,
      description: '',
      category: '',
      visibility: 'everyone',
      color: '',
      label: '',
      startHour: null,
      startMinute: null,
      endYear: null,
      endMonth: null,
      endDay: null,
      endHour: null,
      endMinute: null,
      isRecurring: false,
      recurrenceType: 'yearly',
      saving: false,
      errorMsg: '',
      submitEvent() {
        var self = this;
        if (!self.name.trim()) { self.errorMsg = 'Name is required'; return; }
        self.saving = true;
        self.errorMsg = '';

        var body = {
          name: self.name.trim(),
          year: self.year,
          month: self.month,
          day: self.day,
          visibility: self.visibility
        };
        // Send an explicit null when the operator EMPTIED a field —
        // under the sweep-R4 contract null clears and absent
        // preserves, so "if (self.description)" alone meant a
        // description could never be deleted. Every field the
        // modal does not own (entity link, description_html,
        // start/end times, recurrence, per-player visibility
        // rules) stays absent, which now preserves it.
        body.description = self.description || null;
        body.category = self.category || null;
        body.color = self.color || null;
        body.label = self.label || null;
        if (self.startHour !== null && self.startHour !== '') body.start_hour = parseInt(self.startHour);
        if (self.startMinute !== null && self.startMinute !== '') body.start_minute = parseInt(self.startMinute);
        if (self.endYear !== null && self.endYear !== '') body.end_year = parseInt(self.endYear);
        if (self.endMonth !== null && self.endMonth !== '') body.end_month = parseInt(self.endMonth);
        if (self.endDay !== null && self.endDay !== '') body.end_day = parseInt(self.endDay);
        if (self.endHour !== null && self.endHour !== '') body.end_hour = parseInt(self.endHour);
        if (self.endMinute !== null && self.endMinute !== '') body.end_minute = parseInt(self.endMinute);
        if (self.isRecurring) {
          body.is_recurring = true;
          body.recurrence_type = self.recurrenceType;
        }

        Chronicle.apiFetch('/campaigns/' + campaignID + '/timelines/' + timelineID + '/standalone-events', {
          method: 'POST',
          body: body
        })
        .then(function(r) {
          self.saving = false;
          if (r.ok) {
            document.getElementById('create-event-modal').classList.add('hidden');
            window.location.reload();
          } else {
            return r.json().then(function(data) {
              self.errorMsg = data.message || 'Failed to create event';
            }).catch(function() {
              self.errorMsg = 'Failed to create event';
            });
          }
        })
        .catch(function(err) {
          self.saving = false;
          self.errorMsg = 'Network error: ' + err.message;
        });
      }
    };
  }

  function editStandaloneEventForm() {
    var parts = window.location.pathname.split('/');
    var campaignID = parts[2];
    var timelineID = parts[4];
    return {
      eventID: '',
      name: '',
      year: 1,
      month: 1,
      day: 1,
      description: '',
      category: '',
      visibility: 'everyone',
      color: '',
      label: '',
      saving: false,
      errorMsg: '',
      openEdit(detail) {
        this.eventID = detail.id || '';
        this.name = detail.name || '';
        this.year = detail.year || 1;
        this.month = detail.month || 1;
        this.day = detail.day || 1;
        this.description = detail.description || '';
        this.category = detail.category || '';
        this.visibility = detail.visibility || 'everyone';
        this.color = detail.color || '';
        this.label = detail.label || '';
        this.errorMsg = '';
        document.getElementById('edit-event-modal').classList.remove('hidden');
      },
      closeEdit() {
        document.getElementById('edit-event-modal').classList.add('hidden');
      },
      submitEdit() {
        var self = this;
        if (!self.name.trim()) { self.errorMsg = 'Name is required'; return; }
        self.saving = true;
        self.errorMsg = '';

        var body = {
          name: self.name.trim(),
          year: self.year,
          month: self.month,
          day: self.day,
          visibility: self.visibility
        };
        if (self.description) body.description = self.description;
        if (self.category) body.category = self.category;
        if (self.color) body.color = self.color;
        if (self.label) body.label = self.label;

        Chronicle.apiFetch('/campaigns/' + campaignID + '/timelines/' + timelineID + '/standalone-events/' + self.eventID, {
          method: 'PUT',
          body: body
        })
        .then(function(r) {
          self.saving = false;
          if (r.ok) {
            self.closeEdit();
            window.location.reload();
          } else {
            return r.json().then(function(data) {
              self.errorMsg = data.message || 'Failed to update event';
            }).catch(function() {
              self.errorMsg = 'Failed to update event';
            });
          }
        })
        .catch(function(err) {
          self.saving = false;
          self.errorMsg = 'Network error: ' + err.message;
        });
      }
    };
  }

  function eventVisForm() {
    var parts = window.location.pathname.split('/');
    var campaignID = parts[2];
    var timelineID = parts[4];
    return {
      eventID: '',
      source: '',
      visibility: 'everyone',
      ruleMode: 'none',
      selectedUsers: [],
      members: [],
      saving: false,
      errorMsg: '',
      init() {
        var self = this;
        Chronicle.apiFetch('/campaigns/' + campaignID + '/timelines/members')
          .then(function(r) { return r.json(); })
          .then(function(data) {
            self.members = (data || []).filter(function(m) { return m.role !== 'owner'; });
          })
          .catch(function() {});
      },
      openVis(detail) {
        this.eventID = detail.id || '';
        this.source = detail.source || '';
        this.visibility = detail.visibility || 'everyone';
        this.ruleMode = 'none';
        this.selectedUsers = [];
        this.errorMsg = '';
        if (detail.rules) {
          if (detail.rules.allowed_users && detail.rules.allowed_users.length > 0) {
            this.ruleMode = 'allow';
            this.selectedUsers = detail.rules.allowed_users.slice();
          } else if (detail.rules.denied_users && detail.rules.denied_users.length > 0) {
            this.ruleMode = 'deny';
            this.selectedUsers = detail.rules.denied_users.slice();
          }
        }
        document.getElementById('event-vis-modal').classList.remove('hidden');
      },
      closeVis() {
        document.getElementById('event-vis-modal').classList.add('hidden');
      },
      toggleUser(uid) {
        var idx = this.selectedUsers.indexOf(uid);
        if (idx === -1) { this.selectedUsers.push(uid); }
        else { this.selectedUsers.splice(idx, 1); }
      },
      saveVis() {
        var self = this;
        self.saving = true;
        self.errorMsg = '';
        var rules = null;
        if (self.ruleMode === 'allow' && self.selectedUsers.length > 0) {
          rules = JSON.stringify({ allowed_users: self.selectedUsers });
        } else if (self.ruleMode === 'deny' && self.selectedUsers.length > 0) {
          rules = JSON.stringify({ denied_users: self.selectedUsers });
        }
        // Determine endpoint based on source.
        var url;
        if (self.source === 'standalone') {
          url = '/campaigns/' + campaignID + '/timelines/' + timelineID + '/standalone-events/' + self.eventID + '/visibility';
        } else {
          url = '/campaigns/' + campaignID + '/timelines/' + timelineID + '/events/' + self.eventID + '/visibility';
        }
        var body = { visibility_rules: rules };
        if (self.source === 'standalone') {
          body.visibility = self.visibility;
        } else {
          body.visibility_override = self.visibility === 'dm_only' ? self.visibility : null;
        }
        Chronicle.apiFetch(url, { method: 'PUT', body: body })
          .then(function(r) {
            self.saving = false;
            if (r.ok) { self.closeVis(); window.location.reload(); }
            else { r.json().then(function(d) { self.errorMsg = d.message || 'Failed'; }).catch(function() { self.errorMsg = 'Failed'; }); }
          })
          .catch(function(err) { self.saving = false; self.errorMsg = 'Network error: ' + err.message; });
      }
    };
  }

  function eventPicker() {
    return {
      search: '',
      events: [],
      loading: true,
      init() {
        var parts = window.location.pathname.split('/');
        var campaignID = parts[2];
        var timelineID = parts[4];
        this.apiBase = '/campaigns/' + campaignID + '/timelines/' + timelineID;
        this.loadEvents();
      },
      loadEvents() {
        var self = this;
        self.loading = true;
        Chronicle.apiFetch(self.apiBase + '/available-events')
          .then(function(r) { return r.json(); })
          .then(function(data) { self.events = data || []; self.loading = false; })
          .catch(function(err) { console.warn('[timeline] Failed to load events:', err); self.loading = false; });
      },
      filteredEvents() {
        var q = this.search.toLowerCase();
        if (!q) return this.events;
        return this.events.filter(function(e) {
          return e.name.toLowerCase().indexOf(q) !== -1 ||
            (e.entity_name && e.entity_name.toLowerCase().indexOf(q) !== -1) ||
            (e.category && e.category.toLowerCase().indexOf(q) !== -1);
        });
      },
      linkEvent(evt) {
        var self = this;
        Chronicle.apiFetch(self.apiBase + '/events', {
          method: 'POST',
          body: {event_id: evt.id}
        })
        .then(function(r) {
          if (r.ok) {
            self.events = self.events.filter(function(e) { return e.id !== evt.id; });
          }
        })
        .catch(function(err) { console.warn('[timeline] Failed to link event:', err); });
      }
    };
  }

  // Alpine evaluates x-data="visibilityManager(...)" and the rest by name.
  window.visibilityManager = visibilityManager;
  window.entityGroupManager = entityGroupManager;
  window.standaloneEventForm = standaloneEventForm;
  window.editStandaloneEventForm = editStandaloneEventForm;
  window.eventVisForm = eventVisForm;
  window.eventPicker = eventPicker;

  Chronicle.register('timeline-page', {
    init: function (el) { Chronicle.startAlpine(el); },
  });
})();
