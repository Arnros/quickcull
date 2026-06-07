export type TreeNode = {
  name: string;
  fullPath: string;
  folderInfo?: any;
  childrenArr: TreeNode[];
};

export function buildFolderTree(folders: any[]): TreeNode[] {
  if (!folders || folders.length === 0) return [];
  
  const rootNodes: TreeNode[] = [];
  const nodeMap = new Map<string, TreeNode>();

  function getOrCreateNode(path: string, isActualFolder: boolean, folderInfo?: any): TreeNode {
    const normalizedPath = path.replace(/\\/g, "/");
    const isRoot = normalizedPath === "." || normalizedPath === "" || normalizedPath === "/";
    const key = isRoot ? "." : normalizedPath;
    
    if (nodeMap.has(key)) {
      const existing = nodeMap.get(key)!;
      if (isActualFolder && !existing.folderInfo) {
        existing.folderInfo = folderInfo;
      }
      return existing;
    }

    const parts = isRoot ? [] : normalizedPath.split("/");
    const name = isRoot ? "/" : parts[parts.length - 1];
    
    const node: TreeNode = {
      name,
      fullPath: key,
      folderInfo: isActualFolder ? folderInfo : undefined,
      childrenArr: []
    };
    
    nodeMap.set(key, node);

    if (isRoot || parts.length === 1) {
      rootNodes.push(node);
    } else {
      const parentPath = parts.slice(0, -1).join("/") || ".";
      const parentNode = getOrCreateNode(parentPath, false);
      parentNode.childrenArr.push(node);
    }
    
    return node;
  }

  for (const f of folders) {
    if (!f || !f.path) continue;
    getOrCreateNode(f.path, true, f);
  }

  const sortNodes = (nodes: TreeNode[]) => {
    nodes.sort((a, b) => {
      if (a.name === "/") return -1;
      if (b.name === "/") return 1;
      return a.name.localeCompare(b.name, undefined, { numeric: true });
    });
    nodes.forEach(n => sortNodes(n.childrenArr));
  };

  sortNodes(rootNodes);
  return rootNodes;
}
