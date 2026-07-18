package exif

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestExtractMetadataReturnsErrorForMissingInput(t *testing.T) {
	if _, err := ExtractMetadata("/definitely/missing/image.raw"); err == nil {
		t.Fatal("expected metadata extraction to fail for missing input")
	}
}

func TestMetadataFromExiftoolMapCharacterization(t *testing.T) {
	metadata := metadataFromExiftoolMap(map[string]any{
		"UniqueCameraModel": "Unique",
		"CameraModelName":   "CameraName",
		"Make":              "Maker",
		"ISO":               float64(800),
		"FNumber":           float64(2.8),
		"ExposureTime":      float64(0),
		"FocalLength":       "50 mm",
		"ImageWidth":        float64(6000),
		"ImageHeight":       float64(4000),
		"DateTime":          "2024:01:02 03:04:05",
	})

	if metadata.Model != "Unique" || metadata.ISO != "800" || metadata.Aperture != "f/2.8" {
		t.Fatalf("identity/exposure metadata = %+v", metadata)
	}
	if metadata.Shutter != "" {
		t.Fatalf("zero exposure must not produce a reciprocal shutter, got %q", metadata.Shutter)
	}
	if metadata.Focal != "50 mm" || metadata.Width != 6000 || metadata.Height != 4000 || metadata.Date != "2024:01:02 03:04:05" {
		t.Fatalf("dimensions/date metadata = %+v", metadata)
	}
}

func TestExtractThumbnailCreateDestinationError(t *testing.T) {
	err := ExtractThumbnail("source.raw", filepath.Join(t.TempDir(), "missing-dir", "thumb.jpg"))
	if err == nil {
		t.Fatal("expected destination creation error")
	}
}

func TestExtractThumbnailReturnsErrorForMissingSource(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "thumb.jpg")
	if err := ExtractThumbnail("/definitely/missing/source.raw", dst); err == nil {
		t.Fatal("expected extraction error for missing source")
	}
}

func TestExecuteBinary_DoesNotTruncateWhenPayloadContainsDefaultReadyMarker(t *testing.T) {
	var stdin bytes.Buffer
	// Simulate binary payload that contains "{ready}" bytes before end marker.
	payload := []byte{0xFF, 0xD8, 0x01, '{', 'r', 'e', 'a', 'd', 'y', '}', 0x02, 0xFF, 0xD9}
	execSeq.Store(0)
	stream := append(append([]byte{}, payload...), []byte("{ready1}\n")...)

	s := &Session{
		active: true,
		stdin:  nopWriteCloser{Writer: &stdin},
		stdout: bufio.NewReader(bytes.NewReader(stream)),
	}

	got, err := s.ExecuteBinary(context.Background(), "-b", "-PreviewImage", "/tmp/in.raw")
	if err != nil {
		t.Fatalf("ExecuteBinary returned error: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("binary payload truncated/corrupted: got=%v want=%v", got, payload)
	}
}

// TestExecuteBinary_RespectsContextCancellation ensures that a pre-cancelled context
// causes ExecuteBinary to return promptly with ctx.Err(), even when the pipe reader
// would block forever (simulating a frozen exiftool process).
func TestExecuteBinary_RespectsContextCancellation(t *testing.T) {
	var stdin bytes.Buffer

	// pr blocks Read forever, simulating an exiftool that never responds.
	pr, _ := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Already cancelled.

	s := &Session{
		active:    true,
		stdin:     nopWriteCloser{Writer: &stdin},
		stdoutRaw: pr,
		stdout:    bufio.NewReader(pr),
	}

	done := make(chan error, 1)
	go func() {
		_, err := s.ExecuteBinary(ctx, "-b", "-PreviewImage", "/tmp/fake.raw")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("ExecuteBinary must return an error on cancelled context")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ExecuteBinary did not respect context cancellation within 2s")
	}
}

type nopWriteCloser struct{ io.Writer }

func (n nopWriteCloser) Close() error { return nil }

func TestIsExiftoolAvailable_ResetCache(t *testing.T) {
	// Call once to initialize/cache the value
	_ = IsExiftoolAvailable()

	exiftoolAvailableMu.Lock()
	if !exiftoolAvailableInit {
		exiftoolAvailableMu.Unlock()
		t.Fatal("expected exiftool cache to be initialized")
	}
	exiftoolAvailableMu.Unlock()

	// Reset the cache
	ResetExiftoolAvailabilityCache()

	exiftoolAvailableMu.Lock()
	if exiftoolAvailableInit {
		exiftoolAvailableMu.Unlock()
		t.Fatal("expected exiftool cache to be reset (uninitialized)")
	}
	exiftoolAvailableMu.Unlock()
}

// TestSession_CloseKillsProcessOnTimeout locks in the P1 exiftool fix:
// if `cmd.Wait()` does not return within closeTimeout, Close must kill the
// underlying process so we don't leak zombie exiftool processes (e.g. when
// exiftool itself has hung on a stuck pipe).
//
// We simulate a stuck process by spawning a child that ignores stdin and
// blocks for far longer than closeTimeout. The test asserts that:
//  1. Close returns within ~2×closeTimeout (kill path)
//  2. The child process is actually terminated (no orphan)
func TestSession_CloseKillsProcessOnTimeout(t *testing.T) {
	// Spawn a long-running child that ignores its stdin curl entirely.
	// We invoke `sleep` via bash so we can redirect stdin from /dev/null and
	// ensure the child never sees Close's "-stay_open False" handshake.
	cmd := newBlockingTestCommand("30s")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper process: %v", err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
			_ = cmd.Process.Kill()
		}
	})

	s := &Session{
		active: true,
		cmd:    cmd,
		stdin:  nopWriteCloser{Writer: &bytes.Buffer{}},
	}

	// Patch closeTimeout to a short value for the duration of the test so we
	// don't actually wait 5s. Restore it on cleanup.
	orig := closeTimeout
	defer func() { closeTimeout = orig }()
	closeTimeout = 200 * time.Millisecond

	start := time.Now()
	if err := s.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	elapsed := time.Since(start)

	// Must return within ~1×closeTimeout (kill resolution), not 30s.
	if elapsed > time.Second {
		t.Fatalf("Close took %v, expected < 1s (kill path not exercised)", elapsed)
	}

	// Process must be terminated. cmd.ProcessState is populated only after
	// cmd.Wait() returns, so a non-nil ProcessState means the child actually
	// exited (here: was killed by our timeout). Exited() would return false on
	// a signal kill, so we don't rely on it.
	if cmd.ProcessState == nil {
		t.Fatalf("process state is nil after Close; child was not Wait'd (still running?)")
	}
}

// TestSession_CloseIdempotent verifies that calling Close multiple times on the
// same session is safe (Quit + ResetAppCache may both call it).
func TestSession_CloseIdempotent(t *testing.T) {
	cmd := newBlockingTestCommand("20ms")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper process: %v", err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
			_ = cmd.Process.Kill()
		}
	})

	s := &Session{
		active: true,
		cmd:    cmd,
		stdin:  nopWriteCloser{Writer: &bytes.Buffer{}},
	}

	if err := s.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("third Close: %v", err)
	}
}

func newBlockingTestCommand(duration string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestSessionHelperProcess$")
	cmd.Env = append(os.Environ(), "QUICKCULL_EXIF_HELPER_DURATION="+duration)
	return cmd
}

func TestSessionHelperProcess(t *testing.T) {
	duration := os.Getenv("QUICKCULL_EXIF_HELPER_DURATION")
	if duration == "" {
		return
	}
	d, err := time.ParseDuration(duration)
	if err != nil {
		os.Exit(2)
	}
	time.Sleep(d)
	os.Exit(0)
}
