// invitation_time.js shows a game night's start in the reader's own time
// zone on the invitation card. The server's wording (the table's zone) stays
// when this can't run. The sign-in doors swap their panel in place, so it
// runs again after each swap.
(function () {
  'use strict';
  function localize() {
    document.querySelectorAll('time[data-local-night]:not([data-localized])').forEach(function (el) {
      el.setAttribute('data-localized', '');
      var d = new Date(el.getAttribute('datetime'));
      if (isNaN(d.getTime())) return;
      try {
        el.textContent = d.toLocaleString(undefined, {
          weekday: 'long', month: 'long', day: 'numeric', hour: 'numeric', minute: '2-digit',
        });
      } catch (e) { /* keep the server's wording */ }
    });
  }
  localize();
  document.addEventListener('htmx:afterSettle', localize);
})();
