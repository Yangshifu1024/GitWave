import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  clearAiApiKey,
  formatAppError,
  getAiKeyStatus,
  getWorkspace,
  probeOllama,
  setAiApiKey,
  updateWorkspaceSettings,
  type WorkspaceSettings,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Sparkles } from "lucide-react";

const PROVIDERS = [
  { id: "openai", label: "OpenAI" },
  { id: "anthropic", label: "Anthropic" },
  { id: "ollama", label: "Ollama (local)" },
] as const;

type ProviderId = (typeof PROVIDERS)[number]["id"];

function defaultModel(provider: ProviderId): string {
  if (provider === "anthropic") return "claude-3-5-haiku-latest";
  if (provider === "ollama") return "llama3.2";
  return "gpt-4o-mini";
}

export function AiProviderSettings(): React.JSX.Element {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [provider, setProvider] = useState<ProviderId>("openai");
  const [model, setModel] = useState(defaultModel("openai"));
  const [baseUrl, setBaseUrl] = useState("http://127.0.0.1:11434");
  const [offline, setOffline] = useState(false);
  const [apiKey, setApiKey] = useState("");
  const [ollamaModels, setOllamaModels] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const { data: workspace } = useQuery({
    queryKey: ["workspace", workspaceId],
    queryFn: () => getWorkspace(workspaceId!),
    enabled: Boolean(workspaceId && open),
  });

  const { data: keyStatus } = useQuery({
    queryKey: ["ai-key", workspaceId, provider],
    queryFn: () => getAiKeyStatus(workspaceId!, provider),
    enabled: Boolean(workspaceId && open && provider !== "ollama"),
  });

  useEffect(() => {
    if (!workspace) return;
    const s = workspace.settings;
    const p = (s.ai_provider as ProviderId | null) ?? "openai";
    setProvider(PROVIDERS.some((x) => x.id === p) ? p : "openai");
    setModel(s.ai_model ?? defaultModel((s.ai_provider as ProviderId) || "openai"));
    setBaseUrl(s.ai_base_url ?? "http://127.0.0.1:11434");
    setOffline(Boolean(s.ai_offline));
  }, [workspace]);

  const saveMut = useMutation({
    mutationFn: async () => {
      if (!workspaceId || !workspace) return;
      const settings: WorkspaceSettings = {
        ...workspace.settings,
        ai_provider: provider,
        ai_model: model.trim() || null,
        ai_base_url: provider === "ollama" ? baseUrl.trim() || null : null,
        ai_offline: offline,
      };
      await updateWorkspaceSettings(workspaceId, settings);
      if (provider !== "ollama" && apiKey.trim()) {
        await setAiApiKey(workspaceId, provider, apiKey.trim());
        setApiKey("");
      }
    },
    onSuccess: () => {
      setNotice("AI settings saved (key stays in OS keychain)");
      setError(null);
      void queryClient.invalidateQueries({ queryKey: ["workspace", workspaceId] });
      void queryClient.invalidateQueries({ queryKey: ["ai-key", workspaceId] });
    },
    onError: (e) => setError(formatAppError(e)),
  });

  if (!workspaceId) {
    return (
      <Button variant="ghost" size="sm" className="p-1" disabled title="Select a workspace first">
        <Sparkles size={16} />
      </Button>
    );
  }

  return (
    <>
      <Button
        variant="ghost"
        size="sm"
        className="p-1"
        onClick={() => setOpen(true)}
        aria-label="AI provider settings"
        title="AI provider"
      >
        <Sparkles size={16} />
      </Button>

      {open ? (
        <div className="fixed inset-0 z-modal flex items-center justify-center">
          <div className="fixed inset-0 bg-bg-overlay backdrop-blur-sm" onClick={() => setOpen(false)} />
          <div
            className="relative z-10 w-full max-w-md rounded-xl bg-bg-elevated shadow-modal p-6"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-md font-semibold text-text-primary">AI provider</h2>
              <Button variant="ghost" size="sm" onClick={() => setOpen(false)} className="p-1">
                <span aria-hidden>&#x2715;</span>
              </Button>
            </div>

            <p className="text-xs text-text-muted mb-3">
              Workspace-scoped. Cloud keys use OS keychain (BYOK). AI never auto-commits.
            </p>

            <div className="flex flex-col gap-3">
              <label className="text-xs text-text-secondary">
                Provider
                <select
                  className="mt-1 w-full h-8 rounded-md border border-border-default bg-bg-elevated px-2 text-sm"
                  value={provider}
                  onChange={(e) => {
                    const next = e.target.value as ProviderId;
                    setProvider(next);
                    setModel(defaultModel(next));
                  }}
                >
                  {PROVIDERS.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.label}
                    </option>
                  ))}
                </select>
              </label>

              <Input placeholder="Model" value={model} onChange={setModel} />

              {provider === "ollama" ? (
                <>
                  <Input
                    placeholder="Ollama base URL"
                    value={baseUrl}
                    onChange={setBaseUrl}
                  />
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => {
                      setError(null);
                      probeOllama(baseUrl.trim() || undefined)
                        .then((models) => {
                          setOllamaModels(models);
                          setNotice(
                            models.length
                              ? `Found ${models.length} local model(s)`
                              : "Ollama reachable but no models listed",
                          );
                        })
                        .catch((e) => setError(formatAppError(e)));
                    }}
                  >
                    Detect Ollama
                  </Button>
                  {ollamaModels.length > 0 ? (
                    <p className="text-xs text-text-muted truncate">{ollamaModels.join(", ")}</p>
                  ) : null}
                </>
              ) : (
                <>
                  <Input
                    type="password"
                    placeholder={
                      keyStatus?.has_key ? "API key (leave blank to keep existing)" : "API key"
                    }
                    value={apiKey}
                    onChange={setApiKey}
                  />
                  {keyStatus?.has_key ? (
                    <Button
                      variant="ghost"
                      size="sm"
                      className="self-start text-danger"
                      onClick={() => {
                        void clearAiApiKey(workspaceId, provider)
                          .then(() => {
                            setNotice("API key cleared");
                            void queryClient.invalidateQueries({
                              queryKey: ["ai-key", workspaceId, provider],
                            });
                          })
                          .catch((e) => setError(formatAppError(e)));
                      }}
                    >
                      Clear stored key
                    </Button>
                  ) : null}
                </>
              )}

              <label className="flex items-center gap-2 text-sm text-text-secondary">
                <input
                  type="checkbox"
                  checked={offline}
                  onChange={(e) => setOffline(e.target.checked)}
                />
                Offline mode (block cloud AI)
              </label>

              {error ? <p className="text-xs text-danger">{error}</p> : null}
              {notice ? <p className="text-xs text-text-secondary">{notice}</p> : null}

              <div className="flex justify-end gap-2 mt-2">
                <Button variant="secondary" size="sm" onClick={() => setOpen(false)}>
                  Close
                </Button>
                <Button
                  variant="primary"
                  size="sm"
                  disabled={saveMut.isPending}
                  onClick={() => saveMut.mutate()}
                >
                  Save
                </Button>
              </div>
            </div>
          </div>
        </div>
      ) : null}
    </>
  );
}
