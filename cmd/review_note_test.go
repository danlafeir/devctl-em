package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEditorCommand_SplitsMultiWordEditor(t *testing.T) {
	t.Setenv("EDITOR", "code --wait")
	cmd := editorCommand("/tmp/note.md")
	want := []string{"sh", "-c", `code --wait "$1"`, "sh", "/tmp/note.md"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("args = %v, want %v", cmd.Args, want)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Errorf("arg[%d] = %q, want %q", i, cmd.Args[i], want[i])
		}
	}
}

func TestEditorCommand_DefaultsToVi(t *testing.T) {
	t.Setenv("EDITOR", "")
	cmd := editorCommand("/tmp/note.md")
	if cmd.Args[2] != `vi "$1"` {
		t.Errorf("default editor script = %q, want %q", cmd.Args[2], `vi "$1"`)
	}
}

// A multi-word $EDITOR must actually run with the note path appended; before the
// shell fix, exec.Command treated "touch -t ..." as one missing executable.
func TestEditorCommand_MultiWordExecutesWithPath(t *testing.T) {
	target := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(target, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", "touch -t 200001010000")

	if err := editorCommand(target).Run(); err != nil {
		t.Fatalf("multi-word editor failed to run: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.ModTime().Year() != 2000 {
		t.Errorf("editor did not receive path arg; mtime year = %d", info.ModTime().Year())
	}
}
