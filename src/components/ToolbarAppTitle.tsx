import { useEffect, useState } from "react";

import { getAppVersion } from "@/lib/api";
import { cn } from "@/lib/utils";

/** Static toolbar center title: application name + version, never context-bound. */
export function ToolbarAppTitle({ className }: { className?: string }): React.JSX.Element {
  const [version, setVersion] = useState<string | null>(null);

  useEffect(() => {
    getAppVersion()
      .then(setVersion)
      .catch(() => setVersion(null));
  }, []);

  return (
    <div
      className={cn(
        "absolute inset-x-0 flex justify-center items-center pointer-events-none px-32",
        className,
      )}
    >
      <span
        className="text-xs font-medium truncate max-w-[min(480px,60vw)] text-text-primary"
        title="GitWave"
      >
        GitWave{version ? ` v${version}` : ""}
      </span>
    </div>
  );
}
