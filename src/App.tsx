import { useEffect, useState } from "react";

import { RepoList } from "@/components/RepoList";
import { SshKeyManager } from "@/components/SshKeyManager";
import { WorkspaceSwitcher } from "@/components/WorkspaceSwitcher";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { formatAppError, getAppVersion } from "@/lib/api";

function App(): React.JSX.Element {
  const [version, setVersion] = useState<string>("…");
  const [error, setError] = useState<string>("");
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);

  useEffect(() => {
    getAppVersion()
      .then(setVersion)
      .catch((e: unknown) => setError(formatAppError(e)));
  }, []);

  return (
    <main className="container">
      <h1>GitWave</h1>
      <p>
        Tauri 2 + React + TypeScript. Version: <strong>{version}</strong>
      </p>
      {error ? <p className="error">{error}</p> : null}

      <WorkspaceSwitcher />

      {activeWorkspaceId ? <RepoList workspaceId={activeWorkspaceId} /> : null}

      <SshKeyManager />
    </main>
  );
}

export default App;