package review

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"quickcull/internal/bus"
	"quickcull/internal/domain"
	"quickcull/internal/persistence"
	"testing"
	"time"
)

// setupPhysicalTest prépare un environnement de test avec des fichiers réels
func setupPhysicalTest(t *testing.T) (string, *App, func()) {
	tmpDir, err := os.MkdirTemp("", "quickcull-physical-test")
	if err != nil {
		t.Fatal(err)
	}

	// Création de 3 fichiers de test avec headers JPEG valides
	files := []string{"a.jpg", "b.jpg", "c.jpg"}
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			img.Set(i, j, color.Black)
		}
	}

	for _, f := range files {
		out, _ := os.Create(filepath.Join(tmpDir, f))
		_ = jpeg.Encode(out, img, nil)
		out.Close()
	}

	srv := NewServer()
	// On initialise la persistance
	dbPath := filepath.Join(tmpDir, "test_metadata.db")
	p, _ := persistence.NewMetadataStore(dbPath)
	srv.persistence = p

	if err := srv.LoadState(tmpDir); err != nil {
		t.Fatal(err)
	}

	// STOP background analysis to prevent conflicts with physical file operations in tests
	srv.analysisSched.Cancel()

	app := &App{server: srv}

	cleanup := func() {
		p.Close()
		os.RemoveAll(tmpDir)
	}

	return tmpDir, app, cleanup
}

func TestPhysicalStarAction(t *testing.T) {
	_, app, cleanup := setupPhysicalTest(t)
	defer cleanup()

	photoID := "a.jpg"
	event := bus.Event{
		Type:    bus.TypeCommandToggleStar,
		Payload: bus.CommandToggleStarPayload{PhotoID: photoID, Starred: true, OldStarred: false},
	}

	// 1. Appliquer l'action via le serveur
	applied, _, err := app.server.applyEvent(event)
	if !applied || err != nil {
		t.Fatalf("Action Star failed: %v", err)
	}

	// 2. Vérifier l'état en mémoire
	if !app.server.appState.Photos[photoID].IsStarred {
		t.Error("Memory state should be starred")
	}

	// 3. Simuler un redémarrage
	srv2 := NewServer()
	srv2.persistence = app.server.persistence
	if err := srv2.LoadState(app.server.appState.Root); err != nil {
		t.Fatal(err)
	}

	// 4. VERIFIER LA PERSISTENCE REELLE
	if !srv2.appState.Photos[photoID].IsStarred {
		t.Error("CRITICAL: Star state did not survive reload")
	}
}

func TestPhysicalLabelAction(t *testing.T) {
	_, app, cleanup := setupPhysicalTest(t)
	defer cleanup()

	photoID := "b.jpg"
	event := bus.Event{
		Type:    bus.TypeCommandLabelPhoto,
		Payload: bus.CommandLabelPhotoPayload{PhotoID: photoID, Label: 3},
	}

	app.server.applyEvent(event)

	// Vérifier après rechargement
	srv2 := NewServer()
	srv2.persistence = app.server.persistence
	srv2.LoadState(app.server.appState.Root)

	if srv2.appState.Photos[photoID].Label != 3 {
		t.Errorf("Expected label 3 after reload, got %d", srv2.appState.Photos[photoID].Label)
	}
}

func TestPhysicalTrashAndRestoreSequence(t *testing.T) {
	root, app, cleanup := setupPhysicalTest(t)
	defer cleanup()

	photoID := "c.jpg"
	trashPath := filepath.Join(root, domain.DirTrash, photoID)
	originalPath := filepath.Join(root, photoID)

	// 1. TRASH
	// On simule l'appel synchrone complet
	// a. Mouvement physique
	state := app.server.getState()
	state.Trash(state.FindIndex(photoID))
	
	// b. Mise à jour de l'état immuable (v2) et historique
	trashEvent := bus.Event{
		Type:    bus.TypeCommandTrashPhoto,
		Payload: bus.CommandTrashPhotoPayload{PhotoID: photoID},
	}
	app.server.applyEvent(trashEvent)

	// Vérification physique du Trash
	if _, err := os.Stat(trashPath); os.IsNotExist(err) {
		t.Error("File should be in .trash")
	}

	// 2. UNDO
	// On appelle Undo qui appelle lui-même applyEvent(Undo)
	_, err := app.Undo()
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	// Vérification physique de la restauration
	if _, err := os.Stat(originalPath); os.IsNotExist(err) {
		t.Error("CRITICAL: File was NOT restored by Undo")
	}
	if _, err := os.Stat(trashPath); err == nil {
		t.Error("File should have left the .trash folder")
	}
}

func TestPhysicalRotationAction(t *testing.T) {
	_, app, cleanup := setupPhysicalTest(t)
	defer cleanup()

	photoID := "a.jpg"
	// 1. Rotation visuelle (90 deg)
	app.server.applyEvent(bus.Event{
		Type:    bus.TypeCommandRotatePhoto,
		Payload: bus.CommandRotatePhotoPayload{PhotoID: photoID, Direction: "right"},
	})

	// 2. Vérifier persistance après reload
	srv2 := NewServer()
	srv2.persistence = app.server.persistence
	srv2.LoadState(app.server.appState.Root)

	if srv2.appState.Photos[photoID].Rotation != 90 {
		t.Errorf("Rotation did not survive reload, got %d", srv2.appState.Photos[photoID].Rotation)
	}
}

func TestPhysicalApplyRotation(t *testing.T) {
	// Skip if exiftool is not available
	_, err := exec.LookPath("exiftool")
	if err != nil {
		t.Skip("exiftool not found in PATH, skipping ApplyRotation test")
	}

	_, app, cleanup := setupPhysicalTest(t)
	defer cleanup()

	photoID := "a.jpg"
	// 1. Set visual rotation to 90
	app.server.applyEvent(bus.Event{
		Type:    bus.TypeCommandRotatePhoto,
		Payload: bus.CommandRotatePhotoPayload{PhotoID: photoID, Direction: "right"},
	})

	if app.server.appState.Photos[photoID].Rotation != 90 {
		t.Fatal("Initial rotation should be 90")
	}

	// 2. Apply to EXIF
	err = app.ApplyRotation(0, photoID)
	if err != nil {
		t.Fatalf("ApplyRotation failed: %v", err)
	}

	// 3. Verify visual rotation is now RESET to 0
	if app.server.appState.Photos[photoID].Rotation != 0 {
		t.Errorf("Visual rotation should be reset to 0 after ApplyRotation, got %d", app.server.appState.Photos[photoID].Rotation)
	}
}

func TestPhysicalBatchTrash(t *testing.T) {
	root, app, cleanup := setupPhysicalTest(t)
	defer cleanup()

	paths := []string{"a.jpg", "b.jpg"}
	
	// Simulation du Trash par lot
	_, err := app.Trash(0, "", paths)
	if err != nil {
		t.Fatalf("Batch Trash failed: %v", err)
	}

	// Vérification physique
	for _, p := range paths {
		if _, err := os.Stat(filepath.Join(root, domain.DirTrash, p)); os.IsNotExist(err) {
			t.Errorf("File %s missing from trash", p)
		}
	}
}

func TestPhysicalMetadataReset(t *testing.T) {
	_, app, cleanup := setupPhysicalTest(t)
	defer cleanup()

	// 1. Mettre des étoiles et labels
	app.server.applyEvent(bus.Event{
		Type:    bus.TypeCommandToggleStar,
		Payload: bus.CommandToggleStarPayload{PhotoID: "a.jpg", Starred: true, OldStarred: false},
	})
	app.server.applyEvent(bus.Event{
		Type:    bus.TypeCommandLabelPhoto,
		Payload: bus.CommandLabelPhotoPayload{PhotoID: "b.jpg", Label: 2},
	})

	// 2. RESET complet
	app.server.applyEvent(bus.Event{
		Type:    bus.TypeCommandResetMetadata,
		Payload: bus.CommandResetMetadataPayload{Scope: "all"},
	})

	// 3. Vérifier persistance du vide
	srv2 := NewServer()
	srv2.persistence = app.server.persistence
	srv2.LoadState(app.server.appState.Root)

	if srv2.appState.Photos["a.jpg"].IsStarred || srv2.appState.Photos["b.jpg"].Label != 0 {
		t.Error("Metadata reset failed to persist")
	}
}

func TestPhysicalDuplicateDetection(t *testing.T) {
	root, app, cleanup := setupPhysicalTest(t)
	defer cleanup()

	// Créer deux fichiers identiques (même contenu)
	content := []byte("identical-content-for-hash")
	_ = os.WriteFile(filepath.Join(root, "dup1.jpg"), content, 0644)
	_ = os.WriteFile(filepath.Join(root, "dup2.jpg"), content, 0644)

	// Re-scanner pour inclure les nouveaux fichiers
	app.server.LoadState(root)

	// FORCER LE HACHAGE SYNCHRONE POUR LE TEST
	state2 := app.server.getState()
	for i := 0; i < state2.Len(); i++ {
		abs, _ := state2.AbsPath(i)
		app.server.cache.GetHash(abs) // Calcul immédiat
	}

	// Lancer la détection
	groups, err := app.GetDuplicates(0)
	if err != nil {
		t.Fatalf("Duplicate detection failed: %v", err)
	}

	if len(groups) == 0 {
		t.Error("Should have detected at least one group of duplicates")
	}
}

func TestPhysicalSorting(t *testing.T) {
	root, app, cleanup := setupPhysicalTest(t)
	defer cleanup()

	// 1. Verify initial order (name)
	if app.server.appState.VisibleOrder[0] != "a.jpg" || app.server.appState.VisibleOrder[2] != "c.jpg" {
		t.Errorf("Unexpected initial order: %v", app.server.appState.VisibleOrder)
	}

	// 2. Change mtime of files to force a different date order
	// a.jpg (oldest), b.jpg (newest), c.jpg (middle)
	now := time.Now()
	_ = os.Chtimes(filepath.Join(root, "a.jpg"), now.Add(-10*time.Hour), now.Add(-10*time.Hour))
	_ = os.Chtimes(filepath.Join(root, "b.jpg"), now, now)
	_ = os.Chtimes(filepath.Join(root, "c.jpg"), now.Add(-5*time.Hour), now.Add(-5*time.Hour))

	// 3. Sort by date (ascending by default in quickcull)
	// Expected: [a.jpg, c.jpg, b.jpg]
	err := app.SetSortOrder("date")
	if err != nil {
		t.Fatalf("SetSortOrder failed: %v", err)
	}

	app.server.appStateMu.RLock()
	order := app.server.appState.VisibleOrder
	app.server.appStateMu.RUnlock()

	if order[0] != "a.jpg" || order[1] != "c.jpg" || order[2] != "b.jpg" {
		t.Errorf("Sort by date failed. Got: %v", order)
	}

	// 4. Sort back by name
	_ = app.SetSortOrder("name")
	if app.server.appState.VisibleOrder[0] != "a.jpg" {
		t.Errorf("Sort back by name failed. Got: %v", app.server.appState.VisibleOrder)
	}
}
