// timeline_create_form_load.test.mjs — the Timelines page's "New Timeline"
// form uses x-data="timelineCreateForm()". htmx strips <script> tags from a
// boosted swap, so the component must come from the layout, and before Alpine,
// or Alpine throws "timelineCreateForm is not defined" when the page is
// reached from the sidebar.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (p) => readFileSync(join(root, p), 'utf8');

test('the layout loads timeline_create_form.js before Alpine', () => {
  const base = read('internal/templates/layouts/base.templ');
  const form = base.indexOf('/static/js/timeline_create_form.js');
  const alpine = base.indexOf('/static/vendor/alpine.min.js');
  assert.ok(form > 0, 'base.templ does not load timeline_create_form.js');
  assert.ok(form < alpine, 'timeline_create_form.js must load before alpine.min.js');
});

test('the Timelines page carries no inline timelineCreateForm script', () => {
  assert.doesNotMatch(read('internal/plugins/timeline/timeline.templ'), /function timelineCreateForm/);
});

test('timelineCreateForm returns the state the template binds and loads calendars', async () => {
  let asked = '';
  const sandbox = {
    window: { location: { pathname: '/campaigns/c1/timelines' } },
    console,
    Chronicle: {
      apiFetch(url) {
        asked = url;
        return Promise.resolve({ json: () => Promise.resolve([{ id: 'k1', name: 'Harptos' }]) });
      },
    },
  };
  vm.runInNewContext(read('static/js/timeline_create_form.js'), sandbox);
  const c = sandbox.timelineCreateForm();
  assert.equal(c.calendarId, '');
  assert.deepEqual(Array.from(c.calendars), []);
  c.init();
  await new Promise((r) => setTimeout(r, 0));
  assert.equal(asked, '/campaigns/c1/timelines/calendars');
  assert.equal(c.calendars.length, 1);
  assert.equal(c.calendars[0].name, 'Harptos');
});
