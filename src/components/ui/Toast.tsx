import { Toast, toast } from "@heroui/react";
import { type ReactNode } from "react";

export type ToastVariant = "success" | "danger" | "info" | "warning";

export interface ToastData {
  id?: string;
  title: string;
  description?: string | null;
  variant?: ToastVariant;
  duration?: number;
}

interface ToastContextValue {
  toast: (data: ToastData) => void;
}

export function useToast(): ToastContextValue {
  return {
    toast: (data) => {
      const options = {
        description: data.description ?? undefined,
        timeout: data.duration ?? 4000,
      };
      switch (data.variant) {
        case "success":
          toast.success(data.title, options);
          break;
        case "danger":
          toast.danger(data.title, options);
          break;
        case "warning":
          toast.warning(data.title, options);
          break;
        default:
          toast.info(data.title, options);
      }
    },
  };
}

interface ToastProviderProps {
  children: ReactNode;
}

export function ToastProvider({ children }: ToastProviderProps): React.JSX.Element {
  return (
    <>
      {children}
      <Toast.Provider />
    </>
  );
}

export const ToastPrimitive = Toast;
