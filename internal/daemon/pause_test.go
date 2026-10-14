package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/earlvanze/umbrel-dropbox-client/internal/config"
)

func TestHealthHandlerPauseResumeRoundTrip(t *testing.T) {
	s := testStore(t)
	d := New(config.Config{Root: t.TempDir(), DryRun: true}, s, nil)
	for _, tc := range []struct {
		path   string
		paused bool
	}{{"/api/pause", true}, {"/api/pause", true}, {"/api/resume", false}, {"/api/resume", false}} {
		rr := httptest.NewRecorder()
		d.HealthHandler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, tc.path, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("POST %s: status=%d body=%s", tc.path, rr.Code, rr.Body.String())
		}
		var response struct {
			Paused bool `json:"paused"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil || response.Paused != tc.paused {
			t.Fatalf("POST %s: body=%s err=%v", tc.path, rr.Body.String(), err)
		}
		paused, err := s.IsPaused()
		if err != nil || paused != tc.paused {
			t.Fatalf("POST %s: persisted paused=%v err=%v", tc.path, paused, err)
		}
		rr = httptest.NewRecorder()
		d.HealthHandler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/status", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("status HTTP %d", rr.Code)
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil || response.Paused != tc.paused {
			t.Fatalf("status: body=%s err=%v", rr.Body.String(), err)
		}
	}
}

func TestHealthHandlerPauseResumeRejectNonPost(t *testing.T) {
	for _, path := range []string{"/api/pause", "/api/resume"} {
		for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodPatch} {
			t.Run(method+path, func(t *testing.T) {
				s := testStore(t)
				initial := path == "/api/resume"
				if err := s.SetPaused(initial); err != nil {
					t.Fatal(err)
				}
				d := New(config.Config{Root: t.TempDir(), DryRun: true}, s, nil)
				rr := httptest.NewRecorder()
				d.HealthHandler().ServeHTTP(rr, httptest.NewRequest(method, path, nil))
				if rr.Code != http.StatusMethodNotAllowed {
					t.Fatalf("status=%d", rr.Code)
				}
				paused, err := s.IsPaused()
				if err != nil || paused != initial {
					t.Fatalf("state changed: paused=%v err=%v", paused, err)
				}
			})
		}
	}
}
