package state

import (
	"database/sql"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }
type Status struct {
	Root       string
	Paused     bool
	Entries    int
	PendingOps int
	Conflicts  int
	LastEvent  string
}

func Open(path string) (*Store, error) {
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Init() error {
	stmts := []string{
		`create table if not exists config(key text primary key, value text not null)`,
		`create table if not exists entries(path text primary key, dropbox_id text, rev text, content_hash text, size integer, mtime_unix integer, state text not null default 'clean')`,
		`create table if not exists pending_ops(id integer primary key autoincrement, op text not null, path text not null, payload text, created_at text not null, attempts integer not null default 0, status text not null default 'pending', retry_at text, last_error text, completed_at text)`,
		`create table if not exists conflicts(id integer primary key autoincrement, path text not null, reason text not null, local_path text, remote_rev text, created_at text not null)`,
		`create table if not exists events(id integer primary key autoincrement, type text not null, detail text, created_at text not null)`,
	}
	for _, st := range stmts {
		if _, err := s.db.Exec(st); err != nil {
			return err
		}
	}
	migrations := []string{
		`alter table pending_ops add column status text not null default 'pending'`,
		`alter table pending_ops add column retry_at text`,
		`alter table pending_ops add column last_error text`,
		`alter table pending_ops add column completed_at text`,
	}
	for _, st := range migrations {
		if _, err := s.db.Exec(st); err != nil && !isDuplicateColumn(err) {
			return err
		}
	}
	return nil
}
func (s *Store) SetConfig(k, v string) error {
	_, err := s.db.Exec(`insert into config(key,value) values(?,?) on conflict(key) do update set value=excluded.value`, k, v)
	return err
}
func (s *Store) GetConfig(k string) (string, error) {
	var v string
	err := s.db.QueryRow(`select value from config where key=?`, k).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}
func (s *Store) Event(t, d string) error {
	_, err := s.db.Exec(`insert into events(type,detail,created_at) values(?,?,?)`, t, d, time.Now().Format(time.RFC3339))
	return err
}

func (s *Store) SetPaused(paused bool) error {
	value := "false"
	event := "resume"
	if paused {
		value = "true"
		event = "pause"
	}
	if err := s.SetConfig("paused", value); err != nil {
		return err
	}
	return s.Event(event, "")
}

func (s *Store) IsPaused() (bool, error) {
	v, err := s.GetConfig("paused")
	if err != nil {
		return false, err
	}
	return strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes"), nil
}
func (s *Store) Status() (Status, error) {
	var st Status
	_ = s.db.QueryRow(`select value from config where key='root'`).Scan(&st.Root)
	st.Paused, _ = s.IsPaused()
	_ = s.db.QueryRow(`select count(*) from entries`).Scan(&st.Entries)
	_ = s.db.QueryRow(`select count(*) from pending_ops where status = 'pending'`).Scan(&st.PendingOps)
	_ = s.db.QueryRow(`select count(*) from conflicts`).Scan(&st.Conflicts)
	_ = s.db.QueryRow(`select coalesce(max(created_at),'') from events`).Scan(&st.LastEvent)
	return st, nil
}

type EventItem struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) ListEvents(limit int) ([]EventItem, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`select id, type, detail, created_at from events order by id desc limit ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EventItem
	for rows.Next() {
		var e EventItem
		if err := rows.Scan(&e.ID, &e.Type, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func isDuplicateColumn(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "duplicate column")
}
