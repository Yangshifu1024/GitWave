import {
  cloneElement,
  createContext,
  isValidElement,
  useContext,
  useEffect,
  useId,
  useState,
  type MouseEvent,
  type ReactElement,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { Header, Menu, Popover, Separator } from "@heroui/react";
import { SubmenuTrigger } from "react-aria-components";
import { ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";

interface ContextMenuState {
  isOpen: boolean;
  setOpen: (open: boolean) => void;
  point: { x: number; y: number };
  setPoint: (point: { x: number; y: number }) => void;
}

const ContextMenuStateContext = createContext<ContextMenuState | null>(null);

function useContextMenuState(): ContextMenuState {
  const ctx = useContext(ContextMenuStateContext);
  if (!ctx) {
    throw new Error("ContextMenu components must be used within <ContextMenu>");
  }
  return ctx;
}

export function ContextMenu({ children }: { children: ReactNode }): React.JSX.Element {
  const [isOpen, setOpen] = useState(false);
  const [point, setPoint] = useState({ x: 0, y: 0 });

  useEffect(() => {
    if (!isOpen) return;
    const close = (): void => setOpen(false);
    // scroll 不冒泡，capture 才能覆盖任意内部滚动容器
    window.addEventListener("scroll", close, { capture: true, passive: true });
    return () => window.removeEventListener("scroll", close, { capture: true });
  }, [isOpen]);

  return (
    <ContextMenuStateContext.Provider value={{ isOpen, setOpen, point, setPoint }}>
      {children}
    </ContextMenuStateContext.Provider>
  );
}

type TriggerChildProps = {
  onContextMenu?: (event: MouseEvent<HTMLElement>) => void;
};

export function ContextMenuTrigger({
  asChild = false,
  children,
  className,
}: {
  asChild?: boolean;
  children: ReactNode;
  className?: string;
}): React.JSX.Element {
  const ctx = useContextMenuState();

  const onContextMenu = (event: MouseEvent<HTMLElement>): void => {
    event.preventDefault();
    event.stopPropagation();
    ctx.setPoint({ x: event.clientX, y: event.clientY });
    ctx.setOpen(true);
  };

  if (asChild && isValidElement(children)) {
    const child = children as ReactElement<TriggerChildProps>;
    return cloneElement(child, {
      onContextMenu: (event: MouseEvent<HTMLElement>) => {
        child.props.onContextMenu?.(event);
        onContextMenu(event);
      },
    });
  }

  return (
    <div className={className} onContextMenu={onContextMenu}>
      {children}
    </div>
  );
}

export function ContextMenuContent({
  className,
  children,
}: {
  className?: string;
  children?: ReactNode;
}): React.JSX.Element {
  const ctx = useContextMenuState();

  return (
    <Popover isOpen={ctx.isOpen} onOpenChange={ctx.setOpen}>
      {/* 锚点必须 portal 到 body：CommitGraph 虚拟行 wrapper 带 transform，
          会成为 fixed 后代的包含块，导致 clientX/clientY 被按行坐标解释、菜单漂移 */}
      {createPortal(
        <Popover.Trigger
          aria-hidden
          className="fixed z-popover h-px w-px overflow-hidden p-0 pointer-events-none"
          style={{ left: ctx.point.x, top: ctx.point.y }}
        />,
        document.body,
      )}
      <Popover.Content
        placement="bottom start"
        offset={2}
        className={cn(
          "z-popover min-w-[180px] rounded-lg",
          "bg-bg-elevated border border-border-default shadow-modal",
          "p-1",
          className,
        )}
      >
        <Menu className="outline-none">{children}</Menu>
      </Popover.Content>
    </Popover>
  );
}

export function ContextMenuLabel({
  className,
  children,
  title,
}: {
  className?: string;
  children?: ReactNode;
  title?: string;
}): React.JSX.Element {
  return (
    <Header
      title={title}
      className={cn("px-2 py-1.5 text-xs font-medium text-text-primary truncate", className)}
    >
      {children}
    </Header>
  );
}

/** Fork-style submenu: a trigger item that opens a nested menu on hover /
 *  arrow-right. `children` are the nested items; selecting one closes the
 *  whole menu (handled by ContextMenuItem). */
export function ContextMenuSub({
  children,
  disabled = false,
  icon,
  label,
  title,
}: {
  /** Nested menu items. */
  children: ReactNode;
  disabled?: boolean;
  icon?: ReactNode;
  label: ReactNode;
  title?: string;
}): React.JSX.Element {
  return (
    <SubmenuTrigger>
      <Menu.Item
        textValue={typeof label === "string" ? label : (title ?? "submenu")}
        isDisabled={disabled}
        className={cn(
          "relative flex cursor-pointer select-none items-center gap-2 rounded-md px-2 py-1.5 text-sm",
          "outline-none text-text-primary",
        )}
      >
        {icon}
        {label}
        <ChevronRight size={14} className="ml-auto shrink-0 text-text-muted" />
      </Menu.Item>
      <Popover.Content
        className={cn(
          "z-popover min-w-[140px] rounded-lg",
          "bg-bg-elevated border border-border-default shadow-modal",
          "p-1",
        )}
      >
        <Menu className="outline-none">{children}</Menu>
      </Popover.Content>
    </SubmenuTrigger>
  );
}

export interface ContextMenuItemProps {
  className?: string;
  destructive?: boolean;
  disabled?: boolean;
  title?: string;
  onSelect?: () => void;
  children?: ReactNode;
}

export function ContextMenuItem({
  className,
  destructive = false,
  disabled,
  title,
  onSelect,
  children,
}: ContextMenuItemProps): React.JSX.Element {
  const ctx = useContextMenuState();
  const id = useId();

  return (
    <Menu.Item
      id={id}
      textValue={
        typeof children === "string" ? children : typeof title === "string" ? title : "item"
      }
      isDisabled={disabled}
      variant={destructive ? "danger" : undefined}
      onAction={() => {
        onSelect?.();
        ctx.setOpen(false);
      }}
      className={cn(
        "relative flex cursor-pointer select-none items-center gap-2 rounded-md px-2 py-1.5 text-sm",
        "outline-none",
        destructive ? "text-danger" : "text-text-primary",
        className,
      )}
      data-destructive={destructive ? "true" : undefined}
    >
      {children}
    </Menu.Item>
  );
}

export function ContextMenuSeparator({ className }: { className?: string }): React.JSX.Element {
  return <Separator className={cn("my-1 bg-border-subtle", className)} />;
}

/** Kept so existing barrel exports type-check. */
export const ContextMenuPrimitive = {
  Root: ContextMenu,
  Trigger: ContextMenuTrigger,
  Content: ContextMenuContent,
  Item: ContextMenuItem,
  Label: ContextMenuLabel,
  Separator: ContextMenuSeparator,
  Sub: ContextMenuSub,
};
