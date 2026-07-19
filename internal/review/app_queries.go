package review

import "quickcull/internal/domain"

func (a *App) requireState() (*State, error) {
	state := a.server.getState()
	if state == nil {
		return nil, domain.ErrFolderNotFound
	}
	return state, nil
}

// verifyFile is a helper to ensure state exists and index matches filename.
func (a *App) verifyFile(index int, path string) (*State, string, int, error) {
	state, err := a.requireState()
	if err != nil {
		return nil, "", -1, err
	}
	resolved, err := resolveRequestIndex(state, index, path)
	if err != nil {
		return nil, "", -1, err
	}
	actualPath, _ := state.Get(resolved)
	return state, actualPath, resolved, nil
}

// GetAppState returns the current immutable state.
func (a *App) GetAppState() (AppState, error) {
	a.server.appStateMu.RLock()
	defer a.server.appStateMu.RUnlock()
	if a.server.appState == nil {
		return AppState{}, nil
	}
	return a.server.appState.Clone(true), nil
}

// GetStats returns current folder statistics.
func (a *App) GetStats() (AppStats, error) {
	return a.snapshotStats(), nil
}

func (a *App) snapshotStats() AppStats {
	return a.server.snapshotStats(a.server.getState())
}

func (a *App) getPhoto(id string) (Photo, bool) {
	a.server.appStateMu.RLock()
	defer a.server.appStateMu.RUnlock()
	if a.server.appState == nil {
		return Photo{}, false
	}
	p, ok := a.server.appState.photo(id)
	return p, ok
}

// GetAnalysisProgress returns current background analysis progress.
func (a *App) GetAnalysisProgress() (AnalysisProgressResponse, error) {
	current, total, _, _ := a.server.analysisProgress()
	return AnalysisProgressResponse{
		Current: current,
		Total:   total,
	}, nil
}
