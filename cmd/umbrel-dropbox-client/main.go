package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/earlvanze/umbrel-dropbox-client/internal/auth"
	"github.com/earlvanze/umbrel-dropbox-client/internal/config"
	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
	"github.com/earlvanze/umbrel-dropbox-client/internal/hash"
	"github.com/earlvanze/umbrel-dropbox-client/internal/reconcile"
	"github.com/earlvanze/umbrel-dropbox-client/internal/scan"
	"github.com/earlvanze/umbrel-dropbox-client/internal/state"
	"github.com/earlvanze/umbrel-dropbox-client/internal/worker"
)

const defaultDB = ".umbrel-dropbox-client/state.db"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "init":
		cmdInit(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "doctor":
		cmdDoctor(os.Args[2:])
	case "missing-local":
		cmdMissingLocal(os.Args[2:])
	case "conflicts":
		cmdConflicts(os.Args[2:])
	case "resolve-conflict":
		cmdResolveConflict(os.Args[2:])
	case "pause":
		cmdPause(os.Args[2:], true)
	case "resume":
		cmdPause(os.Args[2:], false)
	case "hash":
		cmdHash(os.Args[2:])
	case "auth":
		cmdAuth(os.Args[2:])
	case "remote-account":
		cmdRemoteAccount(os.Args[2:])
	case "smoke-test":
		cmdSmokeTest(os.Args[2:])
	case "sync":
		cmdSync(os.Args[2:])
	case "worker":
		cmdWorker(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println(`umbrel-dropbox-client

Commands:
  init --root PATH [--db PATH] [--config PATH]
  status [--db PATH]
  missing-local [--db PATH] [--limit N] [--enqueue-review]
  conflicts [--db PATH] [--limit N]
  resolve-conflict --id ID [--note TEXT] [--db PATH]
  doctor [--db PATH] [--root PATH] [--token-file PATH]
  pause [--db PATH]
  resume [--db PATH]
  hash PATH
  auth status [--token-file PATH]
  auth save --token-env DROPBOX_TOKEN [--token-file PATH]
  auth device-code --client-id APP_KEY [--token-file PATH]
  auth pkce --client-id APP_KEY [--redirect-uri URL|--redirect-port PORT] [--token-file PATH]
  remote-account --token-env DROPBOX_TOKEN
  sync --once --dry-run [--db PATH] [--root PATH] [--remote|--remote-delta] [--remote-path PATH] [--token-file PATH|--token-env DROPBOX_TOKEN]
  smoke-test --dry-run|--live --remote-path PATH [--token-file PATH|--token-env DROPBOX_TOKEN] [--i-understand-risk]
  worker --once --dry-run [--db PATH] [--limit N]
  worker --once --live --i-understand-risk [--execute-reviewed-deletes] [--db PATH] [--root PATH] [--limit N] [--token-file PATH|--token-env DROPBOX_TOKEN]

MVP scaffold. Live transfers require explicit --live --i-understand-risk gates.`)
}

func dbPath(root string, explicit string) string {
	if explicit != "" {
		return explicit
	}
	return filepath.Join(root, defaultDB)
}

func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	root := fs.String("root", filepath.Join(os.Getenv("HOME"), "Dropbox"), "local sync root")
	db := fs.String("db", "", "state database path")
	configPath := fs.String("config", "", "optional daemon config path to write")
	_ = fs.Parse(args)
	abs, err := filepath.Abs(*root)
	must(err)
	must(os.MkdirAll(filepath.Dir(dbPath(abs, *db)), 0700))
	s, err := state.Open(dbPath(abs, *db))
	must(err)
	defer s.Close()
	must(s.Init())
	resolvedDB := dbPath(abs, *db)
	must(s.SetConfig("root", abs))
	must(s.Event("init", abs))
	if *configPath != "" {
		cfg := config.Default(abs)
		cfg.DBPath = resolvedDB
		must(os.MkdirAll(filepath.Dir(*configPath), 0700))
		must(config.Save(*configPath, cfg))
	}
	fmt.Printf("initialized root=%s db=%s", abs, resolvedDB)
	if *configPath != "" {
		fmt.Printf(" config=%s", *configPath)
	}
	fmt.Println()
}

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	db := fs.String("db", filepath.Join(os.Getenv("HOME"), "Dropbox", defaultDB), "state database path")
	_ = fs.Parse(args)
	s, err := state.Open(*db)
	must(err)
	defer s.Close()
	st, err := s.Status()
	must(err)
	fmt.Printf("root: %s\npaused: %v\nentries: %d\npending_ops: %d\nconflicts: %d\nlast_event: %s\n", st.Root, st.Paused, st.Entries, st.PendingOps, st.Conflicts, st.LastEvent)
}

func cmdDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	db := fs.String("db", filepath.Join(os.Getenv("HOME"), "Dropbox", defaultDB), "state database path")
	rootFlag := fs.String("root", "", "local sync root override")
	tokenFile := fs.String("token-file", "", "secure token file path")
	_ = fs.Parse(args)
	issues := 0
	s, err := state.Open(*db)
	if err != nil {
		fmt.Printf("FAIL db_open path=%s error=%v\n", *db, err)
		os.Exit(1)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		fmt.Printf("FAIL db_init path=%s error=%v\n", *db, err)
		os.Exit(1)
	}
	fmt.Printf("OK db path=%s\n", *db)
	root := *rootFlag
	if root == "" {
		root, err = s.GetConfig("root")
		if err != nil {
			fmt.Printf("FAIL root_config error=%v\n", err)
			issues++
		}
	}
	if root == "" {
		fmt.Println("WARN root missing; run init --root PATH or pass --root")
		issues++
	} else if st, err := os.Stat(root); err != nil {
		fmt.Printf("FAIL root path=%s error=%v\n", root, err)
		issues++
	} else if !st.IsDir() {
		fmt.Printf("FAIL root path=%s is not a directory\n", root)
		issues++
	} else {
		fmt.Printf("OK root path=%s\n", root)
	}
	path := *tokenFile
	if path == "" {
		path, err = auth.DefaultTokenPath()
		if err != nil {
			fmt.Printf("WARN token_path error=%v\n", err)
			issues++
		}
	}
	if path != "" {
		st, err := auth.TokenStatus(path)
		if err != nil {
			fmt.Printf("WARN token path=%s error=%v\n", path, err)
			issues++
		} else if st.Present {
			fmt.Printf("OK token path=%s has_refresh=%v account_id=%s\n", path, st.HasRefresh, st.AccountID)
		} else {
			fmt.Printf("WARN token missing path=%s\n", path)
			issues++
		}
	}
	if _, err := net.DefaultResolver.LookupHost(context.Background(), "api.dropboxapi.com"); err != nil {
		fmt.Printf("WARN dns api.dropboxapi.com error=%v\n", err)
		issues++
	} else {
		fmt.Println("OK dns api.dropboxapi.com")
	}
	status, err := s.Status()
	if err != nil {
		fmt.Printf("FAIL status error=%v\n", err)
		issues++
	} else {
		fmt.Printf("OK status paused=%v entries=%d pending_ops=%d conflicts=%d\n", status.Paused, status.Entries, status.PendingOps, status.Conflicts)
	}
	if issues > 0 {
		fmt.Printf("doctor: completed with %d issue(s)\n", issues)
		os.Exit(1)
	}
	fmt.Println("doctor: ok")
}

func cmdMissingLocal(args []string) {
	fs := flag.NewFlagSet("missing-local", flag.ExitOnError)
	db := fs.String("db", filepath.Join(os.Getenv("HOME"), "Dropbox", defaultDB), "state database path")
	limit := fs.Int("limit", 50, "maximum missing local entries to list")
	enqueueReview := fs.Bool("enqueue-review", false, "enqueue non-destructive review ops for missing local tombstones")
	_ = fs.Parse(args)
	s, err := state.Open(*db)
	must(err)
	defer s.Close()
	must(s.Init())
	items, err := s.ListMissingLocal(*limit)
	must(err)
	if len(items) == 0 {
		fmt.Println("missing-local: none")
		return
	}
	for _, item := range items {
		fmt.Printf("path=%s rev=%s content_hash=%s size=%d state=%s\n", item.Path, item.Rev, item.ContentHash, item.Size, item.State)
	}
	if *enqueueReview {
		plan := reconcile.BuildDeleteReviewPlan(items, nil)
		created := 0
		for _, op := range plan.Ops {
			if _, ok, err := s.EnqueueOpIfMissing(op.Op, op.Path, op); err != nil {
				must(err)
			} else if ok {
				created++
			}
		}
		fmt.Printf("review_ops_enqueued=%d\n", created)
	}
}

func cmdConflicts(args []string) {
	fs := flag.NewFlagSet("conflicts", flag.ExitOnError)
	db := fs.String("db", filepath.Join(os.Getenv("HOME"), "Dropbox", defaultDB), "state database path")
	limit := fs.Int("limit", 50, "maximum conflicts to list")
	_ = fs.Parse(args)
	s, err := state.Open(*db)
	must(err)
	defer s.Close()
	must(s.Init())
	items, err := s.ListConflicts(*limit)
	must(err)
	if len(items) == 0 {
		fmt.Println("conflicts: none")
		return
	}
	for _, c := range items {
		fmt.Printf("id=%d path=%s reason=%s local_path=%s remote_rev=%s created_at=%s\n", c.ID, c.Path, c.Reason, c.LocalPath, c.RemoteRev, c.CreatedAt.Format(time.RFC3339))
	}
}

func cmdResolveConflict(args []string) {
	fs := flag.NewFlagSet("resolve-conflict", flag.ExitOnError)
	db := fs.String("db", filepath.Join(os.Getenv("HOME"), "Dropbox", defaultDB), "state database path")
	id := fs.Int64("id", 0, "conflict id to mark resolved")
	note := fs.String("note", "", "resolution note")
	_ = fs.Parse(args)
	if *id <= 0 {
		fatal("resolve-conflict requires --id")
	}
	s, err := state.Open(*db)
	must(err)
	defer s.Close()
	must(s.Init())
	ok, err := s.ResolveConflict(*id, *note)
	must(err)
	if !ok {
		fatal("conflict id %d not found", *id)
	}
	fmt.Printf("resolved conflict id=%d db=%s\n", *id, *db)
}

func cmdPause(args []string, paused bool) {
	fs := flag.NewFlagSet("pause", flag.ExitOnError)
	db := fs.String("db", filepath.Join(os.Getenv("HOME"), "Dropbox", defaultDB), "state database path")
	_ = fs.Parse(args)
	s, err := state.Open(*db)
	must(err)
	defer s.Close()
	must(s.Init())
	must(s.SetPaused(paused))
	if paused {
		fmt.Printf("paused db=%s\n", *db)
		return
	}
	fmt.Printf("resumed db=%s\n", *db)
}

func cmdHash(args []string) {
	if len(args) != 1 {
		fmt.Println("usage: hash PATH")
		os.Exit(2)
	}
	h, err := hash.DropboxContentHash(args[0])
	must(err)
	fmt.Println(h)
}

func cmdAuth(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: auth status|save|device-code|pkce")
		os.Exit(2)
	}
	switch args[0] {
	case "status":
		cmdAuthStatus(args[1:])
	case "save":
		cmdAuthSave(args[1:])
	case "device-code":
		cmdAuthDeviceCode(args[1:])
	case "pkce":
		cmdAuthPKCE(args[1:])
	default:
		fmt.Println("usage: auth status|save|device-code|pkce")
		os.Exit(2)
	}
}

func cmdAuthStatus(args []string) {
	fs := flag.NewFlagSet("auth status", flag.ExitOnError)
	tokenFile := fs.String("token-file", "", "secure token file path")
	_ = fs.Parse(args)
	path := *tokenFile
	if path == "" {
		var err error
		path, err = auth.DefaultTokenPath()
		must(err)
	}
	st, err := auth.TokenStatus(path)
	must(err)
	if !st.Present {
		fmt.Printf("token: missing path=%s\n", st.Path)
		return
	}
	fmt.Printf("token: present path=%s token_type=%s account_id=%s has_refresh=%v expires_at=%s scope=%s\n", st.Path, st.TokenType, st.AccountID, st.HasRefresh, st.ExpiresAt.Format(time.RFC3339), st.Scope)
}

func cmdAuthSave(args []string) {
	fs := flag.NewFlagSet("auth save", flag.ExitOnError)
	tokenEnv := fs.String("token-env", "DROPBOX_TOKEN", "environment variable containing Dropbox token")
	tokenFile := fs.String("token-file", "", "secure token file path")
	_ = fs.Parse(args)
	path := *tokenFile
	if path == "" {
		var err error
		path, err = auth.DefaultTokenPath()
		must(err)
	}
	token := os.Getenv(*tokenEnv)
	if token == "" {
		fatal("missing token env %s", *tokenEnv)
	}
	must(auth.SaveToken(path, auth.Token{AccessToken: token}))
	fmt.Printf("token saved path=%s\n", path)
}

func cmdAuthDeviceCode(args []string) {
	fs := flag.NewFlagSet("auth device-code", flag.ExitOnError)
	clientID := fs.String("client-id", os.Getenv("DROPBOX_CLIENT_ID"), "Dropbox app key/client id")
	tokenFile := fs.String("token-file", "", "secure token file path")
	_ = fs.Parse(args)
	if *clientID == "" {
		fatal("missing Dropbox client id; pass --client-id or set DROPBOX_CLIENT_ID")
	}
	path := *tokenFile
	if path == "" {
		var err error
		path, err = auth.DefaultTokenPath()
		must(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	client := dropbox.NewOAuthClient(*clientID)
	code, err := client.StartDeviceCode(ctx)
	must(err)
	verify := code.VerificationURI
	if code.VerificationURIComplete != "" {
		verify = code.VerificationURIComplete
	}
	fmt.Printf("open: %s\ncode: %s\n", verify, code.UserCode)
	interval := code.Interval
	if interval <= 0 {
		interval = 5
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			fatal("device-code auth timed out: %v", ctx.Err())
		case <-ticker.C:
			tok, err := client.PollDeviceToken(ctx, code.DeviceCode)
			if err != nil {
				if oe, ok := err.(dropbox.OAuthError); ok && (oe.Code == "authorization_pending" || oe.Code == "slow_down") {
					continue
				}
				must(err)
			}
			must(auth.SaveToken(path, auth.TokenFromDropbox(tok.AccessToken, tok.RefreshToken, tok.TokenType, tok.ExpiresIn, tok.AccountID, tok.Scope, time.Now())))
			fmt.Printf("token saved path=%s account_id=%s scope=%s\n", path, tok.AccountID, tok.Scope)
			return
		}
	}
}

func cmdAuthPKCE(args []string) {
	fs := flag.NewFlagSet("auth pkce", flag.ExitOnError)
	clientID := fs.String("client-id", os.Getenv("DROPBOX_CLIENT_ID"), "Dropbox app key/client id")
	redirectURI := fs.String("redirect-uri", "", "OAuth redirect URI registered with Dropbox")
	redirectPort := fs.Int("redirect-port", 17653, "localhost callback port used when --redirect-uri is omitted")
	tokenFile := fs.String("token-file", "", "secure token file path")
	_ = fs.Parse(args)
	if *clientID == "" {
		fatal("missing Dropbox client id; pass --client-id or set DROPBOX_CLIENT_ID")
	}
	path := *tokenFile
	if path == "" {
		var err error
		path, err = auth.DefaultTokenPath()
		must(err)
	}
	uri := *redirectURI
	if uri == "" {
		uri = fmt.Sprintf("http://127.0.0.1:%d/callback", *redirectPort)
	}
	parsed, err := url.Parse(uri)
	must(err)
	if parsed.Scheme != "http" || parsed.Host == "" || parsed.Path == "" {
		fatal("redirect URI must be a localhost http URL with a callback path, got %s", uri)
	}
	if parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost" {
		fatal("auth pkce callback only listens on localhost; got host %s", parsed.Hostname())
	}
	stateValue, err := dropbox.GenerateCodeVerifier()
	must(err)
	client := dropbox.NewOAuthClient(*clientID)
	pkce, err := client.StartPKCEAuth(uri, stateValue, []string{"account_info.read", "files.metadata.read", "files.content.read", "files.content.write"})
	must(err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(parsed.Path, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("state"); got != stateValue {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errCh <- fmt.Errorf("oauth state mismatch")
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			errCh <- fmt.Errorf("oauth callback missing code")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, "<html><body><h1>Dropbox connected</h1><p>You can close this tab and return to the terminal.</p></body></html>")
		codeCh <- code
	})
	server := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", parsed.Host)
	must(err)
	defer ln.Close()
	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	fmt.Printf("open: %s\nwaiting for Dropbox callback on %s\n", pkce.AuthorizeURL, uri)
	var code string
	select {
	case <-ctx.Done():
		fatal("pkce auth timed out: %v", ctx.Err())
	case err := <-errCh:
		must(err)
	case code = <-codeCh:
	}
	tok, err := client.ExchangePKCECode(ctx, code, pkce.CodeVerifier, uri)
	must(err)
	must(auth.SaveToken(path, auth.TokenFromDropbox(tok.AccessToken, tok.RefreshToken, tok.TokenType, tok.ExpiresIn, tok.AccountID, tok.Scope, time.Now())))
	fmt.Printf("token saved path=%s account_id=%s scope=%s\n", path, tok.AccountID, tok.Scope)
}

func cmdRemoteAccount(args []string) {
	fs := flag.NewFlagSet("remote-account", flag.ExitOnError)
	tokenEnv := fs.String("token-env", "DROPBOX_TOKEN", "environment variable containing Dropbox token")
	tokenFile := fs.String("token-file", "", "secure token file path")
	_ = fs.Parse(args)
	token := loadAccessToken(*tokenFile, *tokenEnv)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	acct, err := dropbox.New(token).CurrentAccount(ctx)
	must(err)
	fmt.Printf("account_id=%s name=%s email=%s\n", acct.AccountID, acct.Name.DisplayName, acct.Email)
}

func cmdSync(args []string) {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	dry := fs.Bool("dry-run", true, "do not write remote/local changes")
	once := fs.Bool("once", true, "run one sync cycle")
	db := fs.String("db", filepath.Join(os.Getenv("HOME"), "Dropbox", defaultDB), "state database path")
	rootFlag := fs.String("root", "", "local sync root override")
	remote := fs.Bool("remote", false, "also fetch full Dropbox remote metadata for dry-run reconciliation")
	remoteDelta := fs.Bool("remote-delta", false, "fetch incremental Dropbox remote metadata using the stored cursor")
	remotePath := fs.String("remote-path", "", "Dropbox remote path to list")
	tokenEnv := fs.String("token-env", "DROPBOX_TOKEN", "environment variable containing Dropbox token")
	tokenFile := fs.String("token-file", "", "secure token file path")
	_ = fs.Parse(args)
	if !*once {
		fatal("continuous sync is not enabled yet; use --once")
	}
	if !*dry {
		fatal("live sync is not enabled yet; use --dry-run")
	}
	if *remote && *remoteDelta {
		fatal("choose only one remote mode: --remote or --remote-delta")
	}
	s, err := state.Open(*db)
	must(err)
	defer s.Close()
	must(s.Init())
	root := *rootFlag
	if root == "" {
		root, err = s.GetConfig("root")
		must(err)
	}
	if root == "" {
		fatal("missing sync root; run init --root PATH or pass --root PATH")
	}
	root, err = filepath.Abs(root)
	must(err)
	files, err := scan.Walk(root, scan.DefaultOptions())
	must(err)
	for _, f := range files {
		must(s.UpsertEntry(state.Entry{
			Path:        scan.DropboxPath(f.Path),
			ContentHash: f.ContentHash,
			Size:        f.Size,
			MTime:       f.ModTime,
			State:       "local_scanned",
		}))
	}
	remoteFiles := 0
	var remoteEntries []dropbox.Metadata
	if *remote || *remoteDelta {
		token := loadAccessToken(*tokenFile, *tokenEnv)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		client := dropbox.New(token)
		if *remoteDelta {
			stats, err := s.IngestRemoteDelta(ctx, client, *remotePath)
			must(err)
			remoteFiles = stats.AppliedFiles
		} else {
			entries, cursor, err := client.ListFolderAll(ctx, *remotePath, true)
			must(err)
			remoteEntries = entries
			applied, err := s.ApplyRemoteMetadata(entries)
			must(err)
			remoteFiles = applied
			must(s.SetConfig(state.DropboxCursorKey, cursor))
		}
	}
	planOps, planConflicts, planNoop := 0, 0, 0
	if *remote {
		plan := reconcile.BuildDryRunPlan(files, remoteEntries)
		planNoop = plan.Noop
		for _, op := range plan.Ops {
			if _, created, err := s.EnqueueOpIfMissing(op.Op, op.Path, op); err != nil {
				must(err)
			} else if created {
				planOps++
			}
		}
		for _, c := range plan.Conflicts {
			if _, created, err := s.AddConflictIfMissing(c.Path, c.Reason, c.LocalPath, c.RemoteRev); err != nil {
				must(err)
			} else if created {
				planConflicts++
			}
		}
	}
	must(s.Event("sync.dry_run.scan", fmt.Sprintf("root=%s local_files=%d remote_files=%d planned_ops=%d conflicts=%d noop=%d", root, len(files), remoteFiles, planOps, planConflicts, planNoop)))
	fmt.Printf("dry-run scan complete: root=%s local_files=%d remote_files=%d planned_ops=%d conflicts=%d noop=%d db=%s\n", root, len(files), remoteFiles, planOps, planConflicts, planNoop, *db)
}

func cmdSmokeTest(args []string) {
	fs := flag.NewFlagSet("smoke-test", flag.ExitOnError)
	dry := fs.Bool("dry-run", false, "run without Dropbox/local destructive writes")
	live := fs.Bool("live", false, "upload a throwaway file to Dropbox")
	ackRisk := fs.Bool("i-understand-risk", false, "required with --live")
	remotePath := fs.String("remote-path", "/OpenClaw-Smoke-Test", "Dropbox throwaway folder/path prefix")
	tokenEnv := fs.String("token-env", "DROPBOX_TOKEN", "environment variable containing Dropbox token")
	tokenFile := fs.String("token-file", "", "secure token file path")
	_ = fs.Parse(args)
	if *dry && *live {
		fatal("choose exactly one mode: --dry-run or --live")
	}
	if !*dry && !*live {
		fatal("smoke-test requires --dry-run unless --live is explicitly enabled")
	}
	if *live && !*ackRisk {
		fatal("live smoke-test requires --i-understand-risk")
	}
	root := mustTempDir("umbrel-dropbox-client-smoke-root-*")
	db := filepath.Join(root, ".umbrel-dropbox-client", "state.db")
	must(os.MkdirAll(filepath.Dir(db), 0700))
	local := filepath.Join(root, "smoke.txt")
	body := []byte("umbrel-dropbox-client smoke test\n")
	must(os.WriteFile(local, body, 0600))
	s, err := state.Open(db)
	must(err)
	defer s.Close()
	must(s.Init())
	must(s.SetConfig("root", root))
	files, err := scan.Walk(root, scan.DefaultOptions())
	must(err)
	if len(files) != 1 {
		fatal("expected 1 smoke file, got %d", len(files))
	}
	dropboxPath := pathJoinDropbox(*remotePath, "smoke.txt")
	planned := reconcile.PlannedOp{Op: "upload_local", Path: dropboxPath, LocalPath: local, ContentHash: files[0].ContentHash, Size: files[0].Size, Reason: "smoke test upload"}
	_, created, err := s.EnqueueOpIfMissing(planned.Op, planned.Path, planned)
	must(err)
	if !created {
		fatal("smoke op unexpectedly already existed")
	}
	if *dry {
		p := worker.Processor{Store: s, Handler: worker.DryRunHandler{Store: s}}
		res, err := p.ProcessOne(context.Background())
		must(err)
		if !res.Processed || !res.Completed || res.Failed {
			fatal("dry-run smoke worker failed: processed=%v completed=%v failed=%v", res.Processed, res.Completed, res.Failed)
		}
		fmt.Printf("smoke-test dry-run ok: root=%s db=%s path=%s\n", root, db, dropboxPath)
		return
	}
	token := loadAccessToken(*tokenFile, *tokenEnv)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	p := worker.Processor{Store: s, Handler: worker.TransferHandler{Store: s, Client: dropbox.New(token), Root: root, AllowLive: true}}
	res, err := p.ProcessOne(ctx)
	must(err)
	if !res.Processed || !res.Completed || res.Failed {
		fatal("live smoke worker failed: processed=%v completed=%v failed=%v", res.Processed, res.Completed, res.Failed)
	}
	fmt.Printf("smoke-test live upload ok: root=%s db=%s path=%s\n", root, db, dropboxPath)
}

func mustTempDir(pattern string) string {
	path, err := os.MkdirTemp("", pattern)
	must(err)
	return path
}

func pathJoinDropbox(base, name string) string {
	base = filepath.ToSlash(base)
	if base == "" || base == "." || base == "/" {
		return "/" + name
	}
	base = "/" + strings.Trim(strings.TrimSpace(base), "/")
	return base + "/" + strings.TrimLeft(name, "/")
}

func cmdWorker(args []string) {
	fs := flag.NewFlagSet("worker", flag.ExitOnError)
	dry := fs.Bool("dry-run", false, "validate and complete queued work without local or remote writes")
	live := fs.Bool("live", false, "execute guarded live upload/download transfers")
	executeReviewedDeletes := fs.Bool("execute-reviewed-deletes", false, "allow pending review_*_delete ops to execute under live gates")
	ackRisk := fs.Bool("i-understand-risk", false, "required with --live")
	once := fs.Bool("once", true, "process ready queue items once")
	db := fs.String("db", filepath.Join(os.Getenv("HOME"), "Dropbox", defaultDB), "state database path")
	rootFlag := fs.String("root", "", "local sync root override")
	limit := fs.Int("limit", 1, "maximum ready operations to process")
	tokenEnv := fs.String("token-env", "DROPBOX_TOKEN", "environment variable containing Dropbox token")
	tokenFile := fs.String("token-file", "", "secure token file path")
	_ = fs.Parse(args)
	if !*once {
		fatal("continuous worker mode is not enabled yet; use --once")
	}
	if *live && *dry {
		fatal("choose exactly one mode: --dry-run or --live")
	}
	if !*live && !*dry {
		fatal("worker requires --dry-run unless --live is explicitly enabled")
	}
	if *live && !*ackRisk {
		fatal("live worker mode requires --i-understand-risk")
	}
	if *limit < 1 {
		fatal("limit must be >= 1")
	}
	s, err := state.Open(*db)
	must(err)
	defer s.Close()
	must(s.Init())
	handler := worker.Handler(worker.DryRunHandler{Store: s})
	mode := "dry_run"
	if *live {
		root := *rootFlag
		if root == "" {
			root, err = s.GetConfig("root")
			must(err)
		}
		if root == "" {
			fatal("missing sync root; run init --root PATH or pass --root PATH")
		}
		token := ""
		if *tokenFile != "" {
			tok, err := auth.LoadToken(*tokenFile)
			must(err)
			token = tok.AccessToken
		} else {
			token = os.Getenv(*tokenEnv)
		}
		if token == "" {
			fatal("missing access token; pass --token-file or set %s", *tokenEnv)
		}
		dbx := dropbox.New(token)
		handler = worker.LiveHandler{
			Transfer: worker.TransferHandler{Store: s, Client: dbx, Root: root, AllowLive: true},
			Deletes:  worker.ReviewedDeleteHandler{Store: s, Client: dbx, AllowLive: true, AllowReviewedDeletes: *executeReviewedDeletes},
		}
		mode = "live"
	}
	p := worker.Processor{Store: s, Handler: handler}
	processed, completed, failed := 0, 0, 0
	for processed < *limit {
		res, err := p.ProcessOne(context.Background())
		must(err)
		if !res.Processed {
			break
		}
		processed++
		if res.Completed {
			completed++
		}
		if res.Failed {
			failed++
		}
	}
	must(s.Event("worker."+mode+".batch", fmt.Sprintf("processed=%d completed=%d failed=%d limit=%d", processed, completed, failed, *limit)))
	fmt.Printf("%s worker complete: processed=%d completed=%d failed=%d db=%s\n", mode, processed, completed, failed, *db)
}

func loadAccessToken(tokenFile, tokenEnv string) string {
	if tokenFile != "" {
		tok, err := auth.LoadToken(tokenFile)
		must(err)
		return tok.AccessToken
	}
	token := os.Getenv(tokenEnv)
	if token == "" {
		fatal("missing access token; pass --token-file or set %s", tokenEnv)
	}
	return token
}

func must(err error) {
	if err != nil {
		fatal("%v", err)
	}
}
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
