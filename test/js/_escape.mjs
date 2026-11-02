// Chronicle.escapeHtml as boot.js defines it, for tests that load a widget in
// a bare vm context with a stubbed Chronicle.
export function escapeHtml(str) {
  return String(str == null || str === false ? '' : str)
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}
