// Top-left system menu bar (File / Workspace / Repository / Branch).
// Items only dispatch `AppMenuAction` requests through the ui store —
// ActionBar owns the handlers and every dialog, so menu and button behavior
// stay identical by construction. Hovering another title while a menu is
// open switches to it (native menubar behavior); the first menu opens on
// click. On macOS the toolbar's left padding already clears the traffic
// lights, so the bar inherits the clearance from `.app-toolbar--macos`.

import { useEffect, useRef, useState } from "react";
import {
  ArrowDown,
  ArrowDownToLine,
  ArrowDownUp,
  ArrowUp,
  ArrowUpFromLine,
  Download,
  FolderGit2,
  FolderOpen,
  FolderPlus,
  FolderTree,
  GitBranch,
  GitPullRequest,
  Info,
  Package,
  Pencil,
  Power,
  Settings,
  Sparkles,
  Trash2,
  Webhook,
} from "lucide-react";

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "@/components/ui/DropdownMenu";
import { Button } from "@/components/ui/Button";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";
import { quitApp } from "@/lib/api";
import { isMacOS } from "@/lib/platform";
import { cn } from "@/lib/utils";
import { useUiStore, type AppMenuAction } from "@/stores/uiStore";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

type MenuId = "file" | "workspace" | "repository" | "branch";

const MENU_IDS: MenuId[] = ["file", "workspace", "repository", "branch"];

function MenuBarMenu({
  id,
  label,
  openMenu,
  setOpenMenu,
  registerTrigger,
  children,
}: {
  id: MenuId;
  label: string;
  openMenu: MenuId | null;
  setOpenMenu: (id: MenuId | null) => void;
  registerTrigger: (id: MenuId, el: HTMLButtonElement | null) => void;
  children: React.ReactNode;
}): React.JSX.Element {
  const isOpen = openMenu === id;
  // The Button must be a direct child of DropdownMenu: React Aria's
  // PressResponder merges the menu-trigger props into it, avoiding a nested
  // <button> (invalid HTML, double tab stop). aria-haspopup/aria-expanded
  // come from the library; the open state is highlighted via className.
  return (
    <DropdownMenu isOpen={isOpen} onOpenChange={(open) => setOpenMenu(open ? id : null)}>
      <Button
        ref={(el) => registerTrigger(id, el)}
        variant="ghost"
        size="sm"
        className={cn(
          "h-7 px-2.5 text-xs text-text-secondary hover:bg-bg-elevated hover:text-text-primary",
          isOpen && "bg-bg-elevated text-text-primary",
        )}
      >
        {label}
      </Button>
      <DropdownMenuContent placement="bottom start">{children}</DropdownMenuContent>
    </DropdownMenu>
  );
}

export function AppMenuBar({ onAbout }: { onAbout: () => void }): React.JSX.Element {
  const [openMenu, setOpenMenu] = useState<MenuId | null>(null);
  const setSettingsOpen = useUiStore((s) => s.setSettingsOpen);
  const requestMenuAction = useUiStore((s) => s.requestMenuAction);
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const wc = useWorkingCopy();

  // Gating mirrors the (former) ActionBar buttons item by item.
  const noWorkspace = !activeWorkspaceId;
  const noRepo = !activeRepoId;
  const detached = wc.data?.branch === "(detached)";

  // While a menu is open, HeroUI marks the app root `inert`, so sibling
  // triggers stop receiving pointer events and `onMouseEnter` can never fire.
  // Track pointer movement at the document level instead (events retarget to
  // <body>) and hit-test the trigger rects manually to switch menus.
  const triggerRefs = useRef<Partial<Record<MenuId, HTMLButtonElement | null>>>({});
  const registerTrigger = (id: MenuId, el: HTMLButtonElement | null): void => {
    triggerRefs.current[id] = el;
  };

  useEffect(() => {
    if (openMenu === null) return;
    const onPointerMove = (e: PointerEvent): void => {
      for (const id of MENU_IDS) {
        if (id === openMenu) continue;
        const el = triggerRefs.current[id];
        if (!el) continue;
        const r = el.getBoundingClientRect();
        if (
          e.clientX >= r.left &&
          e.clientX <= r.right &&
          e.clientY >= r.top &&
          e.clientY <= r.bottom
        ) {
          setOpenMenu(id);
          return;
        }
      }
    };
    document.addEventListener("pointermove", onPointerMove, { passive: true });
    return () => document.removeEventListener("pointermove", onPointerMove);
  }, [openMenu]);

  const fire =
    (action: AppMenuAction): (() => void) =>
    () =>
      requestMenuAction(action);

  return (
    // pointer-events-auto: the toolbar's content layer is pointer-events-none
    // so the drag region underneath stays clickable between controls.
    <div className="pointer-events-auto flex items-center gap-0.5 select-none">
      <MenuBarMenu
        id="file"
        label="File"
        openMenu={openMenu}
        setOpenMenu={setOpenMenu}
        registerTrigger={registerTrigger}
      >
        <DropdownMenuItem
          textValue="Settings"
          shortcut={isMacOS() ? "⌘," : "Ctrl+,"}
          onSelect={() => setSettingsOpen(true)}
        >
          <Settings size={14} />
          Settings
        </DropdownMenuItem>
        <DropdownMenuItem textValue="About" onSelect={onAbout}>
          <Info size={14} />
          About
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          textValue="Exit"
          onSelect={() => {
            quitApp().catch(() => undefined);
          }}
        >
          <Power size={14} />
          Exit
        </DropdownMenuItem>
      </MenuBarMenu>

      <MenuBarMenu
        id="workspace"
        label="Workspace"
        openMenu={openMenu}
        setOpenMenu={setOpenMenu}
        registerTrigger={registerTrigger}
      >
        <DropdownMenuItem textValue="New workspace" onSelect={fire("workspace:new")}>
          <FolderPlus size={14} />
          New
        </DropdownMenuItem>
        <DropdownMenuItem
          textValue="Rename workspace"
          disabled={noWorkspace}
          onSelect={fire("workspace:rename")}
        >
          <Pencil size={14} />
          Rename
        </DropdownMenuItem>
        <DropdownMenuItem
          textValue="AI provider"
          disabled={noWorkspace}
          onSelect={fire("workspace:ai")}
        >
          <Sparkles size={14} />
          AI
        </DropdownMenuItem>
        <DropdownMenuItem
          textValue="Export workspace"
          disabled={noWorkspace}
          onSelect={fire("workspace:export")}
        >
          <ArrowUpFromLine size={14} />
          Export
        </DropdownMenuItem>
        <DropdownMenuItem textValue="Import workspace" onSelect={fire("workspace:import")}>
          <ArrowDownToLine size={14} />
          Import
        </DropdownMenuItem>
        <DropdownMenuItem
          textValue="Delete workspace"
          disabled={noWorkspace}
          destructive
          onSelect={fire("workspace:delete")}
        >
          <Trash2 size={14} />
          Delete
        </DropdownMenuItem>
      </MenuBarMenu>

      <MenuBarMenu
        id="repository"
        label="Repository"
        openMenu={openMenu}
        setOpenMenu={setOpenMenu}
        registerTrigger={registerTrigger}
      >
        <DropdownMenuItem
          textValue="Initialize new repo"
          disabled={noWorkspace}
          onSelect={fire("repo:init")}
        >
          <FolderGit2 size={14} />
          Init
        </DropdownMenuItem>
        <DropdownMenuItem
          textValue="Clone remote repo"
          disabled={noWorkspace}
          onSelect={fire("repo:clone")}
        >
          <Download size={14} />
          Clone
        </DropdownMenuItem>
        <DropdownMenuItem
          textValue="Add existing local repo"
          disabled={noWorkspace}
          onSelect={fire("repo:add")}
        >
          <FolderOpen size={14} />
          Add
        </DropdownMenuItem>
        <DropdownMenuItem
          textValue="Fetch"
          disabled={noRepo || wc.isSyncBusy}
          onSelect={fire("repo:fetch")}
        >
          <ArrowDownUp size={14} />
          Fetch
        </DropdownMenuItem>
        <DropdownMenuItem
          textValue="Git LFS — track large files"
          disabled={noRepo}
          onSelect={fire("repo:lfs")}
        >
          <Package size={14} />
          LFS
        </DropdownMenuItem>
        <DropdownMenuItem
          textValue="Git hooks editor"
          disabled={noRepo}
          onSelect={fire("repo:hooks")}
        >
          <Webhook size={14} />
          Hooks
        </DropdownMenuItem>
        <DropdownMenuItem
          textValue="Create worktree"
          disabled={noRepo}
          onSelect={fire("repo:worktree-new")}
        >
          <FolderTree size={14} />
          New worktree
        </DropdownMenuItem>
      </MenuBarMenu>

      <MenuBarMenu
        id="branch"
        label="Branch"
        openMenu={openMenu}
        setOpenMenu={setOpenMenu}
        registerTrigger={registerTrigger}
      >
        <DropdownMenuItem
          textValue="New branch"
          disabled={noRepo || detached || !wc.data?.sha}
          onSelect={fire("branch:new")}
        >
          <GitBranch size={14} />
          New
        </DropdownMenuItem>
        <DropdownMenuItem
          textValue="Pull"
          disabled={noRepo || wc.isSyncBusy || detached}
          onSelect={fire("branch:pull")}
        >
          <ArrowDown size={14} />
          Pull
        </DropdownMenuItem>
        <DropdownMenuItem
          textValue="Push"
          disabled={noRepo || wc.isSyncBusy || detached}
          onSelect={fire("branch:push")}
        >
          <ArrowUp size={14} />
          Push
        </DropdownMenuItem>
        <DropdownMenuItem
          textValue="AI PR description for the current branch"
          disabled={noRepo || detached}
          onSelect={fire("branch:pr")}
        >
          <GitPullRequest size={14} />
          PR
        </DropdownMenuItem>
      </MenuBarMenu>
    </div>
  );
}
