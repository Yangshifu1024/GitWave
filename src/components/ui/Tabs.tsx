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
        "flex shrink-0 items-center overflow-x-auto overflow-y-hidden border-b border-border-subtle",
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
  { value: string; className?: string; disabled?: boolean; children?: ReactNode }
>(({ value, className, disabled, children }, _ref) => (
  <HeroTabs.Tab
    id={value}
    isDisabled={disabled}
    className={cn(
      "shrink-0 px-3 py-2 text-sm font-medium text-text-secondary",
      "border-b-2 border-transparent -mb-px",
      className,
    )}
  >
    {children}
    <HeroTabs.Indicator />
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
