package dropbox

import "strings"

// NormalizeDropboxPath canonicalizes paths for case-sensitive API requests
// while matching Dropbox path_lower semantics.
func NormalizeDropboxPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if path == "" || path == "." || path == "/" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}
	return strings.ToLower(path)
}
