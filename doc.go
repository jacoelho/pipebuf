package pipebuf

// Package pipebuf exposes a pipe abstraction that inserts a configurable ring buffer between
// the writer and the reader. It mirrors io.Pipe semantics while giving bursty producers a short
// cushion so writers do not stall when the reader lags for a moment.
