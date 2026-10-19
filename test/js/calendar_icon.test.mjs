// calendar_icon.test.mjs — an event or kind icon renders as an icon.
//
// Icons are stored as a short glyph (an emoji) or as a Font Awesome class
// name, which imports and the AI page write. A class name must become an
// <i>, never print as the words "fa-scroll"; anything else stays escaped
// text, and the plain dot is drawn so it sits in the centre of its ring.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

const src = readFileSync(new URL('../../static/js/widgets/calendar_view.js', import.meta.url), 'utf8');
const sandbox = { module: { exports: {} }, Chronicle: { register: function () {} }, console };
vm.createContext(sandbox);
vm.runInContext(src, sandbox);
const iconHTML = sandbox.Chronicle.calendarIcon;

const cases = [
  ['a bare class name', 'fa-scroll', '<i class="fa-solid fa-scroll" aria-hidden="true"></i>'],
  ['a class with its style prefix', 'fa-solid fa-dragon', '<i class="fa-solid fa-dragon" aria-hidden="true"></i>'],
  ['the short prefix', 'fas fa-skull', '<i class="fa-solid fa-skull" aria-hidden="true"></i>'],
  ['an emoji', '⭐', '⭐'],
  ['the plain dot', '●', '<span class="edot" aria-hidden="true"></span>'],
  ['no icon', '', ''],
  ['markup', '<b>x</b>', '&lt;b&gt;x&lt;/b&gt;'],
  ['a class name with something injected', 'fa-x" onclick="y', 'fa-x&quot; onclick=&quot;y'],
];
for (const [label, icon, want] of cases) {
  test(`calendar icon: ${label}`, () => assert.equal(iconHTML(icon), want));
}
