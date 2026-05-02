package state

import (
	"database/sql"
	"time"
)

type ConflictRecord struct {
	ID        int64
	Path      string
	Reason    string
	LocalPath string
	RemoteRev string
	CreatedAt time.Time
}

func (s *Store) ListConflicts(limit int) ([]ConflictRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`select id, path, reason, coalesce(local_path,''), coalesce(remote_rev,''), created_at from conflicts order by id asc limit ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConflictRecord
	for rows.Next() {
		var c ConflictRecord
		var created string
		if err := rows.Scan(&c.ID, &c.Path, &c.Reason, &c.LocalPath, &c.RemoteRev, &created); err != nil {
			return nil, err
		}
		c.CreatedAt = parseTime(created)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ResolveConflict(id int64, note string) (bool, error) {
	var path string
	err := s.db.QueryRow(`select path from conflicts where id = ?`, id).Scan(&path)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	res, err := s.db.Exec(`delete from conflicts where id = ?`, id)
	if err != nil {
		return false, err
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if deleted == 0 {
		return false, nil
	}
	detail := path
	if note != "" {
		detail += " note=" + note
	}
	if err := s.Event("conflict.resolve", detail); err != nil {
		return true, err
	}
	return true, nil
}
