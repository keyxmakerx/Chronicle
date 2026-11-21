/**
 * Night "Real date" card
 *
 * Sets a date-less night's real date from the card on its page (the
 * set-real-date card in sessions.templ). Sends only scheduled_date to the
 * night's own URL: the update is partial, so every other field keeps its
 * stored value. Mounted by data-widget="night-date" and loaded on sight, so it
 * works after a boosted swap, where inline script tags are stripped. Enter in
 * the date field submits, like the button; an empty field asks for a date.
 */
(function () {
  'use strict';

  function fail(btn) {
    btn.disabled = false;
    Chronicle.notify('Could not set the date', 'error');
  }

  function submit(root, input, btn) {
    if (btn.disabled) return;
    // An empty press says why nothing happened instead of doing nothing.
    if (!input.value) {
      Chronicle.notify('Pick a date first', 'error');
      input.focus();
      return;
    }
    btn.disabled = true;
    Chronicle.apiFetch(root.getAttribute('data-url'), { method: 'PUT', body: { scheduled_date: input.value } })
      .then(function (res) {
        if (res.ok) location.reload();
        else fail(btn);
      })
      .catch(function () { fail(btn); });
  }

  Chronicle.register('night-date', {
    init: function (root) {
      var input = root.querySelector('#night-real-date');
      var btn = root.querySelector('#night-real-date-btn');
      if (!input || !btn) return;
      root._nightDateClick = function () { submit(root, input, btn); };
      root._nightDateKey = function (e) {
        if (e.key !== 'Enter') return;
        e.preventDefault();
        submit(root, input, btn);
      };
      btn.addEventListener('click', root._nightDateClick);
      input.addEventListener('keydown', root._nightDateKey);
    },
    destroy: function (root) {
      var input = root.querySelector('#night-real-date');
      var btn = root.querySelector('#night-real-date-btn');
      if (btn && root._nightDateClick) btn.removeEventListener('click', root._nightDateClick);
      if (input && root._nightDateKey) input.removeEventListener('keydown', root._nightDateKey);
      root._nightDateClick = null;
      root._nightDateKey = null;
    }
  });
})();
