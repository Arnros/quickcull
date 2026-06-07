package review

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"time"

	"quickcull/internal/domain"
	"quickcull/internal/utils"
)

const (
	// OrientationTimeout is the maximum time allowed for an EXIF operation.
	OrientationTimeout = 10 * time.Second
)

// ApplyEXIFOrientation writes the Orientation tag into the file's EXIF.
// ctx is used as the parent for the timeout; pass a.bgContext() from callers.
func ApplyEXIFOrientation(ctx context.Context, absPath string, degrees int) error {
	// 1. Get current physical orientation
	current := GetOrientation(absPath)

	// 2. Calculate new orientation
	currDeg := 0
	switch current {
	case 3: currDeg = 180
	case 6: currDeg = 90
	case 8: currDeg = 270
	}

	newDeg := (currDeg + degrees) % 360
	orientation := 1
	switch newDeg {
	case 90:  orientation = 6
	case 180: orientation = 3
	case 270: orientation = 8
	}

	slog.Info("Applying EXIF orientation", "path", absPath, "target", orientation)

	// Use a dedicated one-shot process for writing
	exe := domain.ExiftoolPath()
	ctx, cancel := context.WithTimeout(ctx, OrientationTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe, "-overwrite_original", "-m", fmt.Sprintf("-Orientation#=%d", orientation), "--", absPath)
	utils.ConfigureSilentCommand(cmd)
	
	err := cmd.Run()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return domain.ErrExiftoolTimeout
		}
		slog.Error("Exiftool apply failed", "path", absPath, "error", err)
		return domain.ErrExiftoolApplyFailed
	}

	return nil
}
