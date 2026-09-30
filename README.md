# extreload

Reload an unpacked Chrome extension from the command line — no trip to
chrome://extensions to press ↻ after every build.

```sh
extreload reload --name "My Extension" --tabs "http://localhost:3000/*"
```

## How it works

```
extreload reload ──HTTP──▶ extreload serve ──WebSocket──▶ extreload helper (in Chrome)
                             (127.0.0.1:7988)               chrome.management: off, on
                                                            chrome.tabs: reload matching tabs
```

- **The helper** (`extension/`) switches the named extension off and on again
  with `chrome.management`, which makes Chrome read an unpacked extension from
  its folder anew. Nothing is added to the extension being reloaded.
- **`--tabs`** reloads the tabs matching a URL pattern afterwards. An
  extension's content scripts in tabs that were already open belong to the old
  extension and stop talking to it once it is reloaded.
- **The server** is started in the background by the first `reload` or `list`
  (log: `~/.extreload/serve.log`). The helper keeps a WebSocket to it; while
  it is up, the helper answers at once, and when it is not, the helper tries
  again every 30 seconds.
- The server takes WebSocket connections only from `chrome-extension://`
  origins, and refuses any HTTP request carrying an `Origin` header, so a web
  page cannot ask for a reload.

## Install

```sh
make install          # → ~/.local/bin/extreload
make install-skills   # the Claude skill → ~/.claude/skills/extreload
```

Then in Chrome: chrome://extensions → Developer mode → Load unpacked →
`extension/` in this repository. Do it in every Chrome profile you want to
reach.

## Commands

| | |
|---|---|
| `extreload reload --name <name>` / `--id <id>` | reload that unpacked extension |
| `--tabs <pattern>` (repeatable) | and then the tabs matching it |
| `--profile <label>` | only in the Chrome whose helper has this label (set in the helper's options) |
| `extreload list` | the unpacked extensions each connected Chrome has |
| `extreload status` | whether the server is up, and which Chromes are connected |
| `extreload serve` | run the server in the foreground |

Port: 7988, or `$EXTRELOAD_PORT` (and the same in the helper's options).
