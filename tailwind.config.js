/** @type {import('tailwindcss').Config} */

// Tailwind CSS Configuration for Chronicle. Uses the standalone Tailwind CLI
// (no Node.js). Content paths point to Templ files and Go template strings.
//
// Semantic color tokens (auto-switch light/dark via CSS custom properties):
//   text-fg, text-fg-body, text-fg-secondary, text-fg-muted, text-fg-faint
//   bg-surface, bg-surface-alt, bg-surface-raised, bg-page
//   border-edge, border-edge-light

module.exports = {
  // Toggle dark mode by adding/removing the "dark" class on <html>.
  darkMode: 'class',
  content: [
    // Templ template files (primary source of Tailwind classes)
    "./internal/**/*.templ",

    // Go files that might contain template strings with Tailwind classes
    "./internal/**/*.go",

    // Static JS files that might set classes dynamically
    "./static/js/**/*.js",
  ],
  theme: {
    extend: {
      // Chronicle brand colors (Kanka-inspired dark sidebar, light content)
      colors: {
        // Sidebar dark theme
        sidebar: {
          // A variable so the campaign's menu colour (Customize) reaches bg-sidebar-bg.
          bg: 'var(--color-sidebar-bg, #1a1c23)',
          hover: '#2d2f3a',
          text: '#9ca3af',
          active: '#e5e7eb',
        },
        // Accent color for links, buttons, active states. References a CSS
        // custom property so per-campaign overrides work.
        accent: {
          DEFAULT: 'rgb(var(--color-accent-rgb, 99 102 241) / <alpha-value>)',
          hover: 'rgb(var(--color-accent-hover-rgb, 79 70 229) / <alpha-value>)',
          light: 'rgb(var(--color-accent-light-rgb, 165 180 252) / <alpha-value>)',
          // A fill words can sit on: the accent, nudged or paired with dark
          // words when it is too light for white (set by Customize).
          fill: 'var(--color-accent-fill, rgb(var(--color-accent-rgb, 99 102 241)))',
          'fill-hover': 'var(--color-accent-fill-hover, rgb(var(--color-accent-hover-rgb, 79 70 229)))',
        },
        // Words on an accent fill: white, or dark ink on a light accent.
        'on-accent': 'var(--color-accent-on-fill, #fff)',

        // Two more semantic accent slots, each falling back through the
        // slot(s) it migrates from so a campaign that customized an earlier
        // slot keeps seeing that color until it sets the new one, and an
        // uncustomized campaign renders the same indigo default as `accent`.

        // Action highlight: primary buttons, hover/press, FABs. Falls back
        // straight to the site accent (no prior analog).
        action: {
          DEFAULT: 'rgb(var(--color-accent-action-rgb, var(--color-accent-rgb, 99 102 241)) / <alpha-value>)',
          hover: 'rgb(var(--color-accent-action-hover-rgb, var(--color-accent-hover-rgb, 79 70 229)) / <alpha-value>)',
          light: 'rgb(var(--color-accent-action-light-rgb, var(--color-accent-light-rgb, 165 180 252)) / <alpha-value>)',
        },
        // App accent: per-app identity — character pages, calendar app,
        // other apps. Falls back through the legacy surface-pair primary,
        // then the site accent.
        app: {
          DEFAULT: 'rgb(var(--color-accent-app-rgb, var(--color-accent-surface-1-rgb, var(--color-accent-rgb, 99 102 241))) / <alpha-value>)',
          hover: 'rgb(var(--color-accent-app-hover-rgb, var(--color-accent-surface-1-hover-rgb, var(--color-accent-hover-rgb, 79 70 229))) / <alpha-value>)',
          light: 'rgb(var(--color-accent-app-light-rgb, var(--color-accent-surface-1-light-rgb, var(--color-accent-light-rgb, 165 180 252))) / <alpha-value>)',
        },

        // ── Semantic theme tokens ──────────────────────────────
        // These reference CSS custom properties that flip for dark mode.
        // Usage: text-fg, bg-surface, border-edge, etc.
        // No `dark:` prefix needed — colours auto-switch.

        // Foreground / text
        fg: {
          DEFAULT: 'var(--color-text-primary)',       // headings, main text
          body:    'var(--color-text-body)',           // body text, values
          secondary: 'var(--color-text-secondary)',   // labels, descriptions
          muted:   'var(--color-text-muted)',          // hints, timestamps
          faint:   'var(--color-text-faint)',          // disabled, placeholders
        },

        // Background surfaces
        surface: {
          DEFAULT: 'var(--color-card-bg)',             // card / panel bg
          alt:     'var(--color-bg-tertiary)',          // alt rows, hover bg
          raised:  'var(--color-bg-secondary)',         // palette items, blocks (slightly elevated)
        },
        page:      'var(--color-bg-primary)',           // main page bg

        // Borders and dividers
        edge: {
          DEFAULT: 'var(--color-border)',               // standard borders
          light:   'var(--color-border-light)',          // subtle dividers
        },

        // State: yes / maybe / no. Take an opacity modifier (bg-ok/10).
        ok:   'rgb(var(--color-ok-rgb, 22 163 74) / <alpha-value>)',
        warn: 'rgb(var(--color-warn-rgb, 217 119 6) / <alpha-value>)',
        bad:  'rgb(var(--color-bad-rgb, 220 38 38) / <alpha-value>)',
      },
      // Use Inter as the default font
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
      },

      // ── Motion vocabulary ───────────────────────────────────────────
      // The CSS custom properties are the source of truth (see the
      // `--ease-*` / `--dur-*` / `--elev-*` declarations in
      // static/css/input.css `:root`); these just publish them as utility
      // classes. `ease-out` intentionally overrides Tailwind's default
      // bezier — every surface using the utility gets the same easing.
      // The rest (`ease-standard`, `duration-*`, `shadow-elev-*`) are new
      // names with no Tailwind collision.
      transitionTimingFunction: {
        out: 'var(--ease-out)',
        in: 'var(--ease-in)',
        'in-out': 'var(--ease-in-out)',
        standard: 'var(--ease-standard)',
      },
      transitionDuration: {
        instant: '80ms',
        micro: '120ms',
        standard: '200ms',
        large: '280ms',
      },
      boxShadow: {
        'elev-static':  'var(--elev-static)',
        'elev-resting': 'var(--elev-resting)',
        'elev-hover':   'var(--elev-hover)',
        'elev-dragged': 'var(--elev-dragged)',
      },
    },
  },
  // Safelist grid column spans used by the dynamic entity page layout renderer.
  // These classes are generated programmatically from layout_json column widths,
  // so Tailwind's JIT scanner can't detect them in source files.
  // `htmx-added` is added by HTMX at runtime to newly-inserted DOM fragments;
  // it never appears in templ/go/js sources but the input.css @starting-style
  // rule keying off it must survive the JIT purge.
  safelist: [
    'col-span-1', 'col-span-2', 'col-span-3', 'col-span-4',
    'col-span-5', 'col-span-6', 'col-span-7', 'col-span-8',
    'col-span-9', 'col-span-10', 'col-span-11', 'col-span-12',
    'grid-cols-12',
    'htmx-added',
  ],
  plugins: [
    require('@tailwindcss/typography'),  // For prose styling (rich text editor)
    require('@tailwindcss/forms'),       // For cleaner form element defaults
  ],
}
