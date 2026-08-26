import { invoke } from "@tauri-apps/api/core";
import { useEffect, useState } from "react";

function App() {
  const [version, setVersion] = useState<string>("…");
  const [workspaceCount, setWorkspaceCount] = useState<number>(-1);
  const [error, setError] = useState<string>("");

  useEffect(() => {
    invoke<string>("get_app_version")
      .then(setVersion)
      .catch((e: unknown) => setError(String(e)));
    invoke<unknown[]>("list_workspaces")
      .then((ws) => setWorkspaceCount(ws.length))
      .catch((e: unknown) => setError(String(e)));
  }, []);

  return (
    <main className="container">
      <h1>Hello GitWave</h1>
      <p>
        Tauri 2 + React + TypeScript scaffold. Version: <strong>{version}</strong>
      </p>
      <p>
        Workspaces: <strong>{workspaceCount === -1 ? "…" : workspaceCount}</strong>{" "}
        (empty for Sprint 0)
      </p>
      {error ? <p style={{ color: "crimson" }}>{error}</p> : null}
      <p>
        Sprint 0 placeholder. Workspace CRUD, Git history, AI commit and other features
        land in subsequent sprints — see{" "}
        <code>docs/pm/core/04-sprint-v0.1.md</code>.
      </p>
    </main>
  );
}

export default App;