import * as ToastPrimitive from "@radix-ui/react-toast";
import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from "react";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";

export type ToastVariant = "success" | "danger" | "info" | "warning";

export interface ToastData {
  id?: string;
  title: string;
  description?: string | null;
  variant?: ToastVariant;
  duration?: number;
}

const variantClasses: Record<ToastVariant, string> = {
  success: "border-l-4 border-l-success",
  danger: "border-l-4 border-l-danger",
  info: "border-l-4 border-l-info",
  warning: "border-l-4 border-l-warning",
};

interface ToastContextValue {
  toast: (data: ToastData) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    // Return a no-op if used outside provider (useful for early dev)
    return { toast: () => {} };
  }
  return ctx;
}

interface ToastProviderProps {
  children: ReactNode;
}

export function ToastProvider({ children }: ToastProviderProps): React.JSX.Element {
  const [toasts, setToasts] = useState<ToastData[]>([]);
  const counterRef = useRef(0);

  const toast = useCallback((data: ToastData) => {
    const id = `toast-${++counterRef.current}`;
    setToasts((prev) => [...prev, { ...data, id }]);
  }, []);

  const dismiss = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  return (
    <ToastContext.Provider value={{ toast }}>
      <ToastPrimitive.Provider swipeDirection="right" duration={4000}>
        {children}
        {toasts.map((t) => (
          <ToastPrimitive.Root
            key={t.id}
            duration={t.duration ?? 4000}
            onOpenChange={(open) => {
              if (!open && t.id) dismiss(t.id);
            }}
            className={cn(
              "group pointer-events-auto relative flex w-80 items-start gap-3",
              "rounded-lg border border-border-default bg-bg-elevated p-4 shadow-modal",
              "transition-all duration-base",
              "data-[state=open]:animate-in data-[state=closed]:animate-out",
              "data-[state=closed]:fade-out-80 data-[state=open]:fade-in-0",
              "data-[state=closed]:slide-out-to-right-full data-[state=open]:slide-in-from-right-full",
              (t.variant ?? "info") === "success" && variantClasses.success,
              (t.variant ?? "info") === "danger" && variantClasses.danger,
              (t.variant ?? "info") === "info" && variantClasses.info,
              (t.variant ?? "info") === "warning" && variantClasses.warning,
            )}
          >
            <div className="flex flex-1 flex-col gap-1">
              <ToastPrimitive.Title className="text-sm font-medium text-text-primary">
                {t.title}
              </ToastPrimitive.Title>
              {t.description ? (
                <ToastPrimitive.Description className="text-xs text-text-secondary">
                  {t.description}
                </ToastPrimitive.Description>
              ) : null}
            </div>
            <ToastPrimitive.Close
              onClick={() => t.id && dismiss(t.id)}
              className={cn(
                "shrink-0 rounded p-0.5",
                "text-text-muted hover:text-text-primary",
                "opacity-0 group-hover:opacity-100 transition-opacity",
                "focus:outline-none focus-visible:ring-2 focus-visible:ring-accent",
              )}
            >
              <X size={14} />
            </ToastPrimitive.Close>
          </ToastPrimitive.Root>
        ))}
        <ToastPrimitive.Viewport className="fixed bottom-4 right-4 z-toast flex flex-col gap-2 outline-none" />
      </ToastPrimitive.Provider>
    </ToastContext.Provider>
  );
}

export { ToastPrimitive };
