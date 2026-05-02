package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/earl/umbrel-dropbox-sync/internal/auth"
	"github.com/earl/umbrel-dropbox-sync/internal/dropbox"
	"github.com/earl/umbrel-dropbox-sync/internal/hash"
	"github.com/earl/umbrel-dropbox-sync/internal/reconcile"
	"github.com/earl/umbrel-dropbox-sync/internal/scan"
	"github.com/earl/umbrel-dropbox-sync/internal/state"
	"github.com/earl/umbrel-dropbox-sync/internal/worker"
)

const defaultDB = ".umbrel-dropbox-sync/state.db"

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
	case "hash":
		cmdHash(os.Args[2:])
	case "auth":
		cmdAuth(os.Args[2:])
	case "remote-account":
		cmdRemoteAccount(os.Args[2:])
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
	fmt.Println(`umbrel-dropbox-sync

Commands:
  init --root PATH [--db PATH]
  status [--db PATH]
  hash PATH
  auth status [--token-file PATH]
  auth save --token-env DROPBOX_TOKEN [--token-file PATH]
  auth device-code --client-id APP_KEY [--token-file PATH]
  remote-account --token-env DROPBOX_TOKEN
  sync --once --dry-run [--db PATH] [--root PATH] [--remote] [--remote-path PATH] [--token-env DROPBOX_TOKEN]
  worker --once --dry-run [--db PATH] [--limit N]

MVP scaffold. Uses DROPBOX_TOKEN or --token-env for API calls.`)
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
	_ = fs.Parse(args)
	abs, err := filepath.Abs(*root)
	must(err)
	must(os.MkdirAll(filepath.Dir(dbPath(abs, *db)), 0700))
	s, err := state.Open(dbPath(abs, *db))
	must(err)
	defer s.Close()
	must(s.Init())
	must(s.SetConfig("root", abs))
	must(s.Event("init", abs))
	fmt.Printf("initialized root=%s db=%s\n", abs, dbPath(abs, *db))
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
	fmt.Printf("root: %s\nentries: %d\npending_ops: %d\nconflicts: %d\nlast_event: %s\n", st.Root, st.Entries, st.PendingOps, st.Conflicts, st.LastEvent)
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
		fmt.Println("usage: auth status|save")
		os.Exit(2)
	}
	switch args[0] {
	case "status":
		cmdAuthStatus(args[1:])
	case "save":
		cmdAuthSave(args[1:])
	case "device-code":
		cmdAuthDeviceCode(args[1:])
	default:
		fmt.Println("usage: auth status|save")
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

func cmdRemoteAccount(args []string) {
	fs := flag.NewFlagSet("remote-account", flag.ExitOnError)
	tokenEnv := fs.String("token-env", "DROPBOX_TOKEN", "environment variable containing Dropbox token")
	_ = fs.Parse(args)
	token := os.Getenv(*tokenEnv)
	if token == "" {
		fatal("missing token env %s", *tokenEnv)
	}
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
	remote := fs.Bool("remote", false, "also fetch Dropbox remote metadata for dry-run reconciliation")
	remotePath := fs.String("remote-path", "", "Dropbox remote path to list")
	tokenEnv := fs.String("token-env", "DROPBOX_TOKEN", "environment variable containing Dropbox token")
	_ = fs.Parse(args)
	if !*once {
		fatal("continuous sync is not enabled yet; use --once")
	}
	if !*dry {
		fatal("live sync is not enabled yet; use --dry-run")
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
	if *remote {
		token := os.Getenv(*tokenEnv)
		if token == "" {
			fatal("missing token env %s for --remote", *tokenEnv)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		entries, cursor, err := dropbox.New(token).ListFolderAll(ctx, *remotePath, true)
		must(err)
		remoteEntries = entries
		for _, e := range entries {
			if e.Tag != "file" {
				continue
			}
			path := e.PathLower
			if path == "" {
				path = e.PathDisplay
			}
			must(s.UpsertEntry(state.Entry{
				Path:        path,
				DropboxID:   e.ID,
				Rev:         e.Rev,
				ContentHash: e.ContentHash,
				Size:        e.Size,
				MTime:       e.ServerMtime,
				State:       "remote_scanned",
			}))
			remoteFiles++
		}
		must(s.SetConfig("dropbox_cursor", cursor))
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

func cmdWorker(args []string) {
	fs := flag.NewFlagSet("worker", flag.ExitOnError)
	dry := fs.Bool("dry-run", true, "validate and complete queued work without local or remote writes")
	once := fs.Bool("once", true, "process ready queue items once")
	db := fs.String("db", filepath.Join(os.Getenv("HOME"), "Dropbox", defaultDB), "state database path")
	limit := fs.Int("limit", 1, "maximum ready operations to process")
	_ = fs.Parse(args)
	if !*once {
		fatal("continuous worker mode is not enabled yet; use --once")
	}
	if !*dry {
		fatal("live worker mode is not enabled yet; use --dry-run")
	}
	if *limit < 1 {
		fatal("limit must be >= 1")
	}
	s, err := state.Open(*db)
	must(err)
	defer s.Close()
	must(s.Init())
	p := worker.Processor{Store: s, Handler: worker.DryRunHandler{Store: s}}
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
	must(s.Event("worker.dry_run.batch", fmt.Sprintf("processed=%d completed=%d failed=%d limit=%d", processed, completed, failed, *limit)))
	fmt.Printf("dry-run worker complete: processed=%d completed=%d failed=%d db=%s\n", processed, completed, failed, *db)
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
