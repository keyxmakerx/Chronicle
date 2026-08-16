// daycard_harness.mjs — the shared boot for C-CALV4-DAYCARD's node tests.
//
// It builds a BENCH-SHAPED fixture (the .benchblock wrapper, one Block host
// with day cells that carry both key namespaces and their dayPick radios, and a
// docked Ledger zone), mounts the shipped card scaffold beside it, and runs
// internal/plugins/calendar/static/js/calendar_daycard.js in a node `vm` over
// the minimal DOM in daycard_dom.mjs.
//
// TIMERS ARE FAKE AND THAT IS THE POINT. The register's close is a real 160ms
// wait in a browser; here it is a queue the test flushes, so "under reduced
// motion the module does not wait for an animation that will never fire" is an
// assertion about a queue length rather than about a stopwatch.

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';
import { El, el, makeDocument } from './daycard_dom.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js', 'calendar_daycard.js');
export const SRC = readFileSync(jsPath, 'utf8');

// The signed shape in miniature: day 3 carries two events (one of them dm_only,
// so it has the gold rail, the GM badge and an audience chip), day 4 carries
// none, and day 5 carries one timed event. Every field here is one the Ledger
// row prints — there is nothing else in the payload to put.
export const PAYLOAD = {
  calendars: [{
    id: 'cal-1',
    slug: 'harptos',
    ledgerDocked: true,
    categories: [
      // THE LOCKED TRIPLE, SEEDED AS THE PRODUCER SEEDS IT. `pattern` is the
      // greyscale identity channel and dayCardCategories derives it from
      // blockPatternFor(slug) — the same key the Block locks its marks to — so
      // the two categories here carry DIFFERENT patterns on purpose. A fixture
      // that gave them both `p1` could not tell a working channel from a
      // channel that has quietly stopped carrying information.
      { slug: 'quest', name: 'Quest', glyph: '▲', axis: '#ef4444', pattern: 'p4' },
      { slug: 'social', name: 'Social', glyph: '◆', axis: '#3b82f6', pattern: 'p7' },
    ],
    days: [
      {
        key: 'harptos-3', ord: '3', day: 3, label: '3 Deepwinter 1523', weekday: 'Thirdday',
        year: 1523, month: 1,
        events: [
          { id: 'ev-1', title: 'Council of Wards', axis: 'var(--own-1)', pattern: 'p3', glyph: '◆' },
          {
            id: 'ev-2', title: 'Barrow scouting', axis: 'var(--own-2)', pattern: 'p1',
            glyph: '▲', gold: true, audience: 'GM only',
          },
        ],
      },
      { key: 'harptos-4', ord: '4', day: 4, label: '4 Deepwinter 1523', weekday: 'Fourthday', year: 1523, month: 1, events: [] },
      {
        key: 'harptos-5', ord: '5', day: 5, label: '5 Deepwinter 1523', weekday: 'Fifthday',
        year: 1523, month: 1,
        events: [{ id: 'ev-3', title: 'Caravan due', time: '18:00', axis: 'var(--own-4)', pattern: 'p2' }],
      },
    ],
  }],
};

function dayCell(day, key, ord) {
  const radio = el('input', {
    type: 'radio', class: 'daypick', 'data-day-pick': ord,
    name: 'dp-harptos', id: 'dp-harptos-' + ord,
  });
  const label = el('label', { class: 'dsel', for: 'dp-harptos-' + ord }, [
    el('span', { class: 'vh' }, [], 'Day ' + day),
  ]);
  const cell = el('div', {
    class: 'cell', 'data-cell': '', 'data-day': key, 'data-day-ord': ord,
  }, [radio, label, el('span', { class: 'dn' }, [], String(day))]);
  // Geometry the module measures. One cell per column, 84px tall, as the grid.
  cell.rect = { left: 100 + day * 84, top: 300, right: 184 + day * 84, bottom: 384, width: 84, height: 84 };
  return cell;
}

function ledgerRow(key, ord, id, title) {
  return el('div', {
    class: 'lrow', 'data-day': key, 'data-lday': ord, 'data-event-id': id,
  }, [el('span', { class: 'nm' }, [], title)]);
}

// buildBenchFixture returns { root, card, host, cells, ledger }.
export function buildBenchFixture(opts) {
  opts = opts || {};
  const payload = opts.payload === undefined ? PAYLOAD : opts.payload;

  const cells = {
    3: dayCell(3, 'harptos-3', '3'),
    4: dayCell(4, 'harptos-4', '4'),
    5: dayCell(5, 'harptos-5', '5'),
  };

  // THE LEDGER'S DAY PANELS, mirroring internal/widgets/calendar_block/
  // ledger.templ (calendar-v4 refinement stage 3). One per selectable day,
  // FIRST in the scroller, sticky. In a browser the sheet hides all of them and
  // the generated ladder reveals the chosen one; that is pure CSS, so in this
  // DOM they are simply all present — which is the right shape for testing the
  // module, because the module must resolve the day from the DOOR and never
  // from "the one that happens to be visible".
  //
  // THE DOOR IS GATED ON `canEdit` EXACTLY AS THE CARD'S IS, because
  // LedgerStub.CanCreate and DayCardMount.CanCreate are filled from ONE
  // predicate (bench.go). The fixture REPRODUCES the producer's gate rather
  // than simulating it, so a player fixture genuinely has no door to find.
  const canEditDoors = opts.canEdit !== false;
  function dayPanel(key, ord, date) {
    const row = el('div', { class: 'ldpr' }, [
      el('span', { class: 'ldd' }, [el('span', { class: 'ldn' }, [], date)]),
      el('span', { class: 'sp' }),
      ...(canEditDoors ? [el('button', {
        type: 'button', class: 'ldnew', 'data-dc-new': '', 'data-day': key,
      }, [], '+ New event')] : []),
    ]);
    return el('div', { class: 'ldp lday', 'data-lday': ord }, [row]);
  }

  const ledger = el('div', { class: 'ledger', 'data-zone': 'ledger' }, [
    el('div', { class: 'lrows' }, [
      dayPanel('harptos-3', '3', '3 Deepwinter, 1523'),
      dayPanel('harptos-4', '4', '4 Deepwinter, 1523'),
      dayPanel('harptos-5', '5', '5 Deepwinter, 1523'),
      ledgerRow('harptos-3', '3', 'ev-1', 'Council of Wards'),
      ledgerRow('harptos-3', '3', 'ev-2', 'Barrow scouting'),
      ledgerRow('harptos-5', '5', 'ev-3', 'Caravan due'),
    ]),
  ]);
  // THE MONTH IS `.inst`, AND IT IS HERE BECAUSE THE DOOR DEPENDS ON IT.
  // `Open in the Ledger` renders only when the Ledger is STACKED BELOW the
  // month rather than docked beside it (calv4 fix R1, item 3 — docked, the day
  // click has already selected the day in that column), and the module answers
  // that from the two rects. The fixture therefore carries the Block's real
  // shape: `.body > .inst` with the grid in it, and the Ledger as its sibling.
  //
  // `opts.stackedLedger` picks the layout. The DEFAULT is DOCKED, which is what
  // every pre-existing placement assertion in this harness was written against
  // — the Ledger's rect is byte-unchanged in that branch.
  const inst = el('div', { class: 'inst' }, [
    el('div', { class: 'grid' }, [cells[3], cells[4], cells[5]]),
  ]);
  if (opts.stackedLedger) {
    // Below 900px of BLOCK width the body is a flex column: the month ends and
    // the Ledger begins, full width.
    inst.rect = { left: 0, top: 200, right: 820, bottom: 620, width: 820, height: 420 };
    ledger.rect = { left: 0, top: 620, right: 820, bottom: 1000, width: 820, height: 380 };
  } else {
    // The docked column sits to the right of the grid, sharing its vertical
    // band — which is also the geometry the card must never cover.
    inst.rect = { left: 0, top: 200, right: 900, bottom: 800, width: 900, height: 600 };
    ledger.rect = { left: 900, top: 200, right: 1200, bottom: 800, width: 300, height: 600 };
  }

  const blockHost = el('div', { class: 'cal-block-host', 'data-cal-block': '' }, [
    el('div', { class: 'block' }, [
      el('div', { class: 'body' }, [inst, ledger]),
    ]),
  ]);

  const host = el('div', {
    class: 'benchblock', 'data-bench-block': 'primary', 'data-calendar-id': 'cal-1',
  }, [blockHost]);

  const card = el('div', {
    id: 'cal-daycard', class: 'cal-daycard', popover: 'manual',
    'data-cal-daycard': '', 'aria-labelledby': 'cal-daycard-head',
  }, [
    el('div', { class: 'dcbox', 'data-dc-box': '' }, [
      el('h2', { class: 'dc-h', id: 'cal-daycard-head', 'data-dc-head': '', 'data-day': '' }),
      el('div', { class: 'dc-rows', 'data-dc-rows': '' }),
      el('p', { class: 'dc-empty', 'data-dc-empty': '', hidden: '' }, [], 'No events on this day.'),
      el('div', { class: 'dc-f', 'data-dc-foot': '' }),
    ]),
  ]);
  card.offsetWidth = 340;
  card.offsetHeight = 24;
  card.querySelector('[data-dc-box]').offsetHeight = 0;
  card.querySelector('[data-dc-box]').scrollHeight = 176;

  // THE CARD'S RECT FOLLOWS THE PLACEMENT THE MODULE JUST WROTE
  // (DC3-DESKTOP-SHEET-OCCLUSION-R4). The card is the EDITOR's anchor — the
  // editor unfolds from where the card was ([DC-7]) — and the stub's default is
  // a static all-zero rect, which anchored every editor in this suite at the
  // viewport origin where nothing can be occluded. That is why the editor half
  // of a blocker measured at 107,604 px² in a real browser was invisible here
  // while the card half was not. Derived, never assigned, so a test cannot set
  // it to a value the placement did not produce.
  Object.defineProperty(card, 'rect', {
    configurable: true,
    get() {
      const box = this.querySelector('[data-dc-box]');
      const left = parseFloat(this.style.left) || 0;
      const top = parseFloat(this.style.top) || 0;
      const w = parseFloat(this.style.width) || this.offsetWidth || 0;
      const chrome = Math.max(0, (this.offsetHeight || 0) - ((box && box.offsetHeight) || 0));
      const h = ((box && box.scrollHeight) || 0) + chrome;
      return { left, top, right: left + w, bottom: top + h, width: w, height: h };
    },
  });

  // THE EDITOR IS RENDERED ONLY FOR A VIEWER THE PRODUCER GATED IN. `canEdit`
  // here IS bench.templ's `if data.DayCard.CanCreate` — the fixture reproduces
  // the gate rather than simulating it, so a player fixture genuinely has no
  // editor DOM to find.
  const canEdit = opts.canEdit !== false;
  const canDelete = opts.canDelete !== false;
  const canGMOnly = opts.canGMOnly !== false;
  if (canEdit) card.setAttribute('data-dc-can-edit', '');
  card.setAttribute('data-campaign-id', 'camp-1');
  if (canEdit) {
    card.querySelector('[data-dc-foot]').appendChild(
      el('button', { type: 'button', class: 'dc-door', 'data-dc-new': '' }, [], '+ New event')
    );
  }

  // THE EDITOR'S CHROME, mirroring internal/plugins/calendar/daycard.templ
  // (C-CALV4-EDITOR-R2b stage 2). THE FIXTURE REPRODUCES THE PRODUCER'S GATES
  // RATHER THAN SIMULATING THEM: `canGMOnly` and `canRestrict` decide whether
  // the DM-only and Restricted cards EXIST, exactly as `if m.CanAuthorDmOnly` /
  // `if m.CanRestrict` do in the template, so a Scribe fixture genuinely has no
  // control to find and a test cannot accidentally assert on one.
  //
  // THE WHOLE VISIBILITY FIELDSET IS GONE BELOW TWO CARDS, which is the
  // template's own `if m.CanAuthorDmOnly || m.CanRestrict` and the state
  // editor-scribe-light.png draws.
  const canRestrict = opts.canRestrict !== false;
  const visCards = [
    el('label', { class: 'viscard' }, [
      el('input', { type: 'radio', class: 'vh', name: 'de-vis', value: 'public', 'data-vis-pick': 'public', checked: '' }),
      el('span', { class: 'nm' }, [], 'Public'),
    ]),
    ...(canGMOnly ? [el('label', { class: 'viscard' }, [
      el('input', { type: 'radio', class: 'vh', name: 'de-vis', value: 'gmonly', 'data-vis-pick': 'gmonly', 'data-de-gmonly': '' }),
      el('span', { class: 'nm' }, [], '◥ DM only'),
    ])] : []),
    ...(canRestrict ? [el('label', { class: 'viscard' }, [
      el('input', { type: 'radio', class: 'vh', name: 'de-vis', value: 'restricted', 'data-vis-pick': 'restricted', 'data-de-restricted': '' }),
      el('span', { class: 'nm' }, [], '◈ Restricted'),
    ])] : []),
  ];
  // THE COLLECT-RSVPs SECTION, MIRRORING `if m.CanCollectRSVPs` IN THE TEMPLATE
  // (C-CALV4-GAMEREADY §4 [GR-6]). Like the visibility cards above, the fixture
  // REPRODUCES the producer's gate rather than simulating it: with
  // canCollectRSVPs false there is genuinely no markup, so a test cannot
  // accidentally assert on a control a Player would never receive.
  const canCollectRSVPs = opts.canCollectRSVPs !== false;
  const rsvpSection = canCollectRSVPs ? [
    el('div', { class: 'fld', 'data-de-rsvp': '' }, [
      el('label', { class: 'viscard' }, [
        el('input', { type: 'checkbox', class: 'vh', 'data-de-rsvp-toggle': '', disabled: '' }),
        el('span', { class: 'nm' }, [], "Ask the party who's coming"),
        el('span', { class: 'sub', 'data-de-rsvp-hint': '' }, [], 'Save the event first, then invite the party'),
      ]),
    ]),
  ] : [];

  const visibility = (canGMOnly || canRestrict) ? [
    el('div', { class: 'fld', 'data-de-visibility': '' }, [
      el('div', { class: 'vis', 'data-de-vis': '' }, visCards),
      ...(canRestrict ? [el('div', { class: 'audb', 'data-de-aud': '', hidden: '' }, [
        el('div', { class: 'audrows', 'data-de-audrows': '' }),
      ])] : []),
    ]),
  ] : [];

  const editor = canEdit ? el('div', {
    id: 'cal-dayeditor', class: 'cal-dayeditor', popover: 'manual',
    'data-cal-dayeditor': '', 'data-campaign-id': 'camp-1',
    ...(canRestrict ? { 'data-de-can-restrict': '' } : {}),
    'aria-labelledby': 'cal-dayeditor-head',
  }, [
    el('div', { class: 'dcbox', 'data-dc-box': '' }, [
      el('div', { class: 'ed-bar', 'data-de-bar': '' }),
      el('div', { class: 'ed-head' }, [
        el('span', { class: 'tymark', 'data-de-tymark': '' }, [
          el('i', { class: 'rail p1', 'data-de-tyrail': '' }),
          el('span', { class: 'g', 'data-de-tyglyph': '' }),
        ]),
        el('h2', { class: 'dc-h t', id: 'cal-dayeditor-head', 'data-de-head': '', 'data-day': '' }),
        el('span', { class: 'sp' }),
        el('span', { class: 'readout mono', 'data-de-id': '' }),
        el('button', { type: 'button', class: 'btn xs ghost', 'data-de-cancel': '' }, [], '✕'),
      ]),
      el('form', { class: 'de-form', 'data-de-form': '' }, [
        el('div', { class: 'ed-body' }, [
          el('div', { class: 'ed-form-col' }, [
            el('input', { type: 'text', class: 'in', 'data-de-name': '' }),
            el('div', { class: 'types', 'data-de-typerail': '' }),
            el('input', { type: 'hidden', 'data-de-category': '' }),
            el('textarea', { class: 'in prose', 'data-de-desc': '' }),
            el('span', { class: 'de-lab', 'data-de-datelab': '' }, [], 'Date'),
            el('div', { class: 'dp', 'data-de-datepicker': '' }),
            el('span', { class: 'readout', 'data-de-dateread': '' }),
            el('input', { type: 'hidden', 'data-de-year': '' }),
            el('input', { type: 'hidden', 'data-de-month': '' }),
            el('input', { type: 'hidden', 'data-de-day': '' }),
            el('input', { type: 'checkbox', 'data-de-allday': '' }),
            el('div', { class: 'frow de-time', 'data-de-timerow': '', hidden: '' }, [
              el('input', { type: 'number', class: 'in num', 'data-de-starth': '' }),
              el('input', { type: 'number', class: 'in num', 'data-de-startm': '' }),
              el('input', { type: 'number', class: 'in num', 'data-de-endh': '' }),
              el('input', { type: 'number', class: 'in num', 'data-de-endm': '' }),
            ]),
            el('button', { type: 'button', class: 'in sel-like', 'data-end-pick': '', 'data-de-endread': '' }, [], 'ends the same day'),
            el('input', { type: 'hidden', 'data-de-endyear': '' }),
            el('input', { type: 'hidden', 'data-de-endmonth': '' }),
            el('input', { type: 'hidden', 'data-de-endday': '' }),
            el('div', { class: 'fld', 'data-de-recurrence': '' }, [
              el('span', { class: 'seg' }, [
                el('button', { type: 'button', 'data-rec-pick': 'once', 'aria-pressed': 'true' }, [], 'Once'),
                el('button', { type: 'button', 'data-rec-pick': 'repeats', 'aria-pressed': 'false' }, [], 'Repeats'),
              ]),
              el('span', { class: 'rec-on', 'data-de-recon': '', hidden: '' }, [
                el('input', { type: 'number', class: 'in num', value: '1', 'data-de-recevery': '' }),
                el('span', { class: 'units', 'data-de-recunits': '' }),
              ]),
              el('span', { class: 'readout', 'data-de-recread': '' }),
              el('div', { class: 'recbox', 'data-de-recbox': '', hidden: '' }, [
                el('span', { class: 'cap' }, [
                  el('span', { class: 'badge need' }, [], 'needs backend'),
                ], 'On days of the week '),
                el('div', { class: 'wdpick', 'data-de-wdpick': '' }),
              ]),
            ]),
            ...visibility,
            ...rsvpSection,
            el('div', { class: 'fld', 'data-de-tie': '' }, [
              el('div', { class: 'frow', 'data-de-tierow': '' }),
              el('input', { type: 'search', class: 'in', 'data-de-tiesearch': '' }),
              el('div', { class: 'tieres', 'data-de-tieres': '', hidden: '' }),
              el('input', { type: 'hidden', 'data-de-entity': '' }),
            ]),
          ]),
          el('div', { class: 'ed-side' }, [el('div', { class: 'pv', 'data-de-preview': '' })]),
        ]),
        el('p', { class: 'de-err', 'data-de-err': '', hidden: '' }),
        el('div', { class: 'de-f ed-foot' }, [
          ...(canDelete ? [el('button', { type: 'button', class: 'btn danger', 'data-de-delete': '', hidden: '' }, [], 'Delete event')] : []),
          el('span', { class: 'sp' }),
          el('button', { type: 'button', class: 'btn', 'data-de-cancel': '' }, [], 'Cancel'),
          el('button', { type: 'submit', class: 'btn fill', 'data-de-save': '' }, [], 'Save changes'),
        ]),
      ]),
    ]),
  ]) : null;
  if (editor) {
    editor.offsetWidth = 760;
    editor.offsetHeight = 24;
    editor.querySelector('[data-dc-box]').offsetHeight = 0;
    editor.querySelector('[data-dc-box]').scrollHeight = 320;
    // THE EDITOR'S RECT FOLLOWS THE PLACEMENT THE MODULE JUST WROTE, exactly
    // as the card's does (DC3-DESKTOP-SHEET-OCCLUSION-R4) — and with
    // C-CALV4-EDITOR-R2b it also has to follow the MORPH'S inline width and
    // height, because the morph's start geometry is written as `style.width` /
    // `style.height` and a rect that ignored them would report a box the
    // module never produced. Derived, never assigned.
    Object.defineProperty(editor, 'rect', {
      configurable: true,
      get() {
        const box = this.querySelector('[data-dc-box]');
        const left = parseFloat(this.style.left) || 0;
        const top = parseFloat(this.style.top) || 0;
        // THE MORPH WRITES THE LOGICAL PROPERTIES, because that is what the
        // carve-out's rule names — `style.width` would leave the declared
        // transition matching nothing and the box would SNAP.
        const w = parseFloat(this.style.getPropertyValue('inline-size')) ||
          parseFloat(this.style.width) || this.offsetWidth || 0;
        const chrome = Math.max(0, (this.offsetHeight || 0) - ((box && box.offsetHeight) || 0));
        const natural = ((box && box.scrollHeight) || 0) + chrome;
        const h = parseFloat(this.style.getPropertyValue('block-size')) || natural;
        return { left, top, right: left + w, bottom: top + h, width: w, height: h };
      },
    });
  }

  // THE DATE-VERB ROW (C-CALV4-GAMEREADY §2, [GR-SIGN-A] SIGNED / [GR-4]),
  // mirroring benchDateVerbRow in internal/plugins/calendar/bench.templ.
  //
  // THE FIXTURE REPRODUCES THE PRODUCER'S TWO GATES RATHER THAN SIMULATING
  // THEM, exactly as the editor's canGMOnly / canRestrict do above: `canStep`
  // is `if v.CanStep` (CanControlWorldState) and `canSet` is `if v.CanSet` (the
  // existing Owner-only floor). A co-DM fixture therefore genuinely has no Set
  // date control to find, and a test cannot accidentally assert on one.
  //
  // opts.dateVerbs is undefined by default, so every pre-existing test in this
  // suite renders a Bench with NO verb row — which is a player's Bench, and the
  // state that must keep working unchanged.
  let verbRow = null;
  if (opts.dateVerbs) {
    const dv = opts.dateVerbs;
    const kids = [el('span', { class: 'badge' }, [], 'In-world date')];
    if (dv.canStep !== false) {
      kids.push(el('button', { type: 'button', class: 'btn xs', 'data-bench-date-step': '-1' }, [], '\u22121 day'));
      kids.push(el('button', { type: 'button', class: 'btn xs', 'data-bench-date-step': '1' }, [], '+1 day'));
    }
    if (dv.canSet) {
      kids.push(el('input', { type: 'number', 'data-bench-date-year': '', value: String(dv.year ?? 1523) }));
      kids.push(el('input', { type: 'number', 'data-bench-date-month': '', value: String(dv.month ?? 1) }));
      kids.push(el('input', { type: 'number', 'data-bench-date-day': '', value: String(dv.day ?? 14) }));
      kids.push(el('button', { type: 'button', class: 'btn xs', 'data-bench-date-set': '' }, [], 'Set date'));
    }
    kids.push(el('span', { class: 'c', 'data-bench-date-say': '', role: 'status', 'aria-live': 'polite' }));
    verbRow = el('div', {
      class: 'manage',
      'data-bench-date-verbs': '',
      'data-verb-url': dv.url || '/campaigns/camp-1/calendar/world-state?calendarId=cal-1',
      'data-verb-csrf': 'fx-csrf',
    }, kids);
  }

  const rootAttrs = { class: 'cal-bench', 'data-cal-bench': '', 'data-cal-dashboard': '' };
  if (payload !== null) rootAttrs['data-cal-daycard-payload'] = JSON.stringify(payload);
  const root = el('div', rootAttrs, [
    el('div', { class: 'bsurf', 'data-bench-surface': '' }, [
      el('div', { class: 'stack', 'data-bench-stack': '' }, verbRow ? [verbRow, host] : [host]),
    ]),
    card,
    ...(editor ? [editor] : []),
  ]);

  return { root, card, editor, host, cells, ledger, blockHost, verbRow };
}

// boot runs the module against a fresh fixture.
//
// opts.reduced → matchMedia reports prefers-reduced-motion, which is the branch
// the register calls INSTANT AND COMPLETE (never a shortened animation).
export function boot(opts) {
  opts = opts || {};
  const fx = buildBenchFixture(opts);
  const document = makeDocument(fx.root);

  const timers = [];
  let nextTimer = 1;

  const winListeners = {};
  const sandbox = {
    // opts.console lets a test capture the module's ONE per-session occlusion
    // warning (DC-CLEAR-1). Real console otherwise, so nothing else goes quiet.
    console: opts.console || console,
    document,
    setTimeout: (fn, ms) => { const id = nextTimer++; timers.push({ id, fn, ms }); return id; },
    clearTimeout: (id) => {
      const i = timers.findIndex((t) => t.id === id);
      if (i >= 0) timers.splice(i, 1);
    },
  };
  sandbox.window = sandbox;
  sandbox.innerWidth = opts.viewportW || 1232;
  sandbox.innerHeight = opts.viewportH || 900;
  sandbox.matchMedia = (q) => ({
    matches: !!opts.reduced && /prefers-reduced-motion:\s*reduce/.test(q),
  });
  sandbox.getComputedStyle = () => ({
    getPropertyValue: (n) => (n === '--disc-close' ? '160ms' : n === '--disc-open' ? '200ms' : ''),
  });
  sandbox.addEventListener = (t, fn) => { (winListeners[t] = winListeners[t] || []).push(fn); };
  sandbox.removeEventListener = () => {};

  // THE WRITE SPY. Every mutating call the editor makes goes through
  // Chronicle.apiFetch — which is what attaches X-CSRF-Token and
  // credentials: same-origin in the browser — so recording it here records
  // exactly what would go on the wire.
  const calls = [];
  let reloads = 0;
  sandbox.location = { pathname: '/campaigns/camp-1/apps/calendar', reload: () => { reloads++; } };
  sandbox.Chronicle = {
    apiFetch: (url, o) => {
      const call = { url, method: (o && o.method) || 'GET', body: o && o.body };
      calls.push(call);
      const canned = (opts.responses || {})[call.method + ' ' + url] || (opts.responses || {})[call.method];
      return Promise.resolve(canned || { ok: true, json: () => Promise.resolve({}) });
    },
  };

  // The shared visibility mapper, loaded exactly as bench.templ mounts it:
  // BEFORE its consumer, in document order.
  const sharedPath = join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js', 'cal_visibility.js');

  vm.createContext(sandbox);
  vm.runInContext(readFileSync(sharedPath, 'utf8'), sandbox, { filename: 'cal_visibility.js' });
  vm.runInContext(SRC, sandbox, { filename: 'calendar_daycard.js' });

  const api = {
    ...fx,
    document,
    timers,
    pure: sandbox.window.__calDayCard,
    calls,
    reloads: () => reloads,
    fire: (type, target, extra) => document._fire(type, { target, preventDefault() {}, ...(extra || {}) }),
    // fireOn dispatches to the ELEMENT'S OWN listeners, which `fire` cannot
    // reach: `fire` drives the module's DELEGATED document handler, and a
    // handler bound with el.addEventListener (the all-day switch, the RSVP
    // opt-in) is never registered there. Two dispatch shapes because the module
    // genuinely uses two, and a test that could only exercise one would silently
    // stop covering whichever control moved to the other.
    fireOn: (type, target, extra) => {
      const handlers = (target && target._on && target._on[type]) || [];
      handlers.forEach((fn) => fn({ target, preventDefault() {}, ...(extra || {}) }));
    },
    flush: () => { const q = timers.splice(0, timers.length); q.forEach((t) => t.fn()); },
    winFire: (type) => (winListeners[type] || []).forEach((fn) => fn({})),
  };
  return api;
}

export { El, el };
