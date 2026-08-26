import { Sun, Moon, Monitor } from "lucide-react";
import { useTheme, type Theme } from "@/hooks/useTheme";
import { Button } from "@/components/ui/Button";
import { Tooltip } from "@/components/ui/Tooltip";

const themeConfig: Record<Theme, { icon: React.ReactNode; label: string }> = {
  light: { icon: <Sun size={16} />, label: "Light" },
  dark: { icon: <Moon size={16} />, label: "Dark" },
  system: { icon: <Monitor size={16} />, label: "System" },
};

/** Cycles through light → dark → system → light on each click. */
export function ThemeToggle(): React.JSX.Element {
  const { theme, setTheme } = useTheme();

  const nextTheme = (): void => {
    const order: Theme[] = ["light", "dark", "system"];
    const idx = order.indexOf(theme);
    const next = order[(idx + 1) % order.length];
    if (next) setTheme(next);
  };

  const { icon, label } = themeConfig[theme];

  return (
    <Tooltip content={label}>
      <Button
        variant="ghost"
        size="sm"
        onClick={nextTheme}
        aria-label={`Current theme: ${label}. Click to change.`}
        className="p-1.5"
      >
        {icon}
      </Button>
    </Tooltip>
  );
}
