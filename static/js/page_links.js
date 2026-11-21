/**
 * page_links.js — a link inside a campaign page changes the page the way the
 * sidebar does: the menu stays and the content swaps in place.
 *
 * Only the menus carry hx-boost, so every ordinary link in the page area was a
 * full document load. Boosting the page area itself would be wrong twice over:
 * the boost inherits into every form in it (a POST form would be swapped into
 * #main-content), and links a widget draws after load (connections, mentions)
 * are never seen by htmx. So one listener on window watches plain left clicks
 * instead, and for an eligible link hands the navigation to a short-lived
 * boosted proxy anchor. That is the same request, history entry, scroll reset,
 * unsaved-changes hold and build-mismatch reload as a sidebar link, because
 * boot.js keys all of them on a boosted GET into #main-content.
 *
 * It runs on window (after every document-level handler) and stands down when
 * something already called preventDefault, so annals_pull.js and any widget
 * that owns its links keep winning.
 */
(function () {
  'use strict';

  // Routes that answer with a file or a bare view rather than the app layout.
  var NOT_A_PAGE = /\/(export|download|print|feed|ics)(\/|\.|$)/i;

  function campaignOf(pathname) {
    var m = /^\/campaigns\/([^/]+)(?:\/|$)/.exec(pathname || '');
    return m ? m[1] : '';
  }

  // Whether following `o.href` from the page at `o.here` should be an
  // in-place page change. o: { href, here (URL string), target, download }.
  function eligible(o) {
    var here, to;
    try {
      here = new URL(o.here);
      to = new URL(o.href, here);
    } catch (e) { return false; }
    if (to.origin !== here.origin) return false;
    if (o.download) return false;
    if (o.target && o.target !== '_self') return false;
    // Only inside the campaign whose menu is on screen: another campaign,
    // /campaigns, /admin and the rest draw a different frame.
    var id = campaignOf(here.pathname);
    if (!id || campaignOf(to.pathname) !== id) return false;
    if (NOT_A_PAGE.test(to.pathname)) return false;
    // A place on this same page is the browser's to scroll to.
    if (to.hash && to.pathname === here.pathname && to.search === here.search) return false;
    return true;
  }

  var api = { eligible: eligible, campaignOf: campaignOf };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (typeof document === 'undefined') return;

  function claimedByAnnals(link) {
    var a = window.ChronicleAnnals;
    return !!(a && a.claims && a.claims(link));
  }

  window.addEventListener('click', function (e) {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    if (!window.htmx || typeof window.htmx.process !== 'function') return;
    var t = e.target;
    var link = t && t.closest ? t.closest('a[href]') : null;
    var main = document.getElementById('main-content');
    if (!link || !main || !main.contains(link) || main.hasAttribute('data-beside-doc')) return;
    // A link that does its own thing: an attribute htmx or a script acts on,
    // text being edited, or an explicit opt-out.
    if (link.closest('[hx-boost="false"], [contenteditable="true"]')) return;
    if (link.hasAttribute('onclick') || link.hasAttribute('hx-get') || link.hasAttribute('hx-boost') ||
        link.getAttribute('role') === 'button' || link.hasAttribute('data-no-boost')) return;
    if (!eligible({
      href: link.getAttribute('href'),
      here: location.href,
      target: link.getAttribute('target'),
      download: link.hasAttribute('download')
    })) return;
    if (claimedByAnnals(link)) return;

    e.preventDefault();
    var proxy = document.createElement('a');
    proxy.href = link.href;
    proxy.hidden = true;
    proxy.setAttribute('hx-boost', 'true');
    proxy.setAttribute('hx-target', '#main-content');
    proxy.setAttribute('hx-select', '#main-content > *');
    proxy.setAttribute('hx-swap', 'innerHTML show:window:top');
    document.body.appendChild(proxy);
    window.htmx.process(proxy);
    proxy.click();
    // The request holds the proxy until it settles; sweep it after.
    setTimeout(function () { if (proxy.parentNode) proxy.parentNode.removeChild(proxy); }, 15000);
  });
})();
