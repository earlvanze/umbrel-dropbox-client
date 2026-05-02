package worker

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/earl/umbrel-dropbox-sync/internal/state"
)

func TestProcessOneCompletesSuccessfulOp(t *testing.T) {
	s := testStore(t)
	id, err := s.EnqueueOp("upload_local", "/ok.txt", map[string]string{"local_path": "/tmp/ok.txt"})
	if err != nil {
		t.Fatal(err)
	}

	var handled state.PendingOp
	p := Processor{
		Store: s,
		Handler: HandlerFunc(func(_ context.Context, op state.PendingOp) error {
			handled = op
			return nil
		}),
		Now: fixedNow,
	}
	res, err := p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Processed || !res.Completed || res.OpID != id {
		t.Fatalf("result=%#v want completed id=%d", res, id)
	}
	if handled.ID != id || handled.Path != "/ok.txt" {
		t.Fatalf("handled=%#v", handled)
	}
	got, err := s.PendingOpByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("completed op still present: %#v", got)
	}
}

func TestProcessOneRetriesWithExponentialBackoff(t *testing.T) {
	s := testStore(t)
	id, err := s.EnqueueOp("upload_local", "/retry.txt", nil)
	if err != nil {
		t.Fatal(err)
	}

	p := Processor{
		Store:       s,
		Handler:     HandlerFunc(func(context.Context, state.PendingOp) error { return errors.New("temporary") }),
		MaxAttempts: 3,
		BaseBackoff: 2 * time.Second,
		MaxBackoff:  time.Minute,
		Now:         fixedNow,
	}
	res, err := p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Processed || res.Completed || res.Failed || !res.RetryAt.Equal(fixedNow().Add(2*time.Second)) {
		t.Fatalf("result=%#v", res)
	}
	got, err := s.PendingOpByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attempts != 1 || got.Status != "pending" || got.LastError != "temporary" || !got.RetryAt.Equal(fixedNow().Add(2*time.Second)) {
		t.Fatalf("op=%#v", got)
	}

	res, err = p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Processed {
		t.Fatalf("future retry should not process: %#v", res)
	}
}

func TestProcessOneHonorsRetryAfterError(t *testing.T) {
	s := testStore(t)
	id, err := s.EnqueueOp("download_remote", "/limited.txt", nil)
	if err != nil {
		t.Fatal(err)
	}

	p := Processor{
		Store: s,
		Handler: HandlerFunc(func(context.Context, state.PendingOp) error {
			return RetryAfterError{After: 17 * time.Second, Err: errors.New("429")}
		}),
		MaxAttempts: 4,
		BaseBackoff: time.Second,
		Now:         fixedNow,
	}
	res, err := p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := fixedNow().Add(17 * time.Second)
	if !res.RetryAt.Equal(want) {
		t.Fatalf("retry_at=%s want %s", res.RetryAt, want)
	}
	got, err := s.PendingOpByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.RetryAt.Equal(want) || got.LastError != "429" {
		t.Fatalf("op=%#v", got)
	}
}

func TestProcessOneMarksTerminalFailureAfterMaxAttempts(t *testing.T) {
	s := testStore(t)
	id, err := s.EnqueueOp("upload_local", "/fail.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RetryOp(id, fixedNow().Add(-time.Second), "first"); err != nil {
		t.Fatal(err)
	}

	p := Processor{
		Store:       s,
		Handler:     HandlerFunc(func(context.Context, state.PendingOp) error { return errors.New("still broken") }),
		MaxAttempts: 2,
		Now:         fixedNow,
	}
	res, err := p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Processed || !res.Failed || res.Completed {
		t.Fatalf("result=%#v", res)
	}
	got, err := s.PendingOpByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Status != "failed" || got.Attempts != 2 || got.LastError != "still broken" || got.Completed.IsZero() {
		t.Fatalf("op=%#v", got)
	}
	next, err := s.NextReadyPendingOp(fixedNow().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if next != nil {
		t.Fatalf("failed op should not be ready: %#v", next)
	}
}

func testStore(t *testing.T) *state.Store {
	t.Helper()
	s, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	return s
}

func fixedNow() time.Time {
	return time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
}
