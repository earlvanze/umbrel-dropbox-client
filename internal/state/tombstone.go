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
		if !seen[normalizeEntryPath(path)] {
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

func normalizeEntryPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if path == "" || path == "." || path == "/" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.ToLower(path)
}
