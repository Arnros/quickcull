package exif

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"quickcull/internal/domain"
	"quickcull/internal/utils"
)

const (
	extractTimeout  = 15 * time.Second
	readyPrefix     = "{ready"
	maxArgLineBytes = 4096
)

// closeTimeout bounds how long Session.Close waits for the exiftool process to
// exit before killing it. Declared as a var (rather than a const) so tests can
// shorten it to avoid waiting 5s on the kill path.
var closeTimeout = 5 * time.Second

var (
	ErrInvalidPath = errors.New("invalid exiftool path argument")
)

type Session struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdoutRaw io.ReadCloser // kept so the context watcher can Close() it and unblock Read
	stdout    *bufio.Reader
	active    bool
}

func NewSession() (*Session, error) {
	exe := domain.ExiftoolPath()
	cmd := exec.Command(exe, "-stay_open", "True", "-@", "-")
	utils.ConfigureSilentCommand(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err == nil {
		utils.SafeGo(func() { _, _ = io.Copy(io.Discard, stderrPipe) })
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdoutPipe.Close()
		return nil, err
	}
	return &Session{cmd: cmd, stdin: stdin, stdoutRaw: stdoutPipe, stdout: bufio.NewReader(stdoutPipe), active: true}, nil
}

func (s *Session) Close() error {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return nil
	}
	s.active = false
	stdin := s.stdin
	s.mu.Unlock()

	_, _ = fmt.Fprint(stdin, "-stay_open\nFalse\n")
	_ = stdin.Close()

	// Wait for the process to exit, but do not block forever.
	done := make(chan struct{})
	go func() {
		_ = s.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(closeTimeout):
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		<-done
	}
	return nil
}

func (s *Session) Execute(ctx context.Context, args ...string) ([]byte, error) {
	return s.ExecuteBinary(ctx, args...)
}

func (s *Session) ExecuteBinary(ctx context.Context, args ...string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return nil, errors.New("inactive")
	}

	for _, arg := range args {
		_, _ = fmt.Fprintln(s.stdin, arg)
	}
	execID := strconv.FormatUint(execSeq.Add(1), 10)
	_, _ = fmt.Fprintf(s.stdin, "-execute%s\n", execID)

	var buf bytes.Buffer
	tmp := make([]byte, 32768)
	marker := []byte(readyPrefix + execID + "}")

	// Watch for context cancellation in a separate goroutine: Read blocks on I/O
	// so ctx.Done() is never reached inside the loop without this watcher.
	// Closing stdoutRaw unblocks the Read call immediately (returns io.ErrClosedPipe).
	stop := make(chan struct{})
	defer close(stop)
	utils.SafeGo(func() {
		select {
		case <-ctx.Done():
			// Close stdoutRaw to unblock the Read loop; the loop sets s.active=false.
			if s.stdoutRaw != nil {
				_ = s.stdoutRaw.Close()
			}
			if s.cmd != nil && s.cmd.Process != nil {
				_ = s.cmd.Process.Kill()
			}
		case <-stop:
		}
	})

	for {
		n, err := s.stdout.Read(tmp)
		if err != nil {
			s.active = false
			if s.cmd != nil && s.cmd.Process != nil {
				_ = s.cmd.Process.Kill()
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}
		buf.Write(tmp[:n])
		data := buf.Bytes()
		if idx := bytes.Index(data, marker); idx >= 0 {
			return data[:idx], nil
		}
	}
}

var (
	sessionPool []*Session
	poolMu      sync.Mutex
	maxSessions = 8
	poolSem     chan struct{}
	poolOnce    sync.Once
	execSeq     atomic.Uint64
)

func initPool() {
	poolSem = make(chan struct{}, maxSessions)
	for i := 0; i < maxSessions; i++ {
		poolSem <- struct{}{}
	}
}

func AcquireSession(ctx context.Context) (*Session, error) {
	poolOnce.Do(initPool)
	select {
	case <-poolSem:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	poolMu.Lock()
	defer poolMu.Unlock()
	if len(sessionPool) > 0 {
		s := sessionPool[len(sessionPool)-1]
		sessionPool = sessionPool[:len(sessionPool)-1]
		return s, nil
	}
	s, err := NewSession()
	if err != nil {
		// Return the acquired token so the pool does not shrink permanently.
		poolSem <- struct{}{}
		return nil, err
	}
	return s, nil
}

func ReleaseSession(s *Session) {
	if s == nil {
		return
	}
	s.mu.Lock()
	active := s.active
	s.mu.Unlock()
	if !active {
		_ = s.Close()
		poolSem <- struct{}{}
		return
	}
	poolMu.Lock()
	if len(sessionPool) < maxSessions {
		sessionPool = append(sessionPool, s)
	} else {
		_ = s.Close()
	}
	poolMu.Unlock()
	poolSem <- struct{}{}
}

func Cleanup() {
	poolMu.Lock()
	sessions := sessionPool
	sessionPool = nil
	poolMu.Unlock()
	for _, s := range sessions {
		_ = s.Close()
	}
}

var (
	exiftoolAvailableMu     sync.RWMutex
	exiftoolAvailableCached bool
	exiftoolAvailableInit   bool
)

func IsExiftoolAvailable() bool {
	exiftoolAvailableMu.RLock()
	if exiftoolAvailableInit {
		res := exiftoolAvailableCached
		exiftoolAvailableMu.RUnlock()
		return res
	}
	exiftoolAvailableMu.RUnlock()

	exiftoolAvailableMu.Lock()
	defer exiftoolAvailableMu.Unlock()
	if exiftoolAvailableInit {
		return exiftoolAvailableCached
	}

	_, err := exec.LookPath(domain.ExiftoolPath())
	exiftoolAvailableCached = err == nil
	exiftoolAvailableInit = true
	return exiftoolAvailableCached
}

// ResetExiftoolAvailabilityCache resets the cached exiftool availability check.
// This should be called whenever the application configuration changes.
func ResetExiftoolAvailabilityCache() {
	exiftoolAvailableMu.Lock()
	exiftoolAvailableInit = false
	exiftoolAvailableMu.Unlock()
}

func ExiftoolSignature() string {
	exe, _ := exec.LookPath(domain.ExiftoolPath())
	if exe == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-ver")
	utils.ConfigureSilentCommand(cmd)
	out, _ := cmd.Output()
	return exe + "|" + strings.TrimSpace(string(out))
}

func ExtractThumbnail(src, dst string) error {
	if err := validatePathArg(src); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), extractTimeout)
	defer cancel()
	session, err := AcquireSession(ctx)
	if err != nil {
		return err
	}
	defer ReleaseSession(session)
	tags := []string{"-PreviewImage", "-JpgFromRaw", "-Composite:PreviewImage", "-OtherImage", "-ThumbnailImage"}
	for _, tag := range tags {
		data, err := session.ExecuteBinary(ctx, "-b", tag, src)
		if err == nil && looksLikeJPEG(data) {
			return os.WriteFile(dst, data, 0o600)
		}
	}
	return errors.New("no preview")
}

// looksLikeJPEG reports whether data starts with the JPEG SOI marker.
func looksLikeJPEG(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	return data[0] == 0xFF && data[1] == 0xD8
}

// validatePathArg rejects paths that could be interpreted as exiftool options
// or split into multiple commands. It returns ErrInvalidPath for unsafe inputs.
func validatePathArg(path string) error {
	if path == "" {
		return fmt.Errorf("%w: empty path", ErrInvalidPath)
	}
	if strings.ContainsRune(path, '\n') || strings.ContainsRune(path, '\r') {
		return fmt.Errorf("%w: path contains newline", ErrInvalidPath)
	}
	if strings.HasPrefix(path, "-") {
		return fmt.Errorf("%w: path starts with '-'", ErrInvalidPath)
	}
	if len(path) > maxArgLineBytes {
		return fmt.Errorf("%w: path too long", ErrInvalidPath)
	}
	return nil
}

type Metadata struct {
	Model, ISO, Aperture, Shutter, Focal, Date string
	Width, Height                              int
}

func ExtractMetadata(path string) (*Metadata, error) {
	if err := validatePathArg(path); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), extractTimeout)
	defer cancel()
	session, err := AcquireSession(ctx)
	if err != nil {
		return nil, err
	}
	defer ReleaseSession(session)
	output, err := session.Execute(ctx, "-j", "-fast", "-UniqueCameraModel", "-CameraModelName", "-Make", "-Model", "-ISO", "-FNumber", "-ExposureTime", "-FocalLength", "-ImageWidth", "-ImageHeight", "-DateTimeOriginal", "-DateTime", path)
	if err != nil {
		return nil, err
	}
	var res []map[string]any
	if err := json.Unmarshal(output, &res); err != nil {
		return nil, fmt.Errorf("decode exiftool metadata: %w", err)
	}
	if len(res) == 0 {
		return nil, errors.New("exiftool metadata response is empty")
	}
	return metadataFromExiftoolMap(res[0]), nil
}

func metadataFromExiftoolMap(values map[string]any) *Metadata {
	metadata := &Metadata{
		Model: firstMetadataString(values, "Model", "UniqueCameraModel", "CameraModelName", "Make"),
		Focal: firstMetadataString(values, "FocalLength"),
		Date:  firstMetadataString(values, "DateTimeOriginal", "DateTime"),
	}
	if value, ok := values["ISO"]; ok {
		metadata.ISO = fmt.Sprintf("%v", value)
	}
	if value, ok := values["FNumber"].(float64); ok {
		metadata.Aperture = fmt.Sprintf("f/%.1f", value)
	}
	metadata.Shutter = formatExiftoolShutter(values["ExposureTime"])
	metadata.Width = metadataDimension(values["ImageWidth"])
	metadata.Height = metadataDimension(values["ImageHeight"])
	return metadata
}

func firstMetadataString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func formatExiftoolShutter(value any) string {
	switch typed := value.(type) {
	case string:
		if typed != "" {
			return typed + " s"
		}
	case float64:
		if typed >= 1 {
			return fmt.Sprintf("%.1f s", typed)
		}
		if typed > 0 {
			return fmt.Sprintf("1/%d s", int(1.0/typed))
		}
	}
	return ""
}

func metadataDimension(value any) int {
	if dimension, ok := value.(float64); ok {
		return int(dimension)
	}
	return 0
}
