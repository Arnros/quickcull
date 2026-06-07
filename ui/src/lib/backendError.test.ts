import { describe, expect, it, vi } from 'vitest';

vi.mock('./i18n.svelte', () => ({
  i18n: {
    t: (key: string) => `tr:${key}`,
  },
}));

import { backendErrorCode, localizeBackendError } from './backendError';

describe('backendError helpers', () => {
  it('extracts backend code when prefixed', () => {
    expect(backendErrorCode('QCERR:folder_not_found')).toBe('folder_not_found');
  });

  it('returns null for non-prefixed message', () => {
    expect(backendErrorCode('plain error')).toBeNull();
  });

  it('localizes known backend code', () => {
    expect(localizeBackendError('QCERR:folder_not_found')).toBe('tr:folder_not_found');
  });

  it('falls back to generic key for unknown backend code', () => {
    expect(localizeBackendError('QCERR:unexpected_code')).toBe('tr:generic_backend_error');
  });

  it('returns message unchanged when not a backend code', () => {
    expect(localizeBackendError('plain error')).toBe('plain error');
  });
});

