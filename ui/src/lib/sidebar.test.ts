import { describe, it, expect } from 'vitest';
import { buildFolderTree } from './treeUtils';

describe('buildFolderTree', () => {
  it('should handle empty folders', () => {
    expect(buildFolderTree([])).toEqual([]);
  });

  it('should create root node for "."', () => {
    const folders = [{ path: '.', count: 10, startIndex: 0 }];
    const tree = buildFolderTree(folders);
    expect(tree.length).toBe(1);
    expect(tree[0].name).toBe('/');
    expect(tree[0].fullPath).toBe('.');
  });

  it('should reconstruct intermediate folders', () => {
    const folders = [
      { path: '2024/01', count: 5, startIndex: 0 }
    ];
    const tree = buildFolderTree(folders);
    
    // Root should be 2024 (auto-created)
    expect(tree.length).toBe(1);
    expect(tree[0].name).toBe('2024');
    expect(tree[0].folderInfo).toBeUndefined(); // Intermediate folder has no info
    
    // Child should be 01
    expect(tree[0].childrenArr.length).toBe(1);
    expect(tree[0].childrenArr[0].name).toBe('01');
    expect(tree[0].childrenArr[0].folderInfo).toBeDefined();
    expect(tree[0].childrenArr[0].folderInfo.count).toBe(5);
  });

  it('should handle Windows backslashes', () => {
    const folders = [
      { path: '2025\\03\\Day1', count: 2, startIndex: 0 }
    ];
    const tree = buildFolderTree(folders);
    expect(tree[0].name).toBe('2025');
    expect(tree[0].childrenArr[0].name).toBe('03');
    expect(tree[0].childrenArr[0].childrenArr[0].name).toBe('Day1');
  });

  it('should sort folders numerically and alphabetically', () => {
    const folders = [
      { path: 'Folder 10', count: 1, startIndex: 0 },
      { path: 'Folder 2', count: 1, startIndex: 1 },
      { path: 'Folder 1', count: 1, startIndex: 2 }
    ];
    const tree = buildFolderTree(folders);
    expect(tree[0].name).toBe('Folder 1');
    expect(tree[1].name).toBe('Folder 2');
    expect(tree[2].name).toBe('Folder 10');
  });
});
