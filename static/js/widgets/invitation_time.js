// invitation_time.js shows a game night's start on the invitation card in the
// reader's own time zone. Chronicle.register('invitation-time', ...); the
// mount is the <time> element, and the server's wording (the table's zone)
// stays when this can't run.
(function () {
  'use strict';
  Chronicle.register('invitation-time', {
    init: function (el) {
      var d = new Date(el.getAttribute('datetime'));
      if (isNaN(d.getTime())) return;
      try {
        el.textContent = d.toLocaleString(undefined, {
          weekday: 'long', month: 'long', day: 'numeric', hour: 'numeric', minute: '2-digit',
        });
      } catch (e) { /* keep the server's wording */ }
    },
    destroy: function () {},
  });
})();
