package audio

import (
	"io"
	"testing"
	"time"
)

type blockedStream struct {
	reading, stopped, release, closed chan struct{}
}

func (*blockedStream) Start() error { return nil }
func (s *blockedStream) Read() error {
	close(s.reading)
	<-s.release
	return io.EOF
}
func (s *blockedStream) Stop() error  { close(s.stopped); return nil }
func (s *blockedStream) Close() error { close(s.closed); return nil }

func TestStopDoesNotCloseStreamDuringRead(t *testing.T) {
	s := &blockedStream{make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})}
	r := NewRecorder()
	r.stream = s
	r.isRecording = true
	go r.readLoop(make([]float32, FrameSize))
	<-s.reading
	done := make(chan struct{})
	go func() { _, _ = r.Stop(); close(done) }()
	<-s.stopped
	select {
	case <-s.closed:
		t.Error("PortAudio stream closed while Read was still using it")
	case <-time.After(50 * time.Millisecond):
	}
	close(s.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop did not finish after Read returned")
	}
}
