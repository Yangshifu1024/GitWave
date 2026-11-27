import { useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  File,
  Folder,
  FoldVertical,
  UnfoldVertical,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import {
  type ChangedFileNode,
  type buildChangedFileTree,
  parentFolders,
} from "@/lib/changedFileTree";
import { cn } from "@/lib/utils";

interface Props {
  tree: ReturnType<typeof buildChangedFileTree>;
  selected: string | undefined;
  onSelect: (path: string) => void;
}

export function HistoryFileTree({ tree, selected, onSelect }: Props) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(() => parentFolders(selected ?? ""));
  const toggle = (path: string) => {
    setExpanded((previous) => {
      const next = new Set(previous);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  };

  function renderRows(nodes: ChangedFileNode[]): React.ReactNode {
    return nodes.map((node, index) => {
      if ("children" in node) {
        const open = expanded.has(node.path);
        const Chevron = open ? ChevronDown : ChevronRight;
        return (
          <li key={`folder:${node.path}`}>
            <button
              type="button"
              className="workbench-file"
              aria-expanded={open}
              title={node.path}
              onClick={() => toggle(node.path)}
            >
              <Chevron size={12} className="shrink-0" />
              <Folder size={14} className="shrink-0" />
              <span className="truncate">{node.name}</span>
            </button>
            {open && <ul className="pl-3">{renderRows(node.children)}</ul>}
          </li>
        );
      }
      return (
        <li key={`file:${node.path}:${index}`}>
          <button
            type="button"
            aria-pressed={node.path === selected}
            aria-label={node.path}
            title={node.path}
            className={cn("workbench-file", node.path === selected && "workbench-file-selected")}
            onClick={() => onSelect(node.path)}
          >
            <File size={14} className="ml-[18px] shrink-0" />
            <span className="truncate font-mono">{node.name}</span>
          </button>
        </li>
      );
    });
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-bg-elevated">
      <div className="workbench-section-heading items-center gap-1">
        <span className="min-w-0 truncate">
          {t("app.workbench.files")} <span className="tabular-nums">{tree.fileCount}</span>
        </span>
        {tree.folders.size > 0 && (
          <div className="flex shrink-0">
            <Button
              size="sm"
              variant="ghost"
              className="size-6 p-0"
              aria-label={t("app.workbench.expandFolders")}
              title={t("app.workbench.expandFolders")}
              onClick={() => setExpanded(new Set(tree.folders.keys()))}
            >
              <UnfoldVertical size={14} />
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="size-6 p-0"
              aria-label={t("app.workbench.collapseFolders")}
              title={t("app.workbench.collapseFolders")}
              onClick={() => setExpanded(new Set())}
            >
              <FoldVertical size={14} />
            </Button>
          </div>
        )}
      </div>
      <div className="min-h-0 flex-1 overflow-auto py-1">
        <ul aria-label={t("app.workbench.files")}>{renderRows(tree.roots)}</ul>
      </div>
    </div>
  );
}
