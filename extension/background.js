// The extreload helper: keeps a WebSocket open to `extreload serve` on
// localhost and does what it asks.
//
//   reload {id|name, tabs}  switch that unpacked extension off and on again —
//                           Chrome reads it from its folder anew — then reload
//                           the tabs matching `tabs`, whose content scripts
//                           were the old extension's
//   list                    the unpacked extensions in this Chrome
//
// A service worker is stopped when idle; the socket's ping every 20s keeps
// this one up while the server is there, and an alarm every 30s brings it back
// to reconnect when it is not.

const PORT = 7988;
const RECONNECT_ALARM = 'extreload-reconnect';

let ws = null;
let pingTimer = null;

function send(msg) {
  if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(msg));
}

async function portNow() {
  const { port } = await chrome.storage.local.get('port');
  return Number(port) || PORT;
}

async function connect() {
  if (ws && (ws.readyState === WebSocket.CONNECTING || ws.readyState === WebSocket.OPEN)) return;
  let sock;
  try {
    sock = new WebSocket(`ws://127.0.0.1:${await portNow()}/ws`);
  } catch (e) {
    return;
  }
  ws = sock;
  sock.onopen = async () => {
    const { label = '' } = await chrome.storage.local.get('label');
    send({ type: 'hello', label });
    clearInterval(pingTimer);
    pingTimer = setInterval(() => send({ type: 'ping' }), 20000);
  };
  sock.onmessage = (e) => {
    let msg;
    try { msg = JSON.parse(e.data); } catch (err) { return; }
    handle(msg).then(
      (body) => send({ type: 'result', req: msg.req, body }),
      (err) => send({ type: 'result', req: msg.req, error: String((err && err.message) || err) }),
    );
  };
  sock.onclose = () => {
    if (ws === sock) ws = null;
    clearInterval(pingTimer);
    setTimeout(connect, 3000);
  };
}

async function unpacked() {
  const all = await chrome.management.getAll();
  return all.filter((x) => x.type === 'extension' && x.installType === 'development' && x.id !== chrome.runtime.id);
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function handle(msg) {
  if (msg.type === 'list') {
    return (await unpacked()).map((x) => ({ id: x.id, name: x.name, version: x.version, enabled: x.enabled }));
  }
  if (msg.type === 'reload') {
    const name = String(msg.name || '').toLowerCase();
    const targets = (await unpacked()).filter((x) => (msg.id ? x.id === msg.id : x.name.toLowerCase() === name));
    for (const t of targets) {
      await chrome.management.setEnabled(t.id, false);
      await chrome.management.setEnabled(t.id, true);
    }
    let tabs = 0;
    const patterns = Array.isArray(msg.tabs) ? msg.tabs.filter(Boolean) : [];
    if (targets.length && patterns.length) {
      // A moment for the reloaded extension to register its content scripts.
      await sleep(500);
      const found = await chrome.tabs.query({ url: patterns });
      await Promise.all(found.map((tab) => chrome.tabs.reload(tab.id)));
      tabs = found.length;
    }
    return { reloaded: targets.map((t) => ({ id: t.id, name: t.name })), tabs };
  }
  throw new Error(`unknown request: ${msg.type}`);
}

chrome.alarms.create(RECONNECT_ALARM, { periodInMinutes: 0.5 });
chrome.alarms.onAlarm.addListener((a) => { if (a.name === RECONNECT_ALARM) connect(); });
chrome.runtime.onStartup.addListener(connect);
chrome.runtime.onInstalled.addListener(connect);
chrome.storage.onChanged.addListener((changes) => {
  // A new label or port: say hello again.
  if ((changes.label || changes.port) && ws) ws.close();
});
connect();
