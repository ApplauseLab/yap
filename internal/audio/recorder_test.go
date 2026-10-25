package audio

import (
	"io"
	"os"
	"testing"
	"time"
)

// Opt-in: exercises the actual device/backend rather than a mocked stream.
// Samples stay in memory and are discarded.
func TestNativeRecorderStop(t *testing.T) {
	if os.Getenv("YAP_TEST_NATIVE_AUDIO") != "1" {
		t.Skip("set YAP_TEST_NATIVE_AUDIO=1 to use the microphone")
	}
	if err := Initialize(); err != nil {
		t.Fatal(err)
	}
	defer Terminate()
	for i := 0; i < 3; i++ {
		r := NewRecorder()
		if err := r.Start(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(250 * time.Millisecond)
		t.Logf("cycle %d: stopping native recorder", i)
		samples, err := r.Stop()
		if err != nil {
			t.Fatal(err)
		}
		if len(samples) == 0 {
			t.Fatal("native recorder returned no samples")
		}
		t.Logf("cycle %d: native recorder stopped, samples=%d", i, len(samples))
	}
}

type blockedStream struct {
	reading, stopped, release, closed chan struct{}
}

func (*blockedStream) Start() error                  { return nil }
func (*blockedStream) AvailableToRead() (int, error) { return FrameSize, nil }
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
	select {
	case <-s.stopped:
		t.Error("PortAudio stream stopped while Read was still using it; CoreAudio may never return that read")
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

type starvedStream struct{ blockedStream }

func (*starvedStream) AvailableToRead() (int, error) { return 0, nil }

func TestStopWithoutAvailableAudio(t *testing.T) {
	s := &starvedStream{blockedStream{make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})}}
	r := NewRecorder()
	r.stream = s
	r.isRecording = true
	go r.readLoop(make([]float32, FrameSize))
	// Give the reader a chance to query availability. It must not enter Read.
	select {
	case <-s.reading:
		t.Error("entered blocking Read without available audio")
	case <-time.After(50 * time.Millisecond):
	}
	done := make(chan struct{})
	go func() { _, _ = r.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Stop stalled waiting for audio on a starved input")
	}
	close(s.release) // Release a broken implementation so the test can clean up.
	<-done
}
