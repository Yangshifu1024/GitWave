import { type ButtonHTMLAttributes, forwardRef } from "react";
import { Button as HeroButton } from "@heroui/react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva("inline-flex items-center justify-center gap-2 font-medium rounded-md", {
  variants: {
    variant: {
      primary: "",
      secondary: "",
      danger: "",
      "danger-soft": "",
      ghost: "",
    },
    size: {
      sm: "h-7 px-2 text-xs",
      md: "h-8 px-3 text-sm",
    },
  },
  defaultVariants: {
    variant: "secondary",
    size: "md",
  },
});

const heroVariant = {
  primary: "primary",
  secondary: "outline",
  danger: "danger",
  "danger-soft": "danger-soft",
  ghost: "ghost",
} as const;

export interface ButtonProps
  extends ButtonHTMLAttributes<HTMLButtonElement>, VariantProps<typeof buttonVariants> {
  variant?: "primary" | "secondary" | "danger" | "danger-soft" | "ghost";
  size?: "sm" | "md";
}

type HeroButtonProps = React.ComponentProps<typeof HeroButton>;

const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  (
    {
      className,
      variant = "secondary",
      size = "md",
      disabled,
      type = "button",
      onClick,
      children,
      title,
      id,
      name,
      autoFocus,
      tabIndex,
      form,
      ...rest
    },
    ref,
  ) => {
    const heroProps = {
      ref,
      id,
      name,
      type,
      form,
      tabIndex,
      autoFocus,
      title,
      variant: heroVariant[variant],
      size,
      isDisabled: disabled,
      className: cn(buttonVariants({ variant, size }), className),
      ...rest,
      onClick,
      children,
    } as HeroButtonProps;

    return <HeroButton {...heroProps} />;
  },
);
Button.displayName = "Button";

export { Button, buttonVariants };
