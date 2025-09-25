# pipebuf

pipebuf is a small Go package that works like io.Pipe but adds a configurable in-memory ring buffer. The buffer absorbs short bursts so the writer does not block on every byte when the reader lags. This behavior helps when producers sometimes outpace consumers (for example logging systems, metrics pipelines, or batch encoders).

## Why pick pipebuf over io.Pipe?

io.Pipe forces the writer to wait until the reader drains each write. That strict hand-off fits continuous streaming but hurts when bursts appear. The writer stalls and the system loses throughput. pipebuf inserts a ring buffer so the writer gets a brief cushion. When the buffer fills, the behavior falls back to the normal blocking semantics, so existing code keeps the same guarantees.

## Quick start

```go
package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/jacoelho/pipebuf"
)

func main() {
	r, w := pipebuf.Pipe(32 * 1024) // 32 KB buffer
	defer r.Close()
	defer w.Close()

	go func() {
		defer w.Close()
		fmt.Fprintln(w, "bursty producer data")
	}()

	if _, err := io.Copy(os.Stdout, r); err != nil {
		log.Fatal(err)
	}
}
```
