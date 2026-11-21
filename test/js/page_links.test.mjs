// page_links.js: which links inside a campaign page change the page in place.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'page_links.js'), 'utf8');
const sandbox = { module: { exports: {} }, URL };
vm.runInNewContext(src, sandbox);
const { eligible, campaignOf } = sandbox.module.exports;

const C = 'c0ffee00-0000-4000-8000-000000000001';
const HERE = `https://chronicle.test/campaigns/${C}/entities/p1`;

test('campaignOf reads the campaign a path belongs to', () => {
  const cases = [
    [`/campaigns/${C}`, C],
    [`/campaigns/${C}/entities`, C],
    ['/campaigns', ''],
    ['/campaigns/new', 'new'],
    ['/admin/users', ''],
    ['/', ''],
  ];
  for (const [p, want] of cases) assert.equal(campaignOf(p), want, p);
});

test('eligible: only same-campaign links that are plain pages', () => {
  const cases = [
    ['a page in this campaign', { href: `/campaigns/${C}/entities/p2` }, true],
    ['an absolute address on this site', { href: `https://chronicle.test/campaigns/${C}/sessions` }, true],
    ['the campaign home', { href: `/campaigns/${C}` }, true],
    ['a page with a query', { href: `/campaigns/${C}/entities?type=3` }, true],
    ['a place on another page', { href: `/campaigns/${C}/entities/p2#history` }, true],
    ['target _self', { href: `/campaigns/${C}/entities/p2`, target: '_self' }, true],
    ['another campaign', { href: '/campaigns/other-id/entities' }, false],
    ['the campaigns list', { href: '/campaigns' }, false],
    ['site admin', { href: '/admin' }, false],
    ['discover', { href: '/' }, false],
    ['another site', { href: 'https://example.com/campaigns/' + C + '/x' }, false],
    ['a new tab', { href: `/campaigns/${C}/entities/p2`, target: '_blank' }, false],
    ['a download', { href: `/campaigns/${C}/entities/p2`, download: true }, false],
    ['a place on this same page', { href: '#history' }, false],
    ['this page with a place', { href: `${HERE}#history` }, false],
    ['an export', { href: `/campaigns/${C}/export` }, false],
    ['a calendar feed', { href: `/campaigns/${C}/calendar/feed.ics` }, false],
    ['a printable view', { href: `/campaigns/${C}/entities/p2/print` }, false],
    ['a mailto link', { href: 'mailto:a@b.c' }, false],
  ];
  for (const [name, o, want] of cases) {
    assert.equal(eligible({ here: HERE, ...o }), want, name);
  }
});

test('eligible: a page outside any campaign boosts nothing', () => {
  assert.equal(eligible({ here: 'https://chronicle.test/campaigns', href: `/campaigns/${C}/entities` }), false);
  assert.equal(eligible({ here: 'https://chronicle.test/admin', href: '/admin/users' }), false);
});
