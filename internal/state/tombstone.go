package state

import "strings"

type MissingLocal struct {
	Path        string
	DropboxID   string
	Rev         string
	ContentHash string
	Size        int64
	State       string
}

func (s *Store) MarkMissingLocal(seen map[string]bool) (int, error) {
	normalizedSeen := make(map[string]bool, len(seen))
	for path, ok := range seen {
		if ok {
			normalizedSeen[normalizeEntryPath(path)] = true
		}
	}
	rows, err := s.db.Query(`select path from entries where state in ('local_scanned','clean')`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var missing []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return 0, err
		}
		if !normalizedSeen[normalizeEntryPath(path)] {
			missing = append(missing, path)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, path := range missing {
		if _, err := s.db.Exec(`update entries set state = 'local_missing' where path = ?`, path); err != nil {
			return 0, err
		}
	}
	return len(missing), nil
}


// MarkMissingLocalInDirs marks entries as local_missing only if they are
// under one of the specified directory prefixes. This is used during
// incremental scans to avoid scanning the entire entries table.
func (s *Store) MarkMissingLocalInDirs(seen map[string]bool, dirPrefixes []string) (int, error) {
	normalizedSeen := make(map[string]bool, len(seen))
	for path, ok := range seen {
		if ok {
			normalizedSeen[normalizeEntryPath(path)] = true
		}
	}
	// Build LIKE clauses for each directory prefix
	var conditions []string
	var args []any
	for _, dir := range dirPrefixes {
		prefix := normalizeEntryPath(dir)
		if prefix == "" {
			conditions = append(conditions, "1=1")
		} else {
			conditions = append(conditions, "(path = ? OR path LIKE ?)")
			args = append(args, prefix, prefix+"/%")
		}
	}
	where := "state in ('local_scanned','clean')"
	if len(conditions) > 0 {
		where = where + " AND (" + strings.Join(conditions, " OR ") + ")"
	}
	query := "select path from entries where " + where
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var missing []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return 0, err
		}
		if !normalizedSeen[normalizeEntryPath(path)] {
			missing = append(missing, path)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, path := range missing {
		if _, err := s.db.Exec(`update entries set state = 'local_missing' where path = ?`, path); err != nil {
			return 0, err
		}
	}
	return len(missing), nil
}

func (s *Store) ListMissingLocal(limit int) ([]MissingLocal, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`select path, coalesce(dropbox_id,''), coalesce(rev,''), coalesce(content_hash,''), size, state from entries where state = 'local_missing' order by path asc limit ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MissingLocal
	for rows.Next() {
		var item MissingLocal
		if err := rows.Scan(&item.Path, &item.DropboxID, &item.Rev, &item.ContentHash, &item.Size, &item.State); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
