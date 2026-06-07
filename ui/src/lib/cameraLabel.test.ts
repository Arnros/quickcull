import { describe, expect, it, vi } from 'vitest';

vi.mock('./i18n.svelte', () => ({
  i18n: {
    t: (key: string) => `tr:${key}`,
  },
}));

import { cameraLabel } from './cameraLabel';

describe('cameraLabel', () => {
  it('maps RAW no-exiftool sentinels to localized label', () => {
    expect(cameraLabel('__raw_no_exiftool__')).toBe('tr:raw_no_exiftool_camera');
    expect(cameraLabel('RAW (No Exiftool)')).toBe('tr:raw_no_exiftool_camera');
  });

  it('maps RAW metadata unavailable sentinel to localized label', () => {
    expect(cameraLabel('__raw_metadata_unavailable__')).toBe('tr:raw_metadata_unavailable_camera');
  });

  it('returns untouched camera value for regular models', () => {
    expect(cameraLabel('Sony A7C')).toBe('Sony A7C');
  });
});

