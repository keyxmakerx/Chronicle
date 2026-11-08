# UI review checklist

Run this against a mockup or a diff before calling a visible change done.
Each line points back to `.ai/ui-standard.md` or a public guideline; a
failing line is fixed, or the PR says why it is a signed exception.

## Fit with the standard

- [ ] The screen is a page or a workshop, and looks like that kind.
- [ ] No older look is left on the screen (raw Tailwind greys, a card around
      every section, emoji icons), or an issue is filed for it.
- [ ] Colours, corners, edges, depth, durations and easing are tokens; no
      fixed hex, radius or millisecond values in new CSS.
- [ ] One accent; state colours mean only state; DM-only content has the
      amber mark.
- [ ] The lightest control weight that fits; nothing opens from a bordered
      "Show all ▾" button.
- [ ] It matches the signed mockup, or the differences went back as a
      "small differences" card.

## Motion

- [ ] Each move is one of the standard's nine, at its listed timing.
- [ ] It starts from the thing that opened it and closes back the same way.
- [ ] Only `transform` and `opacity` animate; heights measure the content.
- [ ] Clicking again mid-move reverses it smoothly.
- [ ] Calm shows only fades; Off and a device set to reduce motion show the
      end state, and the feature still works.
- [ ] Loops use `MotionRest`; nothing flashes more than three times a second.

## Behaviour

- [ ] Every click or key shows a change within 100 ms; a wait over 0.3 s
      shows a spinner or placeholder lines (**Waiting and feedback**).
- [ ] Every control has rest, hover, pressed, focus, working and off.
- [ ] Panels close only on purpose; small menus close on a click off them;
      leaving unsaved changes asks first.
- [ ] Hover only peeks; nothing you act in opens on hover.
- [ ] Single-key shortcuts are silent while typing.
- [ ] Empty, loading, error and very long content all look designed: an
      empty list says what goes there and how to add one; long names wrap
      or truncate with the full name on hover.
- [ ] An action that changes a lot asks first or offers Undo.

## Access (WCAG 2.2 AA)

- [ ] Text contrast at least 4.5:1 (3:1 for large text); icons, field
      edges and focus rings at least 3:1, in light and dark.
- [ ] Everything works with the keyboard alone, in a sensible order, with
      a visible focus ring that nothing covers.
- [ ] Targets are at least 24 by 24 px (aim for 44 on touch).
- [ ] Every control has a name a screen reader can say; icon-only buttons
      have a label; state (open, selected, on) is announced.
- [ ] Colour is never the only signal; there is also a word, icon or shape.
- [ ] Usable at 200% browser zoom and at 320 px wide without sideways
      scrolling.

## Words

- [ ] Plain, short, no developer terms or IDs.
- [ ] Buttons name the action ("Publish"); the confirmation echoes it
      ("Published").
- [ ] Errors sit by what failed and say how to fix it.
- [ ] "Page editor" and "the Customizer" are used for the right things.

## Last look

Open it in light and dark, at phone width, keyboard only, and under Calm
and Off. Check for console errors.
