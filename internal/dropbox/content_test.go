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

func TestUploadFileAtRevisionUsesDropboxUpdateMode(t *testing.T) {
	local := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(local, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var arg struct {
			Path string `json:"path"`
			Mode struct {
				Tag    string `json:".tag"`
				Update string `json:"update"`
			} `json:"mode"`
			StrictConflict bool `json:"strict_conflict"`
		}
		if err := json.Unmarshal([]byte(r.Header.Get("Dropbox-API-Arg")), &arg); err != nil {
			t.Fatal(err)
		}
		if arg.Path != "/a.txt" || arg.Mode.Tag != "update" || arg.Mode.Update != "r1" || !arg.StrictConflict {
			t.Fatalf("arg=%#v", arg)
		}
		_ = json.NewEncoder(w).Encode(Metadata{Tag: "file", Rev: "r2"})
	}))
	defer srv.Close()
	meta, err := NewWithHTTP("token", srv.Client(), srv.URL).UploadFileAtRevision(context.Background(), "/a.txt", local, "r1")
	if err != nil || meta.Rev != "r2" {
		t.Fatalf("meta=%#v err=%v", meta, err)
	}
}

func TestGetMetadataDistinguishesFoundFromMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files/get_metadata" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		var arg map[string]any
		if err := json.NewDecoder(r.Body).Decode(&arg); err != nil {
			t.Fatal(err)
		}
		if arg["path"] == "/missing.txt" {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":{".tag":"path","path":{".tag":"not_found"}}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(Metadata{Tag: "file", Rev: "r1", ContentHash: "hash"})
	}))
	defer srv.Close()
	c := NewWithHTTP("token", srv.Client(), srv.URL)
	meta, err := c.GetMetadata(context.Background(), "/exists.txt")
	if err != nil || meta == nil || meta.Rev != "r1" {
		t.Fatalf("found=%#v err=%v", meta, err)
	}
	meta, err = c.GetMetadata(context.Background(), "/missing.txt")
	if err != nil || meta != nil {
		t.Fatalf("missing=%#v err=%v", meta, err)
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
