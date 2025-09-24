package pipebuf

import (
	"io"
	"sync"
)

var (
	_ io.Reader     = (*PipeReader)(nil)
	_ io.WriterTo   = (*PipeReader)(nil)
	_ io.Closer     = (*PipeReader)(nil)
	_ io.Writer     = (*PipeWriter)(nil)
	_ io.ReaderFrom = (*PipeWriter)(nil)
	_ io.Closer     = (*PipeWriter)(nil)
)

type pipe struct {
	readerClosedErr error
	writerClosedErr error

	writerWait sync.Cond
	readerWait sync.Cond

	buffer   []byte
	writePos int
	readPos  int
	mu       sync.Mutex

	readerClosed bool
	writerClosed bool
}

func newPipe(size int) *pipe {
	p := &pipe{buffer: make([]byte, 0, size)}
	p.writerWait.L = &p.mu
	p.readerWait.L = &p.mu
	return p
}

func (p *pipe) Read(b []byte) (n int, err error) {
	if len(b) == 0 {
		return 0, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.waitForReadableLocked(); err != nil {
		return 0, err
	}

	wasFull := p.full()
	n = p.readChunkLocked(b)

	if wasFull {
		p.writerWait.Signal()
	}

	return n, nil
}

func (p *pipe) Write(b []byte) (n int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for len(b) > 0 {
		if err := p.waitForWritableLocked(); err != nil {
			return n, err
		}
		wasEmpty := p.empty()
		wrote := p.writeChunkLocked(b)
		b = b[wrote:]
		n += wrote
		if wasEmpty {
			p.readerWait.Signal()
		}
	}
	return n, nil
}

func (p *pipe) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeReaderLocked(nil, false)
	return nil
}

func (p *pipe) empty() bool {
	return p.readPos == len(p.buffer)
}

func (p *pipe) full() bool {
	return p.readPos < len(p.buffer) && p.readPos == p.writePos
}

func (p *pipe) closeReaderLocked(err error, withErr bool) {
	p.readerClosed = true
	if withErr && p.writerClosedErr == nil {
		if err == nil {
			err = io.ErrClosedPipe
		}
		p.writerClosedErr = err
	}
	p.readerWait.Broadcast()
	p.writerWait.Broadcast()
}

func (p *pipe) closeWriterLocked(err error, withErr bool) {
	p.writerClosed = true
	if withErr && p.readerClosedErr == nil {
		if err == nil {
			err = io.EOF
		}
		p.readerClosedErr = err
	}
	p.readerWait.Broadcast()
	p.writerWait.Broadcast()
}

func (p *pipe) closeWrite() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeWriterLocked(nil, false)
	return nil
}

func (p *pipe) readChunkLocked(dst []byte) int {
	n := copy(dst, p.buffer[p.readPos:len(p.buffer)])
	p.readPos += n
	if p.readPos == cap(p.buffer) {
		p.readPos = 0
		p.buffer = p.buffer[:p.writePos]
	}
	return n
}

func (p *pipe) writeChunkLocked(src []byte) int {
	end := cap(p.buffer)
	if p.writePos < p.readPos {
		end = p.readPos
	}

	available := min(end-p.writePos, len(src))
	requiredLen := p.writePos + available
	if requiredLen > len(p.buffer) {
		p.buffer = p.buffer[:requiredLen]
	}

	copy(p.buffer[p.writePos:p.writePos+available], src[:available])
	p.writePos += available
	if p.writePos == cap(p.buffer) {
		p.writePos = 0
	}
	return available
}

func (p *pipe) waitForReadableLocked() error {
	for {
		if !p.empty() {
			return nil
		}
		if p.readerClosed {
			if p.writerClosedErr != nil {
				return p.writerClosedErr
			}
			return io.ErrClosedPipe
		}
		if p.writerClosed {
			if p.readerClosedErr != nil {
				return p.readerClosedErr
			}
			return io.EOF
		}
		p.readerWait.Wait()
	}
}

func (p *pipe) waitForWritableLocked() error {
	for {
		if p.readerClosed {
			if p.writerClosedErr != nil {
				return p.writerClosedErr
			}
			return io.ErrClosedPipe
		}
		if p.writerClosed {
			return io.ErrClosedPipe
		}
		if !p.full() {
			return nil
		}
		p.writerWait.Wait()
	}
}

// Pipe creates a buffered pipe with the specified buffer size.
func Pipe(bufferSize int) (*PipeReader, *PipeWriter) {
	if bufferSize <= 0 {
		bufferSize = 1
	}
	p := newPipe(bufferSize)
	return &PipeReader{p}, &PipeWriter{p}
}

// PipeReader is the read half of a pipe.
type PipeReader struct {
	p *pipe
}

// Read implements io.Reader.
func (r *PipeReader) Read(b []byte) (int, error) {
	return r.p.Read(b)
}

// Close closes the reader side of the pipe.
func (r *PipeReader) Close() error {
	return r.p.Close()
}

// WriteTo implements io.WriterTo by reading data from the pipe
// and writing it to w until EOF or an error occurs.
func (r *PipeReader) WriteTo(w io.Writer) (n int64, err error) {
	return copyBuffered(r.Read, w.Write)
}

// CloseWithError closes the reader side of the pipe with an error.
// The error will be returned to future writes on the writer side.
func (r *PipeReader) CloseWithError(err error) error {
	r.p.mu.Lock()
	defer r.p.mu.Unlock()
	r.p.closeReaderLocked(err, true)
	return nil
}

// PipeWriter is the write half of a pipe.
type PipeWriter struct {
	p *pipe
}

// Write implements io.Writer.
func (w *PipeWriter) Write(b []byte) (int, error) {
	return w.p.Write(b)
}

// ReadFrom implements io.ReaderFrom by reading data from r
// and writing it to the pipe until EOF or an error occurs.
func (w *PipeWriter) ReadFrom(r io.Reader) (n int64, err error) {
	return copyBuffered(r.Read, w.Write)
}

// Close closes the writer side of the pipe.
func (w *PipeWriter) Close() error {
	return w.p.closeWrite()
}

// CloseWithError closes the writer side of the pipe with an error.
// The error will be returned to future reads on the reader side.
func (w *PipeWriter) CloseWithError(err error) error {
	w.p.mu.Lock()
	defer w.p.mu.Unlock()
	w.p.closeWriterLocked(err, true)
	return nil
}

func copyBuffered(read func([]byte) (int, error), write func([]byte) (int, error)) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, rErr := read(buf)
		if n > 0 {
			wn, wErr := write(buf[:n])
			if wn < 0 || wn > n {
				wn = 0
				if wErr == nil {
					wErr = io.ErrShortWrite
				}
			}
			total += int64(wn)
			if wErr != nil {
				return total, wErr
			}
			if wn != n {
				return total, io.ErrShortWrite
			}
		}
		if rErr != nil {
			if rErr != io.EOF {
				return total, rErr
			}
			return total, nil
		}
	}
}
