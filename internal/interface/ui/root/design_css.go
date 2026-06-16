package root

import "github.com/maxence-charriere/go-app/v10/pkg/app"

// injectDesignCSS injects the shared component stylesheet ported from the
// "SCADA Widget Editor" design handoff (docs/design_handoff_widget_editor).
// Classes reference the CSS variables defined by buildThemeCSS, so they
// automatically follow the active theme/auto mode — no separate theme class
// is needed on the root element.
func injectDesignCSS() {
	doc := app.Window().Get("document")
	el := doc.Call("getElementById", "gs-design-css")
	if el.Truthy() {
		return // already injected
	}
	el = doc.Call("createElement", "style")
	el.Set("id", "gs-design-css")
	el.Set("textContent", designCSS)
	doc.Get("head").Call("appendChild", el)
}

const designCSS = `
/* ── top bar ───────────────────────────────────────────────────────────── */
.ed-top {
  display: flex; align-items: center; gap: 0;
  height: 40px; flex: 0 0 40px;
  padding: 0 12px 0 4px;
  background: var(--bg-panel);
  border-bottom: 1px solid var(--border);
}
.ed-tab {
  position: relative; padding: 0 14px; height: 40px;
  display: flex; align-items: center; gap: 7px;
  font-size: 12.5px; font-weight: 500; color: var(--text-dim);
  cursor: pointer; border: none; background: none;
}
.ed-tab:hover { color: var(--text); }
.ed-tab.active { color: var(--accent); font-weight: 600; }
.ed-tab.active::after {
  content: ""; position: absolute; left: 12px; right: 12px; bottom: -1px;
  height: 2px; background: var(--accent); border-radius: 2px 2px 0 0;
}
.ed-spacer { flex: 1; }
.ed-brand { display: flex; align-items: center; gap: 8px; padding: 0 12px 0 10px; }
.ed-brand-mark {
  width: 18px; height: 18px; border-radius: 5px; flex: 0 0 auto;
  background: linear-gradient(135deg, var(--accent), color-mix(in oklch, var(--accent), #fff 22%));
  box-shadow: inset 0 0 0 1px rgba(255,255,255,0.18);
}
.ed-brand-name { font-size: 12px; font-weight: 700; letter-spacing: 0.02em; }
.ed-brand-name span { color: var(--text-dim); font-weight: 500; }
.icon-btn {
  width: 28px; height: 28px; border-radius: 7px; border: 1px solid transparent;
  display: flex; align-items: center; justify-content: center;
  color: var(--text-dim); background: none; cursor: pointer; font-size: 14px;
}
.icon-btn:hover { color: var(--text); background: var(--hover); border-color: var(--border-soft); }

/* ── body / columns ───────────────────────────────────────────────────── */
.ed-body { flex: 1; display: flex; min-height: 0; gap: 1px; background: var(--border-soft); overflow: hidden; }
.panel { display: flex; flex-direction: column; min-width: 0; min-height: 0; background: var(--bg-panel); }
.panel-head {
  display: flex; align-items: center; gap: 8px;
  height: 34px; flex: 0 0 34px; padding: 0 8px 0 11px;
  border-bottom: 1px solid var(--border-soft);
}
.panel-title {
  font-size: 11px; font-weight: 700; letter-spacing: 0.07em; text-transform: uppercase;
  color: var(--text-dim); display: flex; align-items: center; gap: 7px; white-space: nowrap;
}
.panel-count {
  font: 600 10px/1 var(--mono); color: var(--text-faint);
  background: var(--bg-elev); border: 1px solid var(--border-soft);
  padding: 2px 5px; border-radius: 5px;
}
.panel-head-actions { margin-left: auto; display: flex; align-items: center; gap: 6px; }
.panel-body { flex: 1; min-height: 0; overflow: auto; }
.panel-foot {
  flex: 0 0 auto; padding: 7px 8px; border-top: 1px solid var(--border-soft);
  display: flex; align-items: center; gap: 6px; background: var(--bg-panel);
}

/* ── buttons ───────────────────────────────────────────────────────────── */
.btn {
  display: inline-flex; align-items: center; gap: 6px; height: 26px; padding: 0 11px;
  font: 600 11.5px/1 inherit; border-radius: 7px; cursor: pointer; white-space: nowrap;
  border: 1px solid var(--border); background: var(--bg-elev); color: var(--text);
}
.btn:hover { border-color: var(--border-strong); background: var(--hover); }
.btn-sm { height: 23px; padding: 0 9px; font-size: 11px; border-radius: 6px; }
.btn-accent { background: var(--accent); border-color: var(--accent); color: #fff; }
.btn-accent:hover { filter: brightness(1.08); background: var(--accent); }
.btn-ghost { background: none; border-color: transparent; color: var(--text-dim); }
.btn-ghost:hover { color: var(--text); background: var(--hover); border-color: var(--border-soft); }
.btn-danger { color: #f0816f; }
.btn-danger:hover { color: #ff6b54; border-color: rgba(255,107,84,0.4); background: rgba(255,107,84,0.08); }
.btn:disabled { opacity: 0.4; cursor: default; }
.btn-block { width: 100%; justify-content: center; }

/* ── widget list ───────────────────────────────────────────────────────── */
.wlist { padding: 6px; display: flex; flex-direction: column; gap: 2px; }
.witem {
  display: flex; align-items: center; gap: 9px; padding: 7px 9px; border-radius: 8px;
  cursor: pointer; color: var(--text); border: 1px solid transparent;
}
.witem:hover { background: var(--hover); }
.witem.active { background: var(--accent-soft); border-color: var(--accent-line); }
.witem-ico {
  width: 26px; height: 26px; border-radius: 7px; flex: 0 0 auto;
  display: flex; align-items: center; justify-content: center; font-size: 13px;
  background: var(--bg-elev); border: 1px solid var(--border-soft); color: var(--text-dim);
}
.witem.active .witem-ico { color: var(--accent); border-color: var(--accent-line); background: var(--bg-panel); }
.witem-main { min-width: 0; flex: 1; }
.witem-name { font-size: 12.5px; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.witem-kind { font: 500 10px/1.3 var(--mono); color: var(--text-faint); margin-top: 2px; }
.witem.active .witem-kind { color: var(--accent); }

/* ── code editor (plain styled textarea, no live highlighting) ──────────── */
.codepanel { display: flex; flex-direction: column; height: 100%; min-height: 0; }
.seg { display: inline-flex; padding: 2px; gap: 2px; border-radius: 8px; background: var(--bg-input); border: 1px solid var(--border-soft); }
.seg-btn { padding: 0 11px; height: 22px; display: flex; align-items: center; gap: 6px; font: 600 11px inherit; color: var(--text-dim); border-radius: 6px; cursor: pointer; border: none; background: none; }
.seg-btn.active { background: var(--bg-panel); color: var(--accent); box-shadow: var(--shadow); }
.code-editor {
  flex: 1; width: 100%; resize: none; outline: none; border: none;
  font: 11.5px/1.65 var(--mono); background: var(--bg-code); color: var(--t-text);
  padding: 10px 14px; box-sizing: border-box; tab-size: 2;
}

/* ── input ports ───────────────────────────────────────────────────────── */
.ports { padding: 7px; display: flex; flex-direction: column; gap: 5px; }
.port-row {
  display: flex; align-items: flex-start; gap: 9px; padding: 8px 9px;
  border-radius: 8px; background: var(--bg-elev); border: 1px solid var(--border-soft);
}
.port-handle { color: var(--accent); margin-top: 1px; }
.port-main { flex: 1; min-width: 0; }
.port-name-row { display: flex; align-items: center; gap: 7px; }
.port-name { font: 600 12px var(--mono); color: var(--text); }
.port-desc { font-size: 11px; color: var(--text-dim); margin-top: 3px; line-height: 1.4; }
.port-x {
  width: 22px; height: 22px; border-radius: 6px; border: 1px solid transparent;
  display: flex; align-items: center; justify-content: center; flex: 0 0 auto;
  color: var(--text-faint); background: none; cursor: pointer;
}
.port-x:hover { color: #ff6b54; background: rgba(255,107,84,0.1); }

.addport { display: flex; flex-direction: column; gap: 7px; padding: 2px 2px 7px; }
.field-label { font-size: 10px; font-weight: 700; letter-spacing: 0.06em; text-transform: uppercase; color: var(--text-faint); }
.inp {
  height: 30px; padding: 0 10px; width: 100%; border-radius: 7px;
  border: 1px solid var(--border); background: var(--bg-input); color: var(--text);
  font: 12px var(--mono); outline: none; box-sizing: border-box;
}
.inp::placeholder { color: var(--text-faint); font-family: inherit; }
.inp:focus { border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-soft); }
select.inp { cursor: pointer; }

/* ── input data ────────────────────────────────────────────────────────── */
.data { padding: 7px; display: flex; flex-direction: column; gap: 6px; }
.data-row {
  display: flex; align-items: center; gap: 9px; padding: 8px 9px;
  border-radius: 8px; background: var(--bg-elev); border: 1px solid var(--border-soft);
}
.data-info { min-width: 0; flex: 0 0 auto; display: flex; flex-direction: column; gap: 4px; width: 88px; }
.data-name { font: 600 12px var(--mono); color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.data-val { flex: 1; }
.data-val .inp { font-weight: 600; }

.toggle { display: inline-flex; align-items: center; gap: 8px; cursor: pointer; }
.toggle-track { width: 38px; height: 22px; border-radius: 11px; background: var(--bg-input); border: 1px solid var(--border); position: relative; transition: 0.16s; }
.toggle-knob { position: absolute; top: 2px; left: 2px; width: 16px; height: 16px; border-radius: 50%; background: var(--text-faint); transition: 0.16s; }
.toggle.on .toggle-track { background: color-mix(in oklch, var(--ok), transparent 70%); border-color: var(--ok); }
.toggle.on .toggle-knob { left: 18px; background: var(--ok); }
.toggle-label { font: 600 11.5px var(--mono); color: var(--text-dim); }
.toggle.on .toggle-label { color: var(--ok); }

/* ── preview ───────────────────────────────────────────────────────────── */
.preview-wrap { display: flex; flex-direction: column; height: 100%; min-height: 0; }
.preview-frame {
  flex: 1; min-height: 0; display: flex; align-items: center; justify-content: center;
  padding: 24px; overflow: hidden;
  background-color: var(--bg-code);
  background-image:
    linear-gradient(var(--grid) 1px, transparent 1px),
    linear-gradient(90deg, var(--grid) 1px, transparent 1px);
  background-size: 22px 22px;
}
.preview-card {
  width: 240px; height: 240px; display: flex; align-items: center; justify-content: center;
  padding: 14px; border-radius: 14px; background: var(--bg-panel);
  border: 1px solid var(--border); box-shadow: var(--shadow); box-sizing: border-box;
}
.preview-meta {
  flex: 0 0 auto; padding: 9px 12px; border-top: 1px solid var(--border-soft);
  display: flex; align-items: center; gap: 10px; font: 11px var(--mono); color: var(--text-dim);
  white-space: nowrap; overflow: hidden;
}
.preview-meta .dot { width: 7px; height: 7px; border-radius: 50%; background: var(--ok); box-shadow: 0 0 8px var(--ok); flex: 0 0 auto; }
.preview-meta-values { margin-left: auto; color: var(--text-faint); overflow: hidden; text-overflow: ellipsis; }

/* ── badges ────────────────────────────────────────────────────────────── */
.badge {
  font: 600 10px/1 var(--mono); padding: 3px 6px; border-radius: 5px; letter-spacing: 0.02em;
  border: 1px solid var(--border-soft); color: var(--text-dim); background: var(--bg-input);
}
.badge-boolean { color: #6cb6ff; border-color: rgba(108,182,255,0.3); background: rgba(108,182,255,0.08); }
.badge-integer { color: #8ddb8c; border-color: rgba(141,219,140,0.3); background: rgba(141,219,140,0.08); }
.badge-string { color: #e0a972; border-color: rgba(224,169,114,0.3); background: rgba(224,169,114,0.08); }
.badge-any { color: var(--text-dim); }

/* ── status bar ────────────────────────────────────────────────────────── */
.statusbar {
  flex: 0 0 24px; height: 24px; display: flex; align-items: center; gap: 0;
  padding: 0 4px; background: var(--accent); color: #fff; font: 600 10.5px/1 var(--mono);
}
.status-item { display: flex; align-items: center; gap: 5px; padding: 0 9px; height: 100%; white-space: nowrap; }
.status-sep { flex: 1; }
.status-soft { opacity: 0.82; font-weight: 500; }
.status-dot { width: 7px; height: 7px; border-radius: 50%; background: #fff; display: inline-block; }

/* ── misc ──────────────────────────────────────────────────────────────── */
.empty { padding: 18px 14px; color: var(--text-faint); font-size: 12px; line-height: 1.5; }
.panel-title-icon { color: var(--text-faint); }
.editor-mono { font-family: var(--mono); }
:root, .editor-root { --mono: ui-monospace, "SF Mono", "JetBrains Mono", "Cascadia Code", Menlo, Consolas, monospace; }
`
