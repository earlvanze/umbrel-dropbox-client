package dropbox

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadFileSendsDropboxAPIArgAndBody(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "a.txt")
	if err := os.WriteFile(local, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files/upload" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("auth=%q", got)
		}
		var arg map[string]any
		if err := json.Unmarshal([]byte(r.Header.Get("Dropbox-API-Arg")), &arg); err != nil {
			t.Fatal(err)
		}
		if arg["path"] != "/a.txt" || arg["mode"] != "add" || arg["autorename"] != false {
			t.Fatalf("arg=%#v", arg)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "hello" {
			t.Fatalf("body=%q", body)
		}
		_ = json.NewEncoder(w).Encode(Metadata{Tag: "file", PathLower: "/a.txt", Rev: "r1", Size: 5})
	}))
	defer srv.Close()

	meta, err := NewWithHTTP("token", srv.Client(), srv.URL).UploadFile(context.Background(), "/a.txt", local)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Rev != "r1" || meta.Size != 5 {
		t.Fatalf("meta=%#v", meta)
	}
}

func TestDownloadFileUsesDropboxAPIArgAndAtomicRename(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "nested", "a.txt")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files/download" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		var arg map[string]any
		if err := json.Unmarshal([]byte(r.Header.Get("Dropbox-API-Arg")), &arg); err != nil {
			t.Fatal(err)
		}
		if arg["path"] != "/a.txt" {
			t.Fatalf("arg=%#v", arg)
		}
		w.Header().Set("Dropbox-API-Result", `{".tag":"file","path_lower":"/a.txt","rev":"r2","size":7}`)
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	meta, err := NewWithHTTP("token", srv.Client(), srv.URL).DownloadFile(context.Background(), "/a.txt", local)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Rev != "r2" || meta.Size != 7 {
		t.Fatalf("meta=%#v", meta)
	}
	body, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "payload" {
		t.Fatalf("body=%q", body)
	}
}
