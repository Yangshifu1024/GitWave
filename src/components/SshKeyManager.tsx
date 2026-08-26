import { useState } from "react";
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
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { ListItem } from "@/components/ui/ListItem";
import { Modal } from "@/components/ui/Modal";
import { EmptyState } from "@/components/ui/EmptyState";
import { PathInput } from "@/components/ui/PathInput";
import { Key, Trash2, Plus, Wifi } from "lucide-react";

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
    <div className="flex flex-col">
      {/* Header */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-border-subtle">
        <h2 className="text-xs font-semibold text-text-muted uppercase tracking-wide">
          SSH Keys
        </h2>
        <span className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setTesting(true)}
            aria-label="Test SSH connection"
          >
            <Wifi size={14} />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setAdding(true)}
            aria-label="Add SSH key"
          >
            <Plus size={14} />
          </Button>
        </span>
      </div>

      {/* List */}
      {isLoading ? (
        <p className="px-3 py-2 text-sm text-text-muted">Loading keys…</p>
      ) : error ? (
        <p className="px-3 py-2 text-sm text-danger">
          Failed to load: {formatAppError(error)}
        </p>
      ) : keys.length === 0 ? (
        <EmptyState
          title="No keys in ssh-agent"
          description="Add a key to enable SSH clone and push."
          className="py-6"
        />
      ) : (
        <ul className="py-1">
          {keys.map((k: SshKey) => (
            <li key={k.fingerprint}>
              <ListItem
                selected={false}
                leading={<Key size={14} className="text-text-muted shrink-0" />}
                trailing={
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={(e) => {
                      e.stopPropagation();
                      deleteMut.mutate(k.path);
                    }}
                    className="p-1 text-danger hover:text-danger"
                    aria-label={`Remove ${k.path}`}
                  >
                    <Trash2 size={13} />
                  </Button>
                }
              >
                <div className="flex flex-col min-w-0">
                  <span className="truncate font-mono text-xs text-text-primary">
                    {k.path}
                  </span>
                  <code className="truncate text-xs text-text-muted">
                    {k.fingerprint}
                  </code>
                </div>
              </ListItem>
            </li>
          ))}
        </ul>
      )}

      {/* Add modal */}
      {adding && (
        <Modal
          open={true}
          onOpenChange={(open) => {
            if (!open) {
              setAdding(false);
              setAddPath("");
              setActionError(null);
            }
          }}
          title="Add SSH key"
          size="sm"
        >
          <div className="flex flex-col gap-3">
            <div className="flex flex-col gap-1">
              <label className="text-xs font-medium text-text-secondary" htmlFor="ssh-key-path">
                Key path
              </label>
              <PathInput
                id="ssh-key-path"
                autoFocus
                value={addPath}
                onChange={setAddPath}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && addPath.trim())
                    addMut.mutate(addPath.trim());
                }}
                placeholder="~/.ssh/id_ed25519"
                error={actionError}
              />
              <p className="text-xs text-text-muted">
                Adds the key to ssh-agent (ssh-add). For passphrase-protected
                keys, the agent may prompt via terminal.
              </p>
            </div>
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={() => setAdding(false)}>
              Cancel
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={() => addMut.mutate(addPath.trim())}
              disabled={!addPath.trim() || addMut.isPending}
            >
              Add
            </Button>
          </div>
        </Modal>
      )}

      {/* Test modal */}
      {testing && (
        <Modal
          open={true}
          onOpenChange={(open) => {
            if (!open) {
              setTesting(false);
              setTestResult(null);
              setActionError(null);
            }
          }}
          title="Test SSH connection"
          size="sm"
        >
          <div className="flex flex-col gap-3">
            <div className="flex flex-col gap-1">
              <label className="text-xs font-medium text-text-secondary" htmlFor="ssh-test-host">
                Host
              </label>
              <Input
                id="ssh-test-host"
                value={testHost}
                onChange={setTestHost}
                placeholder="github.com"
              />
            </div>
            <div className="flex flex-col gap-1">
              <label className="text-xs font-medium text-text-secondary" htmlFor="ssh-test-user">
                User
              </label>
              <Input
                id="ssh-test-user"
                value={testUser}
                onChange={setTestUser}
                placeholder="git"
              />
            </div>
            {actionError && <p className="text-xs text-danger">{actionError}</p>}
            {testResult && (
              <div
                className={
                  testResult.success
                    ? "rounded-md border border-success/30 bg-success/10 p-3"
                    : "rounded-md border border-danger/30 bg-danger/10 p-3"
                }
              >
                <p className="text-sm font-medium text-text-primary">
                  {testResult.success ? "Success" : "Failed"}
                </p>
                <pre className="mt-1 whitespace-pre-wrap break-all font-mono text-xs text-text-secondary">
                  {testResult.message}
                </pre>
              </div>
            )}
          </div>
          <div className="flex items-center justify-between gap-2">
            <Button
              variant="secondary"
              size="sm"
              onClick={runTest}
              disabled={testMut.isPending}
            >
              {testMut.isPending ? "Testing…" : "Run test"}
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setTesting(false)}>
              Close
            </Button>
          </div>
        </Modal>
      )}
    </div>
  );
}
