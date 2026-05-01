package conflict

import "testing"

func TestDecideConcurrentEditsConflict(t *testing.T) {
	res := Decide(Side{Exists: true, ContentHash: "base", Rev: "1"}, Side{Exists: true, ContentHash: "local"}, Side{Exists: true, ContentHash: "remote", Rev: "2"})
	if res.Decision != RecordConflict {
		t.Fatalf("expected conflict, got %#v", res)
	}
}

func TestDecideNewLocalUpload(t *testing.T) {
	res := Decide(Side{}, Side{Exists: true, ContentHash: "local"}, Side{})
	if res.Decision != UploadLocal {
		t.Fatalf("expected upload, got %#v", res)
	}
}

func TestDecideMatchingHashesNoop(t *testing.T) {
	res := Decide(Side{}, Side{Exists: true, ContentHash: "same"}, Side{Exists: true, ContentHash: "same"})
	if res.Decision != Noop {
		t.Fatalf("expected noop, got %#v", res)
	}
}
