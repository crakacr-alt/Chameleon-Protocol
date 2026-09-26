package control

const indexHTML = `<!doctype html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Chameleon</title>
<style>
body{font-family:system-ui,sans-serif;max-width:760px;margin:40px auto;padding:0 18px;background:#111;color:#eee}
.card{background:#1d1d1d;border-radius:16px;padding:20px;margin:14px 0}
h1{margin-bottom:4px} .muted{color:#aaa} .row{display:flex;justify-content:space-between;gap:16px;padding:7px 0;border-bottom:1px solid #333}
button{padding:10px 14px;border:0;border-radius:10px;margin:5px;cursor:pointer}
button.active{font-weight:700;outline:2px solid #fff}
.ok{color:#8fe388}
</style>
</head>
<body>
<h1>Chameleon</h1>
<div class="muted" id="version"></div>
<div class="card">
  <div class="row"><span>Status</span><b class="ok">Connected</b></div>
  <div class="row"><span>Network</span><b id="network">...</b></div>
  <div class="row"><span>SOCKS5</span><b id="socks">...</b></div>
  <div class="row"><span>Mode</span><b id="mode">...</b></div>
  <div class="row"><span>UDP</span><b id="udp">...</b></div>
</div>
<div class="card">
<h3>Adaptive mode</h3>
<div id="presets"></div>
<p class="muted">Пресет меняет реальные веса latency, jitter, throughput и route cost. Накопленная статистика не стирается.</p>
</div>
<div class="card">
<h3>Carriers</h3>
<div class="row"><span>QUIC</span><b id="quic"></b></div>
<div class="row"><span>TLS</span><b id="tls"></b></div>
<div class="row"><span>TCP</span><b id="tcp"></b></div>
</div>
<script>
async function refresh(){
 const s=await fetch('/api/status').then(r=>r.json());
 version.textContent='v'+s.version;
 network.textContent=(s.network.link||'unknown')+' · '+s.network.id;
 socks.textContent=s.socks; mode.textContent=s.mode; udp.textContent=s.udp_mode;
 quic.textContent=s.quic?'ready':'off'; tls.textContent=s.tls?'ready':'off'; tcp.textContent=s.tcp?'ready':'off';
 const names=await fetch('/api/presets').then(r=>r.json());
 presets.innerHTML='';
 for(const name of names.presets){
   const b=document.createElement('button'); b.textContent=name;
   if(name===s.preset)b.className='active';
   b.onclick=async()=>{await fetch('/api/preset',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({preset:name})});await refresh()};
   presets.appendChild(b);
 }
}
refresh(); setInterval(refresh,5000);
</script>
</body>
</html>`
