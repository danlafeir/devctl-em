package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/danlafeir/cli-go/pkg/store"
	"github.com/spf13/cobra"
)

// NoteRecord is a single review signal stored for a person.
type NoteRecord struct {
	Content   string    `json:"content"`
	Category  string    `json:"category"`  // e.g. "note", "technical", "communication", "leadership", "delivery"
	Date      string    `json:"date"`       // YYYY-MM-DD — when the observation happened
	CreatedAt time.Time `json:"created_at"` // when it was recorded
}

var (
	noteCategoryFlag string
	noteDateFlag     string
	noteListJSONFlag bool
)

var reviewNoteCmd = &cobra.Command{
	Use:   "note <person>",
	Short: "Capture a review signal for a person",
	Long: `Open $EDITOR to write a short observation about a team member and save it
as a timestamped signal. Signals are stored chronologically and can be
passed to an agent for review synthesis.

Categories: note (default), technical, communication, leadership, delivery

Examples:
  em review note "Alice Smith"
  em review note "Alice Smith" --category technical --date 2026-05-15`,
	Args: cobra.ExactArgs(1),
	RunE: runReviewNote,
}

var reviewNoteListCmd = &cobra.Command{
	Use:   "list <person>",
	Short: "List all review signals for a person",
	Args:  cobra.ExactArgs(1),
	RunE:  runReviewNoteList,
}

func init() {
	reviewNoteCmd.Flags().StringVar(&noteCategoryFlag, "category", "note", "Signal category (note, technical, communication, leadership, delivery)")
	reviewNoteCmd.Flags().StringVar(&noteDateFlag, "date", "", "Observation date YYYY-MM-DD (default: today)")
	reviewNoteListCmd.Flags().BoolVar(&noteListJSONFlag, "json", false, "Output as JSON for agent consumption")

	reviewNoteCmd.AddCommand(reviewNoteListCmd)
	reviewCmd.AddCommand(reviewNoteCmd)
}

func runReviewNote(cmd *cobra.Command, args []string) error {
	person, err := resolveReviewPerson(args[0])
	if err != nil {
		return err
	}

	date := strings.TrimSpace(noteDateFlag)
	if date == "" {
		date = time.Now().Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", date); err != nil {
		return fmt.Errorf("invalid --date %q: must be YYYY-MM-DD", date)
	}

	content, err := openEditorForNote(person, noteCategoryFlag, date)
	if err != nil {
		return err
	}
	if content == "" {
		fmt.Println("Cancelled.")
		return nil
	}

	s, err := store.New(filepath.Join(emConfigDir(), "review", person.slug))
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	rec := NoteRecord{
		Content:   content,
		Category:  noteCategoryFlag,
		Date:      date,
		CreatedAt: time.Now(),
	}
	if err := store.Open[NoteRecord](s, "notes").Write(rec); err != nil {
		return fmt.Errorf("saving note: %w", err)
	}

	fmt.Printf("Note saved (category: %s, date: %s).\n", rec.Category, rec.Date)
	return nil
}

func runReviewNoteList(cmd *cobra.Command, args []string) error {
	person, err := resolveReviewPerson(args[0])
	if err != nil {
		return err
	}

	s, err := store.New(filepath.Join(emConfigDir(), "review", person.slug))
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	notes, err := store.Open[NoteRecord](s, "notes").ReadAll()
	if err != nil {
		return fmt.Errorf("reading notes: %w", err)
	}

	if len(notes) == 0 {
		fmt.Printf("No notes for %s.\n", person.displayName)
		return nil
	}

	sort.Slice(notes, func(i, j int) bool {
		if notes[i].Date != notes[j].Date {
			return notes[i].Date < notes[j].Date
		}
		return notes[i].CreatedAt.Before(notes[j].CreatedAt)
	})

	if noteListJSONFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(notes)
	}

	fmt.Printf("%s — %d note", person.displayName, len(notes))
	if len(notes) != 1 {
		fmt.Print("s")
	}
	fmt.Println()
	fmt.Println()
	for _, n := range notes {
		fmt.Printf("%-12s  [%-14s]  %s\n", n.Date, n.Category, n.Content)
	}
	return nil
}

// resolveReviewPerson looks up a configured person by display name or slug.
func resolveReviewPerson(nameOrSlug string) (*reviewPerson, error) {
	slug := strings.ToLower(strings.TrimSpace(nameOrSlug))
	for _, p := range getReviewPeople() {
		if p.slug == slug || strings.ToLower(p.displayName) == slug {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("person %q not found. Use 'em review add' to add them first", nameOrSlug)
}

// openEditorForNote writes a template to a temp file, opens $EDITOR, and
// returns the trimmed non-comment content after the editor exits.
func openEditorForNote(person *reviewPerson, category, date string) (string, error) {
	f, err := os.CreateTemp("", "em-note-*.md")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)

	template := fmt.Sprintf("# Note for %s | %s | %s\n#\n# Write your observation above.\n# Lines starting with '#' are stripped. Save and close to capture; leave empty to cancel.\n",
		person.displayName, date, category)
	if _, err := f.WriteString(template); err != nil {
		f.Close()
		return "", fmt.Errorf("writing template: %w", err)
	}
	f.Close()

	editorCmd := editorCommand(tmpPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr
	if err := editorCmd.Run(); err != nil {
		return "", fmt.Errorf("editor exited with error: %w", err)
	}

	raw, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("reading note file: %w", err)
	}

	return stripNoteComments(string(raw)), nil
}

// editorCommand builds the command that opens path in the user's $EDITOR.
// $EDITOR commonly carries arguments ("code --wait", "emacsclient -c", "vim -p"),
// so it is run through the shell instead of being treated as a bare executable
// name. The path is passed as a positional argument ($1) so it is never subject
// to shell word-splitting or interpolation.
func editorCommand(path string) *exec.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	return exec.Command("sh", "-c", editor+` "$1"`, "sh", path)
}

// stripNoteComments removes lines beginning with '#' and trims surrounding whitespace.
func stripNoteComments(s string) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
