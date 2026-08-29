// Top-left system menu bar (File / Workspace / Repository / Branch).
// Windows / Linux only: on macOS the same structure is installed as the
// native system menu instead (useNativeAppMenu) and this bar is not
// rendered. The menu structure lives in appMenuSpec.ts so both surfaces
// gate and dispatch identically by construction; items only dispatch
// requests — ActionBar owns the handlers and every dialog, so menu and
// button behavior stay identical. Hovering another title while a menu is
// open switches to it (native menubar behavior); the first menu opens on
// click.

import { useEffect, useMemo, useRef, useState } from "react";
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
  type LucideIcon,
} from "lucide-react";

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "@/components/ui/DropdownMenu";
import { Button } from "@/components/ui/Button";
import { useAppMenuGating } from "@/hooks/useAppMenuGating";
import { quitApp } from "@/lib/api";
import {
  buildAppMenuSpec,
  dispatchAppMenuItem,
  type AppMenuItemId,
  type AppMenuSpec,
} from "@/lib/appMenuSpec";
import { cn } from "@/lib/utils";
import { useUiStore } from "@/stores/uiStore";

type MenuId = AppMenuSpec["id"];

// Windows / Linux icons; the native macOS menu intentionally renders no
// icons (platform convention). The Record type keeps the map exhaustive
// as menu items come and go.
const ITEM_ICONS: Record<AppMenuItemId, LucideIcon> = {
  settings: Settings,
  about: Info,
  quit: Power,
  "workspace:new": FolderPlus,
  "workspace:rename": Pencil,
  "workspace:ai": Sparkles,
  "workspace:export": ArrowUpFromLine,
  "workspace:import": ArrowDownToLine,
  "workspace:delete": Trash2,
  "repo:init": FolderGit2,
  "repo:clone": Download,
  "repo:add": FolderOpen,
  "repo:fetch": ArrowDownUp,
  "repo:lfs": Package,
  "repo:hooks": Webhook,
  "repo:worktree-new": FolderTree,
  "branch:new": GitBranch,
  "branch:pull": ArrowDown,
  "branch:push": ArrowUp,
  "branch:pr": GitPullRequest,
};

function MenuItemIcon({ id }: { id: AppMenuItemId }): React.JSX.Element {
  const Icon = ITEM_ICONS[id];
  return <Icon size={14} />;
}

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
  const gating = useAppMenuGating();
  const menus = useMemo(() => buildAppMenuSpec(gating), [gating]);

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
      for (const menu of menus) {
        if (menu.id === openMenu) continue;
        const el = triggerRefs.current[menu.id];
        if (!el) continue;
        const r = el.getBoundingClientRect();
        if (
          e.clientX >= r.left &&
          e.clientX <= r.right &&
          e.clientY >= r.top &&
          e.clientY <= r.bottom
        ) {
          setOpenMenu(menu.id);
          return;
        }
      }
    };
    document.addEventListener("pointermove", onPointerMove, { passive: true });
    return () => document.removeEventListener("pointermove", onPointerMove);
  }, [openMenu, menus]);

  const dispatch = (id: AppMenuItemId): void => {
    dispatchAppMenuItem(id, {
      requestMenuAction,
      openSettings: () => setSettingsOpen(true),
      openAbout: onAbout,
      quit: () => {
        quitApp().catch(() => undefined);
      },
    });
  };

  return (
    // pointer-events-auto: the toolbar's content layer is pointer-events-none
    // so the drag region underneath stays clickable between controls.
    <div className="pointer-events-auto flex items-center gap-0.5 select-none">
      {menus.map((menu) => (
        <MenuBarMenu
          key={menu.id}
          id={menu.id}
          label={menu.label}
          openMenu={openMenu}
          setOpenMenu={setOpenMenu}
          registerTrigger={registerTrigger}
        >
          {menu.entries.map((entry, index) =>
            "separator" in entry ? (
              <DropdownMenuSeparator key={`sep-${index}`} />
            ) : (
              <DropdownMenuItem
                key={entry.id}
                textValue={entry.textValue}
                disabled={!entry.enabled}
                destructive={entry.destructive}
                // This bar never mounts on macOS, so the settings shortcut is
                // always the Ctrl+, variant here.
                shortcut={entry.id === "settings" ? "Ctrl+," : undefined}
                onSelect={() => dispatch(entry.id)}
              >
                <MenuItemIcon id={entry.id} />
                {entry.label}
              </DropdownMenuItem>
            ),
          )}
        </MenuBarMenu>
      ))}
    </div>
  );
}
