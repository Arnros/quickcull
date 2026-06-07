import { api } from './api';
import { domain } from '../../wailsjs/go/models';

class ViewState {
  current = $state<'picker' | 'review'>('picker');
  config = $state<domain.Config | null>(null);

  sidebarOpen = $state(false);
  filmstripOpen = $state(true);
  infoOpen = $state(true);
  gridOpen = $state(false);
  gridScrollTop = $state(0);
  gridScrollLeft = $state(0);
  settingsOpen = $state(false);
  helpOpen = $state(false);
  zoomed = $state(false);
  zenMode = $state(false);
  comparisonMode = $state(false);

  toggleTheme() {
    if (this.config) {
      this.config.theme = this.config.theme === 'dark' ? 'light' : 'dark';
      api.updateConfig(this.config);
    }
  }
}

export const viewState = new ViewState();
