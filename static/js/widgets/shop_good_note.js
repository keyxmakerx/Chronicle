/**
 * shop_good_note.js -- the line under a shop good's name.
 *
 * A shop good is a page that exists to be sold: it stays out of Items, page
 * lists, search, Auto-link and sync. Everyone who can open it is told; only
 * the Director gets "Make it a full page", which clears the mark.
 *
 *   <div data-widget="shop_good_note" data-endpoint="/campaigns/:id/entities/:eid/full-page"
 *        data-director="true"></div>
 */
Chronicle.register('shop_good_note', {
  init: function (el) {
    var endpoint = el.dataset.endpoint;
    var director = el.dataset.director === 'true';
    el.className = 'shop-good-note';

    function say(text) {
      while (el.firstChild) el.removeChild(el.firstChild);
      el.appendChild(document.createTextNode(text));
    }

    var tag = document.createElement('span');
    tag.className = 'shop-good-tag';
    tag.textContent = 'Shop good';
    el.appendChild(tag);
    el.appendChild(document.createTextNode(' · sold in a shop and kept out of lists and search. '));
    if (!director) return;

    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'shop-inv-link';
    btn.textContent = 'Make it a full page';
    btn.onclick = function () {
      btn.disabled = true;
      Chronicle.apiFetch(endpoint, { method: 'POST' })
        .then(function (res) { if (!res.ok) throw new Error('promote'); return res.json(); })
        .then(function () {
          say('Now a full page. It shows in lists and search.');
          el.setAttribute('role', 'status');
        })
        .catch(function () {
          btn.disabled = false;
          Chronicle.notify('Could not make this a full page. Try again.', 'error');
        });
    };
    el.appendChild(btn);
  },
  destroy: function (el) { el.innerHTML = ''; }
});
