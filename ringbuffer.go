package pipebuf

// ringBuffer implements a single-producer, single-consumer ring buffer.
type ringBuffer struct {
	data     []byte
	readPos  int
	writePos int
}

// newRingBuffer creates a new ring buffer with the specified size.
// The actual buffer is size+1 to distinguish between full and empty states.
func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{
		data: make([]byte, size+1),
	}
}

// read reads data from the ring buffer into dst and returns the number of bytes read.
func (r *ringBuffer) read(dst []byte) int {
	bufLen := len(r.data)

	var available int
	if r.writePos >= r.readPos {
		available = r.writePos - r.readPos
	} else {
		available = (bufLen - r.readPos) + r.writePos
	}

	toRead := min(available, len(dst))
	if toRead == 0 {
		return 0
	}

	if r.readPos+toRead <= bufLen {
		copy(dst[:toRead], r.data[r.readPos:r.readPos+toRead])
		r.readPos = (r.readPos + toRead) % bufLen
	} else {
		firstChunk := bufLen - r.readPos
		secondChunk := toRead - firstChunk

		copy(dst[:firstChunk], r.data[r.readPos:])
		copy(dst[firstChunk:toRead], r.data[:secondChunk])

		r.readPos = secondChunk
	}

	return toRead
}

// write writes data from src into the ring buffer and returns the number of bytes written.
func (r *ringBuffer) write(src []byte) int {
	bufLen := len(r.data)

	var available int
	if r.writePos >= r.readPos {
		used := r.writePos - r.readPos
		available = bufLen - used - 1
	} else {
		available = r.readPos - r.writePos - 1
	}

	toWrite := min(available, len(src))
	if toWrite == 0 {
		return 0
	}

	if r.writePos+toWrite < bufLen {
		copy(r.data[r.writePos:r.writePos+toWrite], src[:toWrite])
		r.writePos = (r.writePos + toWrite) % bufLen
	} else {
		firstChunk := bufLen - r.writePos
		secondChunk := toWrite - firstChunk

		copy(r.data[r.writePos:], src[:firstChunk])
		copy(r.data[:secondChunk], src[firstChunk:toWrite])

		r.writePos = secondChunk
	}

	return toWrite
}

// empty returns true if the ring buffer is empty.
func (r *ringBuffer) empty() bool {
	return r.readPos == r.writePos
}

// full returns true if the ring buffer is full.
func (r *ringBuffer) full() bool {
	return (r.writePos+1)%len(r.data) == r.readPos
}
