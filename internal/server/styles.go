package server

import "net/http"

const appCSS = `
:root {
  color-scheme: light;
  --bg: #f5f7f8;
  --panel: #ffffff;
  --panel-2: #eef2f5;
  --text: #172026;
  --muted: #687781;
  --line: #dce3e8;
  --accent: #0d9488;
  --accent-2: #155e75;
  --danger: #b42318;
  --warn: #b45309;
  --good: #087443;
  --shadow: 0 12px 34px rgba(23, 32, 38, .08);
}
* { box-sizing: border-box; }
html, body { min-height: 100%; }
body {
  margin: 0;
  background: var(--bg);
  color: var(--text);
  font: 14px/1.45 ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  letter-spacing: 0;
}
button, input, select { font: inherit; }
.page-progress {
  position: fixed;
  z-index: 9999;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
  opacity: 0;
  overflow: hidden;
  pointer-events: none;
  transition: opacity 120ms ease;
}
.page-progress::before {
  content: "";
  display: block;
  width: 100%;
  height: 100%;
  background: linear-gradient(90deg, transparent, var(--accent), var(--accent-2), transparent);
  transform: translateX(-100%);
}
.page-progress.active {
  opacity: 1;
}
.page-progress.active::before {
  animation: page-progress-slide 900ms cubic-bezier(.4, 0, .2, 1) infinite;
}
@keyframes page-progress-slide {
  0% { transform: translateX(-100%); }
  55% { transform: translateX(0%); }
  100% { transform: translateX(100%); }
}
.app {
  min-height: 100vh;
  display: grid;
  grid-template-columns: 248px minmax(0, 1fr);
}
.side {
  background: #15252d;
  color: #e8f1f4;
  padding: 22px 16px;
  display: flex;
  flex-direction: column;
  gap: 22px;
}
.brand { display: flex; align-items: center; gap: 10px; font-weight: 760; font-size: 18px; }
.brand-mark {
  width: 32px; height: 32px; border-radius: 7px; display: grid; place-items: center;
  background: #0d9488; color: white; font-weight: 800;
}
.context { color: #a8bac2; font-size: 12px; display: grid; gap: 5px; }
.context strong { color: #ffffff; font-size: 13px; overflow-wrap: anywhere; }
.nav { display: grid; gap: 5px; }
.nav button {
  width: 100%;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: #cfe0e5;
  display: flex;
  justify-content: space-between;
  align-items: center;
  min-height: 36px;
  padding: 8px 10px;
  cursor: pointer;
  text-align: left;
}
.nav button:hover, .nav button[aria-pressed="true"] { background: rgba(255,255,255,.09); color: #fff; }
.scope { color: #89a0aa; font-size: 11px; text-transform: uppercase; }
.main { min-width: 0; padding: 20px 24px 28px; display: grid; gap: 18px; align-content: start; }
.topbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.topbar h1 { margin: 0; font-size: 22px; line-height: 1.1; }
.updated { color: var(--muted); font-size: 12px; white-space: nowrap; }
.summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(130px, 1fr));
  gap: 12px;
}
.metric {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 14px;
  box-shadow: var(--shadow);
  min-width: 0;
}
.metric .label { color: var(--muted); font-size: 12px; }
.metric .value {
  font-weight: 760;
  font-size: 22px;
  line-height: 1.15;
  margin-top: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.toolbar {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 8px;
  box-shadow: var(--shadow);
  display: grid;
  grid-template-columns: minmax(160px, 260px) minmax(180px, 1fr) auto;
  gap: 10px;
  padding: 12px;
  align-items: end;
}
.field { display: grid; gap: 5px; min-width: 0; }
.field label { color: var(--muted); font-size: 12px; }
.field input, .field select {
  width: 100%;
  height: 36px;
  border: 1px solid var(--line);
  border-radius: 7px;
  background: #fff;
  color: var(--text);
  padding: 0 10px;
  outline: none;
}
.field input:focus, .field select:focus { border-color: var(--accent); box-shadow: 0 0 0 3px rgba(13,148,136,.14); }
.icon-button {
  height: 36px;
  width: 38px;
  align-self: end;
  border: 1px solid var(--line);
  border-radius: 7px;
  background: var(--panel-2);
  color: var(--text);
  cursor: pointer;
}
.icon-button:hover { border-color: var(--accent); color: var(--accent-2); }
.table-panel {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 8px;
  box-shadow: var(--shadow);
  min-width: 0;
  overflow: hidden;
}
.table-head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: center;
  padding: 14px 16px;
  border-bottom: 1px solid var(--line);
}
.table-head h2 { margin: 0; font-size: 16px; line-height: 1.2; }
.table-wrap { overflow: auto; }
table { width: 100%; border-collapse: collapse; min-width: 760px; }
th, td { padding: 10px 12px; border-bottom: 1px solid var(--line); text-align: left; white-space: nowrap; }
th { color: var(--muted); font-size: 12px; background: #f9fbfc; font-weight: 680; }
th .sort-heading {
  appearance: none;
  border: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-height: 24px;
  padding: 0;
  font: inherit;
  font-weight: inherit;
}
th .sort-heading:hover, th .sort-heading.active { color: var(--accent-2); }
th .sort-heading:focus-visible {
  outline: 2px solid rgba(13,148,136,.45);
  outline-offset: 3px;
  border-radius: 4px;
}
.sort-indicator {
  display: inline-grid;
  place-items: center;
  width: 10px;
  color: var(--accent);
  font-size: 11px;
  line-height: 1;
}
td { color: #25323a; }
tr:hover td { background: #fbfcfd; }
.primary { font-weight: 690; color: var(--text); }
td.status { color: inherit; background: transparent; }
td.status > span {
  display: inline-flex;
  align-items: center;
  min-height: 24px;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 680;
}
td.status.good > span { background: #ddf7ec; color: var(--good); }
td.status.warn > span { background: #fff4dd; color: var(--warn); }
td.status.neutral > span { background: #e9f1f5; color: var(--accent-2); }
.notice {
  padding: 14px 16px;
  border-radius: 8px;
  border: 1px solid var(--line);
  background: #fff;
  color: var(--muted);
}
.danger { border-color: #f3b8b2; background: #fff1f0; color: var(--danger); }
.empty { padding: 34px 16px; color: var(--muted); text-align: center; }
@media (max-width: 860px) {
  .app { grid-template-columns: 1fr; }
  .side { position: static; }
  .summary { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .toolbar { grid-template-columns: 1fr; }
  .topbar { align-items: flex-start; flex-direction: column; }
}
`

func (s *Server) handleStyles(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(appCSS))
}
