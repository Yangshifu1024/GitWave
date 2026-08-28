import { Checkbox as HeroCheckbox } from "@heroui/react";
import { type ReactNode } from "react";
import { cn } from "@/lib/utils";

export interface CheckboxProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  children: ReactNode;
  className?: string;
}

export function Checkbox({
  checked,
  onChange,
  disabled,
  children,
  className,
}: CheckboxProps): React.JSX.Element {
  return (
    <HeroCheckbox
      isSelected={checked}
      onChange={onChange}
      isDisabled={disabled}
      className={cn("text-xs", className)}
    >
      <HeroCheckbox.Content className="flex items-center gap-2">
        <HeroCheckbox.Control>
          <HeroCheckbox.Indicator />
        </HeroCheckbox.Control>
        {children}
      </HeroCheckbox.Content>
    </HeroCheckbox>
  );
}
