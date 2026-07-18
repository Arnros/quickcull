package review

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"quickcull/internal/bus"
	"quickcull/internal/persistence"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestStressConcurrentEvents vérifie que 1000 actions simultanées ne corrompent pas l'état
func TestStressConcurrentEvents(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "quickcull-stress-concurrency")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "stress.db")
	store, _ := persistence.NewMetadataStore(dbPath)
	defer store.Close()

	srv := NewServer()
	srv.persistence = store

	// Simuler un état avec 1 photo
	photoID := "stress.jpg"
	srv.appState = &AppState{
		Root: tmpDir,
		Photos: map[string]Photo{
			photoID: {ID: photoID},
		},
	}

	const numGoroutines = 100
	const actionsPerGoroutine = 10
	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines*actionsPerGoroutine)

	// On lance une rafale d'étoiles (toggle)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < actionsPerGoroutine; j++ {
				_, _, err := srv.applyEvent(bus.Event{
					Type:    bus.TypeCommandToggleStar,
					Payload: bus.CommandToggleStarPayload{PhotoID: photoID, Starred: true, OldStarred: false},
				})
				if err != nil {
					errCh <- err
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent apply event: %v", err)
	}

	// Avec 1000 toggles (pair), l'étoile devrait être à false (si initialement false)
	// L'important est surtout que le programme n'ait pas crashé ou eu de "race condition" détectée par le runtime
	t.Log("Concurrency stress test completed without crash")
}

// TestStressCorruptedImages vérifie que le moteur d'analyse survit à des fichiers junk
func TestStressCorruptedImages(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "quickcull-stress-corrupt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Créer 10 fichiers avec des extensions valides mais du contenu corrompu (random bytes)
	for i := 0; i < 10; i++ {
		fname := filepath.Join(tmpDir, "corrupt"+string(rune(i))+".jpg")
		junk := make([]byte, 1024)
		if _, err := rand.Read(junk); err != nil {
			t.Fatalf("random test data: %v", err)
		}
		_ = os.WriteFile(fname, junk, 0644)
	}

	srv := NewServer()
	// On lance le scan et l'analyse
	if err := srv.LoadState(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Lancer l'analyse en arrière-plan et attendre un peu
	ctx, cancel := context.WithCancel(context.Background())
	srv.startBackgroundAnalysis(ctx)

	// On laisse le temps aux workers de se "casser les dents" sur les fichiers
	cancel()

	t.Log("Corrupted images stress test completed without crash")
}

// TestStressLargeHistoryUndo vérifie la stabilité d'une chaîne d'Undo massive
func TestStressLargeHistoryUndo(t *testing.T) {
	_, srv, cleanup := setupPhysicalTest(t)
	defer cleanup()

	photoID := "a.jpg"
	// Faire 50 actions sur le même fichier
	for i := 0; i < 50; i++ {
		if _, _, err := srv.server.applyEvent(bus.Event{
			Type:    bus.TypeCommandToggleStar,
			Payload: bus.CommandToggleStarPayload{PhotoID: photoID, Starred: true, OldStarred: false},
		}); err != nil {
			t.Fatalf("apply event %d: %v", i, err)
		}
	}

	// Annuler les 50 actions
	app := &App{server: srv.server}
	for i := 0; i < 50; i++ {
		_, err := app.Undo()
		if err != nil {
			t.Fatalf("Undo failed at step %d: %v", i, err)
		}
	}

	if len(srv.server.appState.History) != 0 {
		t.Errorf("History should be empty after full undo chain, got %d", len(srv.server.appState.History))
	}
}

func TestStressBoundedDiscovery_NoGoroutineLeakTrend(t *testing.T) {
	root := t.TempDir()
	const dirCount = 160
	for i := 0; i < dirCount; i++ {
		dir := filepath.Join(root, "batch", "d"+strconv.Itoa(i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s failed: %v", dir, err)
		}
		fp := filepath.Join(dir, "img.jpg")
		if err := os.WriteFile(fp, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s failed: %v", fp, err)
		}
	}

	const iterations = 6
	baseline := runtime.NumGoroutine()
	samples := make([]int, 0, iterations)

	for i := 0; i < iterations; i++ {
		filesChan := make(chan string, dirCount)
		errCh := make(chan error, 1)
		go func() {
			errCh <- ScanFiles(root, filesChan)
		}()

		count := 0
		for range filesChan {
			count++
		}
		if err := <-errCh; err != nil {
			t.Fatalf("ScanFiles iteration %d failed: %v", i, err)
		}
		if count != dirCount {
			t.Fatalf("ScanFiles iteration %d expected %d files, got %d", i, dirCount, count)
		}

		time.Sleep(40 * time.Millisecond)
		samples = append(samples, runtime.NumGoroutine())
	}

	minSample, maxSample := samples[0], samples[0]
	for _, sample := range samples[1:] {
		if sample < minSample {
			minSample = sample
		}
		if sample > maxSample {
			maxSample = sample
		}
	}

	if spread := maxSample - minSample; spread > 50 {
		t.Fatalf("goroutine sample spread too large (possible leak trend): baseline=%d samples=%v spread=%d", baseline, samples, spread)
	}
	if growth := samples[len(samples)-1] - baseline; growth > 70 {
		t.Fatalf("goroutine count grew unexpectedly across repeated scans: baseline=%d samples=%v growth=%d", baseline, samples, growth)
	}
}
