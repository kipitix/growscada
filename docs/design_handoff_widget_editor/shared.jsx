/* shared.jsx — sample widget data, syntax highlighter, and reusable
   sub-components shared across the three editor variants. Exported to window. */

const { useState, useRef, useEffect, useLayoutEffect } = React;

/* ============================ ICONS ============================ */
const ic = (p) => (props) =>
  React.createElement(
    'svg',
    {
      width: props && props.s ? props.s : 16,
      height: props && props.s ? props.s : 16,
      viewBox: '0 0 24 24',
      fill: 'none',
      stroke: 'currentColor',
      strokeWidth: 1.8,
      strokeLinecap: 'round',
      strokeLinejoin: 'round',
      style: { display: 'block', flex: '0 0 auto' },
    },
    p
  );
const IconSun = ic([
  React.createElement('circle', { key: 1, cx: 12, cy: 12, r: 4 }),
  React.createElement('path', { key: 2, d: 'M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4' }),
]);
const IconMoon = ic([React.createElement('path', { key: 1, d: 'M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z' })]);
const IconPlus = ic([React.createElement('path', { key: 1, d: 'M12 5v14M5 12h14' })]);
const IconTrash = ic([React.createElement('path', { key: 1, d: 'M3 6h18M8 6V4h8v2M6 6l1 14h10l1-14' })]);
const IconPlay = ic([React.createElement('path', { key: 1, d: 'M6 4l14 8-14 8z', fill: 'currentColor', stroke: 'none' })]);
const IconCheck = ic([React.createElement('path', { key: 1, d: 'M4 12l5 5L20 6' })]);
const IconX = ic([React.createElement('path', { key: 1, d: 'M6 6l12 12M18 6L6 18' })]);
const IconChevron = ic([React.createElement('path', { key: 1, d: 'M6 9l6 6 6-6' })]);
const IconCode = ic([React.createElement('path', { key: 1, d: 'M9 18l-6-6 6-6M15 6l6 6-6 6' })]);
const IconBox = ic([
  React.createElement('path', { key: 1, d: 'M21 8l-9-5-9 5v8l9 5 9-5z' }),
  React.createElement('path', { key: 2, d: 'M3 8l9 5 9-5M12 13v8' }),
]);
const IconActivity = ic([React.createElement('path', { key: 1, d: 'M22 12h-4l-3 9L9 3l-3 9H2' })]);
const IconPlug = ic([
  React.createElement('path', { key: 1, d: 'M9 2v6M15 2v6M6 8h12v3a6 6 0 0 1-12 0z' }),
  React.createElement('path', { key: 2, d: 'M12 17v5' }),
]);
const IconTable = ic([
  React.createElement('rect', { key: 1, x: 3, y: 4, width: 18, height: 16, rx: 1 }),
  React.createElement('path', { key: 2, d: 'M3 10h18M9 4v16' }),
]);
const IconEye = ic([
  React.createElement('path', { key: 1, d: 'M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7z' }),
  React.createElement('circle', { key: 2, cx: 12, cy: 12, r: 3 }),
]);
const IconSearch = ic([
  React.createElement('circle', { key: 1, cx: 11, cy: 11, r: 7 }),
  React.createElement('path', { key: 2, d: 'M21 21l-4-4' }),
]);

/* ============================ SAMPLE DATA ============================ */
const WIDGETS = [
  {
    id: 'bool-circle',
    name: 'Boolean Circle',
    kind: 'boolean',
    template:
      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"\n     width="100%" height="100%" style="display:block">\n  <circle class="ind" cx="50" cy="50" r="40"\n          fill="#888" stroke="#555" stroke-width="2"/>\n</svg>',
    script:
      "function render(inputs) {\n  // green when active, gray when idle\n  var c = el.querySelector('.ind');\n  c.setAttribute('fill', inputs.state ? '#2ecc71' : '#888888');\n}",
    ports: [{ name: 'state', type: 'boolean', desc: 'Active state (true = green, false = gray)' }],
    data: [{ name: 'state', type: 'boolean', value: 'true' }],
    render: (el, inp) => {
      const c = el.querySelector('.ind');
      if (c) c.setAttribute('fill', inp.state ? '#2ecc71' : '#888888');
    },
  },
  {
    id: 'int-speed',
    name: 'Integer Speedometer',
    kind: 'number',
    template:
      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 62"\n     width="100%" height="100%" style="display:block">\n  <path d="M10 50 A40 40 0 0 1 90 50" fill="none"\n        stroke="#3a3f4b" stroke-width="8" stroke-linecap="round"/>\n  <path class="fill" d="M10 50 A40 40 0 0 1 90 50" fill="none"\n        stroke="#4aa3ff" stroke-width="8" stroke-linecap="round"\n        stroke-dasharray="126" stroke-dashoffset="126"/>\n  <line class="needle" x1="50" y1="50" x2="16" y2="50"\n        stroke="#e6e6e6" stroke-width="2.5" stroke-linecap="round"/>\n  <circle cx="50" cy="50" r="4" fill="#e6e6e6"/>\n  <text class="val" x="50" y="44" text-anchor="middle"\n        font-size="13" fill="#e6e6e6" font-family="monospace">0</text>\n</svg>',
    script:
      "function render(inputs) {\n  var v = Math.max(0, Math.min(100, inputs.value | 0));\n  var a = 180 * (v / 100);\n  el.querySelector('.needle')\n    .setAttribute('transform', 'rotate(' + a + ' 50 50)');\n  el.querySelector('.fill')\n    .setAttribute('stroke-dashoffset', 126 * (1 - v / 100));\n  el.querySelector('.val').textContent = v;\n}",
    ports: [{ name: 'value', type: 'number', desc: 'Gauge value, clamped to 0–100' }],
    data: [{ name: 'value', type: 'number', value: '62' }],
    render: (el, inp) => {
      const v = Math.max(0, Math.min(100, parseInt(inp.value, 10) || 0));
      const a = 180 * (v / 100);
      const n = el.querySelector('.needle');
      if (n) n.setAttribute('transform', 'rotate(' + a + ' 50 50)');
      const f = el.querySelector('.fill');
      if (f) f.setAttribute('stroke-dashoffset', String(126 * (1 - v / 100)));
      const t = el.querySelector('.val');
      if (t) t.textContent = String(v);
    },
  },
  {
    id: 'str-ticker',
    name: 'String Ticker',
    kind: 'string',
    template:
      '<div class="ticker" style="font:600 15px ui-monospace,monospace;\n     color:#e6e6e6;background:#171b22;border:1px solid #333;\n     border-radius:6px;padding:10px 14px;letter-spacing:.04em;\n     white-space:nowrap;overflow:hidden">\n  <span class="dot">\u25CF</span> <span class="msg">--</span>\n</div>',
    script:
      "function render(inputs) {\n  el.querySelector('.msg').textContent = inputs.text;\n  var ok = /ok|run|on/i.test(inputs.text || '');\n  el.querySelector('.dot').style.color = ok ? '#2ecc71' : '#e2a04a';\n}",
    ports: [{ name: 'text', type: 'string', desc: 'Message string to display' }],
    data: [{ name: 'text', type: 'string', value: 'PUMP-01 OK' }],
    render: (el, inp) => {
      const m = el.querySelector('.msg');
      if (m) m.textContent = inp.text == null ? '' : String(inp.text);
      const ok = /ok|run|on/i.test(inp.text || '');
      const d = el.querySelector('.dot');
      if (d) d.style.color = ok ? '#2ecc71' : '#e2a04a';
    },
  },
];

function coerce(type, raw) {
  if (type === 'boolean') return raw === 'true' || raw === true;
  if (type === 'number') return parseFloat(raw);
  return raw == null ? '' : String(raw);
}
function buildInputs(widget, dataArr) {
  const o = {};
  (dataArr || widget.data).forEach((d) => {
    o[d.name] = coerce(d.type, d.value);
  });
  return o;
}

/* ============================ SYNTAX HIGHLIGHTER ============================ */
function tokenizeJS(code) {
  const rules = [
    [/^\/\/[^\n]*/, 'comment'],
    [/^\/\*[\s\S]*?\*\//, 'comment'],
    [/^'(?:[^'\\]|\\.)*'/, 'str'],
    [/^"(?:[^"\\]|\\.)*"/, 'str'],
    [/^`(?:[^`\\]|\\.)*`/, 'str'],
    [/^\b(?:function|var|let|const|if|else|return|for|while|new|typeof|in|of|this)\b/, 'kw'],
    [/^\b(?:true|false|null|undefined|NaN)\b/, 'num'],
    [/^\b\d+(?:\.\d+)?\b/, 'num'],
    [/^[A-Za-z_$][\w$]*(?=\s*\()/, 'fn'],
    [/^[A-Za-z_$][\w$]*/, 'ident'],
    [/^=>|^[-+*/%<>=!&|?]+/, 'op'],
    [/^[{}()\[\].;,:]/, 'punct'],
    [/^\s+/, 'ws'],
    [/^./, 'text'],
  ];
  const out = [];
  let s = code;
  while (s.length) {
    let matched = false;
    for (const [re, cls] of rules) {
      const m = re.exec(s);
      if (m) {
        out.push({ text: m[0], cls });
        s = s.slice(m[0].length);
        matched = true;
        break;
      }
    }
    if (!matched) { out.push({ text: s[0], cls: 'text' }); s = s.slice(1); }
  }
  return out;
}

function tokenizeHTML(code) {
  const out = [];
  let pos = 0;
  const len = code.length;
  while (pos < len) {
    if (code[pos] === '<') {
      if (code.startsWith('<!--', pos)) {
        let end = code.indexOf('-->', pos);
        end = end === -1 ? len : end + 3;
        out.push({ text: code.slice(pos, end), cls: 'comment' });
        pos = end;
        continue;
      }
      const m = /^<\/?[A-Za-z][\w:-]*/.exec(code.slice(pos));
      if (m) {
        const full = m[0];
        const sl = full[1] === '/' ? 2 : 1;
        out.push({ text: full.slice(0, sl), cls: 'punct' });
        out.push({ text: full.slice(sl), cls: 'tag' });
        pos += full.length;
        while (pos < len && code[pos] !== '>') {
          const rest = code.slice(pos);
          let mm;
          if ((mm = /^\s+/.exec(rest))) { out.push({ text: mm[0], cls: 'ws' }); pos += mm[0].length; continue; }
          if (code[pos] === '/') { out.push({ text: '/', cls: 'punct' }); pos++; continue; }
          if ((mm = /^[A-Za-z_:][\w:.-]*/.exec(rest))) {
            out.push({ text: mm[0], cls: 'attr' });
            pos += mm[0].length;
            const eq = /^\s*=\s*/.exec(code.slice(pos));
            if (eq) {
              out.push({ text: eq[0], cls: 'punct' });
              pos += eq[0].length;
              const val = /^"[^"]*"|^'[^']*'/.exec(code.slice(pos));
              if (val) { out.push({ text: val[0], cls: 'str' }); pos += val[0].length; }
            }
            continue;
          }
          out.push({ text: code[pos], cls: 'text' });
          pos++;
        }
        if (pos < len && code[pos] === '>') { out.push({ text: '>', cls: 'punct' }); pos++; }
        continue;
      }
    }
    let next = code.indexOf('<', pos);
    if (next === -1) next = len;
    out.push({ text: code.slice(pos, next), cls: 'htext' });
    pos = next;
  }
  return out;
}

// Split flat tokens into lines (array of arrays of {text,cls})
function tokensToLines(tokens) {
  const lines = [[]];
  tokens.forEach((t) => {
    const parts = t.text.split('\n');
    parts.forEach((p, i) => {
      if (i > 0) lines.push([]);
      if (p.length) lines[lines.length - 1].push({ text: p, cls: t.cls });
    });
  });
  return lines;
}

/* CodeBlock — gutter line numbers + highlighted body. lang: 'html' | 'js' */
function CodeBlock({ code, lang, className }) {
  const tokens = lang === 'js' ? tokenizeJS(code) : tokenizeHTML(code);
  const lines = tokensToLines(tokens);
  return (
    <div className={'code ' + (className || '')}>
      <div className="code-gutter" aria-hidden="true">
        {lines.map((_, i) => (
          <div key={i} className="code-lno">{i + 1}</div>
        ))}
      </div>
      <pre className="code-body">
        {lines.map((line, i) => (
          <div key={i} className="code-line">
            {line.length === 0 ? (
              '\u200b'
            ) : (
              line.map((t, j) => (
                <span key={j} className={'t-' + t.cls}>{t.text}</span>
              ))
            )}
          </div>
        ))}
      </pre>
    </div>
  );
}

/* TypeBadge — small monospace type chip */
function TypeBadge({ type }) {
  return <span className={'badge badge-' + type}>{type}</span>;
}

/* PreviewStage — live-renders template + render() with current inputs */
function PreviewStage({ widget, inputs }) {
  const ref = useRef(null);
  useEffect(() => {
    const node = ref.current;
    if (!node) return;
    node.innerHTML = widget.template;
    try { widget.render(node, inputs); } catch (e) { /* noop */ }
  }, [widget, JSON.stringify(inputs)]);
  return <div className="preview-stage" ref={ref} />;
}

/* useEditor — shared editor state for an instance */
function useEditor(initialTheme, initialAccent) {
  const [theme, setTheme] = useState(initialTheme || 'dark');
  const accent = initialAccent || 'blue';
  const [selId, setSelId] = useState('bool-circle');
  const widget = WIDGETS.find((w) => w.id === selId) || WIDGETS[0];
  const [values, setValues] = useState(() => {
    const o = {};
    WIDGETS.forEach((w) => { o[w.id] = {}; w.data.forEach((d) => (o[w.id][d.name] = d.value)); });
    return o;
  });
  const setValue = (name, v) =>
    setValues((prev) => ({ ...prev, [selId]: { ...prev[selId], [name]: v } }));
  const rawVals = values[selId] || {};
  const inputs = {};
  widget.ports.forEach((p) => { inputs[p.name] = coerce(p.type, rawVals[p.name]); });
  return { theme, setTheme, accent, selId, setSelId, widget, rawVals, setValue, inputs };
}

/* DataControl — boolean toggle / number / string input for Input Data rows */
function DataControl({ port, value, onChange }) {
  if (port.type === 'boolean') {
    const on = value === 'true' || value === true;
    return (
      <div className={'toggle' + (on ? ' on' : '')} onClick={() => onChange(on ? 'false' : 'true')}>
        <div className="toggle-track"><div className="toggle-knob" /></div>
        <span className="toggle-label">{on ? 'true' : 'false'}</span>
      </div>
    );
  }
  return (
    <input
      className="inp"
      type={port.type === 'number' ? 'number' : 'text'}
      value={value == null ? '' : value}
      onChange={(e) => onChange(e.target.value)}
    />
  );
}

Object.assign(window, {
  IconSun, IconMoon, IconPlus, IconTrash, IconPlay, IconCheck, IconX, IconChevron,
  IconCode, IconBox, IconActivity, IconPlug, IconTable, IconEye, IconSearch,
  WIDGETS, coerce, buildInputs, CodeBlock, TypeBadge, PreviewStage, useEditor, DataControl,
});
