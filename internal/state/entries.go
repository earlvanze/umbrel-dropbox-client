package state

import "time"

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
