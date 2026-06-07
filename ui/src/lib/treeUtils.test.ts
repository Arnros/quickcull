import { describe, expect, it } from 'vitest';

import { buildFolderTree } from './treeUtils';

describe('buildFolderTree', () => {
  it('returns empty list for empty input', () => {
    expect(buildFolderTree([])).toEqual([]);
    expect(buildFolderTree(undefined as any)).toEqual([]);
  });

  it('builds a nested tree and keeps root first', () => {
    const tree = buildFolderTree([
      { path: '2024/2', count: 1 },
      { path: '2024/10', count: 1 },
      { path: '.', count: 5 },
      { path: 'A', count: 1 },
    ]);

    expect(tree.map((n) => n.name)).toEqual(['/', '2024', 'A']);
    const yearNode = tree.find((n) => n.fullPath === '2024');
    expect(yearNode?.childrenArr.map((n) => n.name)).toEqual(['2', '10']);
  });

  it('normalizes windows-style separators', () => {
    const tree = buildFolderTree([{ path: 'foo\\bar', count: 1 }]);
    expect(tree[0].fullPath).toBe('foo');
    expect(tree[0].childrenArr[0].fullPath).toBe('foo/bar');
  });
});

