// node greys.js : each item's neutral recipe, one grey per role as light as the
// colour the item was designed in, so recolouring stays near the look it was lit for.
import('./src/items/demo.js').then(({ DEMO }) => {
  const order = require('./order.json'), out = {};
  const luma = h => { const n = parseInt(h.slice(1), 16); return .299 * (n >> 16) + .587 * ((n >> 8) & 255) + .114 * (n & 255); };
  for (const k of order) { const d = DEMO[k]; out[k] = ['body', 'trim', 'detail'].map(r => Math.round(Math.max(90, Math.min(200, luma(d[r] || '#808080'))))); }
  require('fs').writeFileSync('greys.json', JSON.stringify(out));
  console.log(Object.keys(out).length);
});
