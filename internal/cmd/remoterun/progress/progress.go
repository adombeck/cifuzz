package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mitchellh/ioprogress"
	"github.com/pkg/errors"
)

func NewReader(reader io.Reader, size int64, successMessage string) *ioprogress.Reader {
	return &ioprogress.Reader{
		Reader:       reader,
		Size:         size,
		DrawFunc:     DrawProgressBar(os.Stdout, ioprogress.DrawTextFormatBar(60), successMessage),
		DrawInterval: 100 * time.Millisecond,
	}
}

func DrawProgressBar(w io.Writer, drawFormatBar ioprogress.DrawTextFormatFunc, successMessage string) ioprogress.DrawFunc {
	var maxLength int

	return func(progress, total int64) error {
		if progress == -1 && total == -1 {
			// Progress completed, so we clear the utils.progress bar and print the success message
			_, err := fmt.Fprintln(w, strings.Repeat(" ", maxLength)+"\r"+successMessage)
			return errors.WithStack(err)
		}

		line := drawFormatBar(progress, total)

		// Make sure we pad the line to the max length we've ever drawn so that
		// we don't have trailing characters.
		if len(line) < maxLength {
			line += strings.Repeat(" ", maxLength-len(line))
		}

		maxLength = len(line)
		_, err := fmt.Fprint(w, line+"\r")
		return errors.WithStack(err)
	}
}
