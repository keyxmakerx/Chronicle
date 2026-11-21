// _kind_icon_css.mjs: the site's own rules for category icons beside names,
// cut out of input.css so a browser test draws what the site draws and a
// change to the rules is seen by the tests that depend on them.

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const css = readFileSync(join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'static/css/input.css'), 'utf8');
const from = css.indexOf('/* Icons (Customize, Icons).');
const to = css.indexOf('/* Menu highlight on the page you are on.');
if (from < 0 || to < from) throw new Error('the icon rules moved in input.css');

export const KIND_ICON_CSS = css.slice(from, to);

// Stand-ins for the tokens the rules read, so colours can be compared.
export const KIND_ICON_TOKENS = ':root{--color-text-muted:rgb(100,107,120);--color-text-body:rgb(55,65,81);--color-accent:rgb(99,102,241)}';
