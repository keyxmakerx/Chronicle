// node jobs.js <kind> <out.json> items... : render jobs with each item's neutral greys.
const fs = require('fs'), G = JSON.parse(fs.readFileSync(__dirname + '/greys.json')), [kind, out, ...items] = process.argv.slice(2), jobs = [];
const hex = v => '#' + v.toString(16).padStart(2, '0').repeat(3);
const neu = k => ({ body: hex(G[k][0]), trim: hex(G[k][1]), detail: hex(G[k][2]) });
for (const k of items) {
  if (kind === 'test') { jobs.push({ item: k, out: 'out/t/' + k + '.png', opts: { size: 1024, samples: 12 } }); }
  if (kind === 'test' || kind === 'still') {
    const d = kind === 'test' ? 'out/t/' : 'out/still/';
    jobs.push({ item: k, out: d + k + '-n.png', recipe: neu(k), opts: { size: 1024, samples: 16 } });
    jobs.push({ item: k, out: d + k + '-m.png', recipe: neu(k), opts: { size: 1024, samples: 4, mask: true } });
  }
  if (kind === 'shelf') {
    jobs.push({ item: k, out: 'out/shelf/' + k + '-n.png', recipe: neu(k), opts: { size: 512, samples: 12, cut: true } });
    jobs.push({ item: k, out: 'out/shelf/' + k + '-m.png', recipe: neu(k), opts: { size: 512, samples: 4, cut: true, mask: true } });
  }
  if (kind === 'clip') for (let f = 0; f < 64; f++) {
    const n = String(f).padStart(2, '0');
    jobs.push({ item: k, out: 'out/clip/' + k + '-' + n + '-n.png', recipe: neu(k), opts: { size: 512, samples: 5, t: f / 64 } });
    jobs.push({ item: k, out: 'out/clip/' + k + '-' + n + '-m.png', recipe: neu(k), opts: { size: 256, samples: 2, t: f / 64, mask: true } });
  }
}
fs.writeFileSync(out, JSON.stringify(jobs)); console.log(jobs.length, 'jobs');
