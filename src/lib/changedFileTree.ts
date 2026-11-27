interface FileNode {
  name: string;
  path: string;
}

interface FolderNode extends FileNode {
  children: ChangedFileNode[];
}

export type ChangedFileNode = FileNode | FolderNode;

export function buildChangedFileTree(files: readonly { path: string }[]) {
  const roots: ChangedFileNode[] = [];
  const folders = new Map<string, FolderNode>();
  const filePaths = new Set<string>();
  for (const { path } of files) {
    const parts = path.split("/");
    const name = parts.pop()!;
    let children = roots;
    let prefix = "";
    for (const part of parts) {
      prefix = prefix ? `${prefix}/${part}` : part;
      let folder = folders.get(prefix);
      if (!folder) {
        folder = { name: part, path: prefix, children: [] };
        folders.set(prefix, folder);
        children.push(folder);
      }
      children = folder.children;
    }
    children.push({ name, path });
    filePaths.add(path);
  }

  function compare(a: ChangedFileNode, b: ChangedFileNode) {
    const byKind = Number("children" in b) - Number("children" in a);
    return byKind || a.name.localeCompare(b.name);
  }
  roots.sort(compare);
  for (const folder of folders.values()) folder.children.sort(compare);

  let first: ChangedFileNode | undefined = roots[0];
  while (first && "children" in first) first = first.children[0];
  return { roots, folders, filePaths, fileCount: files.length, firstPath: first?.path };
}

export function parentFolders(path: string) {
  const parents = new Set<string>();
  for (let i = path.indexOf("/"); i !== -1; i = path.indexOf("/", i + 1)) {
    parents.add(path.slice(0, i));
  }
  return parents;
}
