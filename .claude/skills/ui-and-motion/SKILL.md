---
name: ui-and-motion
description: Designing, building or reviewing any visible Chronicle change, including animation and motion. Use before writing a mockup, a template, CSS or UI JavaScript, and before reviewing a UI diff.
---

# UI and motion in Chronicle

Chronicle already has one written UI standard, `.ai/ui-standard.md`, approved
by the operator. **It is the rule book; this skill is how to use it.** Where
the two seem to differ, the standard wins and this file is wrong.

## Before you design anything

1. Read the sections of `.ai/ui-standard.md` your change touches. Always read
   **How it should feel**, **Two kinds of screen** and **Designing a change**;
   add **Motion** and **Every interaction** for anything that moves.
2. Decide which kind of screen it is: a **page** (read, quiet, no boxes
   around prose) or a **workshop** (a tool, framed). It changes every answer.
3. If the screen still wears an older look (raw Tailwind greys, a card around
   every section), bring the whole screen to the Material look in the same
   work, or file the issue (**Older looks**).
4. A visible change needs a mockup the operator signs first, on Sign-offs,
   one mockup per option, with at least one genuinely new option, researched
   beyond TTRPG tools. Motion choices are shown as playable clips.

## Quick rules (the ones most often broken)

- Colour, corners, edges, depth, durations and easing come from tokens
  (`static/css/input.css`, the style kit tokens), never fixed values. Light
  and dark both work.
- One accent, the campaign's colour, marks what you can act on and where you
  are. Green, amber and red mean state only; amber is also the DM-only mark.
- Controls come in four weights and the lightest that fits wins: a quiet
  word, a tab, an icon, a button. A button is never the default way to
  reveal more; no bordered "Show all ▾" buttons.
- Single-key shortcuts never fire while focus is in an input, textarea or
  editable element.
- Say "page editor" for writing a page's words and "the Customizer" for
  arranging widgets. Never call both "editor".
- Every control has rest, hover, pressed, focus, working and off states.
- Words are plain and short; a button says what happens, an error says how
  to fix it, nothing apologises.
- The paper look is for players' special pages; don't spread it.

## Motion in one screen

The standard names nine moves (Peek, Grow, Fold, Drape, Slide-out, Pull,
Page turn, Settle, Glow) and times every interaction. Pick one of those; a
new kind of move needs a signed mockup. Then:

- **Every move has a physical reason**: it comes from where the thing lives
  and goes back the same way. Nothing appears and then moves.
- **Slow to a stop**: `--ease-out` or `--ease-slide` for arrivals, `--ease-in`
  for exits. No bounce or overshoot (Site admin's entrance is the signed
  exception).
- **Short**: most moves are 120 to 280 ms; nothing over half a second unless
  the standard names it. Closing is quicker than opening.
- **Animate `transform` and `opacity`**; never `transition: all`
  (`tools/check-v2-motion-discipline.sh` fails it in several plugins). Height
  changes measure the content, never a `max-height` guess.
- **Three motion levels**: full, Calm (fades only) and Off (end state). Read
  `html[data-motion]`; a device asking for reduced motion is always Off.
  CSS built from opacity plus transform gets Calm for free; script-driven
  motion checks the attribute itself. Loops take their clock from
  `window.MotionRest`.
- **Feedback within 100 ms** of every click or key, even when the move is
  longer.

The craft behind these, with sources, is in
[references/motion-craft.md](references/motion-craft.md).

## Reviewing a UI change

Run the checklist in [references/ux-review.md](references/ux-review.md)
against the diff or the mockup. Look at it in light and dark, at phone width,
with the keyboard only, and under Calm and Off before calling it done.
