package exif

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"
)

func TestExtractMetadataReturnsErrorForMissingInput(t *testing.T) {
	if _, err := ExtractMetadata("/definitely/missing/image.raw"); err == nil {
		t.Fatal("expected metadata extraction to fail for missing input")
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
