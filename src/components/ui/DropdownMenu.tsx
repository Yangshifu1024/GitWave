import { type ReactNode, forwardRef } from "react";
import { Dropdown, Header, Kbd, Separator } from "@heroui/react";
import { cn } from "@/lib/utils";

export const DropdownMenu = Dropdown;
export const DropdownMenuTrigger = Dropdown.Trigger;
export const DropdownMenuGroup = ({ children }: { children?: ReactNode }): React.JSX.Element => (
  <>{children}</>
);

export const DropdownMenuContent = forwardRef<
  HTMLDivElement,
  {
    className?: string;
    sideOffset?: number;
    placement?: React.ComponentProps<typeof Dropdown.Popover>["placement"];
    children?: ReactNode;
  }
>(({ className, sideOffset = 6, placement, children }, _ref) => (
  <Dropdown.Popover
    placement={placement}
    offset={sideOffset}
    className={cn(
      "z-popover min-w-[180px] rounded-lg",
      "bg-bg-elevated border border-border-default shadow-modal",
      "p-1",
      className,
    )}
  >
    <Dropdown.Menu>{children}</Dropdown.Menu>
  </Dropdown.Popover>
));
DropdownMenuContent.displayName = "DropdownMenuContent";

export interface DropdownMenuItemProps {
  className?: string;
  destructive?: boolean;
  disabled?: boolean;
  onSelect?: () => void;
  children?: ReactNode;
  id?: string;
  /** Plain-text label for the item id / typeahead (pass when children isn't plain text). */
  textValue?: string;
  /** Right-aligned keyboard shortcut hint, rendered inside a Kbd. */
  shortcut?: ReactNode;
}

export const DropdownMenuItem = forwardRef<HTMLDivElement, DropdownMenuItemProps>(
  (
    { className, destructive = false, disabled, onSelect, children, id, textValue, shortcut },
    _ref,
  ) => (
    <Dropdown.Item
      id={id ?? textValue ?? (typeof children === "string" ? children : undefined)}
      textValue={textValue}
      isDisabled={disabled}
      variant={destructive ? "danger" : undefined}
      onAction={onSelect}
      className={cn(
        "relative flex cursor-pointer select-none items-center gap-2 rounded-md px-2 py-1.5 text-sm",
        "outline-none",
        destructive ? "text-danger" : "text-text-primary",
        className,
      )}
    >
      {children}
      {shortcut != null && (
        <Kbd variant="light" className="ms-auto">
          {shortcut}
        </Kbd>
      )}
    </Dropdown.Item>
  ),
);
DropdownMenuItem.displayName = "DropdownMenuItem";

export const DropdownMenuSeparator = forwardRef<HTMLElement, { className?: string }>(
  ({ className }, _ref) => <Separator className={cn("my-1 bg-border-subtle", className)} />,
);
DropdownMenuSeparator.displayName = "DropdownMenuSeparator";

export const DropdownMenuLabel = forwardRef<
  HTMLElement,
  { className?: string; children?: ReactNode }
>(({ className, children }, _ref) => (
  <Header className={cn("px-2 py-1 text-xs font-medium text-text-muted", className)}>
    {children}
  </Header>
));
DropdownMenuLabel.displayName = "DropdownMenuLabel";
