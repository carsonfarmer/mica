package app

import "testing"

func TestSessionLogPaths(t *testing.T) {
	root := "/tmp/mica"

	if got, want := SessionsDir(root), "/tmp/mica/.mica/sessions"; got != want {
		t.Fatalf("SessionsDir() = %q, want %q", got, want)
	}
	if got, want := SessionLogFile(root, "sess"), "/tmp/mica/.mica/sessions/sess.jsonl"; got != want {
		t.Fatalf("SessionLogFile() = %q, want %q", got, want)
	}
}
