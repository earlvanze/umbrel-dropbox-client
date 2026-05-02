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
	Status    string
	RetryAt   time.Time
	LastError string
	Completed time.Time
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
	res, err := s.db.Exec(`insert into pending_ops(op,path,payload,created_at,status) values(?,?,?,?,?)`, op, path, raw, time.Now().UTC().Format(time.RFC3339Nano), "pending")
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) NextPendingOp() (*PendingOp, error) {
	return s.NextReadyPendingOp(time.Now().UTC())
}

func (s *Store) NextReadyPendingOp(now time.Time) (*PendingOp, error) {
	row := s.db.QueryRow(`select id, op, path, coalesce(payload,''), created_at, attempts, status, coalesce(retry_at,''), coalesce(last_error,''), coalesce(completed_at,'') from pending_ops where status = 'pending' and (retry_at is null or retry_at = '' or retry_at <= ?) order by attempts asc, id asc limit 1`, now.UTC().Format(time.RFC3339Nano))
	var out PendingOp
	var created, retryAt, completedAt string
	if err := row.Scan(&out.ID, &out.Op, &out.Path, &out.Payload, &created, &out.Attempts, &out.Status, &retryAt, &out.LastError, &completedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	out.CreatedAt = parseTime(created)
	out.RetryAt = parseTime(retryAt)
	out.Completed = parseTime(completedAt)
	return &out, nil
}

func (s *Store) MarkOpAttempt(id int64) error {
	_, err := s.db.Exec(`update pending_ops set attempts = attempts + 1 where id = ?`, id)
	return err
}

func (s *Store) RetryOp(id int64, retryAt time.Time, lastErr string) error {
	_, err := s.db.Exec(`update pending_ops set attempts = attempts + 1, retry_at = ?, last_error = ?, status = 'pending' where id = ?`, retryAt.UTC().Format(time.RFC3339Nano), lastErr, id)
	return err
}

func (s *Store) FailOp(id int64, lastErr string) error {
	_, err := s.db.Exec(`update pending_ops set attempts = attempts + 1, status = 'failed', last_error = ?, completed_at = ? where id = ?`, lastErr, time.Now().UTC().Format(time.RFC3339Nano), id)
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

func (s *Store) EnqueueOpIfMissing(op, path string, payload any) (int64, bool, error) {
	var id int64
	err := s.db.QueryRow(`select id from pending_ops where op = ? and path = ? and status = 'pending' limit 1`, op, path).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, err
	}
	id, err = s.EnqueueOp(op, path, payload)
	return id, true, err
}

func (s *Store) PendingOpByID(id int64) (*PendingOp, error) {
	row := s.db.QueryRow(`select id, op, path, coalesce(payload,''), created_at, attempts, status, coalesce(retry_at,''), coalesce(last_error,''), coalesce(completed_at,'') from pending_ops where id = ?`, id)
	var out PendingOp
	var created, retryAt, completedAt string
	if err := row.Scan(&out.ID, &out.Op, &out.Path, &out.Payload, &created, &out.Attempts, &out.Status, &retryAt, &out.LastError, &completedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	out.CreatedAt = parseTime(created)
	out.RetryAt = parseTime(retryAt)
	out.Completed = parseTime(completedAt)
	return &out, nil
}

func parseTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t
	}
	return time.Time{}
}

func (s *Store) AddConflictIfMissing(path, reason, localPath, remoteRev string) (int64, bool, error) {
	var id int64
	err := s.db.QueryRow(`select id from conflicts where path = ? and reason = ? limit 1`, path, reason).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, err
	}
	id, err = s.AddConflict(path, reason, localPath, remoteRev)
	return id, true, err
}
