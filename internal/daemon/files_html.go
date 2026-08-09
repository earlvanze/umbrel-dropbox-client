package daemon

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// serveFilesHTML renders a simple HTML file manager for the sync root.
// Used as the /files page in the v1.0 dashboard; the v1.2 dashboard
// renders its own tab-based file manager so this is a fallback.
func (d *Daemon) serveFilesHTML(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	clean, err := d.sanitizeBrowsePath(rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entries, err := d.browseListing(clean)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	parent := filepath.ToSlash(filepath.Dir(clean))
	if parent == "." {
		parent = ""
	}
	crumbs := d.breadcrumbs(clean)
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>SyncNest - Files</title>`)
	b.WriteString(`<style>body{font-family:sans-serif;max-width:900px;margin:1em auto;padding:0 1em}a{color:#0a66c2;text-decoration:none}a:hover{text-decoration:underline}table{border-collapse:collapse;width:100%}td,th{text-align:left;padding:.4em .6em;border-bottom:1px solid #eee}tr.dir td:first-child::before{content:"📁 "}tr.file td:first-child::before{content:"📄 "}.crumbs{margin:.5em 0 1em}</style>`)
	b.WriteString(`</head><body>`)
	b.WriteString(`<h1>Files</h1><div class="crumbs">`)
	b.WriteString(`<a href="/files">/</a> / `)
	for i, c := range crumbs {
		if i > 0 {
			b.WriteString(" / ")
		}
		u := url.URL{Path: "/files", RawQuery: "path=" + url.QueryEscape(strings.Join(crumbs[:i+1], "/"))}
		b.WriteString(`<a href="` + u.String() + `">` + html.EscapeString(c) + `</a>`)
	}
	b.WriteString(`</div><table><tr><th>Name</th><th>Size</th><th>Modified</th></tr>`)
	if clean != "" {
		pu := url.URL{Path: "/files", RawQuery: "path=" + url.QueryEscape(parent)}
		b.WriteString(`<tr class="dir"><td><a href="` + pu.String() + `">..</a></td><td></td><td></td></tr>`)
	}
	for _, e := range entries {
		var href string
		if e.IsDir {
			href = "/files?path=" + url.QueryEscape(filepath.ToSlash(filepath.Join(clean, e.Name)))
		} else {
			href = "/download?path=" + url.QueryEscape(filepath.ToSlash(filepath.Join(clean, e.Name)))
		}
		b.WriteString(`<tr class="` + map[bool]string{true: "dir", false: "file"}[e.IsDir] + `">`)
		b.WriteString(`<td><a href="` + href + `">` + html.EscapeString(e.Name) + `</a></td>`)
		b.WriteString(`<td>` + html.EscapeString(formatBytes(e.Size, e.IsDir)) + `</td>`)
		b.WriteString(`<td>` + e.ModTime.Format("2006-01-02 15:04:05") + `</td>`)
		b.WriteString(`</tr>`)
	}
	b.WriteString(`</table></body></html>`)
	_, _ = w.Write([]byte(b.String()))
}

func (d *Daemon) sanitizeBrowsePath(rel string) (string, error) {
	rootAbs, err := filepath.Abs(d.cfg.Root)
	if err != nil {
		return "", err
	}
	if rel == "" {
		return "", nil
	}
	clean := filepath.Clean("/" + filepath.ToSlash(rel))
	full := filepath.Join(rootAbs, clean)
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	relToRoot, err := filepath.Rel(rootAbs, fullAbs)
	if err != nil {
		return "", err
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes configured root")
	}
	return relToRoot, nil
}

type browseEntry struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

func (d *Daemon) browseListing(rel string) ([]browseEntry, error) {
	rootAbs, err := filepath.Abs(d.cfg.Root)
	if err != nil {
		return nil, err
	}
	target := rootAbs
	if rel != "" {
		target = filepath.Join(rootAbs, rel)
	}
	f, err := os.Open(target)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	var out []browseEntry
	for _, n := range names {
		if n == ".umbrel-dropbox-client" {
			continue
		}
		info, err := os.Stat(filepath.Join(target, n))
		if err != nil {
			continue
		}
		out = append(out, browseEntry{Name: n, IsDir: info.IsDir(), Size: info.Size(), ModTime: info.ModTime()})
	}
	return out, nil
}

func (d *Daemon) breadcrumbs(rel string) []string {
	if rel == "" {
		return nil
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	out := make([]string, 0, len(parts))
	for i := range parts {
		out = append(out, strings.Join(parts[:i+1], "/"))
	}
	return out
}
func formatBytes(size int64, isDir bool) string {
	if isDir {
		return ""
	}
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}
