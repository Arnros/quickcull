import { translations, type Lang } from './translations';

class I18n {
  lang = $state<Lang>('en');

  constructor() {
    if (typeof window === 'undefined') return;

    let saved: Lang | null = null;
    if (typeof localStorage !== 'undefined' && typeof localStorage.getItem === 'function') {
      try {
        saved = localStorage.getItem('lang') as Lang;
      } catch (e) {
        // Ignore potential mock/access errors
      }
    }

    if (saved && translations[saved]) {
      this.lang = saved;
    } else if (typeof navigator !== 'undefined' && navigator.language) {
      const userLang = navigator.language.split('-')[0] as Lang;
      this.lang = translations[userLang] ? (userLang as Lang) : 'en';
    } else {
      this.lang = 'en';
    }
  }

  setLang(l: Lang) {
    if (translations[l]) {
      this.lang = l;
      localStorage.setItem('lang', l);
    }
  }

  t(key: keyof typeof translations['en']): string {
    return translations[this.lang][key] || translations['en'][key] || key;
  }
}

export const i18n = new I18n();
