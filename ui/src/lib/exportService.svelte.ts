import { api } from './api';
import { logger } from './logger';
import { toastService } from './toast.svelte';
import { i18n } from './i18n.svelte';
import { getSelectedPaths, runExportFlow } from './appState.helpers';

interface IAppState {
  isExporting: boolean;
  selectedIndices: number[];
  v2: { VisibleOrder: string[] } | null;
}

class ExportService {
  async exportSelection(appState: IAppState, criteria: 'starred' | 'label', label: number = 0, move: boolean = false) {
    logger.info('Exporting selection', { criteria, label, move });
    try {
      await runExportFlow(appState, (destPath) => api.exportSelection(criteria, label, destPath, move));
    } catch (e) {
      console.error('Export failed', e);
      toastService.show(i18n.t('export_failed'), 'error');
    }
  }

  async exportSelected(appState: IAppState, move: boolean = false) {
    if (appState.selectedIndices.length === 0) return;
    logger.info('Exporting manual selection', { count: appState.selectedIndices.length, move });
    try {
      await runExportFlow(appState, async (destPath) => {
        const selectedPaths = getSelectedPaths(appState.selectedIndices, appState.v2?.VisibleOrder || []);
        await api.exportFiles(selectedPaths, destPath, move);
        appState.selectedIndices = []; // Clear selection after export start
      });
    } catch (e) {
      console.error('Export failed', e);
      toastService.show(i18n.t('export_failed'), 'error');
    }
  }
}

export const exportService = new ExportService();
