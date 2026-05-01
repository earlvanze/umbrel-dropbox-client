package state

import (
	"database/sql"
	"time"
	_ "modernc.org/sqlite"
)

type Store struct { db *sql.DB }
type Status struct { Root string; Entries int; PendingOps int; Conflicts int; LastEvent string }
func Open(path string) (*Store, error) { db, err := sql.Open("sqlite", path); if err != nil { return nil, err }; return &Store{db:db}, nil }
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Init() error {
	stmts := []string{
		`create table if not exists config(key text primary key, value text not null)`,
		`create table if not exists entries(path text primary key, dropbox_id text, rev text, content_hash text, size integer, mtime_unix integer, state text not null default 'clean')`,
		`create table if not exists pending_ops(id integer primary key autoincrement, op text not null, path text not null, payload text, created_at text not null, attempts integer not null default 0)`,
		`create table if not exists conflicts(id integer primary key autoincrement, path text not null, reason text not null, local_path text, remote_rev text, created_at text not null)`,
		`create table if not exists events(id integer primary key autoincrement, type text not null, detail text, created_at text not null)`,
	}
	for _, st := range stmts { if _, err := s.db.Exec(st); err != nil { return err } }
	return nil
}
func (s *Store) SetConfig(k,v string) error { _, err := s.db.Exec(`insert into config(key,value) values(?,?) on conflict(key) do update set value=excluded.value`, k, v); return err }
func (s *Store) Event(t,d string) error { _, err := s.db.Exec(`insert into events(type,detail,created_at) values(?,?,?)`, t, d, time.Now().Format(time.RFC3339)); return err }
func (s *Store) Status() (Status, error) {
	var st Status
	_ = s.db.QueryRow(`select value from config where key='root'`).Scan(&st.Root)
	_ = s.db.QueryRow(`select count(*) from entries`).Scan(&st.Entries)
	_ = s.db.QueryRow(`select count(*) from pending_ops`).Scan(&st.PendingOps)
	_ = s.db.QueryRow(`select count(*) from conflicts`).Scan(&st.Conflicts)
	_ = s.db.QueryRow(`select coalesce(max(created_at),'') from events`).Scan(&st.LastEvent)
	return st, nil
}
