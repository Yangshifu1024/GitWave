import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

import {
  addSshKey,
  deleteSshKey,
  formatAppError,
  listSshKeys,
  startSshAgentService,
  testSshConnection,
  type SshKey,
  type SshTestResult,
} from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { ListItem } from "@/components/ui/ListItem";
import { EmptyState } from "@/components/ui/EmptyState";
import { PathInput } from "@/components/ui/PathInput";
import { Label } from "@/components/ui/Label";
import { Key, Trash2, Plus, Wifi, Power } from "lucide-react";

const isWindows = navigator.userAgent.includes("Windows");

export function SshKeyManager(): React.JSX.Element {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const [adding, setAdding] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<SshTestResult | null>(null);

  const [addPath, setAddPath] = useState("");
  const [testHost, setTestHost] = useState("github.com");
  const [testUser, setTestUser] = useState("git");

  const [actionError, setActionError] = useState<string | null>(null);
  const [startingAgent, setStartingAgent] = useState(false);

  const {
    data: result,
    isLoading,
    error,
    refetch,
  } = useQuery({
    queryKey: ["ssh-keys"],
    queryFn: listSshKeys,
  });

  const keys: SshKey[] = result?.keys ?? [];
  const agentDown: boolean = result !== undefined && !result.agent_running;

  const refresh = (): void => {
    void queryClient.invalidateQueries({ queryKey: ["ssh-keys"] });
  };

  // After the UAC request is handed off, poll the agent for ~16s — the
  // elevated process itself is not observable from the app.
  const startAgent = (): void => {
    setActionError(null);
    void startSshAgentService()
      .then(() => {
        setStartingAgent(true);
        let tries = 0;
        const timer = setInterval(() => {
          tries += 1;
          void refetch().then(({ data }) => {
            if (data?.agent_running || tries >= 8) {
              clearInterval(timer);
              setStartingAgent(false);
              if (!data?.agent_running) {
                setActionError(t("ssh.agentDown.retryHint"));
              }
            }
          });
        }, 2000);
      })
      .catch((e: unknown) => setActionError(formatAppError(e)));
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
    mutationFn: ({ host, user }: { host: string; user: string }) => testSshConnection(host, user),
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

  const closeAdding = (): void => {
    setAdding(false);
    setAddPath("");
    setActionError(null);
  };

  const closeTesting = (): void => {
    setTesting(false);
    setTestResult(null);
    setActionError(null);
  };

  const showAdding = (): void => {
    closeTesting();
    setAdding(true);
  };

  const showTesting = (): void => {
    closeAdding();
    setTesting(true);
  };

  return (
    <div className="flex flex-col">
      {/* Actions */}
      <div className="flex items-center justify-end gap-1 pb-2">
        <Button
          variant="ghost"
          size="sm"
          onClick={showTesting}
          aria-label={t("ssh.actions.testConnection")}
        >
          <Wifi size={14} />
        </Button>
        <Button variant="ghost" size="sm" onClick={showAdding} aria-label={t("ssh.actions.addKey")}>
          <Plus size={14} />
        </Button>
      </div>

      {/* Add form (inline so the settings dialog never stacks dialog layers).
          Escape is swallowed here so it closes only the form, not the dialog. */}
      {adding && (
        <div
          className="mb-3 flex flex-col gap-3 rounded-xl bg-bg-primary p-3"
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              e.preventDefault();
              e.stopPropagation();
              closeAdding();
            }
          }}
        >
          <div className="flex flex-col gap-1">
            <Label className="text-xs font-medium text-text-secondary" htmlFor="ssh-key-path">
              {t("ssh.add.keyPath")}
            </Label>
            <PathInput
              id="ssh-key-path"
              autoFocus
              value={addPath}
              onChange={setAddPath}
              onKeyDown={(e) => {
                if (e.key === "Enter" && addPath.trim()) addMut.mutate(addPath.trim());
              }}
              placeholder="~/.ssh/id_ed25519"
              error={actionError}
            />
            <p className="text-xs text-text-muted">{t("ssh.add.hint")}</p>
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={closeAdding}>
              {t("ssh.actions.cancel")}
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={() => addMut.mutate(addPath.trim())}
              disabled={!addPath.trim() || addMut.isPending}
            >
              {t("ssh.actions.add")}
            </Button>
          </div>
        </div>
      )}

      {/* Test form (inline) */}
      {testing && (
        <div
          className="mb-3 flex flex-col gap-3 rounded-md border border-border-default p-3"
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              e.preventDefault();
              e.stopPropagation();
              closeTesting();
            }
          }}
        >
          <div className="flex flex-col gap-1">
            <Label className="text-xs font-medium text-text-secondary" htmlFor="ssh-test-host">
              {t("ssh.test.host")}
            </Label>
            <Input
              id="ssh-test-host"
              value={testHost}
              onChange={setTestHost}
              placeholder="github.com"
            />
          </div>
          <div className="flex flex-col gap-1">
            <Label className="text-xs font-medium text-text-secondary" htmlFor="ssh-test-user">
              {t("ssh.test.user")}
            </Label>
            <Input id="ssh-test-user" value={testUser} onChange={setTestUser} placeholder="git" />
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
                {testResult.success ? t("ssh.test.success") : t("ssh.test.failed")}
              </p>
              <pre className="mt-1 whitespace-pre-wrap break-all font-mono text-xs text-text-secondary">
                {testResult.message}
              </pre>
            </div>
          )}
          <div className="flex items-center justify-between gap-2">
            <Button variant="secondary" size="sm" onClick={runTest} disabled={testMut.isPending}>
              {testMut.isPending ? t("ssh.test.testing") : t("ssh.test.run")}
            </Button>
            <Button variant="ghost" size="sm" onClick={closeTesting}>
              {t("ssh.actions.close")}
            </Button>
          </div>
        </div>
      )}

      {/* List */}
      {isLoading ? (
        <p className="px-3 py-2 text-sm text-text-muted">{t("ssh.loading")}</p>
      ) : error ? (
        <p className="px-3 py-2 text-sm text-danger">
          {t("ssh.loadFailed", { message: formatAppError(error) })}
        </p>
      ) : agentDown ? (
        <div className="flex flex-col items-center gap-3 py-6">
          <EmptyState
            title={t("ssh.agentDown.title")}
            description={t("ssh.agentDown.description")}
          />
          {isWindows && (
            <Button
              variant="secondary"
              size="sm"
              onClick={startAgent}
              disabled={startingAgent}
            >
              <Power size={14} />
              {startingAgent ? t("ssh.agentDown.starting") : t("ssh.agentDown.startButton")}
            </Button>
          )}
          {actionError && <p className="text-xs text-danger">{actionError}</p>}
        </div>
      ) : keys.length === 0 ? (
        <EmptyState
          title={t("ssh.empty.title")}
          description={t("ssh.empty.description")}
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
                    aria-label={t("ssh.removeKey", { path: k.path })}
                  >
                    <Trash2 size={13} />
                  </Button>
                }
              >
                <div className="flex flex-col min-w-0">
                  <span className="truncate font-mono text-xs text-text-primary">{k.path}</span>
                  <code className="truncate text-xs text-text-muted">{k.fingerprint}</code>
                </div>
              </ListItem>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
