// A tiny cookie-jar HTTP client for the player-eyes crawl. Plain fetch, no
// browser: the crawl reads response bodies, it does not run page scripts.

export class Client {
  constructor(base, label) {
    this.base = base.replace(/\/$/, '');
    this.label = label;
    this.jar = new Map();
  }

  cookieHeader() {
    return [...this.jar].map(([k, v]) => `${k}=${v}`).join('; ');
  }

  csrf() {
    return this.jar.get('chronicle_csrf') || '';
  }

  store(res) {
    for (const sc of res.headers.getSetCookie?.() || []) {
      const [pair] = sc.split(';');
      const i = pair.indexOf('=');
      const name = pair.slice(0, i).trim();
      const val = pair.slice(i + 1).trim();
      if (/expires=Thu, 01 Jan 1970/i.test(sc) || val === '') this.jar.delete(name);
      else this.jar.set(name, val);
    }
  }

  // Sends a request and follows redirects by hand so cookies set on the way
  // are kept. Returns {status, headers, text, url}.
  async req(method, path, { body, json, headers = {}, follow = true } = {}) {
    let url = path.startsWith('http') ? path : this.base + path;
    let m = method;
    let b;
    const h = { ...headers };
    if (json !== undefined) { b = JSON.stringify(json); h['Content-Type'] = 'application/json'; }
    else if (body) { b = new URLSearchParams(body).toString(); h['Content-Type'] = 'application/x-www-form-urlencoded'; }
    for (let hop = 0; hop < 8; hop++) {
      if (!this.csrf()) {
        // Prime the CSRF cookie with a harmless GET.
        const p = await fetch(this.base + '/login', { redirect: 'manual' });
        this.store(p);
      }
      const res = await fetch(url, {
        method: m,
        body: m === 'GET' || m === 'HEAD' ? undefined : b,
        redirect: 'manual',
        headers: { Cookie: this.cookieHeader(), 'X-CSRF-Token': this.csrf(), ...h },
      });
      this.store(res);
      const loc = res.headers.get('location');
      if (follow && loc && res.status >= 300 && res.status < 400) {
        url = new URL(loc, url).toString();
        if (res.status !== 307 && res.status !== 308) { m = 'GET'; b = undefined; delete h['Content-Type']; }
        continue;
      }
      return { status: res.status, headers: res.headers, text: await res.text(), url };
    }
    throw new Error('too many redirects ' + path);
  }

  get(path, opts) { return this.req('GET', path, opts); }
  post(path, o) { return this.req('POST', path, o); }
  put(path, o) { return this.req('PUT', path, o); }
  del(path, o) { return this.req('DELETE', path, o); }

  async register(email, name, password) {
    // Registration is rate limited per address; wait the window out.
    this.creds = { email, password };
    for (let i = 0; i < 8; i++) {
      await this.get('/register');
      const r = await this.post('/register', { body: { email, display_name: name, password, confirm: password, csrf_token: this.csrf() } });
      if (r.status !== 429) return r;
      await new Promise((res) => setTimeout(res, 15000));
    }
    throw new Error('registration stayed rate limited');
  }

  // True while this client's session still works.
  async signedIn() {
    const r = await this.get('/campaigns', { follow: false });
    return r.status === 200;
  }

  async login(email, password) {
    this.creds = { email, password };
    await this.get('/login');
    return this.post('/login', { body: { email, password, csrf_token: this.csrf() } });
  }
}
