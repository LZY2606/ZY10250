package server

const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<title>共振归属工坊</title>
<style>
 body{font-family:system-ui,"PingFang SC",sans-serif;margin:0;background:#f6f7f9;color:#222}
 header{background:#1f2a44;color:#fff;padding:12px 20px}
 header h1{margin:0;font-size:20px}
 main{display:grid;grid-template-columns:1fr 1fr;gap:14px;padding:14px}
 .card{background:#fff;border:1px solid #ddd;border-radius:8px;padding:12px}
 .card h2{margin:0 0 8px;font-size:15px}
 table{border-collapse:collapse;width:100%;font-size:12px}
 th,td{border:1px solid #e2e2e2;padding:3px 6px;text-align:left}
 th{background:#f0f2f6}
 button{cursor:pointer;font-size:12px;padding:2px 8px}
 .full{grid-column:1/3}
 .tag{display:inline-block;background:#eef;border-radius:4px;padding:0 5px;margin:1px;font-size:11px}
 .ovl{background:#ffe9c7}
 .conf{color:#b00020}
 input[type=text]{width:64px;font-size:12px}
 .sol{border-top:1px dashed #ccc;padding-top:6px;margin-top:6px;font-size:12px}
 #opsbar{background:#fff8e6}
</style>
</head>
<body>
<header><h1>共振归属工坊 <span style="font-size:12px;font-weight:normal">Resonance Assignment Workshop</span></h1></header>
<main>
 <div class="card full" id="opsbar">
   <b>操作：</b>
   锁定 <select id="lockPeak"></select> → <select id="lockAtom"></select>
   <button onclick="addLock()">锁定峰-原子对</button>
   &nbsp;拒绝连通 <select id="rejConn"></select>
   <button onclick="rejectConn()">拒绝证据</button>
   &nbsp;偏置 <select id="biasExp"></select> <span id="biasDims"></span>
   <button onclick="applyBias()">调整实验偏置</button>
   &nbsp;<button onclick="resetDB()">清空并重导 fixture</button>
   &nbsp;<a href="/api/runs" target="_blank">导出运行记录</a>
   <div id="activeOps" style="margin-top:6px"></div>
 </div>
 <div class="card"><h2>双部图（峰 ↔ 原子位点）</h2><svg id="graph" width="100%" height="360"></svg></div>
 <div class="card"><h2>归属候选方案</h2><div id="solutions">…</div></div>
 <div class="card"><h2>实验与位移容差窗（按核种×实验分组）</h2><div id="exps"></div></div>
 <div class="card"><h2>峰列</h2><div id="peaks"></div></div>
 <div class="card"><h2>连通证据</h2><div id="conns"></div></div>
 <div class="card"><h2>排他冲突（单实验内）与未归属峰</h2><div id="conflicts"></div></div>
</main>
<script>
let state=null, result=null, candidates=[];
let ops={Locks:[],RejectedConn:[],BiasOverride:{}};

async function load(){
  const r=await fetch('/api/state'); const j=await r.json();
  state=j.dataset; result=j.result; candidates=j.candidates||[];
  render();
}
async function solve(){
  const r=await fetch('/api/solve',{method:'POST',body:JSON.stringify(ops)});
  const j=await r.json(); result=j.result; candidates=j.candidates||[];
  render();
}
async function resetDB(){ await fetch('/api/reset',{method:'POST'});
  ops={Locks:[],RejectedConn:[],BiasOverride:{}}; await load(); }

function peakLabel(id){const p=state.Peaks.find(p=>p.ID===id);return p?p.Label:id;}
function atomLabel(id){const a=state.Atoms.find(a=>a.ID===id);return a?a.Label:id;}
function expName(id){const e=state.Experiments.find(e=>e.ID===id);return e?e.Name:id;}

function addLock(){
  const p=+document.getElementById('lockPeak').value;
  const a=+document.getElementById('lockAtom').value;
  ops.Locks.push({PeakID:p,AtomID:a}); solve();
}
function rejectConn(){
  const id=+document.getElementById('rejConn').value;
  ops.RejectedConn.push(id); solve();
}
function applyBias(){
  const eid=+document.getElementById('biasExp').value;
  const vals=[...document.querySelectorAll('.biasIn')].map(i=>parseFloat(i.value)||0);
  ops.BiasOverride[eid]=vals; solve();
}
function renderOps(){
  const parts=[];
  ops.Locks.forEach(l=>parts.push('<span class="tag">锁 '+peakLabel(l.PeakID)+'→'+atomLabel(l.AtomID)+'</span>'));
  ops.RejectedConn.forEach(c=>parts.push('<span class="tag">拒连通#'+c+'</span>'));
  Object.keys(ops.BiasOverride).forEach(e=>parts.push('<span class="tag">偏置 '+expName(+e)+'=['+ops.BiasOverride[e].join(',')+']</span>'));
  document.getElementById('activeOps').innerHTML=parts.length?('当前操作：'+parts.join(' ')):'';
}

function render(){
  renderOps();
  // selectors
  const pk=document.getElementById('lockPeak'); pk.innerHTML=state.Peaks.map(p=>'<option value="'+p.ID+'">'+p.Label+'</option>').join('');
  const at=document.getElementById('lockAtom'); at.innerHTML=state.Atoms.map(a=>'<option value="'+a.ID+'">'+a.Label+'</option>').join('');
  const cn=document.getElementById('rejConn'); cn.innerHTML=state.Conns.map(c=>'<option value="'+c.ID+'">#'+c.ID+' '+atomLabel(c.FromID)+'↔'+atomLabel(c.ToID)+'('+c.Kind+')</option>').join('');
  const be=document.getElementById('biasExp'); be.innerHTML=state.Experiments.map(e=>'<option value="'+e.ID+'">'+e.Name+'</option>').join('');
  const updDims=()=>{const e=state.Experiments.find(x=>x.ID===+be.value);
    document.getElementById('biasDims').innerHTML=e.Dims.map((d,i)=>'<input type=text class="biasIn" value="'+(e.ShiftBias[i]||0)+'"> '+d.Nucleus).join(' ');};
  be.onchange=updDims; updDims();

  // experiments & tolerances
  document.getElementById('exps').innerHTML='<table><tr><th>实验</th><th>维(核种/容差ppm)</th><th>当前偏置</th></tr>'+
    state.Experiments.map(e=>'<tr><td>'+e.Name+'</td><td>'+e.Dims.map(d=>d.Nucleus+' ±'+d.Tol).join('， ')+'</td><td>['+(ops.BiasOverride[e.ID]||e.ShiftBias).join(', ')+']</td></tr>').join('')+'</table>';

  // peaks
  document.getElementById('peaks').innerHTML='<table><tr><th>峰</th><th>实验</th><th>位移</th><th>强度</th><th>重叠/容量</th></tr>'+
    state.Peaks.map(p=>'<tr><td>'+p.Label+'</td><td>'+expName(p.ExpID)+'</td><td>'+p.Shifts.join(', ')+'</td><td>'+p.Intensity+'</td><td>'+(p.Overlap?'<span class="tag ovl">重叠 cap='+p.Capacity+'</span>':'cap='+p.Capacity)+'</td></tr>').join('')+'</table>';

  // connectivities
  document.getElementById('conns').innerHTML='<table><tr><th>#</th><th>原子对</th><th>类型</th><th>权重</th><th>状态</th></tr>'+
    state.Conns.map(c=>'<tr><td>'+c.ID+'</td><td>'+atomLabel(c.FromID)+'↔'+atomLabel(c.ToID)+'</td><td>'+c.Kind+'</td><td>'+c.Weight+'</td><td>'+(ops.RejectedConn.includes(c.ID)?'<span class="conf">已拒绝</span>':'有效')+'</td></tr>').join('')+'</table>';

  // exclusion conflicts: same experiment, same atom, multiple candidate peaks
  const byEA={};
  candidates.forEach(c=>{const k=c.exp_id+'/'+c.atom_id;(byEA[k]=byEA[k]||[]).push(c.peak_id);});
  const confRows=Object.keys(byEA).filter(k=>byEA[k].length>1).map(k=>{
    const [e,a]=k.split('/');return '<tr><td>'+expName(+e)+'</td><td>'+atomLabel(+a)+'</td><td>'+byEA[k].map(peakLabel).join(' ↔ ')+'</td></tr>';}).join('');
  let confHTML='<table><tr><th>实验</th><th>原子</th><th>竞争峰(互斥)</th></tr>'+(confRows||'<tr><td colspan=3>无</td></tr>')+'</table>';
  confHTML+='<p>未归属峰（最优方案）：'+(result.Unassigned&&result.Unassigned.length?result.Unassigned.map(peakLabel).join(', '):'无')+'</p>';
  if(result.Conflicts&&result.Conflicts.length)confHTML+='<p class="conf">最小冲突集（需移除的锁定）：'+result.Conflicts.join('；')+'</p>';
  document.getElementById('conflicts').innerHTML=confHTML;

  // solutions
  let sh='<p>峰列版本：<code>'+result.Version+'</code>（每个结论固定该版本）</p>';
  (result.Solutions||[]).forEach((s,i)=>{
    sh+='<div class="sol"><b>方案 '+(i+1)+'</b> 分数='+s.Score.toFixed(4)+'<br>'+
      s.Assignments.map(a=>expName(a.ExpID)+':'+peakLabel(a.PeakID)+'→'+atomLabel(a.AtomID)).join('； ')+
      (s.Unassigned.length?('<br><span class="conf">未归属：'+s.Unassigned.map(peakLabel).join(', ')+'</span>'):'')+'</div>';
  });
  if(!(result.Solutions||[]).length)sh+='<p class="conf">无可行方案</p>';
  document.getElementById('solutions').innerHTML=sh;

  drawGraph();
}

function drawGraph(){
  const svg=document.getElementById('graph');
  const W=svg.clientWidth||600,H=360,px=90,ax=W-110;
  const peaks=state.Peaks, atoms=state.Atoms;
  const py={},ay={};
  peaks.forEach((p,i)=>py[p.ID]=40+i*(H-70)/Math.max(1,peaks.length-1));
  atoms.forEach((a,i)=>ay[a.ID]=40+i*(H-70)/Math.max(1,atoms.length-1));
  const locked={}; ops.Locks.forEach(l=>locked[l.PeakID+'/'+l.AtomID]=1);
  const inBest={};
  if(result.Solutions&&result.Solutions.length)result.Solutions[0].Assignments.forEach(a=>inBest[a.PeakID+'/'+a.AtomID]=1);
  let s='<rect width="100%" height="100%" fill="#fff"/>';
  candidates.forEach(c=>{
    const k=c.peak_id+'/'+c.atom_id;
    let col='#c9d4e8',wd=1.2;
    if(inBest[k]){col='#2b6cb0';wd=2.6;}
    if(locked[k]){col='#c53030';wd=3;}
    s+='<line x1="'+px+'" y1="'+py[c.peak_id]+'" x2="'+ax+'" y2="'+ay[c.atom_id]+'" stroke="'+col+'" stroke-width="'+wd+'"/>';
  });
  peaks.forEach(p=>{
    s+='<circle cx="'+px+'" cy="'+py[p.ID]+'" r="9" fill="'+(p.Overlap?'#dd6b20':'#4a5568')+'"/>';
    s+='<text x="'+(px-14)+'" y="'+(py[p.ID]+4)+'" font-size="11" text-anchor="end">'+p.Label+'</text>';
  });
  atoms.forEach(a=>{
    s+='<rect x="'+(ax-9)+'" y="'+(ay[a.ID]-9)+'" width="18" height="18" rx="4" fill="#2f855a"/>';
    s+='<text x="'+(ax+16)+'" y="'+(ay[a.ID]+4)+'" font-size="11">'+a.Label+'</text>';
  });
  s+='<text x="'+px+'" y="18" font-size="11" text-anchor="middle">峰</text><text x="'+ax+'" y="18" font-size="11" text-anchor="middle">原子位点</text>';
  svg.innerHTML=s;
}
load();
</script>
</body>
</html>
`
