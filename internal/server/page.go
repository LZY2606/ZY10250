package server

const page = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<title>共振归属工坊</title>
<style>
  body { font-family: -apple-system, "PingFang SC", sans-serif; margin: 0; background: #f5f6f8; color: #222; }
  header { background: #1f2a44; color: #fff; padding: 12px 20px; display: flex; align-items: baseline; gap: 16px; }
  header h1 { font-size: 20px; margin: 0; }
  header .ver { font-size: 12px; opacity: .8; }
  main { display: grid; grid-template-columns: 340px 1fr 360px; gap: 12px; padding: 12px; }
  section { background: #fff; border: 1px solid #ddd; border-radius: 8px; padding: 10px 12px; margin-bottom: 12px; }
  h2 { font-size: 14px; margin: 4px 0 8px; color: #1f2a44; }
  table { border-collapse: collapse; width: 100%; font-size: 12px; }
  th, td { border: 1px solid #e2e2e2; padding: 3px 6px; text-align: left; }
  th { background: #f0f2f7; }
  button { font-size: 12px; padding: 2px 8px; border: 1px solid #1f2a44; background: #fff; border-radius: 4px; cursor: pointer; }
  button.primary { background: #1f2a44; color: #fff; }
  button.warn { border-color: #b03030; color: #b03030; }
  input[type=number] { width: 70px; font-size: 12px; }
  .tag { display: inline-block; padding: 0 6px; border-radius: 8px; font-size: 11px; background: #e8ecf5; margin-right: 4px; }
  .tag.red { background: #f8dcdc; color: #a02020; }
  .tag.green { background: #dcf0dc; color: #1c6b1c; }
  .sol { border: 1px solid #cfd6e4; border-radius: 6px; padding: 6px 8px; margin-bottom: 6px; font-size: 12px; }
  .sol .score { font-weight: 600; color: #1f2a44; }
  .muted { color: #777; font-size: 12px; }
  #graph { width: 100%; background: #fff; border: 1px solid #ddd; border-radius: 8px; }
  .toolbar { display: flex; gap: 8px; margin-bottom: 10px; }
</style>
</head>
<body>
<header>
  <h1>共振归属工坊</h1>
  <span class="ver" id="ver"></span>
  <span class="ver">峰-原子双部图 · 多方案保留 · 结论固定峰列版本</span>
</header>
<main>
  <div>
    <section>
      <h2>实验与位移容差窗（按核种分组）</h2>
      <div id="experiments"></div>
    </section>
    <section>
      <h2>连通证据</h2>
      <div id="connectivity"></div>
    </section>
    <section>
      <h2>跨实验对应（独立校验）</h2>
      <div id="correspondence"></div>
    </section>
  </div>
  <div>
    <div class="toolbar">
      <button class="primary" onclick="solve()">求解（记录运行）</button>
      <button onclick="exportLog()">导出运行记录</button>
      <button class="warn" onclick="resetAll()">清空数据库并重新导入</button>
    </div>
    <svg id="graph" height="520"></svg>
    <section>
      <h2>归属候选方案（分数相近全部保留）</h2>
      <div id="solutions"></div>
      <div id="unassigned"></div>
      <div id="conflicts"></div>
    </section>
  </div>
  <div>
    <section>
      <h2>实验峰列表</h2>
      <div id="peaks"></div>
    </section>
    <section>
      <h2>原子位点</h2>
      <div id="atoms"></div>
    </section>
  </div>
</main>
<script>
let DATA = null;
const atomName = id => (DATA.state.atoms.find(a => a.id === id) || {}).name || ('#' + id);
const peakLabel = id => (DATA.state.peaks.find(p => p.id === id) || {}).label || ('#' + id);
const isLocked = (p, a) => DATA.state.locks.some(l => l.peak_id === p && l.atom_id === a);

function normalize() {
  const st = DATA.state, res = DATA.result;
  st.experiments = st.experiments || [];
  st.peaks = st.peaks || [];
  st.atoms = st.atoms || [];
  st.connectivities = st.connectivities || [];
  st.correspondence = st.correspondence || [];
  st.locks = st.locks || [];
  res.candidates = res.candidates || [];
  res.solutions = res.solutions || [];
  res.unassigned_peak_ids = res.unassigned_peak_ids || [];
  res.conflict_lock_ids = res.conflict_lock_ids || [];
  res.solutions.forEach(s => {
    s.assignments = s.assignments || [];
    s.correspondence_violations = s.correspondence_violations || [];
  });
}

async function refresh() {
  const r = await fetch('/api/state');
  DATA = await r.json();
  normalize();
  render();
}
async function post(url, body) {
  const r = await fetch(url, {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});
  DATA = await r.json();
  normalize();
  render();
}
async function solve() {
  const r = await fetch('/api/solve', {method: 'POST'});
  DATA = await r.json();
  normalize();
  render();
}
function exportLog() { window.location = '/api/export'; }
async function resetAll() {
  if (!confirm('清空数据库并重新导入 fixture？')) return;
  const r = await fetch('/api/reset', {method: 'POST'});
  DATA = await r.json();
  normalize();
  render();
}
function setLock(p, a, on) { post('/api/lock', {peak_id: p, atom_id: a, locked: on}); }
function setConn(id, rej) { post('/api/connectivity', {id: id, rejected: rej}); }
function setBias(exp, nuc) {
  const v = parseFloat(document.getElementById('bias-' + exp + '-' + nuc).value);
  post('/api/bias', {experiment_id: exp, nucleus: nuc, value: v});
}

function render() {
  const st = DATA.state, res = DATA.result;
  document.getElementById('ver').textContent =
    '峰列版本 #' + st.version.id + '（' + st.version.name + '，导入于 ' + st.version.created_at + '）';

  // Experiments & tolerances & bias
  document.getElementById('experiments').innerHTML = st.experiments.map(e => {
    const dims = e.nuclei.map(n =>
      '<tr><td>' + n + '</td><td>±' + e.tolerances[n] + ' ppm</td>' +
      '<td><input type="number" step="0.01" id="bias-' + e.id + '-' + n + '" value="' + (e.bias[n] || 0) + '"></td>' +
      '<td><button onclick="setBias(' + e.id + ',\'' + n + '\')">调偏置</button></td></tr>').join('');
    return '<h2>' + e.name + '（窗口 ' + e.window + ' 容差单位）</h2>' +
      '<table><tr><th>核种</th><th>容差窗</th><th>系统偏置</th><th></th></tr>' + dims + '</table>';
  }).join('');

  // Connectivity
  document.getElementById('connectivity').innerHTML = st.connectivities.map(c =>
    '<div>' + (c.rejected ? '<span class="tag red">已拒绝</span>' : '<span class="tag green">有效</span>') +
    atomName(c.atom_a) + ' ↔ ' + atomName(c.atom_b) + '（' + c.kind + '，权重 ' + c.weight + '）' +
    ' <button onclick="setConn(' + c.id + ',' + (!c.rejected) + ')">' + (c.rejected ? '恢复' : '拒绝') + '</button></div>'
  ).join('') || '<span class="muted">无</span>';

  // Correspondence
  document.getElementById('correspondence').innerHTML = st.correspondence.map(c =>
    '<div><span class="tag">对应 #' + c.id + '</span>' + peakLabel(c.peak_a) + ' ↔ ' + peakLabel(c.peak_b) + '</div>'
  ).join('') || '<span class="muted">无</span>';

  // Peaks table
  const expName = id => (st.experiments.find(e => e.id === id) || {}).name || id;
  document.getElementById('peaks').innerHTML = '<table><tr><th>峰</th><th>实验</th><th>位移</th><th>强度</th><th>重叠/容量</th></tr>' +
    st.peaks.map(p => '<tr><td>' + p.label + '</td><td>' + expName(p.experiment_id) + '</td><td>' +
      p.shifts.map(x => x.toFixed(2)).join(', ') + '</td><td>' + p.intensity + '</td><td>' +
      (p.overlap ? '<span class="tag red">重叠</span>容量 ' + p.capacity : '容量 ' + p.capacity) + '</td></tr>').join('') + '</table>';

  // Atoms table
  document.getElementById('atoms').innerHTML = '<table><tr><th>位点</th><th>残基</th><th>参考位移</th></tr>' +
    st.atoms.map(a => '<tr><td>' + a.name + '</td><td>' + a.residue + '</td><td>' +
      Object.entries(a.shifts).map(([k, v]) => k + ' ' + v.toFixed(2)).join(', ') + '</td></tr>').join('') + '</table>';

  // Solutions
  document.getElementById('solutions').innerHTML = res.solutions.map((s, i) =>
    '<div class="sol"><span class="score">方案 ' + (i + 1) + ' · 得分 ' + s.score.toFixed(4) + '</span>' +
    (s.correspondence_violations.length ? ' <span class="tag red">跨实验对应冲突 #' + s.correspondence_violations.join(',#') + '</span>' : '') +
    '<br>' + s.assignments.map(a => {
      const locked = isLocked(a.peak_id, a.atom_id);
      return '<span class="tag">' + peakLabel(a.peak_id) + ' → ' + atomName(a.atom_id) +
        '（' + a.edge_score.toFixed(3) + '）</span>' +
        '<button onclick="setLock(' + a.peak_id + ',' + a.atom_id + ',' + !locked + ')">' +
        (locked ? '解锁' : '锁定') + '</button>';
    }).join(' ') + '</div>'
  ).join('') || '<span class="muted">无可行方案</span>';

  document.getElementById('unassigned').innerHTML = res.unassigned_peak_ids.length ?
    '<h2>未归属峰</h2>' + res.unassigned_peak_ids.map(id => '<span class="tag red">' + peakLabel(id) + '</span>').join('') : '';
  document.getElementById('conflicts').innerHTML = res.conflict_lock_ids.length ?
    '<h2>最小冲突集</h2><span class="tag red">锁定 #' + res.conflict_lock_ids.join('，#') + ' 相互矛盾，已跳过求解</span>' : '';

  drawGraph(st, res);
}

function drawGraph(st, res) {
  const svg = document.getElementById('graph');
  const W = svg.clientWidth || 600, H = 520;
  const peaks = st.peaks, atoms = st.atoms;
  const py = {}, ay = {};
  peaks.forEach((p, i) => py[p.id] = 40 + i * (H - 80) / Math.max(1, peaks.length - 1));
  atoms.forEach((a, i) => ay[a.id] = 40 + i * (H - 80) / Math.max(1, atoms.length - 1));
  const px = 90, ax = W - 90;
  let s = '';
  // candidate edges
  res.candidates.forEach(c => {
    s += '<line x1="' + px + '" y1="' + py[c.peak_id] + '" x2="' + ax + '" y2="' + ay[c.atom_id] +
      '" stroke="#c9d2e3" stroke-width="1"><title>距离 ' + c.distance.toFixed(3) + '</title></line>';
  });
  // best solution edges
  const colors = ['#1f6fd6', '#e08000', '#2a9d4a'];
  res.solutions.slice(0, 3).forEach((sol, si) => {
    sol.assignments.forEach(a => {
      s += '<line x1="' + px + '" y1="' + py[a.peak_id] + '" x2="' + ax + '" y2="' + ay[a.atom_id] +
        '" stroke="' + colors[si] + '" stroke-width="' + (3 - si) + '" opacity="0.75"></line>';
    });
  });
  // locks
  st.locks.forEach(l => {
    s += '<line x1="' + px + '" y1="' + py[l.peak_id] + '" x2="' + ax + '" y2="' + ay[l.atom_id] +
      '" stroke="#c02020" stroke-width="4" stroke-dasharray="6,3"></line>';
  });
  peaks.forEach(p => {
    s += '<circle cx="' + px + '" cy="' + py[p.id] + '" r="7" fill="' + (p.overlap ? '#e08000' : '#1f2a44') + '"></circle>' +
      '<text x="' + (px - 12) + '" y="' + (py[p.id] + 4) + '" text-anchor="end" font-size="11">' + p.label + '</text>';
  });
  atoms.forEach(a => {
    s += '<circle cx="' + ax + '" cy="' + ay[a.id] + '" r="7" fill="#2a9d4a"></circle>' +
      '<text x="' + (ax + 12) + '" y="' + (ay[a.id] + 4) + '" font-size="11">' + a.name + '</text>';
  });
  s += '<text x="' + px + '" y="20" text-anchor="middle" font-size="12" fill="#555">峰（左）</text>' +
    '<text x="' + ax + '" y="20" text-anchor="middle" font-size="12" fill="#555">原子位点（右）</text>';
  svg.setAttribute('viewBox', '0 0 ' + W + ' ' + H);
  svg.innerHTML = s;
}

refresh();
</script>
</body>
</html>
`
