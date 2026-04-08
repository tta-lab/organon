package diff

import (
	"fmt"
	"io"

	"github.com/aymanbagabas/go-udiff"
)

// Show writes a unified diff between old and new content to w.
// filename is used in the diff header ("--- a/<filename>" / "+++ b/<filename>").
// Returns nil without output if old and new are identical.
func Show(w io.Writer, old, new []byte, filename string) error {
	unified := udiff.Unified("a/"+filename, "b/"+filename, string(old), string(new))
	if unified == "" {
		return nil
	}
	_, err := fmt.Fprint(w, unified)
	return err
}
