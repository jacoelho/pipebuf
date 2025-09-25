package pipebuf

import (
	"fmt"
	"io"
	"os"
)

func ExamplePipe() {
	r, w := Pipe(32 * 1024)
	defer r.Close()
	defer w.Close()

	go func() {
		defer w.Close()
		for i := range 5 {
			fmt.Fprintf(w, "message %d\n", i)
		}
	}()

	_, _ = io.Copy(os.Stdout, r)
	// Output:
	// message 0
	// message 1
	// message 2
	// message 3
	// message 4
}
