// node render.js '<jobs json>' : each job is {item, out, recipe?, preset?, opts}.
const { chromium } = require('playwright'); const fs = require('fs');
(async () => {
  const jobs = JSON.parse(process.argv[2][0] === '[' ? process.argv[2] : fs.readFileSync(process.argv[2], 'utf8'));
  const b = await chromium.launch({ args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader', '--ignore-gpu-blocklist'] });
  const p = await b.newPage(); p.on('pageerror', e => console.log('PAGEERR', e.message)); p.on('console', m => { if (m.type() === 'error') console.log('CONSOLE', m.text().slice(0, 300)); });
  await p.setContent('<canvas id=c width=64 height=64></canvas>'); await p.addScriptTag({ path: __dirname + '/dist/set.js' });
  await p.evaluate(() => { window.R = new RealItems.THREE.WebGLRenderer({ canvas: document.getElementById('c'), antialias: true, alpha: true, preserveDrawingBuffer: true }); window.E = {}; });
  for (const j of jobs) {
    const t = Date.now();
    try { const url = await p.evaluate(j => RealItems.still(R, E, j.item, j.recipe, j.preset || 'painted', j.opts).toDataURL('image/png'), j); fs.writeFileSync(j.out, Buffer.from(url.split(',')[1], 'base64')); console.log(j.out, Date.now() - t, 'ms'); }
    catch (e) { console.log('FAIL', j.item, String(e).slice(0, 400)); }
  }
  await b.close();
})();
