/**
 * attributes.js -- Chronicle Attributes Widget
 *
 * Displays entity custom fields (attributes) with inline editing support.
 * Shows field values in a read-only card by default; clicking "Edit" reveals
 * editable inputs that match the field type definitions (text, number, select,
 * textarea, checkbox, url). Auto-mounted by boot.js on elements with
 * data-widget="attributes".
 *
 * Config (from data-* attributes):
 *   data-endpoint   - Fields API endpoint (GET/PUT),
 *                     e.g. /campaigns/:id/entities/:eid/fields
 *   data-editable   - "true" if user can modify fields (Scribe+)
 *   data-era        - "true" when the endpoint is a page's era version: the
 *                     per-page field settings (the gear) belong to the usual
 *                     page, so they are left out
 *   data-era-highlight - "true" to mark the fields the version changes, with
 *                     their usual value (the response's era_changed, GM only)
 */
Chronicle.register('attributes', {
  init: function (el, config) {
    var state = {
      fields: [],            // Effective (merged) field definitions
      typeFields: [],        // Original type-level fields (for override reference)
      fieldsData: {},        // Current field values
      fieldOverrides: null,  // Per-entity overrides (added, hidden, modified)
      isEditing: false,      // Whether the edit form is shown
      isCustomizing: false,  // Whether the field override panel is shown
      isSaving: false,
      loadingChoices: false, // Edit clicked, pick lists still arriving
      choiceLists: {},       // fieldKey -> {systemName, choices} for text fields
      eraChanged: null,      // fieldKey -> usual value, for an era version's changes
      error: null
    };

    // Campaign and entity ids for the pick-from-the-system control.
    var idMatch = config.endpoint.match(/\/campaigns\/([^/]+)\/entities\/([^/]+)\/fields$/);
    var campaignId = idMatch ? idMatch[1] : String(config.campaignId || '');
    var entityId = idMatch ? idMatch[2] : '';

    // Fields people commonly expect a system list for; when the system has
    // none we say so instead of leaving a bare text box unexplained.
    var LISTABLE = ['ancestry', 'race', 'species', 'heritage', 'class', 'subclass',
      'kit', 'career', 'culture', 'background'];

    // Fetches the pick list for every text field before the form opens, so the
    // form renders once. A failed fetch leaves that field a plain text box.
    function loadChoiceLists() {
      if (!campaignId || !Chronicle.fetchChoices) return Promise.resolve();
      var jobs = state.fields.filter(function (f) { return f.type === 'text'; }).slice(0, 40)
        .map(function (f) {
          return Chronicle.fetchChoices(campaignId, f.key).then(function (l) {
            state.choiceLists[f.key] = l;
          }).catch(function () { /* plain text box */ });
        });
      return Promise.all(jobs);
    }

    el._attributesState = state;

    // Derive the overrides endpoint from the fields endpoint.
    var overridesEndpoint = config.endpoint.replace(/\/fields$/, '/field-overrides');

    // Load field definitions and current values.
    function reload() {
      return Chronicle.apiFetch(config.endpoint)
        .then(function (r) {
          if (!r.ok) throw new Error('Failed to load fields');
          return r.json();
        })
        .then(function (data) {
          state.fields = data.fields || [];
          state.typeFields = data.type_fields || data.fields || [];
          state.fieldsData = data.fields_data || {};
          state.fieldOverrides = data.field_overrides || null;
          state.eraChanged = data.era_changed || null;
          state.shown = data.shown || {};
          state.calendar = data.calendar || null;
          render();
        })
        .catch(function (err) {
          console.error('[attributes] Failed to load:', err);
          state.error = 'Failed to load attributes.';
          render();
        });
    }
    reload();

    // --- Render ---

    function render() {
      el.innerHTML = '';

      // Nothing to show if no fields defined.
      if (state.fields.length === 0 && !state.error) {
        return;
      }

      var card = document.createElement('div');
      card.className = 'card cb-card cb-facts p-4';

      // Header with title and edit toggle.
      var header = document.createElement('div');
      header.className = 'flex items-center justify-between mb-3';

      var title = document.createElement('h3');
      title.className = 'cb-head text-xs font-semibold uppercase tracking-wider';
      title.style.color = 'var(--color-text-secondary)';
      title.textContent = 'Attributes';
      header.appendChild(title);

      if (config.editable) {
        var btnGroup = document.createElement('div');
        btnGroup.className = 'flex items-center gap-1.5';

        // Gear icon for field customization (per-entity overrides).
        var gearBtn = document.createElement('button');
        gearBtn.type = 'button';
        gearBtn.className = 'chronicle-editor__edit-btn';
        gearBtn.title = 'Customize fields for this page';
        gearBtn.innerHTML = '<i class="fa-solid fa-gear" style="font-size:11px"></i>';
        gearBtn.addEventListener('click', function () {
          state.isCustomizing = !state.isCustomizing;
          state.isEditing = false;
          render();
        });
        if (!config.era) btnGroup.appendChild(gearBtn);

        var editBtn = document.createElement('button');
        editBtn.type = 'button';

        if (state.isEditing) {
          editBtn.className = 'chronicle-editor__edit-btn chronicle-editor__edit-btn--done';
          editBtn.innerHTML = '<i class="fa-solid fa-check" style="font-size:11px"></i> Done';
          editBtn.addEventListener('click', function () {
            saveFields();
          });
        } else {
          editBtn.className = 'chronicle-editor__edit-btn';
          editBtn.innerHTML = '<i class="fa-solid fa-pen" style="font-size:11px"></i> Edit';
          editBtn.addEventListener('click', function () {
            if (state.loadingChoices) return;
            state.loadingChoices = true;
            editBtn.disabled = true;
            editBtn.innerHTML = '<i class="fa-solid fa-spinner fa-spin" style="font-size:11px"></i> Loading';
            loadChoiceLists().then(function () {
              state.loadingChoices = false;
              state.draft = null;
              state.isEditing = true;
              state.isCustomizing = false;
              render();
            });
          });
        }
        btnGroup.appendChild(editBtn);
        header.appendChild(btnGroup);
      }

      card.appendChild(header);

      // Error display.
      if (state.error) {
        var errorEl = document.createElement('div');
        errorEl.className = 'text-sm text-red-500 mb-2';
        errorEl.textContent = state.error;
        card.appendChild(errorEl);
      }

      // Fields content.
      var content = document.createElement('div');
      content.className = 'space-y-3';

      if (state.isCustomizing) {
        renderCustomizePanel(content);
      } else if (state.isEditing) {
        renderEditForm(content);
      } else {
        renderReadOnly(content);
      }

      card.appendChild(content);
      el.appendChild(card);
    }

    // --- Read-only view ---

    function renderReadOnly(container) {
      var hasValues = false;
      // Fields are grouped by section in the edit form's order. Each group is
      // a cb-section; its heading shows only in a page look that draws it.
      var groups = {};
      var order = [];
      state.fields.forEach(function (field) {
        var sec = field.section || 'General';
        if (!groups[sec]) {
          groups[sec] = document.createElement('div');
          groups[sec].className = 'cb-section space-y-3';
          order.push(sec);
        }
      });

      state.fields.forEach(function (field) {
        var val = state.fieldsData[field.key];
        var marked = isMarked(field.key);
        // shown is how a field reads when that isn't its stored value: a
        // link's page name, a rule's result, a local word.
        var sh = shownOf(field.key);
        var blank = sh ? !sh.text : (val === undefined || val === null || val === '');
        if (blank && !marked) return;

        hasValues = true;
        var group = groups[field.section || 'General'];
        if (!group.childNodes.length && order.length > 1) {
          var head = document.createElement('div');
          head.className = 'cb-section-h';
          head.textContent = field.section || 'General';
          group.appendChild(head);
        }
        var row = document.createElement('div');
        row.className = 'cb-fact';

        var label = document.createElement('dt');
        label.className = 'cb-fact-l text-xs font-medium uppercase tracking-wider';
        label.style.color = 'var(--color-text-secondary)';
        label.textContent = field.label;
        row.appendChild(label);

        var value = document.createElement('dd');
        value.className = 'cb-fact-v text-sm mt-0.5';
        value.style.color = 'var(--color-text-primary)';

        if (field.type === 'calendar_date' && sh && !blank && !marked) {
          renderDate(group, row, value, sh);
          return;
        }

        // Format value based on field type.
        if (field.type === 'checkbox') {
          value.textContent = val === true || val === 'true' || val === 'on' ? 'Yes' : 'No';
        } else if (field.type === 'url' && val) {
          var link = document.createElement('a');
          link.href = String(val);
          link.textContent = String(val);
          link.className = 'text-accent hover:underline';
          link.target = '_blank';
          link.rel = 'noopener noreferrer';
          value.appendChild(link);
        } else if (blank) {
          value.textContent = 'Cleared in this era';
          value.style.color = 'var(--color-text-muted)';
        } else if (sh && sh.href) {
          var page = document.createElement('a');
          page.href = sh.href;
          page.textContent = sh.text;
          page.className = 'text-accent hover:underline';
          page.setAttribute('hx-boost', 'true');
          value.appendChild(page);
        } else if (sh) {
          value.textContent = sh.text;
          if (sh.usual) value.title = 'Usually called ' + sh.usual;
        } else {
          value.textContent = String(val);
        }
        if (sh && sh.byHand && sh.ruleGives) {
          row.appendChild(value);
          var hand = document.createElement('div');
          hand.className = 'era-was';
          hand.textContent = 'Set by hand. The rule gives ' + sh.ruleGives + '.';
          row.appendChild(hand);
          group.appendChild(row);
          return;
        }

        if (marked) {
          value.classList.add('era-hl-field');
          row.appendChild(value);
          var was = document.createElement('div');
          was.className = 'era-was';
          was.textContent = 'Usually ' + usualText(field);
          row.appendChild(was);
          group.appendChild(row);
          return;
        }
        row.appendChild(value);
        group.appendChild(row);
      });
      order.forEach(function (sec) {
        if (groups[sec].childNodes.length) container.appendChild(groups[sec]);
      });

      if (!hasValues) {
        var empty = document.createElement('div');
        empty.className = 'text-sm';
        empty.style.color = 'var(--color-text-muted)';
        empty.textContent = config.editable
          ? 'No attributes set. Click Edit to add values.'
          : 'No attributes set.';
        container.appendChild(empty);
      }
    }

    // renderDate shows a calendar date field: its date, how long ago it was,
    // and a row per countdown as of the calendar's today. The server has
    // already left out what this viewer may not see.
    function renderDate(group, row, value, sh) {
      value.textContent = sh.text;
      var d = sh.date || {};
      if (d.ago) value.appendChild(whenSpan(d.ago));
      row.appendChild(value);
      group.appendChild(row);
      (d.countdowns || []).forEach(function (cd) {
        var r = document.createElement('div');
        r.className = 'cb-fact';
        var l = document.createElement('dt');
        l.className = 'cb-fact-l text-xs font-medium uppercase tracking-wider';
        l.style.color = 'var(--color-text-secondary)';
        l.textContent = cd.name;
        if (cd.directorOnly) {
          var dm = document.createElement('i');
          dm.className = 'fa-solid fa-eye-slash cd-dm';
          dm.title = 'Only you see this countdown';
          dm.setAttribute('aria-label', 'Only you see this countdown');
          l.appendChild(dm);
        }
        r.appendChild(l);
        var v = document.createElement('dd');
        v.className = 'cb-fact-v text-sm mt-0.5';
        v.style.color = 'var(--color-text-primary)';
        var len = cd.days + (cd.days === 1 ? ' day' : ' days') + (cd.length ? ', ' + cd.length : '');
        if (cd.state === 'before') {
          v.textContent = len + ' from the date';
        } else if (cd.state === 'ended') {
          var e = document.createElement('b');
          e.textContent = 'Ended';
          v.appendChild(e);
          v.appendChild(whenSpan('on ' + cd.last));
        } else {
          var left = document.createElement('span');
          left.className = 'cd-left';
          var b = document.createElement('b');
          b.textContent = cd.state === 'last' ? 'Last day today' : cd.left + (cd.left === 1 ? ' day left' : ' days left');
          left.appendChild(b);
          left.appendChild(ticks(cd));
          v.appendChild(left);
          if (cd.state === 'running') v.appendChild(whenSpan('Last day ' + cd.last));
        }
        if (cd.length && cd.state !== 'before') v.appendChild(whenSpan(len));
        r.appendChild(v);
        group.appendChild(r);
      });
    }

    function whenSpan(text) {
      var w = document.createElement('span');
      w.className = 'cd-when';
      w.textContent = text;
      return w;
    }

    // ticks draws a short countdown's days, lit while still to come.
    function ticks(cd) {
      var t = document.createElement('span');
      t.className = 'cd-ticks';
      t.setAttribute('aria-hidden', 'true');
      if (cd.days > 12) return t;
      for (var i = 1; i <= cd.days; i++) {
        var tick = document.createElement('i');
        if (i > cd.done) tick.className = 'on';
        t.appendChild(tick);
      }
      return t;
    }

    function shownOf(key) {
      return state.shown && Object.prototype.hasOwnProperty.call(state.shown, key) ? state.shown[key] : null;
    }

    // isMarked reports whether a field is one the shown era version changes
    // and highlights are on.
    function isMarked(key) {
      return !!(config.eraHighlight && state.eraChanged &&
        Object.prototype.hasOwnProperty.call(state.eraChanged, key));
    }

    // usualText is a changed field's value on the usual page, as shown.
    function usualText(field) {
      var u = state.eraChanged ? state.eraChanged[field.key] : null;
      if (u === undefined || u === null || u === '') return 'empty';
      if (field.type === 'checkbox') return u === true || u === 'true' || u === 'on' ? 'Yes' : 'No';
      return String(u);
    }

    // --- Edit form ---

    function renderEditForm(container) {
      // Group fields by section.
      var sections = {};
      var sectionOrder = [];

      state.fields.forEach(function (field) {
        var sec = field.section || 'General';
        if (!sections[sec]) {
          sections[sec] = [];
          sectionOrder.push(sec);
        }
        sections[sec].push(field);
      });

      sectionOrder.forEach(function (sectionName) {
        // Section header (only if more than one section).
        if (sectionOrder.length > 1) {
          var secHeader = document.createElement('div');
          secHeader.className = 'text-xs font-semibold uppercase tracking-wider pt-2 pb-1';
          secHeader.style.color = 'var(--color-text-muted)';
          secHeader.textContent = sectionName;
          container.appendChild(secHeader);
        }

        sections[sectionName].forEach(function (field) {
          var row = document.createElement('div');

          var label = document.createElement('label');
          label.className = 'block text-xs font-medium mb-1';
          label.style.color = 'var(--color-text-secondary)';
          label.textContent = field.label;
          label.htmlFor = 'attr-' + field.key;
          row.appendChild(label);

          // A refused save keeps what was typed (state.draft) so it can be fixed.
          var currentVal = (state.draft || state.fieldsData)[field.key];
          if (currentVal === undefined || currentVal === null) currentVal = '';

          var input;

          switch (field.type) {
            case 'textarea':
              input = document.createElement('textarea');
              input.className = 'input';
              input.rows = 3;
              input.value = String(currentVal);
              break;

            case 'select':
              input = document.createElement('select');
              input.className = 'input';
              var emptyOpt = document.createElement('option');
              emptyOpt.value = '';
              emptyOpt.textContent = 'Select...';
              input.appendChild(emptyOpt);
              (field.options || []).forEach(function (opt) {
                var option = document.createElement('option');
                option.value = opt;
                option.textContent = opt;
                if (String(currentVal) === opt) option.selected = true;
                input.appendChild(option);
              });
              break;

            case 'link':
              row.appendChild(buildPagePicker(field, String(currentVal)));
              container.appendChild(row);
              return;

            case 'calendar_date':
              row.appendChild(buildDatePicker(field, String(currentVal)));
              container.appendChild(row);
              return;

            case 'rule':
              // Empty follows the rule; anything typed wins over it.
              var sh = shownOf(field.key);
              var gives = sh ? (sh.byHand ? sh.ruleGives || '' : sh.text) : '';
              input = document.createElement('input');
              input.type = 'text';
              input.className = 'input';
              input.value = String(currentVal);
              input.placeholder = gives ? gives + ' (worked out)' : 'Worked out by its rule';
              var ruleNote = document.createElement('p');
              ruleNote.className = 'ag-note';
              ruleNote.textContent = 'Leave empty to follow the rule, or type a value to set it by hand.';
              row.appendChild(ruleNote);
              break;

            case 'checkbox':
              input = document.createElement('input');
              input.type = 'checkbox';
              input.className = 'h-4 w-4 rounded border-edge text-accent focus:ring-accent';
              input.checked = currentVal === true || currentVal === 'true' || currentVal === 'on';
              break;

            default: // text, number, url
              var pl = field.type === 'text' ? state.choiceLists[field.key] : null;
              if (pl && pl.choices.length > 0) {
                row.appendChild(buildPicker(field, pl, String(currentVal)));
                container.appendChild(row);
                return;
              }
              if (pl && (field.choices || LISTABLE.indexOf(field.key) >= 0)) {
                var note = document.createElement('p');
                note.className = 'ag-note';
                note.textContent = (pl.systemName || 'This game system') + ' doesn\u2019t list these yet, type it in.';
                row.appendChild(note);
              }
              input = document.createElement('input');
              input.type = field.type === 'number' ? 'number' : (field.type === 'url' ? 'url' : 'text');
              input.className = 'input';
              input.value = String(currentVal);
              break;
          }

          input.id = 'attr-' + field.key;
          input.setAttribute('data-field-key', field.key);
          row.appendChild(input);
          if (state.eraChanged && Object.prototype.hasOwnProperty.call(state.eraChanged, field.key)) {
            row.appendChild(usualButton(field, input));
          }

          container.appendChild(row);
        });
      });
    }

    // usualButton puts a field the era version changes back to its usual
    // value; the version then stops changing it once saved.
    function usualButton(field, input) {
      var b = document.createElement('button');
      b.type = 'button';
      b.className = 'era-usual-val';
      b.textContent = 'Use the usual value (' + usualText(field) + ')';
      b.addEventListener('click', function () {
        var u = state.eraChanged[field.key];
        if (field.type === 'checkbox') input.checked = u === true || u === 'true' || u === 'on';
        else input.value = u === undefined || u === null ? '' : String(u);
        b.remove();
      });
      return b;
    }

    // --- Pick a date for a calendar date field ---

    // The hidden input holds "year-month-day|picks", the stored form
    // (field_dates.go); the visible year, month and day boxes and each
    // countdown's length choice write it. Without the campaign calendar the
    // month is a number.
    function buildDatePicker(field, currentVal) {
      var wrap = document.createElement('div');
      wrap.className = 'cd-pick';
      var hidden = document.createElement('input');
      hidden.type = 'hidden';
      hidden.setAttribute('data-field-key', field.key);
      hidden.value = currentVal;
      var m = /^(-?\d+)-(\d+)-(\d+)(?:\|([\d,]+))?$/.exec(currentVal.trim());
      var today = state.calendar ? state.calendar.today : null;
      var cur = m ? { y: m[1], m: m[2], d: m[3] } : { y: '', m: '', d: '' };
      var picks = m && m[4] ? m[4].split(',').map(Number) : [];

      var row = document.createElement('div');
      row.className = 'flex flex-wrap items-center gap-2';
      var day = numBox('Day', cur.d, 'w-16');
      var month;
      if (state.calendar && state.calendar.months && state.calendar.months.length) {
        month = document.createElement('select');
        month.className = 'input w-auto';
        month.setAttribute('aria-label', 'Month');
        var none = document.createElement('option');
        none.value = '';
        none.textContent = 'Month';
        month.appendChild(none);
        state.calendar.months.forEach(function (mo, i) {
          var o = document.createElement('option');
          o.value = String(i + 1);
          o.textContent = mo.name;
          if (String(i + 1) === cur.m) o.selected = true;
          month.appendChild(o);
        });
      } else {
        month = numBox('Month', cur.m, 'w-16');
      }
      var year = numBox('Year', cur.y, 'w-24');
      row.appendChild(day);
      row.appendChild(month);
      row.appendChild(year);
      if (today) {
        var t = document.createElement('button');
        t.type = 'button';
        t.className = 'btn-secondary text-xs';
        t.textContent = 'Today';
        t.addEventListener('click', function () {
          day.value = today.day; month.value = String(today.month); year.value = today.year;
          write();
        });
        row.appendChild(t);
      }
      var clear = document.createElement('button');
      clear.type = 'button';
      clear.className = 'btn-secondary text-xs';
      clear.textContent = 'Clear';
      clear.addEventListener('click', function () {
        day.value = ''; month.value = ''; year.value = '';
        write();
      });
      row.appendChild(clear);
      wrap.appendChild(row);

      // A countdown with more than one length lets this page pick one.
      var lens = [];
      (field.countdowns || []).forEach(function (cd, i) {
        if (!cd.lengths || cd.lengths.length < 2) { lens.push(null); return; }
        var lab = document.createElement('label');
        lab.className = 'flex flex-wrap items-center gap-2 text-xs mt-2';
        lab.style.color = 'var(--color-text-secondary)';
        lab.appendChild(document.createTextNode(cd.name));
        var sel = document.createElement('select');
        sel.className = 'input w-auto text-xs';
        cd.lengths.forEach(function (l, li) {
          var o = document.createElement('option');
          o.value = String(li);
          o.textContent = (li === 0 ? 'Usually' : l.name) + ', ' + l.days + (l.days === 1 ? ' day' : ' days');
          if ((picks[i] || 0) === li) o.selected = true;
          sel.appendChild(o);
        });
        sel.addEventListener('change', write);
        lab.appendChild(sel);
        wrap.appendChild(lab);
        lens.push(sel);
      });

      [day, month, year].forEach(function (b) { b.addEventListener('input', write); b.addEventListener('change', write); });

      function write() {
        if (!day.value || !month.value || !year.value) { hidden.value = ''; return; }
        var v = parseInt(year.value, 10) + '-' + parseInt(month.value, 10) + '-' + parseInt(day.value, 10);
        var p = lens.map(function (s) { return s ? parseInt(s.value, 10) : 0; });
        while (p.length && !p[p.length - 1]) p.pop();
        hidden.value = p.length ? v + '|' + p.join(',') : v;
      }

      function numBox(label, value, cls) {
        var i = document.createElement('input');
        i.type = 'number';
        i.className = 'input ' + cls;
        i.placeholder = label;
        i.setAttribute('aria-label', label);
        i.value = value;
        return i;
      }

      wrap.appendChild(hidden);
      return wrap;
    }

    // --- Pick a page for a link field ---

    // The hidden input holds the page id the save step reads; the visible box
    // searches the field's category by name. Pages the viewer can't open
    // never come back from the search, so none can be picked.
    function buildPagePicker(field, currentVal) {
      var wrap = document.createElement('div');
      var hidden = document.createElement('input');
      hidden.type = 'hidden';
      hidden.value = currentVal;
      hidden.setAttribute('data-field-key', field.key);
      wrap.appendChild(hidden);

      var listId = 'attr-' + field.key + '-pages';
      var box = document.createElement('input');
      box.type = 'search';
      box.className = 'input';
      box.id = 'attr-' + field.key;
      box.setAttribute('list', listId);
      box.placeholder = 'Type a page name';
      var sh = shownOf(field.key);
      box.value = sh && sh.text ? sh.text : '';
      var list = document.createElement('datalist');
      list.id = listId;
      wrap.appendChild(box);
      wrap.appendChild(list);

      var found = {}, timer = null, seq = 0;
      function search() {
        var q = box.value.trim(), mine = ++seq;
        var url = '/campaigns/' + encodeURIComponent(campaignId) + '/entities/search?q=' + encodeURIComponent(q) +
          (field.link_to ? '&type=' + encodeURIComponent(field.link_to) : '');
        Chronicle.apiFetch(url).then(function (r) { return r.ok ? r.json() : { results: [] }; }).then(function (d) {
          if (mine !== seq) return;
          list.innerHTML = '';
          found = {};
          (d.results || []).forEach(function (p) {
            if (p.id === entityId) return;
            found[p.name.toLowerCase()] = p.id;
            var o = document.createElement('option');
            o.value = p.name;
            list.appendChild(o);
          });
          pick();
        }).catch(function () {});
      }
      // An exact name sets the link; an emptied box clears it; anything else
      // keeps the page already linked until a name matches.
      function pick() {
        var name = box.value.trim().toLowerCase();
        if (!name) hidden.value = '';
        else if (found[name]) hidden.value = found[name];
      }
      box.addEventListener('input', function () {
        pick();
        clearTimeout(timer);
        timer = setTimeout(search, 200);
      });
      search();
      return wrap;
    }

    // --- Pick from the system's list ---

    // A hidden input keeps the value the save step already reads
    // (data-field-key); the visible button opens the shared picker, which
    // only hands the chosen NAME back. Nothing is saved until Done.
    function buildPicker(field, list, currentVal) {
      var wrap = document.createElement('div');

      var hidden = document.createElement('input');
      hidden.type = 'hidden';
      hidden.id = 'attr-' + field.key;
      hidden.value = currentVal;
      hidden.setAttribute('data-field-key', field.key);
      wrap.appendChild(hidden);

      var btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'ag-pick';
      btn.setAttribute('aria-expanded', 'false');
      function paint() {
        btn.innerHTML = '';
        var span = document.createElement('span');
        if (hidden.value) {
          span.textContent = hidden.value;
        } else {
          span.className = 'ph';
          span.textContent = 'Pick ' + field.label.toLowerCase();
        }
        btn.appendChild(span);
        var chev = document.createElement('i');
        chev.className = 'fa-solid fa-chevron-down chev';
        btn.appendChild(chev);
      }
      paint();
      btn.addEventListener('click', function () {
        Chronicle.pickChoice({
          campaignId: campaignId,
          fieldKey: field.key,
          label: field.label,
          current: hidden.value,
          anchorEl: btn,
          save: false,
          choices: list
        }).then(function (res) {
          if (res) { hidden.value = res.value; paint(); }
        });
      });
      wrap.appendChild(btn);
      return wrap;
    }

    // --- Customize panel (per-entity field overrides) ---

    function renderCustomizePanel(container) {
      var overrides = state.fieldOverrides || { added: [], hidden: [], modified: {} };

      // Header.
      var info = document.createElement('div');
      info.className = 'text-xs mb-3';
      info.style.color = 'var(--color-text-muted)';
      info.textContent = 'Toggle fields on/off or add custom fields for this page only.';
      container.appendChild(info);

      // Toggle list for type-level fields.
      var toggleLabel = document.createElement('div');
      toggleLabel.className = 'text-[10px] font-semibold uppercase tracking-wider mb-1.5';
      toggleLabel.style.color = 'var(--color-text-muted)';
      toggleLabel.textContent = 'Category Fields';
      container.appendChild(toggleLabel);

      var hiddenSet = {};
      (overrides.hidden || []).forEach(function (k) { hiddenSet[k] = true; });

      state.typeFields.forEach(function (field) {
        var row = document.createElement('div');
        row.className = 'flex items-center justify-between py-1';

        var lbl = document.createElement('span');
        lbl.className = 'text-sm';
        lbl.style.color = hiddenSet[field.key] ? 'var(--color-text-muted)' : 'var(--color-text-primary)';
        lbl.textContent = field.label;
        if (hiddenSet[field.key]) {
          lbl.style.textDecoration = 'line-through';
        }
        row.appendChild(lbl);

        var toggle = document.createElement('button');
        toggle.type = 'button';
        toggle.className = 'text-xs px-2 py-0.5 rounded border';
        if (hiddenSet[field.key]) {
          toggle.textContent = 'Hidden';
          toggle.style.color = 'var(--color-text-muted)';
          toggle.style.borderColor = 'var(--color-border)';
        } else {
          toggle.textContent = 'Visible';
          toggle.style.color = 'var(--color-accent)';
          toggle.style.borderColor = 'var(--color-accent)';
        }
        toggle.addEventListener('click', function () {
          if (hiddenSet[field.key]) {
            delete hiddenSet[field.key];
          } else {
            hiddenSet[field.key] = true;
          }
          overrides.hidden = Object.keys(hiddenSet);
          render();
        });
        row.appendChild(toggle);
        container.appendChild(row);
      });

      // Added fields section.
      var addedLabel = document.createElement('div');
      addedLabel.className = 'text-[10px] font-semibold uppercase tracking-wider mb-1.5 mt-4';
      addedLabel.style.color = 'var(--color-text-muted)';
      addedLabel.textContent = 'Custom Fields (this page only)';
      container.appendChild(addedLabel);

      var addedFields = overrides.added || [];
      addedFields.forEach(function (f, idx) {
        var row = document.createElement('div');
        row.className = 'flex items-center gap-2 mb-1';

        var nameInput = document.createElement('input');
        nameInput.type = 'text';
        nameInput.className = 'input text-sm flex-1';
        nameInput.placeholder = 'Field name';
        nameInput.value = f.label || '';
        nameInput.addEventListener('input', function () {
          f.label = nameInput.value;
          f.key = nameInput.value.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '') || 'custom_field';
        });
        row.appendChild(nameInput);

        var typeSelect = document.createElement('select');
        typeSelect.className = 'input text-sm w-24';
        ['text', 'number', 'textarea', 'url', 'checkbox', 'select'].forEach(function (t) {
          var opt = document.createElement('option');
          opt.value = t;
          opt.textContent = t.charAt(0).toUpperCase() + t.slice(1);
          if (f.type === t) opt.selected = true;
          typeSelect.appendChild(opt);
        });
        typeSelect.addEventListener('change', function () { f.type = typeSelect.value; });
        row.appendChild(typeSelect);

        var delBtn = document.createElement('button');
        delBtn.type = 'button';
        delBtn.className = 'text-red-400 hover:text-red-600 text-xs p-1';
        delBtn.innerHTML = '<i class="fa-solid fa-xmark"></i>';
        delBtn.addEventListener('click', function () {
          addedFields.splice(idx, 1);
          overrides.added = addedFields;
          render();
        });
        row.appendChild(delBtn);

        container.appendChild(row);
      });

      // Add field button.
      var addBtn = document.createElement('button');
      addBtn.type = 'button';
      addBtn.className = 'text-xs text-accent hover:text-accent-hover mt-1';
      addBtn.innerHTML = '<i class="fa-solid fa-plus mr-1"></i> Add Field';
      addBtn.addEventListener('click', function () {
        addedFields.push({ key: '', label: '', type: 'text', section: 'Custom' });
        overrides.added = addedFields;
        render();
      });
      container.appendChild(addBtn);

      // Button row: Save + Reset.
      var btnRow = document.createElement('div');
      btnRow.className = 'flex items-center gap-2 mt-4';

      // Save button.
      var saveBtn = document.createElement('button');
      saveBtn.type = 'button';
      saveBtn.className = 'chronicle-editor__edit-btn chronicle-editor__edit-btn--done flex-1';
      saveBtn.innerHTML = '<i class="fa-solid fa-check" style="font-size:11px"></i> Save Customizations';
      saveBtn.addEventListener('click', function () {
        saveOverrides(overrides);
      });
      btnRow.appendChild(saveBtn);

      // Reset to template button (only shown if overrides exist).
      if (state.fieldOverrides) {
        var resetBtn = document.createElement('button');
        resetBtn.type = 'button';
        resetBtn.className = 'chronicle-editor__edit-btn flex-shrink-0';
        resetBtn.style.color = 'var(--color-text-muted)';
        resetBtn.title = 'Reset to category template';
        resetBtn.innerHTML = '<i class="fa-solid fa-rotate-left" style="font-size:11px"></i> Reset';
        resetBtn.addEventListener('click', function () {
          askReset(btnRow);
        });
        btnRow.appendChild(resetBtn);
      }

      container.appendChild(btnRow);
    }

    // askReset puts the question in the button row's place, so it sits right
    // where the person clicked. The row is only hidden, not rebuilt, because
    // rebuilding would throw away customizations not saved yet.
    function askReset(btnRow) {
      if (btnRow.querySelector('[data-reset-ask]')) return;
      var ask = document.createElement('div');
      ask.className = 'ag-confirm flex-1';
      ask.setAttribute('data-reset-ask', '1');
      ask.setAttribute('role', 'alertdialog');
      ask.setAttribute('aria-label', 'Reset to the category template');
      var p = document.createElement('p');
      p.textContent = 'Reset to the category template?';
      var small = document.createElement('small');
      small.textContent = 'Custom fields and visibility changes on this page will be lost.';
      var row = document.createElement('div');
      row.className = 'flex items-center gap-2 mt-2';
      var yes = document.createElement('button');
      yes.type = 'button';
      yes.className = 'chronicle-editor__edit-btn';
      yes.style.color = 'var(--color-danger-strong, #dc2626)';
      yes.textContent = 'Reset';
      var keep = document.createElement('button');
      keep.type = 'button';
      keep.className = 'chronicle-editor__edit-btn';
      keep.textContent = 'Keep my changes';
      row.appendChild(yes);
      row.appendChild(keep);
      ask.appendChild(p);
      ask.appendChild(small);
      ask.appendChild(row);

      var shown = Array.prototype.slice.call(btnRow.children);
      shown.forEach(function (c) { c.hidden = true; });
      btnRow.appendChild(ask);
      yes.focus();

      function close() {
        ask.remove();
        shown.forEach(function (c) { c.hidden = false; });
      }
      keep.addEventListener('click', close);
      ask.addEventListener('keydown', function (e) {
        if (e.key === 'Escape') { e.stopPropagation(); close(); }
      });
      yes.addEventListener('click', function () {
        yes.disabled = true;
        keep.disabled = true;
        yes.textContent = 'Resetting\u2026';
        resetOverrides();
      });
    }

    function resetOverrides() {
      if (state.isSaving) return;
      state.isSaving = true;

      Chronicle.apiFetch(overridesEndpoint, {
        method: 'DELETE'
      })
        .then(function (r) {
          if (!r.ok) throw new Error('Failed to reset overrides');
          state.fieldOverrides = null;
          state.isCustomizing = false;
          state.isSaving = false;
          // Reload to get type-level fields without overrides.
          return Chronicle.apiFetch(config.endpoint);
        })
        .then(function (r) {
          if (!r.ok) throw new Error('Failed to reload fields');
          return r.json();
        })
        .then(function (data) {
          state.fields = data.fields || [];
          state.typeFields = data.type_fields || data.fields || [];
          state.fieldsData = data.fields_data || {};
          state.fieldOverrides = data.field_overrides || null;
          render();
        })
        .catch(function (err) {
          console.error('[attributes] Reset overrides failed:', err);
          state.error = 'Failed to reset customizations.';
          state.isSaving = false;
          render();
        });
    }

    function saveOverrides(overrides) {
      if (state.isSaving) return;
      state.isSaving = true;

      Chronicle.apiFetch(overridesEndpoint, {
        method: 'PUT',
        body: overrides
      })
        .then(function (r) {
          if (!r.ok) throw new Error('Failed to save overrides');
          state.fieldOverrides = overrides;
          state.isCustomizing = false;
          state.isSaving = false;
          // Reload to get merged fields.
          return Chronicle.apiFetch(config.endpoint);
        })
        .then(function (r) {
          if (!r.ok) throw new Error('Failed to reload fields');
          return r.json();
        })
        .then(function (data) {
          state.fields = data.fields || [];
          state.fieldsData = data.fields_data || {};
          state.fieldOverrides = data.field_overrides || null;
          render();
        })
        .catch(function (err) {
          console.error('[attributes] Save overrides failed:', err);
          state.error = 'Failed to save customizations.';
          state.isSaving = false;
          render();
        });
    }

    // --- Save ---

    function saveFields() {
      if (state.isSaving) return;
      state.isSaving = true;
      state.error = null;

      // Collect values from form inputs.
      var newData = {};
      state.fields.forEach(function (field) {
        var input = el.querySelector('[data-field-key="' + field.key + '"]');
        if (!input) return;

        if (field.type === 'checkbox') {
          newData[field.key] = input.checked;
        } else {
          newData[field.key] = input.value;
        }
      });

      Chronicle.apiFetch(config.endpoint, {
        method: 'PUT',
        body: { fields_data: newData }
      })
        .then(function (r) {
          if (!r.ok) {
            return r.json().catch(function () { return {}; }).then(function (b) {
              var e = new Error('Failed to save fields');
              e.said = b && b.message;
              throw e;
            });
          }
          state.fieldsData = newData;
          state.draft = null;
          state.isEditing = false;
          state.isSaving = false;
          render();
          // An era version keeps only what differs from the usual page, so
          // read back which fields it now changes.
          // Links, rules and local words are worked out by the server, so
          // they are read back too.
          if (config.era || Object.keys(state.shown || {}).length ||
            state.fields.some(function (f) { return f.type === 'link' || f.type === 'rule' || f.type === 'calendar_date'; })) reload();
        })
        .catch(function (err) {
          console.error('[attributes] Save failed:', err);
          state.draft = newData;
          state.error = err.said || 'Failed to save. Please try again.';
          state.isSaving = false;
          render();
        });
    }
  },

  destroy: function (el) {
    el.innerHTML = '';
    delete el._attributesState;
  }
});
