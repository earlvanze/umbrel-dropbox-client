package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/earlvanze/umbrel-dropbox-client/internal/auth"
	"github.com/earlvanze/umbrel-dropbox-client/internal/config"
	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
)

var (
	deviceAuthMu   sync.Mutex
	deviceAuthCode string
	deviceToken    *auth.Token
	deviceErr      error
)

func (d *Daemon) serveSetupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tokStatus, err := auth.TokenStatus(d.cfg.TokenFile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	accountEmail := ""
	if tokStatus.Present {
		accessToken, err := d.loadDropboxAccessToken(r.Context())
		if err == nil {
			client := dropbox.New(accessToken)
			acct, _ := client.CurrentAccount(r.Context())
			if acct != nil {
				accountEmail = acct.Email
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"has_token":     tokStatus.Present,
		"has_root":      d.cfg.Root != "",
		"account_email": accountEmail,
		"config": map[string]any{
			"root":          d.cfg.Root,
			"remote_path":   d.cfg.RemotePath,
			"dry_run":       d.cfg.DryRun,
			"allow_live":    d.cfg.AllowLive,
			"sync_paths":    d.cfg.SyncPaths,
			"exclude_paths": d.cfg.ExcludePaths,
		},
	})
}

func (d *Daemon) serveConfigAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		redacted := d.cfg
		redacted.TokenFile = ""
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(redacted)
	case http.MethodPost:
		var newCfg config.Config
		if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		newCfg.TokenFile = d.cfg.TokenFile
		newCfg.DBPath = d.cfg.DBPath
		newCfg.HealthAddr = d.cfg.HealthAddr
		if newCfg.Root == "" {
			http.Error(w, "root is required", http.StatusBadRequest)
			return
		}
		if err := config.Save(d.onDiskConfigPath(), newCfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		prev := d.cfg
		d.cfg = newCfg
		restartRequired := ConfigRestartRequired(prev, newCfg)
		_ = d.store.Event("config.updated", "")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":               true,
			"restart_required": restartRequired,
			"restart_fields":   RequiredRestartFields,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (d *Daemon) onDiskConfigPath() string {
	if d.cfg.TokenFile != "" {
		dir := filepath.Dir(d.cfg.TokenFile)
		return filepath.Join(dir, "config.json")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "umbrel-dropbox-client", "config.json")
}

func (d *Daemon) serveAuthDeviceStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		clientID = os.Getenv("DROPBOX_CLIENT_ID")
	}
	if clientID == "" {
		http.Error(w, "client_id required", http.StatusBadRequest)
		return
	}
	oauth := dropbox.NewOAuthClient(clientID)
	deviceCode, err := oauth.StartDeviceCode(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	deviceAuthMu.Lock()
	deviceAuthCode = deviceCode.DeviceCode
	deviceToken = nil
	deviceErr = nil
	deviceAuthMu.Unlock()

	go d.pollDeviceToken(clientID, deviceCode.DeviceCode, deviceCode.Interval)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"user_code":                 deviceCode.UserCode,
		"verification_uri":          deviceCode.VerificationURI,
		"verification_uri_complete": deviceCode.VerificationURIComplete,
		"device_code":               deviceCode.DeviceCode,
		"expires_in":                deviceCode.ExpiresIn,
		"interval":                  deviceCode.Interval,
	})
}

func (d *Daemon) pollDeviceToken(clientID, deviceCode string, interval int) {
	oauth := dropbox.NewOAuthClient(clientID)
	ctx := context.Background()
	tick := time.Duration(interval) * time.Second
	if tick <= 0 {
		tick = 5 * time.Second
	}
	expires := time.Now().Add(10 * time.Minute)
	for time.Now().Before(expires) {
		time.Sleep(tick)
		tok, err := oauth.PollDeviceToken(ctx, deviceCode)
		if err != nil {
			if strings.Contains(err.Error(), "authorization_pending") {
				continue
			}
			deviceAuthMu.Lock()
			deviceErr = err
			deviceAuthMu.Unlock()
			return
		}
		at := auth.TokenFromDropbox(tok.AccessToken, tok.RefreshToken, tok.TokenType, tok.ExpiresIn, tok.AccountID, tok.Scope, time.Now())
		at.ClientID = clientID
		if err := auth.SaveToken(d.cfg.TokenFile, at); err != nil {
			deviceAuthMu.Lock()
			deviceErr = err
			deviceAuthMu.Unlock()
			return
		}
		deviceAuthMu.Lock()
		deviceToken = &at
		deviceAuthMu.Unlock()
		return
	}
	deviceAuthMu.Lock()
	deviceErr = fmt.Errorf("device code expired")
	deviceAuthMu.Unlock()
}

func (d *Daemon) serveAuthDevicePoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	deviceAuthMu.Lock()
	defer deviceAuthMu.Unlock()
	status := "pending"
	if deviceToken != nil {
		status = "success"
	} else if deviceErr != nil {
		status = "error"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": status,
		"error": func() string {
			if deviceErr != nil {
				return deviceErr.Error()
			}
			return ""
		}(),
	})
}

func (d *Daemon) serveRemoteFolders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	accessToken, err := d.loadDropboxAccessToken(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	client := dropbox.New(accessToken)
	result, err := client.ListFolder(r.Context(), "", false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type folderItem struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Dir  bool   `json:"dir"`
	}
	var items []folderItem
	for _, e := range result.Entries {
		items = append(items, folderItem{Name: e.Name, Path: e.PathLower, Dir: e.Tag == "folder"})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"folders": items})
}

const setupHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Setup - Dropbox Client</title>
<style>
*{box-sizing:border-box}
body{font-family:system-ui,sans-serif;max-width:720px;margin:2rem auto;padding:0 1rem;line-height:1.5;color:#18181b}
h1{margin-bottom:.5rem}
.step{display:none}
.step.active{display:block}
.card{border:1px solid #e4e4e7;border-radius:.75rem;padding:1.25rem;margin:1rem 0;background:#fff}
.card h3{margin-top:0}
.btn{display:inline-block;padding:.5rem 1rem;border:0;border-radius:.5rem;background:#0061ff;color:#fff;font-size:1rem;cursor:pointer}
.btn:hover{background:#0050d8}
.btn:disabled{background:#a1a1aa;cursor:not-allowed}
.btn-secondary{background:#f4f4f5;color:#18181b;border:1px solid #d4d4d8}
.btn-secondary:hover{background:#e4e4e7}
input[type=text]{width:100%;padding:.5rem;border:1px solid #d4d4d8;border-radius:.5rem;font-size:1rem}
.folder-list{max-height:320px;overflow:auto;border:1px solid #e4e4e7;border-radius:.5rem;padding:.5rem}
.folder-item{display:flex;align-items:center;gap:.5rem;padding:.35rem .5rem;border-radius:.35rem}
.folder-item:hover{background:#f4f4f5}
.folder-item input{margin:0}
.muted{color:#71717a}
.pill{display:inline-block;font-size:.75rem;border:1px solid #d4d4d8;border-radius:999px;padding:.1rem .45rem;color:#52525b;background:#fafafa}
.error{color:#dc2626;background:#fef2f2;padding:.75rem;border-radius:.5rem;margin:.5rem 0}
.success{color:#16a34a;background:#f0fdf4;padding:.75rem;border-radius:.5rem;margin:.5rem 0}
#spinner{display:none}
#spinner.active{display:inline-block;width:1rem;height:1rem;border:2px solid #e4e4e7;border-top-color:#0061ff;border-radius:50%;animation:spin 1s linear infinite;margin-left:.5rem}
@keyframes spin{to{transform:rotate(360deg)}}
</style>
</head>
<body>
<h1>Dropbox Client Setup</h1>
<p class="muted">Complete these steps to start syncing your Dropbox.</p>

<div class="step active" id="step1">
  <div class="card">
    <h3>Step 1: Connect Dropbox</h3>
    <p>Link your Dropbox account using device authentication.</p>
    <div id="auth-result"></div>
    <button class="btn" id="btn-auth" onclick="startAuth()">Connect Dropbox</button>
    <span id="spinner"></span>
  </div>
</div>

<div class="step" id="step2">
  <div class="card">
    <h3>Step 2: Local Folder</h3>
    <p>Choose where synced files will be stored on this device.</p>
    <label>Local folder path</label><br>
    <input type="text" id="root-input" value="/home/umbrel/Dropbox" placeholder="/home/umbrel/Dropbox">
    <p class="muted">This folder will be created if it does not exist.</p>
    <div style="margin-top:1rem">
      <button class="btn" onclick="saveRoot()">Continue</button>
    </div>
    <div id="root-error" class="error" style="display:none"></div>
  </div>
</div>

<div class="step" id="step3">
  <div class="card">
    <h3>Step 3: Selective Sync</h3>
    <p>Choose which top-level Dropbox folders to sync. Leave all unchecked to sync everything.</p>
    <div id="folder-loading" class="muted">Loading folders...</div>
    <div class="folder-list" id="folder-list" style="display:none"></div>
    <div style="margin-top:1rem">
      <button class="btn" onclick="saveSyncSettings()">Continue</button>
      <button class="btn btn-secondary" onclick="skipSelectiveSync()">Sync everything</button>
    </div>
  </div>
</div>

<div class="step" id="step4">
  <div class="card">
    <h3>Step 4: Review & Start</h3>
    <div id="review-content"></div>
    <div style="margin-top:1rem">
      <button class="btn" id="btn-start" onclick="startSync()">Start Syncing</button>
      <span id="spinner-start"></span>
    </div>
    <div id="start-result"></div>
  </div>
</div>

<script>
let syncPaths = [];
let excludePaths = [];

async function api(method, path, body) {
  const opts = {method};
  if (body) {
    opts.headers = {'Content-Type': 'application/json'};
    opts.body = JSON.stringify(body);
  }
  const r = await fetch(path, opts);
  if (!r.ok) throw new Error((await r.text()).slice(0,200));
  return r.json();
}

function showStep(n) {
  for (let i=1;i<=4;i++) document.getElementById('step'+i).classList.toggle('active', i===n);
}

async function checkSetup() {
  const s = await api('GET', '/api/setup');
  if (s.has_token && s.has_root) {
    if (s.config.sync_paths && s.config.sync_paths.length || s.config.exclude_paths && s.config.exclude_paths.length) {
      showReview(s.config);
      showStep(4);
    } else {
      showStep(3);
      loadFolders();
    }
  } else if (s.has_token) {
    showStep(2);
  } else {
    showStep(1);
  }
}

checkSetup();

async function startAuth() {
  const btn = document.getElementById('btn-auth');
  const spin = document.getElementById('spinner');
  const res = document.getElementById('auth-result');
  btn.disabled = true;
  spin.classList.add('active');
  try {
    const clientID = prompt('Enter your Dropbox App Key (Client ID):');
    if (!clientID) { btn.disabled = false; spin.classList.remove('active'); return; }
    const data = await api('POST', '/api/auth/device?client_id=' + encodeURIComponent(clientID));
    res.innerHTML = '<div class="success">Go to <a href="' + data.verification_uri_complete + '" target="_blank">' + data.verification_uri + '</a> and enter code: <strong>' + data.user_code + '</strong></div>';
    pollAuth();
  } catch (e) {
    res.innerHTML = '<div class="error">' + e.message + '</div>';
    btn.disabled = false;
  }
  spin.classList.remove('active');
}

async function pollAuth() {
  for (let i=0;i<60;i++) {
    await new Promise(r => setTimeout(r, 5000));
    const p = await api('GET', '/api/auth/device-poll');
    if (p.status === 'success') {
      document.getElementById('auth-result').innerHTML = '<div class="success">Authenticated successfully.</div>';
      await new Promise(r => setTimeout(r, 800));
      showStep(2);
      return;
    } else if (p.status === 'error') {
      document.getElementById('auth-result').innerHTML = '<div class="error">' + (p.error || 'Auth failed') + '</div>';
      document.getElementById('btn-auth').disabled = false;
      return;
    }
  }
  document.getElementById('auth-result').innerHTML += '<div class="error">Timed out. Try again.</div>';
  document.getElementById('btn-auth').disabled = false;
}

async function saveRoot() {
  const val = document.getElementById('root-input').value.trim();
  if (!val) { document.getElementById('root-error').style.display='block'; document.getElementById('root-error').textContent='Path required'; return; }
  const cfg = await api('GET', '/api/config');
  cfg.root = val;
  await api('POST', '/api/config', cfg);
  showStep(3);
  loadFolders();
}

async function loadFolders() {
  const list = document.getElementById('folder-list');
  const loading = document.getElementById('folder-loading');
  try {
    const data = await api('GET', '/api/remote/folders');
    loading.style.display = 'none';
    list.style.display = 'block';
    let html = '';
    for (const f of data.folders) {
      if (!f.dir) continue;
      html += '<div class="folder-item"><input type="checkbox" id="fld-'+f.name+'" value="'+f.path+'" checked><label for="fld-'+f.name+'">' + f.name + '</label></div>';
    }
    list.innerHTML = html || '<div class="muted">No folders found.</div>';
  } catch (e) {
    loading.textContent = 'Error loading folders: ' + e.message;
  }
}

function saveSyncSettings() {
  const checked = Array.from(document.querySelectorAll('#folder-list input:checked')).map(i => i.value);
  syncPaths = checked;
  showReview();
  showStep(4);
}

function skipSelectiveSync() {
  syncPaths = [];
  excludePaths = [];
  showReview();
  showStep(4);
}

function showReview(cfg) {
  const c = cfg || {};
  document.getElementById('review-content').innerHTML = '<p><span class="pill">Local root</span> <code>' + (c.root || document.getElementById('root-input').value) + '</code></p>' +
    '<p><span class="pill">Sync scope</span> ' + (syncPaths.length ? syncPaths.join(', ') : 'All folders') + '</p>' +
    '<p><span class="pill">Mode</span> ' + (c.dry_run !== false ? 'Dry-run (safe)' : 'Live sync') + '</p>';
}

async function startSync() {
  const btn = document.getElementById('btn-start');
  const spin = document.getElementById('spinner-start');
  const res = document.getElementById('start-result');
  btn.disabled = true;
  spin.classList.add('active');
  try {
    const cfg = await api('GET', '/api/config');
    cfg.sync_paths = syncPaths;
    cfg.exclude_paths = excludePaths;
    cfg.remote_path = cfg.remote_path || '';
    cfg.dry_run = true;
    cfg.allow_live = false;
    await api('POST', '/api/config', cfg);
    res.innerHTML = '<div class="success">Settings saved. <strong>Restart the app</strong> in your Umbrel dashboard to begin syncing.</div>';
  } catch (e) {
    res.innerHTML = '<div class="error">' + e.message + '</div>';
    btn.disabled = false;
  }
  spin.classList.remove('active');
}
</script>
</body>
</html>
`

func (d *Daemon) serveSetupHTML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, setupHTML)
}
