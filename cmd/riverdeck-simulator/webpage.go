package main

// webpageTemplate is the HTML served at GET / for the graphical simulator UI.
// Template variables:
//
//	.ModelName  - human-readable device name  (string)
//	.Cols       - number of key columns        (int)
//	.Keys       - total number of keys         (int)
//	.KeySize    - CSS pixel size per key button (int, e.g. 100)
//	.Gap        - CSS gap between key buttons   (int, e.g. 8)
//	.WsPort     - TCP port riverdeck connects to (int, for status display)
const webpageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Riverdeck Simulator - {{.ModelName}}</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

    body {
      background: #111;
      color: #ccc;
      font-family: system-ui, -apple-system, sans-serif;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
      padding: 24px;
      gap: 20px;
    }

    header {
      text-align: center;
    }
    header h1 { font-size: 1.2rem; font-weight: 600; color: #eee; }
    header p  { font-size: 0.75rem; color: #555; margin-top: 4px; }

    #deck {
      display: grid;
      grid-template-columns: repeat({{.Cols}}, {{.KeySize}}px);
      gap: {{.Gap}}px;
      padding: 18px;
      background: #1c1c1e;
      border-radius: 14px;
      box-shadow: 0 8px 32px rgba(0,0,0,0.6);
    }

    .key {
      width:  {{.KeySize}}px;
      height: {{.KeySize}}px;
      background: #2a2a2e;
      border-radius: 8px;
      overflow: hidden;
      cursor: pointer;
      position: relative;
      border: 2px solid #3a3a3e;
      transition: border-color 0.07s, transform 0.07s, filter 0.07s;
      user-select: none;
    }
    .key:hover             { border-color: #666; }
    .key.pressed           { border-color: #fff; transform: scale(0.93); filter: brightness(1.3); }

    .key canvas, .key img  { width: 100%; height: 100%; display: block; object-fit: cover; }

    .key-index {
      position: absolute;
      bottom: 3px; right: 5px;
      font-size: 9px;
      color: rgba(255,255,255,0.2);
      pointer-events: none;
    }

    footer {
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 8px;
    }

    #status {
      font-size: 0.75rem;
      padding: 3px 10px;
      border-radius: 100px;
      background: #2a2a2e;
      color: #888;
      transition: background 0.3s, color 0.3s;
    }
    #status.connected    { background: #143314; color: #4caf50; }
    #status.disconnected { background: #331414; color: #f44336; }

    #brightness-row {
      display: flex;
      align-items: center;
      gap: 10px;
      font-size: 0.75rem;
      color: #555;
    }
    #brightness-bar {
      width: 160px; height: 6px;
      background: #2a2a2e;
      border-radius: 3px;
      overflow: hidden;
    }
    #brightness-fill {
      height: 100%;
      width: 100%;
      background: linear-gradient(90deg, #f90, #ffe066);
      border-radius: 3px;
      transition: width 0.3s;
    }
  </style>
</head>
<body>
  <header>
    <h1>{{.ModelName}}</h1>
    <p>Riverdeck Simulator &nbsp;·&nbsp; riverdeck connects on port {{.WsPort}}</p>
  </header>

  <div id="deck">
    {{range $i := .KeyIndices}}
    <div class="key" id="key-{{$i}}" data-index="{{$i}}">
      <span class="key-index">{{$i}}</span>
    </div>
    {{end}}
  </div>

  <footer>
    <div id="status">Waiting for riverdeck connection...</div>
    <div id="brightness-row">
      Brightness&nbsp;
      <div id="brightness-bar"><div id="brightness-fill"></div></div>
      &nbsp;<span id="brightness-val">100</span>%
    </div>
  </footer>

  <script>
    const deck = document.getElementById('deck');
    const statusEl = document.getElementById('status');
    const bFill = document.getElementById('brightness-fill');
    const bVal  = document.getElementById('brightness-val');

    // ── SSE event stream from Go server ─────────────────────────────────────
    const es = new EventSource('/events');

    es.onopen = () => {
      statusEl.textContent = 'Connected';
      statusEl.className = 'connected';
    };
    es.onerror = () => {
      statusEl.textContent = 'Disconnected - reload to reconnect';
      statusEl.className = 'disconnected';
    };

    es.addEventListener('setimage', e => {
      const d = JSON.parse(e.data);
      const cell = document.getElementById('key-' + d.key);
      if (!cell) return;
      let img = cell.querySelector('img');
      if (!img) {
        img = document.createElement('img');
        img.draggable = false;
        cell.prepend(img);
      }
      cell.style.background = '';
      img.src = 'data:image/png;base64,' + d.data;
    });

    es.addEventListener('setkeycolor', e => {
      const d = JSON.parse(e.data);
      const cell = document.getElementById('key-' + d.key);
      if (!cell) return;
      const img = cell.querySelector('img');
      if (img) img.remove();
      cell.style.background = 'rgb(' + d.r + ',' + d.g + ',' + d.b + ')';
    });

    es.addEventListener('clear', () => {
      document.querySelectorAll('.key').forEach(cell => {
        cell.style.background = '';
        const img = cell.querySelector('img');
        if (img) img.remove();
      });
    });

    es.addEventListener('setbrightness', e => {
      const d = JSON.parse(e.data);
      const pct = Math.max(0, Math.min(100, d.value));
      bFill.style.width = pct + '%';
      bVal.textContent  = pct;
      deck.style.opacity = (0.15 + 0.85 * pct / 100).toFixed(3);
    });

    es.addEventListener('connected', () => {
      statusEl.textContent = 'riverdeck connected';
      statusEl.className = 'connected';
    });
    es.addEventListener('disconnected', () => {
      statusEl.textContent = 'riverdeck disconnected - waiting...';
      statusEl.className = 'disconnected';
    });

    // ── Key press / release event forwarding ────────────────────────────────
    function sendKey(index, pressed) {
      fetch('/keyevent', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key: index, pressed: pressed })
      }).catch(() => {});
    }

    document.querySelectorAll('.key').forEach(cell => {
      const idx = parseInt(cell.dataset.index, 10);

      cell.addEventListener('mousedown', ev => {
        if (ev.button !== 0) return;
        cell.classList.add('pressed');
        sendKey(idx, true);
      });
      cell.addEventListener('mouseup', ev => {
        if (!cell.classList.contains('pressed')) return;
        cell.classList.remove('pressed');
        sendKey(idx, false);
      });
      cell.addEventListener('mouseleave', () => {
        if (cell.classList.contains('pressed')) {
          cell.classList.remove('pressed');
          sendKey(idx, false);
        }
      });

      // Touch support
      cell.addEventListener('touchstart', ev => {
        ev.preventDefault();
        cell.classList.add('pressed');
        sendKey(idx, true);
      }, { passive: false });
      cell.addEventListener('touchend', ev => {
        ev.preventDefault();
        cell.classList.remove('pressed');
        sendKey(idx, false);
      }, { passive: false });
    });

    // Prevent right-click context menus on key cells.
    deck.addEventListener('contextmenu', e => e.preventDefault());
  </script>
</body>
</html>
`
