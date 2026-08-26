import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  addSshKey,
  deleteSshKey,
  formatAppError,
  listSshKeys,
  testSshConnection,
  type SshKey,
  type SshTestResult,
} from "@/lib/api";

function Modal({
  title,
  onClose,
  children,
}: {
  title: string;
  onClose: () => void;
  children: React.ReactNode;
}): React.JSX.Element {
  const dialogRef = useRef<HTMLDialogElement | null>(null);
  useEffect(() => {
    dialogRef.current?.showModal();
  }, []);
  return (
    <dialog
      ref={dialogRef}
      onClose={onClose}
      onClick={(e) => {
        if (e.target === dialogRef.current) onClose();
      }}
    >
      <div className="modal-inner">
        <h3>{title}</h3>
        {children}
      </div>
    </dialog>
  );
}

export function SshKeyManager(): React.JSX.Element {
  const queryClient = useQueryClient();

  const [adding, setAdding] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<SshTestResult | null>(null);

  const [addPath, setAddPath] = useState("");
  const [testHost, setTestHost] = useState("github.com");
  const [testUser, setTestUser] = useState("git");

  const [actionError, setActionError] = useState<string | null>(null);

  const {
    data: keys = [],
    isLoading,
    error,
  } = useQuery({
    queryKey: ["ssh-keys"],
    queryFn: listSshKeys,
  });

  const refresh = (): void => {
    void queryClient.invalidateQueries({ queryKey: ["ssh-keys"] });
  };

  const addMut = useMutation({
    mutationFn: (path: string) => addSshKey(path),
    onSuccess: () => {
      refresh();
      setAddPath("");
      setAdding(false);
      setActionError(null);
    },
    onError: (e: unknown) => setActionError(formatAppError(e)),
  });

  const deleteMut = useMutation({
    mutationFn: (path: string) => deleteSshKey(path),
    onSuccess: () => {
      refresh();
    },
  });

  const testMut = useMutation({
    mutationFn: ({ host, user }: { host: string; user: string }) =>
      testSshConnection(host, user),
    onSuccess: (res) => {
      setTestResult(res);
    },
    onError: (e: unknown) => setActionError(formatAppError(e)),
  });

  function runTest(): void {
    setTestResult(null);
    setActionError(null);
    testMut.mutate({ host: testHost.trim(), user: testUser.trim() });
  }

  return (
    <section className="ssh-key-manager">
      <header>
        <h2>SSH Keys</h2>
        <span className="actions">
          <button type="button" onClick={() => setTesting(true)}>
            Test
           
          </button>
          <button type="button" onClick={() => setAdding(true)}>
            Add
          </button>
        </span>
      </header>

      {isLoading ? (
        <p>Loading keys…</p>
      ) : error ? (
        <p className="error">Failed to load: {formatAppError(error)}</p>
      ) : keys.length === 0 ? (
        <p className="empty">
          No keys in ssh-agent. Add a key to enable SSH clone.
        </p>
      ) : (
        <ul>
          {keys.map((k: SshKey) => (
            <li key={k.fingerprint}>
              <span className="path">{k.path}</span>
              <span className="fingerprint">
                <code>{k.fingerprint}</code>
              </span>
              <span className="row-actions">
                <button
                  type="button"
                  className="danger"
                  onClick={() => deleteMut.mutate(k.path)}
                  disabled={deleteMut.isPending}
                >
                  remove
                </button>
              </span>
            </li>
          ))}
        </ul>
      )}

      {adding ? (
        <Modal title="Add SSH key" onClose={() => setAdding(false)}>
          <label>
            Key path
            <input
              autoFocus
              value={addPath}
              onChange={(e) => setAddPath(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && addPath.trim())
                  addMut.mutate(addPath.trim());
              }}
              placeholder="~/.ssh/id_ed25519"
            />
            <small>
              Adds the key to ssh-agent (ssh-add). For passphrase-protected
              keys, the agent may prompt via terminal.
            </small>
          </label>
          {actionError ? <p className="error">{actionError}</p> : null}
          <div className="modal-actions">
            <button type="button" onClick={() => setAdding(false)}>
              Cancel
            </button>
            <button
              type="button"
              onClick={() => addMut.mutate(addPath.trim())}
              disabled={!addPath.trim() || addMut.isPending}
            >
              Add
            </button>
          </div>
        </Modal>
      ) : null}

      {testing ? (
        <Modal title="Test SSH connection" onClose={() => setTesting(false)}>
          <label>
            Host
            <input
              value={testHost}
              onChange={(e) => setTestHost(e.currentTarget.value)}
            />
          </label>
          <label>
            User
            <input
              value={testUser}
              onChange={(e) => setTestUser(e.currentTarget.value)}
            />
          </label>
          <button
            type="button"
            onClick={runTest}
            disabled={testMut.isPending}
          >
            {testMut.isPending ? "Testing..." : "Run"}
          </button>
          {actionError ? <p className="error">{actionError}</p> : null}
          {testResult ? (
            <div className={testResult.success ? "ssh-test-ok" : "ssh-test-fail"}>
              <strong>{testResult.success ? "Success" : "Failed"}</strong>
              <pre>{testResult.message}</pre>
            </div>
          ) : null}
          <div className="modal-actions">
            <button type="button" onClick={() => setTesting(false)}>
              Close
            </button>
          </div>
        </Modal>
      ) : null}
    </section>
  );
}