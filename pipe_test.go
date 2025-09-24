package pipebuf

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
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

func TestPipeCloseReader(t *testing.T) {
	r, w := Pipe(10)

	r.Close()

	_, err := w.Write([]byte("test"))
	if err != io.ErrClosedPipe {
		t.Fatalf("expected ErrClosedPipe, got %v", err)
	}
}

func TestPipeCloseWriter(t *testing.T) {
	r, w := Pipe(10)
	defer r.Close()

	w.Write([]byte("test"))
	w.Close()

	buf := make([]byte, 4)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != 4 || string(buf) != "test" {
		t.Fatalf("expected 'test', got %q", buf[:n])
	}

	_, err = r.Read(buf)
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestPipeRingBuffer(t *testing.T) {
	bufSize := 4
	r, w := Pipe(bufSize)
	defer r.Close()
	defer w.Close()

	for i := range 3 {
		data := []byte("ab")
		w.Write(data)

		buf := make([]byte, 2)
		n, err := r.Read(buf)
		if err != nil {
			t.Fatalf("Read %d failed: %v", i, err)
		}
		if n != 2 || string(buf) != "ab" {
			t.Fatalf("Read %d: expected 'ab', got %q", i, buf[:n])
		}
	}
}

func TestPipeCloseWithError(t *testing.T) {
	t.Run("WriterCloseWithError", func(t *testing.T) {
		r, w := Pipe(10)
		defer r.Close()

		customErr := errors.New("custom write error")
		w.CloseWithError(customErr)

		buf := make([]byte, 10)
		_, err := r.Read(buf)
		if err != customErr {
			t.Fatalf("expected custom error %v, got %v", customErr, err)
		}
	})

	t.Run("WriterCloseWithNilError", func(t *testing.T) {
		r, w := Pipe(10)
		defer r.Close()

		w.CloseWithError(nil)

		buf := make([]byte, 10)
		_, err := r.Read(buf)
		if err != io.EOF {
			t.Fatalf("expected EOF, got %v", err)
		}
	})

	t.Run("ReaderCloseWithError", func(t *testing.T) {
		r, w := Pipe(10)
		defer w.Close()

		customErr := errors.New("custom read error")
		r.CloseWithError(customErr)

		_, err := w.Write([]byte("test"))
		if err != customErr {
			t.Fatalf("expected custom error %v, got %v", customErr, err)
		}
	})

	t.Run("ReaderCloseWithNilError", func(t *testing.T) {
		r, w := Pipe(10)
		defer w.Close()

		r.CloseWithError(nil)

		_, err := w.Write([]byte("test"))
		if err != io.ErrClosedPipe {
			t.Fatalf("expected ErrClosedPipe, got %v", err)
		}
	})

	t.Run("CloseWithErrorDoesNotOverwrite", func(t *testing.T) {
		r, w := Pipe(10)

		firstErr := errors.New("first error")
		secondErr := errors.New("second error")

		w.CloseWithError(firstErr)
		w.CloseWithError(secondErr) // not overwrite

		buf := make([]byte, 10)
		_, err := r.Read(buf)
		if err != firstErr {
			t.Fatalf("expected first error %v, got %v", firstErr, err)
		}
	})
}

func TestWriteTo(t *testing.T) {
	r, w := Pipe(10)
	defer r.Close()
	defer w.Close()

	input := "hello world from WriteTo"
	output := &bytes.Buffer{}

	go func() {
		defer w.Close()
		w.Write([]byte(input))
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
	r, w := Pipe(10)
	defer r.Close()
	defer w.Close()

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

func TestIOCopyOptimization(t *testing.T) {
	r, w := Pipe(100)
	defer r.Close()
	defer w.Close()

	input := "test data for io.Copy optimization"
	output := &bytes.Buffer{}

	go func() {
		defer w.Close()
		w.Write([]byte(input))
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

func TestIOCopyFromSourceOptimization(t *testing.T) {
	r, w := Pipe(100)
	defer r.Close()
	defer w.Close()

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

func TestBufferedDataAvailableAfterReaderClose(t *testing.T) {
	r, w := Pipe(10)
	defer w.Close()

	testData := "test"
	n, err := w.Write([]byte(testData))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(testData) {
		t.Fatalf("expected to write %d bytes, wrote %d", len(testData), n)
	}

	r.Close()

	buf := make([]byte, len(testData))
	n, err = r.Read(buf)
	if err != nil {
		t.Fatalf("Read failed after close: %v", err)
	}
	if n != len(testData) {
		t.Fatalf("expected to read %d bytes, read %d", len(testData), n)
	}
	if string(buf) != testData {
		t.Fatalf("expected %q, got %q", testData, string(buf))
	}

	_, err = r.Read(buf)
	if err != io.ErrClosedPipe {
		t.Fatalf("expected ErrClosedPipe after buffer emptied, got %v", err)
	}
}

func TestBufferedDataAvailableAfterReaderCloseWithError(t *testing.T) {
	r, w := Pipe(10)
	defer w.Close()

	testData := "test"
	w.Write([]byte(testData))

	customErr := errors.New("custom close error")
	r.CloseWithError(customErr)

	buf := make([]byte, len(testData))
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("Read failed after CloseWithError: %v", err)
	}
	if n != len(testData) {
		t.Fatalf("expected to read %d bytes, read %d", len(testData), n)
	}
	if string(buf) != testData {
		t.Fatalf("expected %q, got %q", testData, string(buf))
	}

	_, err = r.Read(buf)
	if err != customErr {
		t.Fatalf("expected custom error after buffer emptied, got %v", err)
	}
}

func TestZeroSizeBuffer(t *testing.T) {
	r, w := Pipe(0)
	defer r.Close()
	defer w.Close()

	testData := "zero buffer test"
	var wg sync.WaitGroup
	var readResult []byte
	var readErr error

	wg.Go(func() {
		buf := make([]byte, len(testData))
		n, err := io.ReadFull(r, buf)
		readErr = err
		readResult = buf[:n]
	})

	time.Sleep(10 * time.Millisecond)

	n, err := w.Write([]byte(testData))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(testData) {
		t.Fatalf("expected to write %d bytes, wrote %d", len(testData), n)
	}

	wg.Wait()

	if readErr != nil {
		t.Fatalf("Read failed: %v", readErr)
	}
	if string(readResult) != testData {
		t.Fatalf("expected %q, got %q", testData, string(readResult))
	}
}

func TestLargeDataIntegrity(t *testing.T) {
	bufferSize := 1024
	r, w := Pipe(bufferSize)
	defer r.Close()
	defer w.Close()

	dataSize := 10 * 1024 * 1024
	testData := make([]byte, dataSize)

	for i := range dataSize {
		testData[i] = byte(i % 256)
	}

	var wg sync.WaitGroup
	var writeErr, readErr error
	receivedData := make([]byte, dataSize)

	wg.Go(func() {
		defer w.Close()

		written := 0
		for written < dataSize {
			n, err := w.Write(testData[written:])
			if err != nil {
				writeErr = err
				return
			}
			written += n
		}
	})

	wg.Go(func() {
		totalRead := 0
		for totalRead < dataSize {
			n, err := r.Read(receivedData[totalRead:])
			if err != nil && err != io.EOF {
				readErr = err
				return
			}
			totalRead += n
			if err == io.EOF {
				break
			}
		}

		if totalRead != dataSize {
			readErr = errors.New("incomplete read")
		}
	})

	wg.Wait()

	if writeErr != nil {
		t.Fatalf("Write error: %v", writeErr)
	}
	if readErr != nil {
		t.Fatalf("Read error: %v", readErr)
	}

	if !bytes.Equal(testData, receivedData) {
		for i := range dataSize {
			if testData[i] != receivedData[i] {
				t.Fatalf("Data corruption at byte %d: expected %d, got %d", i, testData[i], receivedData[i])
			}
		}
		t.Fatalf("Data corruption detected but no specific byte difference found")
	}
}

func TestLargeDataWithSmallBuffer(t *testing.T) {
	bufferSize := 64
	r, w := Pipe(bufferSize)
	defer r.Close()
	defer w.Close()

	dataSize := 100 * 1024
	testData := make([]byte, dataSize)

	pattern := "The quick brown fox jumps over the lazy dog. "
	for i := range dataSize {
		testData[i] = pattern[i%len(pattern)]
	}

	var wg sync.WaitGroup
	var writeErr, readErr error
	var receivedBuffer bytes.Buffer

	wg.Go(func() {
		defer w.Close()

		chunkSize := 17
		for i := 0; i < dataSize; i += chunkSize {
			end := min(i+chunkSize, dataSize)

			_, err := w.Write(testData[i:end])
			if err != nil {
				writeErr = err
				return
			}

			if i%1000 == 0 {
				time.Sleep(time.Microsecond)
			}
		}
	})

	wg.Go(func() {
		_, err := io.Copy(&receivedBuffer, r)
		if err != nil {
			readErr = err
		}
	})

	wg.Wait()

	if writeErr != nil {
		t.Fatalf("Write error: %v", writeErr)
	}
	if readErr != nil {
		t.Fatalf("Read error: %v", readErr)
	}

	receivedData := receivedBuffer.Bytes()
	if len(receivedData) != dataSize {
		t.Fatalf("Size mismatch: expected %d bytes, got %d", dataSize, len(receivedData))
	}

	if !bytes.Equal(testData, receivedData) {
		for i := 0; i < min(len(testData), len(receivedData)); i++ {
			if testData[i] != receivedData[i] {
				t.Fatalf("Data corruption at byte %d: expected %q, got %q",
					i, string(testData[i:min(i+10, len(testData))]),
					string(receivedData[i:min(i+10, len(receivedData))]))
			}
		}
		t.Fatalf("Data corruption detected")
	}
}

func TestFirstWriteEmptyPipe(t *testing.T) {
	r, w := Pipe(10)
	defer r.Close()
	defer w.Close()

	data := []byte("first")
	n, err := w.Write(data)
	if err != nil {
		t.Fatalf("First write failed: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Expected to write %d bytes, wrote %d", len(data), n)
	}

	buf := make([]byte, len(data))
	n, err = r.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Expected to read %d bytes, read %d", len(data), n)
	}
	if string(buf) != string(data) {
		t.Fatalf("Expected %q, got %q", data, buf)
	}
}

func TestWrapAroundWriteAfterTrim(t *testing.T) {
	r, w := Pipe(4)
	defer r.Close()
	defer w.Close()

	data1 := []byte("abcd")
	n, err := w.Write(data1)
	if err != nil {
		t.Fatalf("First write failed: %v", err)
	}
	if n != len(data1) {
		t.Fatalf("Expected to write %d bytes, wrote %d", len(data1), n)
	}

	buf := make([]byte, 2)
	n, err = r.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != 2 {
		t.Fatalf("Expected to read 2 bytes, read %d", n)
	}
	if string(buf) != "ab" {
		t.Fatalf("Expected 'ab', got %q", buf)
	}

	data2 := []byte("xy")
	n, err = w.Write(data2)
	if err != nil {
		t.Fatalf("Wrap-around write failed: %v", err)
	}
	if n != len(data2) {
		t.Fatalf("Expected to write %d bytes, wrote %d", len(data2), n)
	}

	remaining := make([]byte, 4) // "cd" + "xy"
	_, err = io.ReadFull(r, remaining)
	if err != nil {
		t.Fatalf("Reading remaining data failed: %v", err)
	}
	expected := "cdxy"
	if string(remaining) != expected {
		t.Fatalf("Expected %q, got %q", expected, remaining)
	}
}

func TestZeroSizeBufferFirstWrite(t *testing.T) {
	r, w := Pipe(0)
	defer r.Close()
	defer w.Close()

	data := []byte("test")
	var wg sync.WaitGroup
	var writeErr error

	wg.Go(func() {
		_, writeErr = w.Write(data)
	})

	time.Sleep(10 * time.Millisecond)

	buf := make([]byte, len(data))
	n, err := io.ReadFull(r, buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Expected to read %d bytes, read %d", len(data), n)
	}

	wg.Wait()
	if writeErr != nil {
		t.Fatalf("Write failed: %v", writeErr)
	}

	if string(buf) != string(data) {
		t.Fatalf("Expected %q, got %q", data, buf)
	}
}

func TestReadZeroLengthBuffer(t *testing.T) {
	r, w := Pipe(10)
	defer r.Close()
	defer w.Close()

	zeroBuf := make([]byte, 0)
	n, err := r.Read(zeroBuf)
	if n != 0 {
		t.Fatalf("Expected n=0 for zero-length buffer, got n=%d", n)
	}
	if err != nil {
		t.Fatalf("Expected err=nil for zero-length buffer, got err=%v", err)
	}

	data := []byte("test")
	w.Write(data)

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

func TestNegativeBufferSize(t *testing.T) {
	r, w := Pipe(-1)
	defer r.Close()
	defer w.Close()

	data := []byte("test")
	var (
		wg       sync.WaitGroup
		writeErr error
	)

	wg.Go(func() {
		if _, err := w.Write(data); err != nil {
			writeErr = err
		}
	})

	buf := make([]byte, len(data))
	if _, err := io.ReadFull(r, buf); err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	wg.Wait()
	if writeErr != nil {
		t.Fatalf("Write failed with negative buffer size: %v", writeErr)
	}
	if string(buf) != string(data) {
		t.Fatalf("Expected %q, got %q", data, buf)
	}
}

func TestBufferSizeOne(t *testing.T) {
	r, w := Pipe(1)
	defer r.Close()
	defer w.Close()

	n, err := w.Write([]byte("a"))
	if err != nil {
		t.Fatalf("First write failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("Expected to write 1 byte, wrote %d", n)
	}

	var writeErr error
	var wg sync.WaitGroup
	wg.Go(func() {
		_, writeErr = w.Write([]byte("b"))
	})

	time.Sleep(10 * time.Millisecond)

	buf := make([]byte, 1)
	_, err = r.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(buf) != "a" {
		t.Fatalf("Expected 'a', got %q", buf)
	}

	wg.Wait()
	if writeErr != nil {
		t.Fatalf("Second write failed: %v", writeErr)
	}

	_, err = r.Read(buf)
	if err != nil {
		t.Fatalf("Second read failed: %v", err)
	}
	if string(buf) != "b" {
		t.Fatalf("Expected 'b', got %q", buf)
	}
}

func TestDoubleClose(t *testing.T) {
	t.Run("ReaderDoubleClose", func(t *testing.T) {
		r, w := Pipe(10)
		defer w.Close()

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
		r, w := Pipe(10)
		defer r.Close()

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
	r, w := Pipe(bufferSize)
	defer r.Close()
	defer w.Close()

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

func TestWriteToErrorPropagation(t *testing.T) {
	r, w := Pipe(10)
	defer r.Close()
	defer w.Close()

	data := []byte("test data")
	w.Write(data)
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

func TestReadFromErrorPropagation(t *testing.T) {
	r, w := Pipe(10)
	defer r.Close()
	defer w.Close()

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
		r, w := Pipe(1)
		defer w.Close()

		var readErr error
		var wg sync.WaitGroup
		wg.Go(func() {
			buf := make([]byte, 10)
			_, readErr = r.Read(buf)
		})

		time.Sleep(10 * time.Millisecond)
		r.Close()

		wg.Wait()
		if readErr != io.ErrClosedPipe {
			t.Fatalf("Expected ErrClosedPipe, got %v", readErr)
		}
	})

	t.Run("CloseWhileWriting", func(t *testing.T) {
		r, w := Pipe(1)
		defer r.Close()

		w.Write([]byte("x"))

		var writeErr error
		var wg sync.WaitGroup
		wg.Go(func() {
			_, writeErr = w.Write([]byte("will block")) // block
		})

		time.Sleep(10 * time.Millisecond)
		r.Close() // close reader while write is blocked

		wg.Wait()
		if writeErr != io.ErrClosedPipe {
			t.Fatalf("Expected ErrClosedPipe, got %v", writeErr)
		}
	})
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

func newTestPipe(t *testing.T, size int) (*PipeReader, *PipeWriter) {
	t.Helper()
	r, w := Pipe(size)
	t.Cleanup(func() {
		r.Close()
		w.Close()
	})
	return r, w
}

func mustWrite(t *testing.T, w *PipeWriter, data []byte) int {
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
func TestWrappedReadPartial(t *testing.T) {
	r, w := Pipe(4)
	defer w.Close()

	if _, err := w.Write([]byte("abcd")); err != nil {
		t.Fatalf("initial write failed: %v", err)
	}

	buf := make([]byte, 2)
	if _, err := r.Read(buf); err != nil {
		t.Fatalf("first read failed: %v", err)
	}
	if string(buf) != "ab" {
		t.Fatalf("expected \"ab\", got %q", buf)
	}

	if _, err := w.Write([]byte("ef")); err != nil {
		t.Fatalf("second write failed: %v", err)
	}

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

func TestWriteAfterClose(t *testing.T) {
	r, w := Pipe(8)
	r.Close()

	if _, err := w.Write([]byte("data")); err != io.ErrClosedPipe {
		t.Fatalf("expected io.ErrClosedPipe, got %v", err)
	}
}

func TestReadAfterClose(t *testing.T) {
	r, w := Pipe(8)
	w.Write([]byte("data"))
	r.Close()

	buf := make([]byte, 4)
	if n, err := r.Read(buf); err != nil {
		t.Fatalf("unexpected error while draining buffer: %v", err)
	} else if string(buf[:n]) != "data" {
		t.Fatalf("expected to read \"data\", got %q", buf[:n])
	}

	if _, err := r.Read(buf); err != io.ErrClosedPipe {
		t.Fatalf("expected io.ErrClosedPipe, got %v", err)
	}
}

func TestWriteAfterCloseWriter(t *testing.T) {
	r, w := Pipe(4)
	defer r.Close()

	if err := w.Close(); err != nil {
		t.Fatalf("writer close failed: %v", err)
	}

	if _, err := w.Write([]byte("data")); err != io.ErrClosedPipe {
		t.Fatalf("expected io.ErrClosedPipe, got %v", err)
	}
}

