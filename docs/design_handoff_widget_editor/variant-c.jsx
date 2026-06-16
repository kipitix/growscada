/* variant-c.jsx — "Focus": one editor column with selectable CODE LAYOUT MODES:
   · Tabs    — segmented Template / Script switch (one editor at a time)
   · Split ↑ — two tiled editors, Script on top, Template below
   · Split ↓ — two tiled editors, Template on top, Script below
   Tiled panes share a draggable divider. Roomy Ports + Data column and a large
   live preview. Teal accent. */

const { useState: useStateC, useRef: useRefC } = React;

/* layout-mode icons (16px) */
const LIco = (children) => (props) =>
  React.createElement('svg', { width: 16, height: 16, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 1.7, strokeLinejoin: 'round', style: { display: 'block' } }, children);
const IcoTabs = LIco([
  React.createElement('rect', { key: 1, x: 3, y: 4.5, width: 18, height: 15, rx: 2 }),
  React.createElement('path', { key: 2, d: 'M3 9h18' }),
  React.createElement('rect', { key: 3, x: 5, y: 6, width: 5, height: 1.6, rx: 0.6, fill: 'currentColor', stroke: 'none' }),
]);
const IcoSplitScriptTop = LIco([
  React.createElement('rect', { key: 1, x: 3, y: 4.5, width: 18, height: 15, rx: 2 }),
  React.createElement('path', { key: 2, d: 'M3 12h18' }),
  React.createElement('rect', { key: 3, x: 4.2, y: 5.7, width: 15.6, height: 5.1, rx: 1, fill: 'currentColor', stroke: 'none', opacity: 0.32 }),
]);
const IcoSplitScriptBottom = LIco([
  React.createElement('rect', { key: 1, x: 3, y: 4.5, width: 18, height: 15, rx: 2 }),
  React.createElement('path', { key: 2, d: 'M3 12h18' }),
  React.createElement('rect', { key: 3, x: 4.2, y: 13.2, width: 15.6, height: 5.1, rx: 1, fill: 'currentColor', stroke: 'none', opacity: 0.32 }),
]);

/* a single tiled editor pane (slim header + code body) */
function CodePane({ file, lang, code, dotColor, grow }) {
  return (
    <div className="cpane" style={{ flexGrow: grow }}>
      <div className="cpane-head">
        <span className="cpane-name"><span className="dot-mini" style={{ background: dotColor }} /> {file}</span>
      </div>
      <div className="cpane-body">
        <CodeBlock code={code} lang={lang} />
      </div>
    </div>
  );
}

function VariantWorkbench({ initialTheme, initialLayout }) {
  const ed = useEditor(initialTheme, 'teal');
  const { theme, setTheme, widget, inputs, rawVals, setValue } = ed;
  const [codeTab, setCodeTab] = useStateC('html');
  const [layout, setLayout] = useStateC(initialLayout || 'tabs'); // tabs | split-st | split-sb
  const [ratio, setRatio] = useStateC(0.5);
  const splitRef = useRefC(null);

  const startDrag = (e) => {
    e.preventDefault();
    const move = (ev) => {
      const box = splitRef.current;
      if (!box) return;
      const r = box.getBoundingClientRect();
      let f = (ev.clientY - r.top) / r.height;
      f = Math.max(0.18, Math.min(0.82, f));
      setRatio(f);
    };
    const up = () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
  };

  const tmplPane = { file: 'template.svg', lang: 'html', code: widget.template, dot: 'var(--t-tag)' };
  const scrptPane = { file: 'render.js', lang: 'js', code: widget.script, dot: 'var(--t-fn)' };
  const isSplit = layout !== 'tabs';
  const topPane = layout === 'split-st' ? scrptPane : tmplPane;
  const botPane = layout === 'split-st' ? tmplPane : scrptPane;

  const modeBtn = (id, Icon, title) => (
    <button className={'cmode-btn' + (layout === id ? ' active' : '')} onClick={() => setLayout(id)} title={title}>
      <Icon />
    </button>
  );

  return (
    <div className={'editor theme-' + theme + ' acc-teal'}>
      <style>{`
        .cmode { display:inline-flex; padding:2px; gap:2px; border-radius:8px; background:var(--bg-input); border:1px solid var(--border-soft); }
        .cmode-btn { width:30px; height:23px; display:flex; align-items:center; justify-content:center; border-radius:6px; color:var(--text-dim); cursor:pointer; border:none; background:none; }
        .cmode-btn:hover { color:var(--text); }
        .cmode-btn.active { background:var(--bg-panel); color:var(--accent); box-shadow:var(--shadow); }
        .csplit { display:flex; flex-direction:column; height:100%; min-height:0; }
        .cpane { display:flex; flex-direction:column; min-height:0; flex-basis:0; overflow:hidden; }
        .cpane-head { display:flex; align-items:center; gap:8px; height:30px; flex:0 0 30px; padding:0 6px 0 13px; background:var(--bg-app); border-bottom:1px solid var(--border-soft); }
        .cpane-name { font:600 11.5px var(--mono); color:var(--text-dim); display:flex; align-items:center; gap:8px; }
        .cpane-body { flex:1; min-height:0; }
        .cresize { height:8px; flex:0 0 8px; cursor:row-resize; background:var(--bg-app); border-top:1px solid var(--border-soft); border-bottom:1px solid var(--border-soft); position:relative; }
        .cresize::after { content:""; position:absolute; left:50%; top:50%; transform:translate(-50%,-50%); width:38px; height:3px; border-radius:2px; background:var(--border-strong); }
        .cresize:hover::after { background:var(--accent); }
      `}</style>

      {/* top bar */}
      <div className="ed-top">
        {['Library', 'Project', 'Operation', 'History'].map((t, i) => (
          <button key={t} className={'ed-tab' + (i === 0 ? ' active' : '')}>{t}</button>
        ))}
        <div className="ed-spacer" />
        <div className="ed-brand">
          <div className="ed-brand-mark" />
          <div className="ed-brand-name">Grow<span>SCADA</span></div>
        </div>
        <button className="icon-btn" onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')} title="Toggle theme">
          {theme === 'dark' ? <IconSun /> : <IconMoon />}
        </button>
      </div>

      <div className="ed-body">
        {/* Widget Types */}
        <div className="panel" style={{ flex: '0 0 224px' }}>
          <div className="panel-head">
            <div className="panel-title"><IconBox s={13} /> Widget Types</div>
            <span className="panel-count">{WIDGETS.length}</span>
            <div className="panel-head-actions">
              <button className="btn btn-accent btn-sm" style={{ padding: '0 8px' }}><IconPlus /></button>
            </div>
          </div>
          <div className="panel-body">
            <div className="wlist">
              {WIDGETS.map((w) => (
                <div key={w.id} className={'witem' + (w.id === ed.selId ? ' active' : '')} onClick={() => ed.setSelId(w.id)}>
                  <div className="witem-ico">
                    {w.kind === 'boolean' ? <IconActivity s={14} /> : w.kind === 'number' ? <IconBox s={14} /> : <IconCode s={14} />}
                  </div>
                  <div className="witem-main">
                    <div className="witem-name">{w.name}</div>
                    <div className="witem-kind">{w.kind}</div>
                  </div>
                </div>
              ))}
            </div>
          </div>
          <div className="panel-foot">
            <button className="btn btn-ghost btn-danger btn-sm btn-block"><IconTrash /> Delete widget</button>
          </div>
        </div>

        {/* Code column with layout modes */}
        <div className="panel" style={{ flex: '1.3 1 0' }}>
          <div className="panel-head" style={{ height: 40, flexBasis: 40 }}>
            {layout === 'tabs' ? (
              <div className="seg">
                <button className={'seg-btn' + (codeTab === 'html' ? ' active' : '')} onClick={() => setCodeTab('html')}><IconCode s={13} /> Template</button>
                <button className={'seg-btn' + (codeTab === 'js' ? ' active' : '')} onClick={() => setCodeTab('js')}><IconActivity s={13} /> Script</button>
              </div>
            ) : (
              <div className="panel-title"><IconCode s={13} /> Template <span style={{ color: 'var(--text-faint)' }}>+</span> Script</div>
            )}
            <div className="panel-head-actions">
              <div className="cmode">
                {modeBtn('tabs', IcoTabs, 'Вкладки')}
                {modeBtn('split-st', IcoSplitScriptTop, 'Тайлинг · скрипт сверху')}
                {modeBtn('split-sb', IcoSplitScriptBottom, 'Тайлинг · скрипт снизу')}
              </div>
              <button className="btn btn-accent btn-sm"><IconCheck /> Apply</button>
            </div>
          </div>
          <div className="panel-body" style={{ padding: 0 }}>
            {!isSplit ? (
              codeTab === 'html'
                ? <CodeBlock code={widget.template} lang="html" />
                : <CodeBlock code={widget.script} lang="js" />
            ) : (
              <div className="csplit" ref={splitRef}>
                <CodePane file={topPane.file} lang={topPane.lang} code={topPane.code} dotColor={topPane.dot} grow={ratio} />
                <div className="cresize" onPointerDown={startDrag} title="Перетащите, чтобы изменить размер" />
                <CodePane file={botPane.file} lang={botPane.lang} code={botPane.code} dotColor={botPane.dot} grow={1 - ratio} />
              </div>
            )}
          </div>
        </div>

        {/* Ports + Data combined */}
        <div className="panel" style={{ flex: '0 0 296px', display: 'flex', flexDirection: 'column' }}>
          <div className="panel-head">
            <div className="panel-title"><IconPlug s={13} /> Input Ports</div>
            <span className="panel-count">{widget.ports.length}</span>
            <div className="panel-head-actions">
              <button className="btn btn-accent btn-sm"><IconCheck /> Apply</button>
            </div>
          </div>
          <div className="panel-body" style={{ flex: '0 0 auto', maxHeight: '46%' }}>
            <div className="ports">
              {widget.ports.map((p) => (
                <div key={p.name} className="port-row">
                  <div className="port-handle"><IconPlug s={14} /></div>
                  <div className="port-main">
                    <div className="port-name-row"><span className="port-name">{p.name}</span><TypeBadge type={p.type} /></div>
                    <div className="port-desc">{p.desc}</div>
                  </div>
                  <button className="port-x"><IconX s={13} /></button>
                </div>
              ))}
              <button className="btn btn-sm btn-block" style={{ marginTop: 2 }}><IconPlus /> Add Port</button>
            </div>
          </div>
          <div className="panel-head" style={{ borderTop: '1px solid var(--border-soft)' }}>
            <div className="panel-title"><IconTable s={13} /> Input Data</div>
            <span className="panel-count">{widget.ports.length}</span>
          </div>
          <div className="panel-body" style={{ flex: '1 1 0' }}>
            <div className="data">
              {widget.ports.map((p) => (
                <div key={p.name} className="data-row">
                  <div className="data-info"><span className="data-name">{p.name}</span><TypeBadge type={p.type} /></div>
                  <div className="data-val"><DataControl port={p} value={rawVals[p.name]} onChange={(v) => setValue(p.name, v)} /></div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Large Preview */}
        <div className="panel" style={{ flex: '1.05 1 0' }}>
          <div className="panel-head">
            <div className="panel-title"><IconEye s={13} /> Preview</div>
            <div className="panel-head-actions">
              <span className="badge">live</span>
            </div>
          </div>
          <div className="preview-wrap">
            <div className="preview-frame" style={{ padding: 36 }}>
              <div className="preview-card" style={{ width: 240, height: 240 }}>
                <PreviewStage widget={widget} inputs={inputs} />
              </div>
            </div>
            <div className="preview-meta">
              <span className="dot" />
              <span>{widget.name}</span>
              <span style={{ marginLeft: 'auto', color: 'var(--text-faint)' }}>
                {widget.ports.map((p) => p.name + ' = ' + JSON.stringify(inputs[p.name])).join('   ')}
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* status bar */}
      <div className="statusbar">
        <div className="status-item"><IconBox s={12} /> {widget.name}</div>
        <div className="status-item status-soft">{layout === 'tabs' ? (codeTab === 'html' ? 'template.svg' : 'render.js') : 'split view'}</div>
        <div className="status-sep" />
        <div className="status-item status-soft">{widget.ports.length} ports bound</div>
        <div className="status-item"><span style={{ width: 7, height: 7, borderRadius: '50%', background: '#fff', display: 'inline-block' }} /> Live</div>
      </div>
    </div>
  );
}

Object.assign(window, { VariantWorkbench });
