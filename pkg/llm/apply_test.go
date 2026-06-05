package llm

import (
	"testing"
)

func TestApplyEdits(t *testing.T) {
	tests := []struct {
		name    string
		content string
		edits   []Edit
		want    string
		err     bool
	}{
		{
			name:    "replace middle lines",
			content: "line1\nline2\nline3\nline4\nline5",
			edits:   []Edit{{Start: 1, End: 3, New: "newA\nnewB"}},
			want:    "line1\nnewA\nnewB\nline4\nline5",
		},
		{
			name:    "delete lines",
			content: "line1\nline2\nline3\nline4",
			edits:   []Edit{{Start: 2, End: 4, New: ""}},
			want:    "line1\nline2",
		},
		{
			name:    "insert at end",
			content: "a\nb",
			edits:   []Edit{{Start: 2, End: 2, New: "c\nd"}},
			want:    "a\nb\nc\nd",
		},
		{
			name:    "two non-overlapping edits",
			content: "1\n2\n3\n4\n5\n6",
			edits:   []Edit{{Start: 0, End: 1, New: "A"}, {Start: 3, End: 4, New: "B"}},
			want:    "A\n2\n3\nB\n5\n6",
		},
		{
			name:    "overlapping edits",
			content: "1\n2\n3\n4",
			edits:   []Edit{{Start: 1, End: 3, New: "A"}, {Start: 2, End: 3, New: "B"}},
			err:     true,
		},
		{
			name:    "out of range",
			content: "a\nb\nc",
			edits:   []Edit{{Start: 0, End: 5, New: "X"}},
			err:     true,
		},
		{
			name:    "negative start",
			content: "a\nb",
			edits:   []Edit{{Start: -1, End: 1, New: "X"}},
			err:     true,
		},
		{
			name:    "reversed range",
			content: "a\nb",
			edits:   []Edit{{Start: 2, End: 1, New: "X"}},
			err:     true,
		},
		{
			name:    "empty edits",
			content: "a\nb",
			edits:   []Edit{},
			err:     true,
		},
		{
			name:    "multiline replacement with newlines in New",
			content: "1\n2\n3\n4\n5",
			edits:   []Edit{{Start: 1, End: 3, New: "A\nB\nC"}},
			want:    "1\nA\nB\nC\n4\n5",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyEdits(tt.content, tt.edits)
			if tt.err {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}
