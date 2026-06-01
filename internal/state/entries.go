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

func (s *Store) LocalEntries() (map[string]Entry, error) {
	rows, err := s.db.Query(`select path, coalesce(dropbox_id,''), coalesce(rev,''), coalesce(content_hash,''), size, mtime_unix, state from entries where content_hash != '' and state in ('local_scanned','clean')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]Entry)
	for rows.Next() {
		var e Entry
		var mtime int64
		if err := rows.Scan(&e.Path, &e.DropboxID, &e.Rev, &e.ContentHash, &e.Size, &mtime, &e.State); err != nil {
			return nil, err
		}
		e.MTime = time.Unix(mtime, 0).UTC()
		out[e.Path] = e
	}
	return out, rows.Err()
}

// UpsertEntryIfChanged upserts the entry only if content_hash, size, or mtime
// differ from the stored row. Returns true if a row was inserted or updated.
func (s *Store) UpsertEntryIfChanged(e Entry) (bool, error) {
	e.Path = normalizeEntryPath(e.Path)
	if e.Path == "" {
		return false, fmt.Errorf("state: upsert entry path normalizes to empty")
	}
	state := e.State
	if state == "" {
		state = "clean"
	}
	var existingHash string
	var existingSize int64
	var existingMtime int64
	var existingState string
	err := s.db.QueryRow(
		`select coalesce(content_hash,''), size, mtime_unix, state from entries where path = ?`,
		e.Path,
	).Scan(&existingHash, &existingSize, &existingMtime, &existingState)
	if err == sql.ErrNoRows {
		_, err = s.db.Exec(`insert into entries(path,dropbox_id,rev,content_hash,size,mtime_unix,state) values(?,?,?,?,?,?,?)`,
			e.Path, e.DropboxID, e.Rev, e.ContentHash, e.Size, e.MTime.Unix(), state)
		return true, err
	}
	if err != nil {
		return false, err
	}
	if existingHash == e.ContentHash && existingSize == e.Size && existingMtime == e.MTime.Unix() && existingState == state {
		return false, nil
	}
	_, err = s.db.Exec(`update entries set dropbox_id=?, rev=?, content_hash=?, size=?, mtime_unix=?, state=? where path=?`,
		e.DropboxID, e.Rev, e.ContentHash, e.Size, e.MTime.Unix(), state, e.Path)
	return true, err
}

// UpsertBatch upserts entries, skipping rows where content_hash, size, and
// mtime are unchanged. Returns the number of rows actually inserted or updated.
func (s *Store) UpsertBatch(entries []Entry) (int, error) {
	changed := 0
	for _, e := range entries {
		didChange, err := s.UpsertEntryIfChanged(e)
		if err != nil {
			return changed, err
		}
		if didChange {
			changed++
		}
	}
	return changed, nil
}

func (s *Store) DeleteEntry(path string) error {
	path = normalizeEntryPath(path)
	_, err := s.db.Exec(`delete from entries where path = ?`, path)
	return err
}
