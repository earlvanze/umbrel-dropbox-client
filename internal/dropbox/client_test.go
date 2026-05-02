package dropbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListFolderAllUsesContinueAndAuth(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization header = %q", got)
		}
		seen = append(seen, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/files/list_folder":
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req["path"] != "/Root" || req["recursive"] != true {
				t.Fatalf("unexpected list_folder request: %#v", req)
			}
			_, _ = w.Write([]byte(`{"entries":[{".tag":"file","name":"a.txt","path_lower":"/root/a.txt","rev":"r1","size":3,"content_hash":"h1"}],"cursor":"c1","has_more":true}`))
		case "/files/list_folder/continue":
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req["cursor"] != "c1" {
				t.Fatalf("cursor = %q", req["cursor"])
			}
			_, _ = w.Write([]byte(`{"entries":[{".tag":"file","name":"b.txt","path_lower":"/root/b.txt","rev":"r2","size":4,"content_hash":"h2"}],"cursor":"c2","has_more":false}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	entries, cursor, err := NewWithHTTP("test-token", srv.Client(), srv.URL).ListFolderAll(context.Background(), "/Root", true)
	if err != nil {
		t.Fatal(err)
	}
	if cursor != "c2" || len(entries) != 2 || entries[0].Rev != "r1" || entries[1].Rev != "r2" {
		t.Fatalf("unexpected entries/cursor: cursor=%q entries=%#v", cursor, entries)
	}
	if len(seen) != 2 || seen[0] != "/files/list_folder" || seen[1] != "/files/list_folder/continue" {
		t.Fatalf("unexpected call order: %#v", seen)
	}
}

func TestListFolderLatestCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files/list_folder/get_latest_cursor" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization header = %q", got)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req["path"] != "/Root" || req["recursive"] != true {
			t.Fatalf("request=%#v", req)
		}
		_, _ = w.Write([]byte(`{"cursor":"latest-cursor"}`))
	}))
	defer srv.Close()
	cursor, err := NewWithHTTP("test-token", srv.Client(), srv.URL).ListFolderLatestCursor(context.Background(), "/Root", true)
	if err != nil {
		t.Fatal(err)
	}
	if cursor != "latest-cursor" {
		t.Fatalf("cursor=%q", cursor)
	}
}
