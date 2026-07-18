import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const { refresh, loadFile, mockedAppState } = vi.hoisted(() => {
  const loadFile = vi.fn(async () => undefined);
  return {
    refresh: vi.fn(),
    loadFile,
    mockedAppState: {
      config: { autoRefresh: true, autoRefreshSeconds: 5 },
      view: 'review',
      loading: false,
      stats: { total: 1 },
      currentIndex: 0,
      currentFile: { filename: 'a.jpg' } as { filename: string } | null,
      sessionVersion: 1,
      loadFile,
    },
  };
});

vi.mock('./api', () => ({ api: { refresh } }));
vi.mock('./appState.svelte', () => ({ appState: mockedAppState }));
vi.mock('./logger', () => ({
  logger: { info: vi.fn(), debug: vi.fn() },
}));

import { watchService } from './watchService.svelte';

describe('WatchService auto-refresh', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    mockedAppState.loading = false;
    mockedAppState.stats = { total: 1 };
    mockedAppState.currentIndex = 0;
    mockedAppState.currentFile = { filename: 'a.jpg' };
    (watchService as any).autoRefreshRunning = true;
    (watchService as any).autoRefreshBusy = false;
  });

  afterEach(() => {
    watchService.stop();
    vi.useRealTimers();
  });

  it('does not load a stale index when auto-refresh empties the folder', async () => {
    refresh.mockResolvedValue({ total: 0, index: -1, stats: { total: 0 } });

    await (watchService as any).tick();

    expect(loadFile).not.toHaveBeenCalled();
    expect(mockedAppState.currentFile).toBeNull();
    expect(mockedAppState.currentIndex).toBe(0);
  });

  it('loads the backend fallback when the current photo disappears', async () => {
	  mockedAppState.stats = { total: 2 };
	  mockedAppState.currentIndex = 1;
	  mockedAppState.currentFile = { filename: 'b.jpg' };
	  refresh.mockResolvedValue({ total: 1, index: 0, stats: { total: 1 } });

	  await (watchService as any).tick();

	  expect(loadFile).toHaveBeenCalledOnce();
	  expect(loadFile).toHaveBeenCalledWith(0);
  });
});
