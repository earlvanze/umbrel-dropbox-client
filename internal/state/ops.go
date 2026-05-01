package state

import (
	"database/sql"
	"encoding/json"
	"time"
)

type PendingOp struct {
	ID        int64
	Op        string
	Path      string
	Payload   string
	CreatedAt time.Time
	Attempts  int
}

func (s *Store) EnqueueOp(op, path string, payload any) (int64, error) {
	var raw string
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return 0, err
		}
		raw = string(b)
	}
	res, err := s.db.Exec(`insert into pending_ops(op,path,payload,created_at) values(?,?,?,?)`, op, path, raw, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) NextPendingOp() (*PendingOp, error) {
	row := s.db.QueryRow(`select id, op, path, coalesce(payload,''), created_at, attempts from pending_ops order by attempts asc, id asc limit 1`)
	var out PendingOp
	var created string
	if err := row.Scan(&out.ID, &out.Op, &out.Path, &out.Payload, &created, &out.Attempts); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if t, err := time.Parse(time.RFC3339Nano, created); err == nil {
		out.CreatedAt = t
	} else if t, err := time.Parse(time.RFC3339, created); err == nil {
		out.CreatedAt = t
	}
	return &out, nil
}

func (s *Store) MarkOpAttempt(id int64) error {
	_, err := s.db.Exec(`update pending_ops set attempts = attempts + 1 where id = ?`, id)
	return err
}

func (s *Store) CompleteOp(id int64) error {
	_, err := s.db.Exec(`delete from pending_ops where id = ?`, id)
	return err
}

func (s *Store) AddConflict(path, reason, localPath, remoteRev string) (int64, error) {
	res, err := s.db.Exec(`insert into conflicts(path,reason,local_path,remote_rev,created_at) values(?,?,?,?,?)`, path, reason, localPath, remoteRev, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}
