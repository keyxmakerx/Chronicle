/** @type {import('tailwindcss').Config} */

// ============================================================================
// Tailwind CSS Configuration for Chronicle
// ============================================================================
// Uses the standalone Tailwind CSS CLI (no Node.js).
// Content paths point to Templ files and Go template strings.
// ============================================================================

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
          bg: '#1a1c23',
          hover: '#2d2f3a',
          text: '#9ca3af',
          active: '#e5e7eb',
        },
        // Accent color for links, buttons, active states
        accent: {
          DEFAULT: '#6366f1',  // Indigo-500
          hover: '#4f46e5',    // Indigo-600
          light: '#a5b4fc',    // Indigo-300
        },
      },
      // Use Inter as the default font
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
      },
    },
  },
  // Safelist grid column spans used by the dynamic entity page layout renderer.
  // These classes are generated programmatically from layout_json column widths,
  // so Tailwind's JIT scanner can't detect them in source files.
  safelist: [
    'col-span-1', 'col-span-2', 'col-span-3', 'col-span-4',
    'col-span-5', 'col-span-6', 'col-span-7', 'col-span-8',
    'col-span-9', 'col-span-10', 'col-span-11', 'col-span-12',
    'grid-cols-12',
  ],
  plugins: [
    require('@tailwindcss/typography'),  // For prose styling (rich text editor)
    require('@tailwindcss/forms'),       // For cleaner form element defaults
  ],
}
