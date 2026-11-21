# topbar_tools.js -- the Tools tab and shelf on a phone's top bar

Mounts on `data-widget="topbar-tools"`, drawn by `topbarToolsTab()`
(`internal/templates/layouts/topbar_tools.templ`) inside `Topbar()` only when
`topbarShowsToolsShelf` holds: two or more of the AI, the Customize brush,
the Table, the DM Screen, the eye and the bell. The brush's entry is drawn
whenever the owner may have the brush and hides while it is put away
(`chronicle:brush` refreshes the shelf). Who has which tool is decided once, in
`internal/templates/layouts/topbar_tools.go`, and both the bar's buttons and
the shelf's entries read it, so the shelf can never list a tool the bar
would not draw. A member's AI starts hidden until its plugin allows it, so it
never earns the shelf alone; its entry shows when its host does.

Under 768 wide (`phone_shell.css`, same breakpoint as the menu button) the
header's `data-tb-tools` mark pulls each tool's host out of the row and hides
only its real button (`visibility`, never `display:none` on a host: the AI
bubble, the bell's list and the Table hang inside them). An entry presses the
real button a tick after the tap, so the tool opens as it always did, and
counts are mirrored from the real badges with a MutationObserver. The tab
carries the sum of the tools it hides.

"Choose what stays on the bar" keeps up to two tools (`chronicle-topbar-kept`
in localStorage, per browser, try/catch); a kept host gets `.tb-kept` and its
real button returns. A tool's button that is not kept hands focus to the
tab only on a phone; on a wider bar it takes its own focus. Closes on a tap outside, Escape (focus to the tab), a
boosted navigation, and a window wider than a phone. Motion is the Drape,
transform and opacity only; Calm fades, Off just shows.

Tests: `test/js/topbar_tools.test.mjs` (the pure rules),
`internal/templates/layouts/topbar_tools_test.go` (who sees which entry),
`test/live/topbar_tools_shelf.spec.mjs` (a phone, a player, a computer).
