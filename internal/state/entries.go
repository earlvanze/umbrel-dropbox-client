package state

import (
	"database/sql"
	"fmt"
	"time"
)

type Entry struct {
	Path        string
	DropboxID   string
	Rev         string
	ContentHash string
	Size        int64
	MTime       time.Time
	State       string
}

func (s *Store) UpsertEntry(e Entry) error {
	e.Path = normalizeEntryPath(e.Path)
	if e.Path == "" {
		return fmt.Errorf("state: upsert entry path normalizes to empty")
	}
	state := e.State
	if state == "" {
		state = "clean"
	}
	_, err := s.db.Exec(`insert into entries(path,dropbox_id,rev,content_hash,size,mtime_unix,state)
		values(?,?,?,?,?,?,?)
		on conflict(path) do update set
			dropbox_id=excluded.dropbox_id,
			rev=excluded.rev,
			content_hash=excluded.content_hash,
			size=excluded.size,
			mtime_unix=excluded.mtime_unix,
			state=excluded.state`, e.Path, e.DropboxID, e.Rev, e.ContentHash, e.Size, e.MTime.Unix(), state)
	return err
}

func (s *Store) EntryByPath(path string) (*Entry, error) {
	path = normalizeEntryPath(path)
	row := s.db.QueryRow(`select path, coalesce(dropbox_id,''), coalesce(rev,''), coalesce(content_hash,''), size, mtime_unix, state from entries where path = ?`, path)
	var out Entry
	var mtime int64
	if err := row.Scan(&out.Path, &out.DropboxID, &out.Rev, &out.ContentHash, &out.Size, &mtime, &out.State); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	out.MTime = time.Unix(mtime, 0).UTC()
	return &out, nil
}

func (s *Store) DeleteEntry(path string) error {
	path = normalizeEntryPath(path)
	_, err := s.db.Exec(`delete from entries where path = ?`, path)
	return err
}
