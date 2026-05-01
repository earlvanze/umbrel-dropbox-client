package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/earl/umbrel-dropbox-sync/internal/dropbox"
	"github.com/earl/umbrel-dropbox-sync/internal/hash"
	"github.com/earl/umbrel-dropbox-sync/internal/scan"
	"github.com/earl/umbrel-dropbox-sync/internal/state"
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
	case "remote-account":
		cmdRemoteAccount(os.Args[2:])
	case "sync":
		cmdSync(os.Args[2:])
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
  remote-account --token-env DROPBOX_TOKEN
  sync --once --dry-run [--db PATH] [--root PATH]

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
	must(s.Event("sync.dry_run.local_scan", fmt.Sprintf("root=%s files=%d", root, len(files))))
	fmt.Printf("dry-run local scan complete: root=%s files=%d db=%s\n", root, len(files), *db)
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
