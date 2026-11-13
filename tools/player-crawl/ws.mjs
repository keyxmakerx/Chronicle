// Opens the campaign WebSocket with a viewer's own session and records every
// message, so the crawl can check what the live channel carries while the
// owner edits hidden things.

export async function watch(client, base, cid) {
  const url = base.replace(/^http/, 'ws') + `/ws?campaign=${cid}`;
  const messages = [];
  let opened = false;
  const ws = new WebSocket(url, { headers: { Cookie: client.cookieHeader(), Origin: base } });
  ws.onmessage = (e) => messages.push(typeof e.data === 'string' ? e.data : '[binary]');
  await new Promise((resolve) => {
    ws.onopen = () => { opened = true; resolve(); };
    ws.onerror = () => resolve();
    ws.onclose = () => resolve();
    setTimeout(resolve, 5000);
  });
  return { opened, messages, close: () => { try { ws.close(); } catch { /* already closed */ } } };
}
