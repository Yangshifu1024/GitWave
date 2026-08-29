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
    disabled?: boolean;
    children?: ReactNode;
    /** DOM passthroughs — the TabList collection drops non-Tab wrappers, so
     * per-tab attributes (title, onContextMenu, …) must ride on the Tab. */
    title?: string;
    onContextMenu?: (event: React.MouseEvent<HTMLDivElement>) => void;
  }
>(({ value, className, disabled, children, title, onContextMenu }, ref) => (
  // React Aria wraps Tab in a Pressable that needs the DOM element: the
  // ref MUST be forwarded or the tab tree crashes on mount.
  <HeroTabs.Tab
    ref={ref}
    id={value}
    isDisabled={disabled}
    // spread bypasses TabProps' narrow typing; RAC forwards standard DOM
    // attributes (title / handlers) to the rendered tab element.
    {...{ title, onContextMenu }}
    className={cn(
      // "group" lets the underline child react to this tab's data-selected;
      // grow shares the full row across tabs (HeroUI's w-full would make
      // every tab row-wide instead); w-auto keeps intrinsic width as basis.
      "group w-auto grow px-3 text-sm font-medium text-text-secondary",
      // Connected-tab look: every tab carries the row's bottom hairline so
      // the line reads as continuous; the selected tab drops its own segment
      // and grows to the row height, letting its panel-colored body merge
      // into the content surface below.
      "rounded-t-md rounded-b-none border-b border-border-subtle",
      "data-[selected=true]:border-bg-panel",
      // Hover = text emphasis only; a filled hover slab looks odd on
      // row-wide tabs.
      "data-[hovered=true]:text-text-primary data-[hovered=true]:opacity-100",
      // Selected reads as "pinned hover": raised bg, bold text, soft shadow.
      "data-[selected=true]:bg-bg-panel data-[selected=true]:text-text-primary data-[selected=true]:opacity-100",
      "data-[selected=true]:font-semibold",
      "data-[selected=true]:shadow-[0_2px_8px_rgba(0,0,0,0.15)]",
      // Kill HeroUI's browser-y focus ring (ring-2 accent) — it re-appears
      // after alt-tab via data-focus-visible. Keyboard focus gets the raised
      // background instead, desktop-style.
      "focus-visible:outline-none focus-visible:ring-0",
      "data-[focus-visible=true]:outline-none data-[focus-visible=true]:ring-0 data-[focus-visible=true]:bg-bg-panel data-[focus-visible=true]:text-text-primary",
      className,
    )}
  >
    {children}
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
