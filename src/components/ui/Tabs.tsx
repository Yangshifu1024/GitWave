import { type ReactNode, forwardRef } from "react";
import { Tabs as HeroTabs } from "@heroui/react";
import { cn } from "@/lib/utils";

export interface TabsProps {
  value?: string;
  defaultValue?: string;
  onValueChange?: (value: string) => void;
  className?: string;
  orientation?: "horizontal" | "vertical";
  children?: ReactNode;
}

export function Tabs({
  value,
  defaultValue,
  onValueChange,
  className,
  orientation,
  children,
}: TabsProps): React.JSX.Element {
  return (
    <HeroTabs
      selectedKey={value}
      defaultSelectedKey={defaultValue}
      onSelectionChange={(key) => {
        if (key != null) onValueChange?.(String(key));
      }}
      className={className}
      orientation={orientation}
    >
      {children}
    </HeroTabs>
  );
}

export const TabsList = forwardRef<HTMLDivElement, { className?: string; children?: ReactNode }>(
  ({ className, children }, _ref) => (
    <HeroTabs.ListContainer
      className={cn(
        // items-end sits tabs on the row's bottom edge so a selected tab's
        // panel background can flow into the content surface below. No
        // container border: each tab paints its own bottom hairline, and a
        // container-wide border would reappear under the selected tab,
        // breaking the connected look.
        "flex shrink-0 items-end overflow-x-auto overflow-y-hidden",
        "[scrollbar-width:none]",
        "[&::-webkit-scrollbar]:hidden",
        className,
      )}
    >
      <HeroTabs.List>{children}</HeroTabs.List>
    </HeroTabs.ListContainer>
  ),
);
TabsList.displayName = "TabsList";

export const TabsTrigger = forwardRef<
  HTMLDivElement,
  {
    value: string;
    className?: string;
    /** Full interactive kill: HeroUI's disabled tab applies
     * pointer-events:none, so onContextMenu / onPointerDown never fire
     * either. Tabs that must stay right-clickable or draggable (e.g. a
     * missing repo tab whose only escape hatch is the context menu) should
     * use visual de-emphasis classes instead. */
    disabled?: boolean;
    children?: ReactNode;
    /** DOM passthroughs — the TabList collection drops non-Tab wrappers, so
     * per-tab attributes (title, onContextMenu, onPointerDown, …) must ride
     * on the Tab. */
    title?: string;
    onContextMenu?: (event: React.MouseEvent<HTMLDivElement>) => void;
    onPointerDown?: (event: React.PointerEvent<HTMLDivElement>) => void;
  }
>(({ value, className, disabled, children, title, onContextMenu, onPointerDown }, ref) => (
  // React Aria wraps Tab in a Pressable that needs the DOM element: the
  // ref MUST be forwarded or the tab tree crashes on mount.
  <HeroTabs.Tab
    ref={ref}
    id={value}
    isDisabled={disabled}
    // spread bypasses TabProps' narrow typing; RAC forwards standard DOM
    // attributes (title / handlers) to the rendered tab element.
    {...{ title, onContextMenu, onPointerDown }}
    className={cn(
      // "group" lets the underline child react to this tab's data-selected;
      // grow shares the full row across tabs (HeroUI's w-full would make
      // every tab row-wide instead); w-auto keeps intrinsic width as basis.
      "group w-auto grow px-3 text-[13px] font-medium text-text-secondary",
      // Native-style tab: no rounded top, no shadow, no connected-body trick.
      // Selected state is a single bottom accent line plus a very subtle fill.
      "rounded-none border-0 border-b border-border-subtle",
      "data-[selected=true]:border-transparent",
      // Hover = text emphasis only; a filled hover slab looks odd on
      // row-wide tabs.
      "data-[hovered=true]:text-text-primary data-[hovered=true]:opacity-100",
      // Selected: subtle background + bottom accent line via pseudo child.
      "data-[selected=true]:bg-black/[0.03] dark:data-[selected=true]:bg-white/[0.03]",
      "data-[selected=true]:text-text-primary data-[selected=true]:opacity-100",
      "data-[selected=true]:font-medium",
      // Kill HeroUI's browser-y focus ring (ring-2 accent) — it re-appears
      // after alt-tab via data-focus-visible. Keyboard focus gets the subtle
      // background instead, desktop-style.
      "focus-visible:outline-none focus-visible:ring-0",
      "data-[focus-visible=true]:outline-none data-[focus-visible=true]:ring-0 data-[focus-visible=true]:bg-black/[0.03] dark:data-[focus-visible=true]:bg-white/[0.03] data-[focus-visible=true]:text-text-primary",
      className,
    )}
  >
    <span className="relative inline-flex items-center justify-center gap-1.5 h-full w-full">
      {children}
      <span
        className={cn(
          "absolute bottom-0 left-2 right-2 h-[2px] bg-accent opacity-0",
          "group-data-[selected=true]:opacity-100",
        )}
        aria-hidden
      />
    </span>
  </HeroTabs.Tab>
));
TabsTrigger.displayName = "TabsTrigger";

export const TabsContent = forwardRef<
  HTMLDivElement,
  { value: string; className?: string; children?: ReactNode }
>(({ value, className, children }, _ref) => (
  <HeroTabs.Panel
    id={value}
    className={cn("flex-1 overflow-auto p-4 focus-visible:outline-none", className)}
  >
    {children}
  </HeroTabs.Panel>
));
TabsContent.displayName = "TabsContent";
