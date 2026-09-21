import {
  cloneElement,
  createContext,
  isValidElement,
  useCallback,
  useContext,
  useEffect,
  useId,
  useMemo,
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

  // setOpen / setPoint 是 React 稳定 setter，无需进依赖；value 只在真正变化时重建，
  // 历史列表里每行的 ContextMenu 才不会因为父级重渲染而连锁重渲染。
  const value = useMemo(() => ({ isOpen, setOpen, point, setPoint }), [isOpen, point]);

  return (
    <ContextMenuStateContext.Provider value={value}>{children}</ContextMenuStateContext.Provider>
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
  const { setOpen, setPoint } = ctx;

  // 引用稳定：cloneElement 注入的 handler 每帧换新会让被注入的元素无法被 memo。
  const onContextMenu = useCallback(
    (event: MouseEvent<HTMLElement>): void => {
      event.preventDefault();
      event.stopPropagation();
      setPoint({ x: event.clientX, y: event.clientY });
      setOpen(true);
    },
    [setPoint, setOpen],
  );

  const childOnContextMenu = isValidElement(children)
    ? (children as ReactElement<TriggerChildProps>).props.onContextMenu
    : undefined;
  // 引用稳定：cloneElement 注入的 handler 每帧换新会让被注入的元素无法被 memo。
  const triggerOnContextMenu = useCallback(
    (event: MouseEvent<HTMLElement>): void => {
      childOnContextMenu?.(event);
      onContextMenu(event);
    },
    [childOnContextMenu, onContextMenu],
  );

  if (asChild && isValidElement(children)) {
    return cloneElement(children as ReactElement<TriggerChildProps>, {
      onContextMenu: triggerOnContextMenu,
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

  // 关闭态不渲染隐形锚点：历史列表每行最多 4 套菜单，视口内约 200 个常驻 body 级
  // fixed 节点是 macOS 滚动空白/卡顿的放大器之一。锚点在菜单打开这一帧才挂载，
  // 坐标取 contextmenu 时写入的 ctx.point（最新 clientX/clientY），定位与
  // docs/tasks/fix-history-menu-drift 的修复一致。
  // 保留 <Popover> 本体挂载，是为了让 HeroUI 样式表驱动的退场动画
  // （.popover[data-exiting=true]）仍然能播放。
  const mountAnchor = ctx.isOpen;

  return (
    <Popover isOpen={ctx.isOpen} onOpenChange={ctx.setOpen}>
      {/* 锚点必须 portal 到 body：CommitGraph 虚拟行 wrapper 带 transform，
          会成为 fixed 后代的包含块，导致 clientX/clientY 被按行坐标解释、菜单漂移 */}
      {mountAnchor
        ? createPortal(
            <Popover.Trigger
              aria-hidden
              className="fixed z-popover h-px w-px overflow-hidden p-0 pointer-events-none"
              style={{ left: ctx.point.x, top: ctx.point.y }}
            />,
            document.body,
          )
        : null}
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
