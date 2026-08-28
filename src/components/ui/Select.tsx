import { ListBox, Select as HeroSelect } from "@heroui/react";
import { cn } from "@/lib/utils";

export interface SelectOption {
  value: string;
  label: string;
}

export interface SelectProps {
  id?: string;
  value: string;
  onChange: (value: string) => void;
  options: SelectOption[];
  disabled?: boolean;
  className?: string;
  "aria-label"?: string;
}

export function Select({
  id,
  value,
  onChange,
  options,
  disabled,
  className,
  "aria-label": ariaLabel,
}: SelectProps): React.JSX.Element {
  return (
    <HeroSelect
      selectedKey={value}
      onSelectionChange={(key) => {
        if (key == null) return;
        onChange(String(key));
      }}
      isDisabled={disabled}
      aria-label={ariaLabel}
      className="min-w-0 flex-1"
    >
      <HeroSelect.Trigger
        id={id}
        className={cn(
          "h-8 min-w-0 w-full justify-between rounded-md border border-border-default bg-bg-elevated px-2 text-sm",
          className,
        )}
      >
        <HeroSelect.Value />
        <HeroSelect.Indicator />
      </HeroSelect.Trigger>
      <HeroSelect.Popover>
        <ListBox>
          {options.map((option) => (
            <ListBox.Item key={option.value} id={option.value} textValue={option.label}>
              {option.label}
            </ListBox.Item>
          ))}
        </ListBox>
      </HeroSelect.Popover>
    </HeroSelect>
  );
}
