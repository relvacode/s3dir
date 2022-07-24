package s3dir

import (
	"bytes"
	"net/http"
	"strconv"
)

const streamBufferCapacity = 16_777_216 // 16mb

func NewStream(rw http.ResponseWriter, writeHeaders func(rw http.ResponseWriter)) *Stream {
	return &Stream{
		rw:           rw,
		writeHeaders: writeHeaders,
		b:            bytes.NewBuffer(make([]byte, 0, streamBufferCapacity)),
	}
}

type Stream struct {
	rw           http.ResponseWriter
	writeHeaders func(rw http.ResponseWriter)
	b            *bytes.Buffer
	flushed      bool
}

func (s *Stream) Write(b []byte) (int, error) {
	if s.flushed {
		return s.rw.Write(b)
	}

	var capacity = s.b.Cap() - s.b.Len()
	if capacity >= len(b) {
		// Can write to buffer
		return s.b.Write(b)
	}

	// Flush, then write b
	s.writeHeaders(s.rw)

	if s.b.Len() > 0 {
		_, _ = s.b.WriteTo(s.rw)
	}
	s.flushed = true
	s.b = nil

	return s.rw.Write(b)
}

// Complete writes the buffered stream data if the response has not already been flushed.
// It is safe to always defer Complete().
func (s *Stream) Complete() {
	if s.flushed {
		return
	}

	s.rw.Header().Set("Content-Length", strconv.Itoa(s.b.Len()))
	s.writeHeaders(s.rw)

	s.flushed = true
	_, _ = s.b.WriteTo(s.rw)
}

// Abort can be called to abort the stream and let the caller write an error to the response.
// If the response has already been flushed then the callback will not be called.
func (s *Stream) Abort(callback func(rw http.ResponseWriter)) {
	if s.flushed {
		return
	}

	s.b = nil
	s.flushed = true
	callback(s.rw)
}
