import { api } from './api';
import { appState } from './appState.svelte';
import { logger } from './logger';

class WatchService {
  private autoRefreshRunning = false;
  private autoRefreshBusy = false;
  private autoRefreshTimer: ReturnType<typeof setTimeout> | null = null;
  private stableTicks = 0;

  start() {
    if (this.autoRefreshRunning) return;
    this.autoRefreshRunning = true;
    this.stableTicks = 0;
    logger.info('Auto-refresh service started');
    this.tick();
  }

  debugBackoffFactor() {
    return Math.min(6, Math.max(1, this.stableTicks + 1));
  }

  debugCurrentIntervalSeconds() {
    const baseSeconds = Math.max(1, Number(appState.config?.autoRefreshSeconds || 5));
    return baseSeconds * this.debugBackoffFactor();
  }

  stop() {
    this.autoRefreshRunning = false;
    if (this.autoRefreshTimer) {
      clearTimeout(this.autoRefreshTimer);
      this.autoRefreshTimer = null;
    }
    logger.info('Auto-refresh service stopped');
  }

  private async tick() {
    if (!this.autoRefreshRunning) return;

    const cfg = appState.config;
    const baseSeconds = Math.max(1, Number(cfg?.autoRefreshSeconds || 5));
    const backoffFactor = this.debugBackoffFactor();
    const seconds = baseSeconds * backoffFactor;
    this.autoRefreshTimer = setTimeout(() => {
      void this.tick();
    }, seconds * 1000);

    if (!cfg?.autoRefresh) return;
    if (appState.view !== 'review') return;
    if (appState.loading || this.autoRefreshBusy) return;

    this.autoRefreshBusy = true;
    try {
      const beforeTotal = appState.stats.total;
      const beforeIndex = appState.currentIndex;
      const beforeFilename = appState.currentFile?.filename;

      const res = await api.refresh(appState.currentIndex);
      if (!res) return;

      // Only update stats if they actually changed to avoid triggering unnecessary reactivity
      if (JSON.stringify(res.stats) !== JSON.stringify(appState.stats)) {
        appState.stats = res.stats;
      }

      const totalChanged = res.total !== beforeTotal;
      // If backend returns -1, it means it couldn't find our current file identity.
      // We should stay on our current index and NOT trigger a reload.
      const indexChanged = res.index !== -1 && res.index !== beforeIndex;

      if (totalChanged || indexChanged) {
        this.stableTicks = 0;
        logger.info('Auto-refresh detected changes', { 
          totalChanged, 
          indexChanged,
          beforeTotal, 
          afterTotal: res.total,
          beforeIndex,
          afterIndex: res.index
        });
        
        appState.sessionVersion = Date.now();
        
        // Parallel refresh of core data
        const updates: Promise<any>[] = [];
        
        // Only load the file if the identity actually shifted and we have a valid new index
        if (totalChanged || indexChanged || !beforeFilename) {
          const targetIndex = (res.index !== -1 && res.index !== undefined) ? res.index : beforeIndex;
          updates.push(appState.loadFile(targetIndex));
        }
        
        await Promise.all(updates);
      } else {
        this.stableTicks = Math.min(5, this.stableTicks + 1);
      }
    } catch (e) {
      logger.debug('Auto-refresh tick failed (silent)', { error: e });
    } finally {
      this.autoRefreshBusy = false;
    }
  }
}

export const watchService = new WatchService();
