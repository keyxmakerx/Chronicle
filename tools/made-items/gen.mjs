// node gen.mjs : prints the item tables for made_items.js and internal/madeitems
// (gen-js.txt, gen-go.txt) from items/demo.js, in three groups split by ---:
// the signed set, the ten added later, and the eight still waiting.
import { DEMO } from './src/items/demo.js';
import fs from 'fs';
const order = JSON.parse(fs.readFileSync('order.json')), G = JSON.parse(fs.readFileSync('greys.json'));
const MOV = 'bomb candle censer conch core egg fang geode hand heart horn ink knot lantern orb pie potion runestone scale skull starfrag stew tentacle tongs torch unicorn wing wisp'.split(' ');
const MORE = 'meat hand geode unicorn starfrag ink conch mandrake egg wing'.split(' ');
const WAIT = 'potion ore bolt herbs boots torch bomb gems'.split(' ');
const groups = [order.filter(k => !MORE.includes(k) && !WAIT.includes(k)), MORE, WAIT];
const row = k => { const d = DEMO[k]; return `    ${k}: [${JSON.stringify(d.name)}, '${d.body}', '${d.trim}', '${d.detail}', ${G[k].join(', ')}, ${MOV.includes(k) ? 1 : 0}]`; };
const go = k => { const d = DEMO[k]; return `\t{"${k}", "${d.body}", "${d.trim}", "${d.detail}"},`; };
fs.writeFileSync('gen-js.txt', groups.map(g => g.map(row).join(',\n')).join('\n---\n'));
fs.writeFileSync('gen-go.txt', groups.map(g => g.map(go).join('\n')).join('\n---\n'));
