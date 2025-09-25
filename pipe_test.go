package pipebuf_test

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/jacoelho/pipebuf"
)

func TestPipeBasic(t *testing.T) {
	r, w := newTestPipe(t, 10)

	data := []byte("hello world")
	go func() {
		mustWrite(t, w, data)
		w.Close()
	}()

	buf := make([]byte, len(data))
	mustReadFull(t, r, buf)
	if !bytes.Equal(buf, data) {
		t.Fatalf("expected %q, got %q", data, buf)
	}

	_, err := r.Read(buf)
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestPipeBuffering(t *testing.T) {
	r, w := newTestPipe(t, 5)

	data := []byte("hello")

	mustWrite(t, w, data)

	buf := make([]byte, len(data))
	mustReadFull(t, r, buf)
	if !bytes.Equal(buf, data) {
		t.Fatalf("expected %q, got %q", data, buf)
	}
}

func TestPipeBlocking(t *testing.T) {
	r, w := newTestPipe(t, 2)

	data := []byte("hello")

	var (
		wg       sync.WaitGroup
		writeErr error
	)

	wg.Go(func() {
		_, writeErr = w.Write(data)
	})

	time.Sleep(10 * time.Millisecond)

	buf := make([]byte, len(data))
	mustReadFull(t, r, buf)

	wg.Wait()
	if writeErr != nil {
		t.Fatalf("Write failed: %v", writeErr)
	}

	if !bytes.Equal(buf, data) {
		t.Fatalf("expected %q, got %q", data, buf)
	}
}

func TestWriteFailsAfterReaderClose(t *testing.T) {
	r, w := newTestPipe(t, 10)

	r.Close()

	_, err := w.Write([]byte("test"))
	expectError(t, err, io.ErrClosedPipe)
}

func TestReadAfterWriterClose(t *testing.T) {
	r, w := newTestPipe(t, 10)

	mustWrite(t, w, []byte("test"))
	w.Close()

	mustRead(t, r, []byte("test"))
	expectEOF(t, r)
}

func TestPipeRingBuffer(t *testing.T) {
	r, w := newTestPipe(t, 4)

	for range 3 {
		data := []byte("ab")
		mustWrite(t, w, data)
		mustRead(t, r, data)
	}
}

func TestPipeCloseWithError(t *testing.T) {
	t.Run("WriterCloseWithError", func(t *testing.T) {
		r, w := newTestPipe(t, 10)

		customErr := errors.New("custom write error")
		w.CloseWithError(customErr)

		buf := make([]byte, 10)
		_, err := r.Read(buf)
		expectError(t, err, customErr)
	})

	t.Run("WriterCloseWithNilError", func(t *testing.T) {
		r, w := newTestPipe(t, 10)

		w.CloseWithError(nil)

		buf := make([]byte, 10)
		_, err := r.Read(buf)
		expectError(t, err, io.EOF)
	})

	t.Run("ReaderCloseWithError", func(t *testing.T) {
		r, w := newTestPipe(t, 10)

		customErr := errors.New("custom read error")
		r.CloseWithError(customErr)

		_, err := w.Write([]byte("test"))
		expectError(t, err, customErr)
	})

	t.Run("ReaderCloseWithNilError", func(t *testing.T) {
		r, w := newTestPipe(t, 10)

		r.CloseWithError(nil)

		_, err := w.Write([]byte("test"))
		expectError(t, err, io.ErrClosedPipe)
	})

	t.Run("CloseWithErrorDoesNotOverwrite", func(t *testing.T) {
		r, w := newTestPipe(t, 10)

		firstErr := errors.New("first error")
		secondErr := errors.New("second error")

		w.CloseWithError(firstErr)
		w.CloseWithError(secondErr) // not overwrite

		buf := make([]byte, 10)
		_, err := r.Read(buf)
		expectError(t, err, firstErr)
	})
}

func TestWriteTo(t *testing.T) {
	r, w := newTestPipe(t, 10)

	input := "hello world from WriteTo"
	output := &bytes.Buffer{}

	go func() {
		defer w.Close()
		mustWrite(t, w, []byte(input))
	}()

	n, err := r.WriteTo(output)
	if err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}

	if int(n) != len(input) {
		t.Fatalf("expected to copy %d bytes, copied %d", len(input), n)
	}
	if output.String() != input {
		t.Fatalf("expected %q, got %q", input, output.String())
	}
}

func TestReadFrom(t *testing.T) {
	r, w := newTestPipe(t, 10)

	input := "hello world from ReadFrom"
	source := bytes.NewReader([]byte(input))
	output := &bytes.Buffer{}

	go func() {
		defer w.Close()
		n, err := w.ReadFrom(source)
		if err != nil {
			t.Errorf("ReadFrom failed: %v", err)
		}
		if int(n) != len(input) {
			t.Errorf("expected to copy %d bytes, copied %d", len(input), n)
		}
	}()

	n, err := io.Copy(output, r)
	if err != nil {
		t.Fatalf("io.Copy failed: %v", err)
	}

	if int(n) != len(input) {
		t.Fatalf("expected to copy %d bytes, copied %d", len(input), n)
	}
	if output.String() != input {
		t.Fatalf("expected %q, got %q", input, output.String())
	}
}

func TestIOCopy(t *testing.T) {
	r, w := newTestPipe(t, 100)

	input := "test data for io.Copy"
	output := &bytes.Buffer{}

	go func() {
		defer w.Close()
		mustWrite(t, w, []byte(input))
	}()

	n, err := io.Copy(output, r)
	if err != nil {
		t.Fatalf("io.Copy failed: %v", err)
	}

	if int(n) != len(input) {
		t.Fatalf("expected to copy %d bytes, copied %d", len(input), n)
	}
	if output.String() != input {
		t.Fatalf("expected %q, got %q", input, output.String())
	}
}

func TestIOCopyFromReader(t *testing.T) {
	r, w := newTestPipe(t, 100)

	input := "test data for ReadFrom optimization"
	source := bytes.NewReader([]byte(input))
	output := &bytes.Buffer{}

	var (
		wg      sync.WaitGroup
		copyErr error
	)

	wg.Go(func() {
		if _, err := io.Copy(output, r); err != nil {
			copyErr = err
		}
	})

	n, err := io.Copy(w, source)
	if err != nil {
		t.Fatalf("io.Copy failed: %v", err)
	}
	w.Close()

	wg.Wait()
	if copyErr != nil {
		t.Fatalf("reader copy failed: %v", copyErr)
	}

	if int(n) != len(input) {
		t.Fatalf("expected to copy %d bytes, copied %d", len(input), n)
	}
	if output.String() != input {
		t.Fatalf("expected %q, got %q", input, output.String())
	}
}

func TestReadBufferedDataAfterReaderClose(t *testing.T) {
	r, w := newTestPipe(t, 10)

	testData := "test"
	mustWrite(t, w, []byte(testData))

	r.Close()

	mustRead(t, r, []byte(testData))

	buf := make([]byte, 1)
	_, err := r.Read(buf)
	expectError(t, err, io.ErrClosedPipe)
}

func TestReadBufferedDataAfterReaderCloseWithError(t *testing.T) {
	r, w := newTestPipe(t, 10)

	testData := "test"
	mustWrite(t, w, []byte(testData))

	customErr := errors.New("custom close error")
	r.CloseWithError(customErr)

	mustRead(t, r, []byte(testData))

	buf := make([]byte, 1)
	_, err := r.Read(buf)
	expectError(t, err, customErr)
}

func TestBufferSizes(t *testing.T) {
	tests := []struct {
		name       string
		testData   string
		bufferSize int
	}{
		{"ZeroSize", "zero buffer test", 0},
		{"NegativeSize", "negative test", -1},
		{"SizeOne", "a", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w := newTestPipe(t, tt.bufferSize)

			var wg sync.WaitGroup
			var readResult []byte

			wg.Go(func() {
				buf := make([]byte, len(tt.testData))
				mustReadFull(t, r, buf)
				readResult = buf
			})

			time.Sleep(10 * time.Millisecond)
			mustWrite(t, w, []byte(tt.testData))

			wg.Wait()

			if string(readResult) != tt.testData {
				t.Fatalf("expected %q, got %q", tt.testData, string(readResult))
			}
		})
	}
}

func TestLargeDataIntegrity(t *testing.T) {
	r, w := newTestPipe(t, 1024)

	testData := make([]byte, 1024*1024)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	var wg sync.WaitGroup
	var writeErr, readErr error
	var receivedBuffer bytes.Buffer

	wg.Go(func() {
		defer w.Close()
		_, writeErr = w.Write(testData)
	})

	wg.Go(func() {
		_, readErr = io.Copy(&receivedBuffer, r)
	})

	wg.Wait()

	if writeErr != nil {
		t.Fatalf("Write failed: %v", writeErr)
	}
	if readErr != nil {
		t.Fatalf("Read failed: %v", readErr)
	}

	receivedData := receivedBuffer.Bytes()
	if !bytes.Equal(testData, receivedData) {
		t.Fatalf("Data integrity check failed")
	}
}

func TestChunkedWriteIntegrity(t *testing.T) {
	r, w := newTestPipe(t, 64)

	testData := make([]byte, 100*1024)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	var wg sync.WaitGroup
	var writeErr, readErr error
	var receivedBuffer bytes.Buffer

	wg.Go(func() {
		defer w.Close()
		chunkSize := 17
		for i := 0; i < len(testData); i += chunkSize {
			end := min(i+chunkSize, len(testData))
			_, err := w.Write(testData[i:end])
			if err != nil {
				writeErr = err
				return
			}
		}
	})

	wg.Go(func() {
		_, readErr = io.Copy(&receivedBuffer, r)
	})

	wg.Wait()

	if writeErr != nil {
		t.Fatalf("Write failed: %v", writeErr)
	}
	if readErr != nil {
		t.Fatalf("Read failed: %v", readErr)
	}

	receivedData := receivedBuffer.Bytes()
	if !bytes.Equal(testData, receivedData) {
		t.Fatalf("Data integrity check failed")
	}
}

func TestBasicWriteRead(t *testing.T) {
	r, w := newTestPipe(t, 10)

	data := []byte("first")
	mustWrite(t, w, data)

	mustRead(t, r, data)
}

func TestRingBufferWrapAround(t *testing.T) {
	r, w := newTestPipe(t, 4)

	data1 := []byte("abcd")
	mustWrite(t, w, data1)

	mustRead(t, r, []byte("ab"))

	data2 := []byte("xy")
	mustWrite(t, w, data2)

	remaining := make([]byte, 4) // "cd" + "xy"
	mustReadFull(t, r, remaining)
	expected := "cdxy"
	if string(remaining) != expected {
		t.Fatalf("Expected %q, got %q", expected, remaining)
	}
}

func TestReadWithZeroLengthBuffer(t *testing.T) {
	r, w := newTestPipe(t, 10)

	zeroBuf := make([]byte, 0)
	n, err := r.Read(zeroBuf)
	if n != 0 {
		t.Fatalf("Expected n=0 for zero-length buffer, got n=%d", n)
	}
	if err != nil {
		t.Fatalf("Expected err=nil for zero-length buffer, got err=%v", err)
	}

	data := []byte("test")
	mustWrite(t, w, data)

	n, err = r.Read(zeroBuf)
	if n != 0 {
		t.Fatalf("Expected n=0 for zero-length buffer with data, got n=%d", n)
	}
	if err != nil {
		t.Fatalf("Expected err=nil for zero-length buffer with data, got err=%v", err)
	}

	normalBuf := make([]byte, len(data))
	n, err = r.Read(normalBuf)
	if err != nil {
		t.Fatalf("Normal read failed: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Expected to read %d bytes, got %d", len(data), n)
	}
	if string(normalBuf) != string(data) {
		t.Fatalf("Expected %q, got %q", data, normalBuf)
	}
}

func TestDoubleClose(t *testing.T) {
	t.Run("ReaderDoubleClose", func(t *testing.T) {
		r, _ := newTestPipe(t, 10)

		err := r.Close()
		if err != nil {
			t.Fatalf("First close failed: %v", err)
		}

		err = r.Close()
		if err != nil {
			t.Fatalf("Second close failed: %v", err)
		}
	})

	t.Run("WriterDoubleClose", func(t *testing.T) {
		_, w := newTestPipe(t, 10)

		err := w.Close()
		if err != nil {
			t.Fatalf("First close failed: %v", err)
		}

		err = w.Close()
		if err != nil {
			t.Fatalf("Second close failed: %v", err)
		}
	})
}

func TestLargeSingleWrite(t *testing.T) {
	bufferSize := 10
	r, w := newTestPipe(t, bufferSize)

	largeData := make([]byte, bufferSize*5)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	var writeErr error
	var wg sync.WaitGroup
	wg.Go(func() {
		defer w.Close()
		_, writeErr = w.Write(largeData)
	})

	received := make([]byte, len(largeData))
	totalRead := 0
	for totalRead < len(largeData) {
		n, err := r.Read(received[totalRead:])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		totalRead += n
	}

	wg.Wait()
	if writeErr != nil {
		t.Fatalf("Large write failed: %v", writeErr)
	}

	if totalRead != len(largeData) {
		t.Fatalf("Expected to read %d bytes, read %d", len(largeData), totalRead)
	}

	if !bytes.Equal(received, largeData) {
		t.Fatalf("Data corruption in large single write")
	}
}

func TestWriteToWithWriteError(t *testing.T) {
	r, w := newTestPipe(t, 10)

	data := []byte("test data")
	mustWrite(t, w, data)
	w.Close()

	failingWriter := &failingWriterTest{failAfter: 4}

	_, err := r.WriteTo(failingWriter)
	if err == nil {
		t.Fatalf("Expected error from WriteTo, got nil")
	}
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Expected io.ErrShortWrite, got %v", err)
	}
}

func TestReadFromWithReadError(t *testing.T) {
	_, w := newTestPipe(t, 10)

	failingReader := &failingReaderTest{data: []byte("test data"), failAfter: 4}

	_, err := w.ReadFrom(failingReader)
	if err == nil {
		t.Fatalf("Expected error from ReadFrom, got nil")
	}
	if err.Error() != "read failed" {
		t.Fatalf("Expected 'read failed', got %q", err.Error())
	}
}

func TestCloseRaceCondition(t *testing.T) {
	t.Run("CloseWhileReading", func(t *testing.T) {
		r, _ := newTestPipe(t, 1)

		var readErr error
		var wg sync.WaitGroup
		wg.Go(func() {
			buf := make([]byte, 10)
			_, readErr = r.Read(buf)
		})

		time.Sleep(10 * time.Millisecond)
		r.Close()

		wg.Wait()
		expectError(t, readErr, io.ErrClosedPipe)
	})

	t.Run("CloseWhileWriting", func(t *testing.T) {
		r, w := newTestPipe(t, 1)

		mustWrite(t, w, []byte("x"))

		var writeErr error
		var wg sync.WaitGroup
		wg.Go(func() {
			_, writeErr = w.Write([]byte("will block")) // block
		})

		time.Sleep(10 * time.Millisecond)
		r.Close() // close reader while write is blocked

		wg.Wait()
		expectError(t, writeErr, io.ErrClosedPipe)
	})
}

func TestWriteFailsWhenReaderClosed(t *testing.T) {
	r, w := newTestPipe(t, 8)
	r.Close()

	_, err := w.Write([]byte("data"))
	expectError(t, err, io.ErrClosedPipe)
}

func TestReadAfterReaderClose(t *testing.T) {
	r, w := newTestPipe(t, 8)
	mustWrite(t, w, []byte("data"))
	r.Close()

	mustRead(t, r, []byte("data"))

	buf := make([]byte, 1)
	_, err := r.Read(buf)
	expectError(t, err, io.ErrClosedPipe)
}

func TestWriteFailsAfterWriterClose(t *testing.T) {
	_, w := newTestPipe(t, 4)

	if err := w.Close(); err != nil {
		t.Fatalf("writer close failed: %v", err)
	}

	_, err := w.Write([]byte("data"))
	expectError(t, err, io.ErrClosedPipe)
}

type failingWriterTest struct {
	written   int
	failAfter int
}

func (fw *failingWriterTest) Write(p []byte) (int, error) {
	if fw.written >= fw.failAfter {
		return 0, errors.New("write failed")
	}
	n := min(len(p), fw.failAfter-fw.written)
	fw.written += n
	return n, nil
}

type failingReaderTest struct {
	data      []byte
	pos       int
	failAfter int
}

func (fr *failingReaderTest) Read(p []byte) (int, error) {
	if fr.pos >= fr.failAfter {
		return 0, errors.New("read failed")
	}
	n := copy(p, fr.data[fr.pos:])
	fr.pos += n
	return n, nil
}

func newTestPipe(t *testing.T, size int) (*pipebuf.PipeReader, *pipebuf.PipeWriter) {
	t.Helper()
	r, w := pipebuf.Pipe(size)
	t.Cleanup(func() {
		r.Close()
		w.Close()
	})
	return r, w
}

func mustWrite(t *testing.T, w *pipebuf.PipeWriter, data []byte) int {
	t.Helper()
	n, err := w.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(data) {
		t.Fatalf("expected to write %d bytes, wrote %d", len(data), n)
	}
	return n
}

func mustReadFull(t *testing.T, r io.Reader, buf []byte) int {
	t.Helper()
	n, err := io.ReadFull(r, buf)
	if err != nil {
		t.Fatalf("ReadFull failed: %v", err)
	}
	return n
}

func mustRead(t *testing.T, r io.Reader, expected []byte) {
	t.Helper()
	buf := make([]byte, len(expected))
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != len(expected) {
		t.Fatalf("expected to read %d bytes, read %d", len(expected), n)
	}
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %q, got %q", expected, buf)
	}
}

func expectError(t *testing.T, err, expected error) {
	t.Helper()
	if err != expected {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func expectEOF(t *testing.T, r io.Reader) {
	t.Helper()
	buf := make([]byte, 1)
	_, err := r.Read(buf)
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}
func TestPartialReadAfterWrite(t *testing.T) {
	r, w := newTestPipe(t, 4)

	mustWrite(t, w, []byte("abcd"))

	buf := make([]byte, 2)
	if _, err := r.Read(buf); err != nil {
		t.Fatalf("first read failed: %v", err)
	}
	if string(buf) != "ab" {
		t.Fatalf("expected \"ab\", got %q", buf)
	}

	mustWrite(t, w, []byte("ef"))

	if _, err := r.Read(buf); err != nil {
		t.Fatalf("second read failed: %v", err)
	}
	if string(buf) != "cd" {
		t.Fatalf("expected \"cd\", got %q", buf)
	}

	if _, err := r.Read(buf); err != nil {
		t.Fatalf("third read failed: %v", err)
	}
	if string(buf) != "ef" {
		t.Fatalf("expected \"ef\", got %q", buf)
	}
}
