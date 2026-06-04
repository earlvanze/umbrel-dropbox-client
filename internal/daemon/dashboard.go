package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/earlvanze/umbrel-dropbox-client/internal/auth"
)

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Dropbox — Dashboard</title>
<style>
:root{--primary:#0061ff;--primary-dark:#0050d8;--bg:#f8f9fb;--card:#fff;--text:#18181b;--muted:#71717a;--border:#e4e4e7;--danger:#dc2626;--success:#16a34a;--warning:#ca8a04;--radius:.5rem;--shadow:0 1px 3px rgba(0,0,0,.08)}
*{box-sizing:border-box}
body{margin:0;font-family:system-ui,-apple-system,BlinkMacSystemFont,Segoe UI,Roboto,sans-serif;background:var(--bg);color:var(--text);line-height:1.5}
.app{display:flex;min-height:100vh}
.sidebar{width:220px;background:var(--card);border-right:1px solid var(--border);padding:1.5rem 1rem;display:flex;flex-direction:column}
.brand{font-weight:700;font-size:1.15rem;margin-bottom:1.5rem;color:var(--primary);display:flex;align-items:center;gap:.5rem}
.brand svg{width:24px;height:24px;fill:var(--primary)}
nav{display:flex;flex-direction:column;gap:.25rem}
nav a{padding:.6rem .75rem;border-radius:var(--radius);color:var(--text);text-decoration:none;font-size:.95rem;display:flex;align-items:center;gap:.5rem;cursor:pointer}
nav a:hover{background:#f4f4f5}
nav a.active{background:var(--primary);color:#fff;font-weight:500}
main{flex:1;padding:1.5rem 2rem;max-width:1200px;width:100%}
h1{margin:0 0 1rem;font-size:1.5rem}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:1rem;margin-bottom:1.5rem}
.card{background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:1.25rem;box-shadow:var(--shadow)}
.card h3{margin:0 0 .5rem;font-size:.875rem;color:var(--muted);text-transform:uppercase;letter-spacing:.03em}
.card .value{font-size:1.75rem;font-weight:700}
.card .sub{font-size:.85rem;color:var(--muted);margin-top:.25rem}
.actions{display:flex;gap:.5rem;margin-bottom:1.5rem;flex-wrap:wrap}
.btn{padding:.5rem 1rem;border:1px solid transparent;border-radius:var(--radius);font-size:.9rem;cursor:pointer;background:var(--primary);color:#fff;text-decoration:none;display:inline-flex;align-items:center;gap:.4rem}
.btn:hover{background:var(--primary-dark)}
.btn.secondary{background:#fff;color:var(--text);border-color:var(--border)}
.btn.secondary:hover{background:#f4f4f5}
.btn.danger{background:var(--danger);border-color:var(--danger)}
.btn.danger:hover{background:#b91c1c}
.btn:disabled{opacity:.5;cursor:not-allowed}
table{width:100%;border-collapse:collapse;background:var(--card);border:1px solid var(--border);border-radius:var(--radius);overflow:hidden}
th,td{padding:.65rem .85rem;text-align:left;font-size:.9rem;border-bottom:1px solid var(--border)}
th{background:#fafafa;color:var(--muted);font-weight:600;font-size:.8rem;text-transform:uppercase;letter-spacing:.03em}
tr:last-child td{border-bottom:0}
tr:hover td{background:#f4f4f5}
.pathbar{display:flex;align-items:center;gap:.5rem;margin-bottom:1rem;padding:.5rem .75rem;background:var(--card);border:1px solid var(--border);border-radius:var(--radius)}
.pathbar a{color:var(--primary);text-decoration:none}
.pathbar a:hover{text-decoration:underline}
.file-list{display:grid;gap:.25rem}
.file-row{display:flex;align-items:center;gap:.75rem;padding:.5rem .75rem;background:var(--card);border:1px solid var(--border);border-radius:var(--radius);cursor:pointer}
.file-row:hover{background:#f4f4f5}
.file-row .name{flex:1;font-weight:500}
.file-row .meta{font-size:.8rem;color:var(--muted);display:flex;gap:1rem}
.file-row .actions{display:flex;gap:.25rem}
.file-row .actions a{font-size:.8rem;color:var(--muted);text-decoration:none;padding:.2rem .4rem;border-radius:.25rem}
.file-row .actions a:hover{background:#e4e4e7;color:var(--text)}
.file-row .actions a.delete:hover{color:var(--danger);background:#fef2f2}
.icon{width:20px;height:20px;display:inline-flex;align-items:center;justify-content:center}
.icon.folder{color:var(--warning)}
.icon.file{color:var(--muted)}
.empty{text-align:center;padding:2rem;color:var(--muted)}
.form-group{margin-bottom:1rem}
.form-group label{display:block;font-size:.85rem;font-weight:600;margin-bottom:.35rem;color:var(--muted)}
.form-group input,.form-group select,.form-group textarea{width:100%;padding:.5rem .6rem;border:1px solid var(--border);border-radius:var(--radius);font-size:.9rem;background:#fff}
.form-group input:focus,.form-group select:focus,.form-group textarea:focus{outline:0;border-color:var(--primary)}
.form-group small{display:block;margin-top:.25rem;color:var(--muted);font-size:.8rem}
.field-row{display:grid;grid-template-columns:1fr 1fr;gap:1rem}
@media(max-width:768px){.field-row{grid-template-columns:1fr}.sidebar{width:100%;position:fixed;bottom:0;left:0;z-index:10;border-right:0;border-top:1px solid var(--border);flex-direction:row;padding:.5rem;justify-content:space-around}.sidebar .brand{display:none}nav{flex-direction:row}main{padding:1rem 1rem 5rem}}
.hidden{display:none}
.spinner{display:inline-block;width:1rem;height:1rem;border:2px solid var(--border);border-top-color:var(--primary);border-radius:50%;animation:spin 1s linear infinite}
@keyframes spin{to{transform:rotate(360deg)}}
.badge{display:inline-block;font-size:.7rem;padding:.15rem .4rem;border-radius:999px;background:#f4f4f5;color:var(--muted);border:1px solid var(--border)}
.badge.paused{background:#fef2f2;color:var(--danger);border-color:#fecaca}
.badge.active{background:#f0fdf4;color:var(--success);border-color:#bbf7d0}
.toast{position:fixed;top:1rem;right:1rem;padding:.75rem 1rem;border-radius:var(--radius);box-shadow:var(--shadow);color:#fff;font-size:.9rem;z-index:100;opacity:0;transition:opacity .3s}
.toast.show{opacity:1}
.toast.success{background:var(--success)}
.toast.error{background:var(--danger)}
.dropzone{border:2px dashed var(--border);border-radius:var(--radius);padding:2rem;text-align:center;color:var(--muted);cursor:pointer;margin-bottom:1rem}
.dropzone.dragover{border-color:var(--primary);background:#eff6ff}
</style>
</head>
<body>
<div class="app">
<aside class="sidebar">
<div class="brand"><svg viewBox="0 0 24 24"><path d="M12 2L2 9l10 7 10-7-10-7zM2 15l10 7 10-7M2 18l10 7 10-7"/></svg>
Dropbox</div>
<nav>
<a href="/dashboard" data-tab="dashboard" class="active">Dashboard</a>
<a href="#" data-tab="files">Files</a>
<a href="#" data-tab="settings">Settings</a>
<a href="#" data-tab="conflicts">Conflicts</a>
</nav>
</aside>
<main>
<div id="tab-dashboard">
<h1>Dashboard</h1>
<div class="grid" id="status-cards"></div>
<div class="actions">
<button class="btn" id="btn-pause" onclick="togglePause()">Pause</button>
<button class="btn secondary" onclick="triggerScan()">Trigger Scan</button>
<a class="btn secondary" href="/setup" id="setup-link">Setup Wizard</a>
</div>
<h3>Recent Events</h3>
<table><thead><tr><th>Time</th><th>Type</th><th>Detail</th></tr></thead><tbody id="events-body"><tr><td colspan="3" class="empty">Loading...</td></tr></tbody></table>
</div>
<div id="tab-files" class="hidden">
<h1>Files</h1>
<div class="actions">
<div class="dropzone" id="dropzone" onclick="document.getElementById('file-input').click()">
Click or drag files here to upload
<input type="file" id="file-input" multiple style="display:none" onchange="handleFiles(this.files)">
</div>
</div>
<div class="pathbar" id="pathbar"><a href="#" onclick="navigate('')">root</a></div>
<div class="file-list" id="file-list"><div class="empty">Loading...</div></div>
<div class="actions" style="margin-top:1rem">
<button class="btn secondary" onclick="createFolder()">New Folder</button>
</div>
</div>
<div id="tab-settings" class="hidden">
<h1>Settings</h1>
<div class="card"><form id="settings-form" onsubmit="saveSettings(event)">
<div class="field-row">
<div class="form-group"><label>Local Root Path</label><input type="text" id="setting-root" required></div>
<div class="form-group"><label>Remote Path</label><input type="text" id="setting-remote" placeholder="(empty for full Dropbox)"></div>
</div>
<div class="field-row">
<div class="form-group"><label>Dry Run</label><select id="setting-dryrun"><option value="true">Enabled (safe)</option><option value="false">Disabled (live sync)</option></select></div>
<div class="form-group"><label>Allow Live</label><select id="setting-allowlive"><option value="false">No</option><option value="true">Yes</option></select></div>
</div>
<div class="field-row">
<div class="form-group"><label>Scan Interval (seconds)</label><input type="number" id="setting-interval" min="30"></div>
<div class="form-group"><label>Watch File System</label><select id="setting-watch"><option value="true">Yes</option><option value="false">No</option></select></div>
</div>
<div class="form-group"><label>Sync Paths (comma-separated Dropbox folder paths)</label><input type="text" id="setting-syncpaths" placeholder="/Work,/Photos"></div>
<div class="form-group"><label>Exclude Paths (comma-separated)</label><input type="text" id="setting-exclude" placeholder="/Temp,/Cache"></div>
<div class="form-group"><label>Ignore Dirs (comma-separated directory names)</label><input type="text" id="setting-ignore" placeholder=".git,node_modules"></div>
<div style="margin-top:1rem"><button type="submit" class="btn">Save Settings</button></div>
</form></div>
</div>
<div id="tab-conflicts" class="hidden">
<h1>Conflicts</h1>
<table><thead><tr><th>Path</th><th>Reason</th><th>Created</th><th></th></tr></thead><tbody id="conflicts-body"><tr><td colspan="4" class="empty">Loading...</td></tr></tbody></table>
</div>
</main>
</div>
<div class="toast" id="toast"></div>
<script>
let currentPath="",isPaused=false;
function $1(sel){return document.querySelector(sel)}
function showToast(msg,type){const t=$1('#toast');t.textContent=msg;t.className='toast '+type+' show';setTimeout(()=>t.classList.remove('show'),3000)}
async function api(method,path,body){const opts={method};if(body){opts.headers={'Content-Type':'application/json'};opts.body=JSON.stringify(body)}const r=await fetch(path,opts);if(!r.ok){const txt=await r.text();throw new Error(txt.slice(0,200))}if(r.status===204)return{};return r.json()}

function switchTab(tab){document.querySelectorAll('nav a').forEach(a=>a.classList.toggle('active',a.dataset.tab===tab));['dashboard','files','settings','conflicts'].forEach(t=>$1('#tab-'+t).classList.toggle('hidden',t!==tab));if(tab==='files')loadFiles();if(tab==='dashboard')loadDashboard();if(tab==='settings')loadSettings();if(tab==='conflicts')loadConflicts();const target=tab==='dashboard'?'/dashboard':'/'+tab;if(location.pathname!==target){window.history.replaceState({},'',target)}}
document.querySelectorAll('nav a').forEach(a=>a.addEventListener('click',e=>{e.preventDefault();if(a.dataset.tab==='dashboard'){window.history.pushState({},'','/dashboard');switchTab('dashboard');return}switchTab(a.dataset.tab)}));

async function loadDashboard(){try{const s=await api('GET','/api/status');isPaused=s.paused;$1('#btn-pause').textContent=isPaused?'Resume':'Pause';const cards=[{t:'Status',v:isPaused?'Paused':'Active',sub:isPaused?'Sync is paused':'Running normally',cls:isPaused?'paused':'active'},{t:'Entries',v:s.entries,sub:'tracked files'},{t:'Pending Ops',v:s.pending_ops,sub:'queued operations'},{t:'Conflicts',v:s.conflicts,sub:'unresolved'}];$1('#status-cards').innerHTML=cards.map(c=>'<div class="card"><h3>'+c.t+'</h3><div class="value">'+c.v+'</div><div class="sub">'+c.sub+'</div></div>').join('');$1('#setup-link').style.display=s.has_token&&s.has_root?'none':'inline-flex';const ev=await api('GET','/api/events');$1('#events-body').innerHTML=ev.events&&ev.events.length?ev.events.map(e=>'<tr><td>'+new Date(e.created_at).toLocaleString()+'</td><td><span class="badge">'+e.type+'</span></td><td>'+(e.detail||'')+'</td></tr>').join(''):'<tr><td colspan="3" class="empty">No events yet</td></tr>';}catch(e){showToast(e.message,'error')}}

async function togglePause(){try{await api('POST',isPaused?'/api/resume':'/api/pause');showToast(isPaused?'Resumed':'Paused','success');loadDashboard()}catch(e){showToast(e.message,'error')}}
async function triggerScan(){try{await api('POST','/api/scan');showToast('Scan triggered','success')}catch(e){showToast(e.message,'error')}}

async function loadFiles(){try{const data=await api('GET','/api/files?path='+encodeURIComponent(currentPath));$1('#pathbar').innerHTML='<a href="#" onclick="navigate(\'\')">root</a>'+(currentPath?currentPath.split('/').map((p,i)=>{const path=currentPath.split('/').slice(0,i+1).join('/');return ' / <a href="#" onclick="navigate(\''+path+'\')">'+p+'</a>'}).join(''):'');const list=$1('#file-list');if(!data.items||!data.items.length){list.innerHTML='<div class="empty">This folder is empty</div>';return}list.innerHTML=data.items.map(item=>{const icon=item.dir?'<svg class="icon folder" viewBox="0 0 24 24" fill="currentColor"><path d="M10 4H4a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-8l-2-2z"/></svg>':'<svg class="icon file" viewBox="0 0 24 24" fill="currentColor"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8l-6-6z"/></svg>';const click=item.dir?'onclick="navigate(\''+item.path+'\')"':'';const dl=item.dir?'':'<a href="/download?path='+encodeURIComponent(item.path)+'" title="Download">&#x2193;</a>';return '<div class="file-row" '+click+'><span class="icon">'+icon+'</span><span class="name">'+item.name+'</span><span class="meta">'+(item.size?formatBytes(item.size):'')+' '+(item.modified||'')+'</span><span class="actions">'+dl+'<a href="#" class="delete" title="Delete" onclick="deleteItem(\''+item.path+'\','+item.dir+');return false;">&#x2715;</a></span></div>';}).join('');}catch(e){showToast(e.message,'error')}}
function navigate(path){currentPath=path;switchTab('files')}
function formatBytes(b){if(b<1024)return b+' B';if(b<1024*1024)return (b/1024).toFixed(1)+' KB';return (b/(1024*1024)).toFixed(1)+' MB'}
async function handleFiles(files){if(!files.length)return;const form=new FormData();for(const f of files)form.append('files',f);try{await fetch('/api/files/upload?path='+encodeURIComponent(currentPath),{method:'POST',body:form});showToast('Upload complete','success');loadFiles()}catch(e){showToast(e.message,'error')}}
const dz=$1('#dropzone');dz.addEventListener('dragover',e=>{e.preventDefault();dz.classList.add('dragover')});dz.addEventListener('dragleave',()=>dz.classList.remove('dragover'));dz.addEventListener('drop',e=>{e.preventDefault();dz.classList.remove('dragover');handleFiles(e.dataTransfer.files)});
async function createFolder(){const name=prompt('Folder name:');if(!name)return;try{await api('POST','/api/files/mkdir',{path:(currentPath?currentPath+'/':'')+name});showToast('Folder created','success');loadFiles()}catch(e){showToast(e.message,'error')}}
async function deleteItem(path,isDir){if(!confirm('Delete '+(isDir?'folder':'file')+'?'))return;try{await api('DELETE','/api/files/delete?path='+encodeURIComponent(path));showToast('Deleted','success');loadFiles()}catch(e){showToast(e.message,'error')}}

async function loadSettings(){try{const c=await api('GET','/api/config');$1('#setting-root').value=c.root||'';$1('#setting-remote').value=c.remote_path||'';$1('#setting-dryrun').value=String(c.dry_run!=='false');$1('#setting-allowlive').value=String(c.allow_live==='true');$1('#setting-interval').value=c.scan_interval_seconds||300;$1('#setting-watch').value=String(c.watch!=='false');$1('#setting-syncpaths').value=(c.sync_paths||[]).join(',');$1('#setting-exclude').value=(c.exclude_paths||[]).join(',');$1('#setting-ignore').value=c.ignore_dirs||''}catch(e){showToast(e.message,'error')}}
async function saveSettings(e){e.preventDefault();const root=$1('#setting-root').value.trim();if(!root){showToast('Root path is required','error');return}const sync=$1('#setting-syncpaths').value.split(',').map(s=>s.trim()).filter(Boolean);const excl=$1('#setting-exclude').value.split(',').map(s=>s.trim()).filter(Boolean);const cfg={root:root,remote_path:$1('#setting-remote').value.trim(),dry_run:$1('#setting-dryrun').value==='true',allow_live:$1('#setting-allowlive').value==='true',scan_interval_seconds:parseInt($1('#setting-interval').value)||300,watch:$1('#setting-watch').value==='true',sync_paths:sync,exclude_paths:excl,ignore_dirs:$1('#setting-ignore').value.trim()};try{await api('POST','/api/config',cfg);showToast('Settings saved','success')}catch(e){showToast(e.message,'error')}}

async function loadConflicts(){try{const data=await api('GET','/api/conflicts');const tbody=$1('#conflicts-body');if(!data.conflicts||!data.conflicts.length){tbody.innerHTML='<tr><td colspan="4" class="empty">No conflicts</td></tr>';return}tbody.innerHTML=data.conflicts.map(c=>'<tr><td>'+c.path+'</td><td>'+c.reason+'</td><td>'+new Date(c.created_at).toLocaleString()+'</td><td><button class="btn secondary" onclick="resolveConflict('+c.id+')">Resolve</button></td></tr>').join('');}catch(e){showToast(e.message,'error')}}
async function resolveConflict(id){if(!confirm('Resolve this conflict?'))return;try{await api('POST','/api/conflicts/resolve',{id:id});showToast('Resolved','success');loadConflicts();loadDashboard()}catch(e){showToast(e.message,'error')}}

function syncTabFromLocation(){
  const path=location.pathname.replace(/^\/+|>+$/g,'');
  let tab='dashboard';
  if(path==='files')tab='files';
  else if(path==='settings')tab='settings';
  else if(path==='conflicts')tab='conflicts';
  else if(path==='dashboard'||path==='')tab='dashboard';
  switchTab(tab);
}
window.addEventListener('popstate',syncTabFromLocation);
syncTabFromLocation();
</script>
</body>
</html>
`

func (d *Daemon) serveDashboard(w http.ResponseWriter, r *http.Request) {
	tokStatus, _ := auth.TokenStatus(d.cfg.TokenFile)
	if d.cfg.Root == "" || !tokStatus.Present {
		http.Redirect(w, r, "/setup", http.StatusTemporaryRedirect)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, dashboardHTML)
}

func (d *Daemon) serveEventsJSON(w http.ResponseWriter, _ *http.Request) {
	events, err := d.store.ListEvents(50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"events": events})
}

func (d *Daemon) servePause(w http.ResponseWriter, _ *http.Request) {
	if err := d.store.SetPaused(true); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"paused": true})
}

func (d *Daemon) serveResume(w http.ResponseWriter, _ *http.Request) {
	if err := d.store.SetPaused(false); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"paused": false})
}

func (d *Daemon) serveTriggerScan(w http.ResponseWriter, _ *http.Request) {
	select {
	case d.scanTrigger <- struct{}{}:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"scanning": true})
	default:
		http.Error(w, "scan already triggered", http.StatusTooManyRequests)
	}
}

func (d *Daemon) serveUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	relDir := r.URL.Query().Get("path")
	baseDir, _, err := d.resolveLocalPath(relDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	files := r.MultipartForm.File["files"]
	for _, fh := range files {
		src, err := fh.Open()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		dstPath := filepath.Join(baseDir, filepath.Base(fh.Filename))
		dst, err := os.Create(dstPath)
		if err != nil {
			src.Close()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, err = io.Copy(dst, src)
		src.Close()
		dst.Close()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = d.store.Event("file.upload", dstPath)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d *Daemon) serveMkdir(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	full, _, err := d.resolveLocalPath(body.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(full, 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = d.store.Event("file.mkdir", full)
	w.WriteHeader(http.StatusNoContent)
}

func (d *Daemon) serveDelete(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	full, _, err := d.resolveLocalPath(rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.RemoveAll(full); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = d.store.Event("file.delete", full)
	w.WriteHeader(http.StatusNoContent)
}

func (d *Daemon) serveResolveConflict(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := d.store.ResolveConflict(body.ID, "resolved via dashboard")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = d.store.Event("conflict.resolve", fmt.Sprintf("id=%d", body.ID))
	w.WriteHeader(http.StatusNoContent)
}

func (d *Daemon) serveAPIConflicts(w http.ResponseWriter, _ *http.Request) {
	items, err := d.store.ListConflicts(50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"conflicts": items})
}
