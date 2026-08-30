import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, X } from "lucide-react";
import {
  clearAiApiKey,
  formatAppError,
  getAiKeyStatus,
  getRepoAiRules,
  getWorkspace,
  probeOllama,
  setAiApiKey,
  updateWorkspaceSettings,
  type AiProviderConfig,
  type WorkspaceSettings,
} from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { Select } from "@/components/ui/Select";
import { Label } from "@/components/ui/Label";
import { Checkbox } from "@/components/ui/Checkbox";
import { Textarea } from "@/components/ui/Textarea";

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

function defaultBaseUrl(provider: ProviderId): string {
  if (provider === "anthropic") return "https://api.anthropic.com";
  if (provider === "ollama") return "http://127.0.0.1:11434";
  return "https://api.openai.com";
}

function baseUrlHint(provider: ProviderId): string {
  if (provider === "anthropic") return "Default: https://api.anthropic.com";
  if (provider === "ollama") return "Default: http://127.0.0.1:11434";
  return "Default: https://api.openai.com (compatible /v1/chat/completions)";
}

export function AiProviderSettings({
  workspaceId,
  workspaceName,
  open,
  onOpenChange,
}: {
  workspaceId: string | null;
  workspaceName?: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}): React.JSX.Element | null {
  const queryClient = useQueryClient();
  const [provider, setProvider] = useState<ProviderId>("openai");
  const [model, setModel] = useState(defaultModel("openai"));
  const [baseUrl, setBaseUrl] = useState(defaultBaseUrl("openai"));
  const [apiKey, setApiKey] = useState("");
  const [ollamaModels, setOllamaModels] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [offline, setOffline] = useState(false);
  const [failover, setFailover] = useState<AiProviderConfig[]>([]);
  const [templatesOpen, setTemplatesOpen] = useState(false);
  const [tplCommit, setTplCommit] = useState("");
  const [tplConflict, setTplConflict] = useState("");
  const [tplPr, setTplPr] = useState("");
  const [tplReflog, setTplReflog] = useState("");
  const [tplHealth, setTplHealth] = useState("");

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

  const { data: repoRules } = useQuery({
    queryKey: ["repo-ai-rules", workspaceId],
    queryFn: () => getRepoAiRules(workspaceId!),
    enabled: Boolean(workspaceId && open),
  });

  useEffect(() => {
    if (!workspace) return;
    const s = workspace.settings;
    const p = (s.ai_provider as ProviderId | null) ?? "openai";
    const resolved = PROVIDERS.some((x) => x.id === p) ? p : "openai";
    setProvider(resolved);
    setModel(s.ai_model ?? defaultModel(resolved));
    setBaseUrl(s.ai_base_url ?? defaultBaseUrl(resolved));
    setOffline(Boolean(s.ai_offline));
    setFailover(s.ai_failover ? s.ai_failover.map((fb) => ({ ...fb })) : []);
    setTplCommit(s.prompt_templates.commit ?? "");
    setTplConflict(s.prompt_templates.conflict ?? "");
    setTplPr(s.prompt_templates.pr ?? "");
    setTplReflog(s.prompt_templates.reflog ?? "");
    setTplHealth(s.prompt_templates.health ?? "");
  }, [workspace]);

  const updateFallback = (index: number, patch: Partial<AiProviderConfig>) => {
    setFailover((list) => list.map((fb, i) => (i === index ? { ...fb, ...patch } : fb)));
  };

  const moveFallback = (index: number, dir: -1 | 1) => {
    setFailover((list) => {
      const target = index + dir;
      const current = list[index];
      const swap = list[target];
      if (!current || !swap) return list;
      const next = [...list];
      next[index] = swap;
      next[target] = current;
      return next;
    });
  };

  const removeFallback = (index: number) => {
    setFailover((list) => list.filter((_, i) => i !== index));
  };

  const addFallback = () => {
    setFailover((list) => [...list, { provider: "ollama", model: null, base_url: null }]);
  };

  const saveMut = useMutation({
    mutationFn: async () => {
      if (!workspaceId || !workspace) return;
      const trimmedBase = baseUrl.trim();
      const settings: WorkspaceSettings = {
        ...workspace.settings,
        ai_provider: provider,
        ai_model: model.trim() || null,
        ai_base_url:
          trimmedBase && trimmedBase !== defaultBaseUrl(provider)
            ? trimmedBase
            : trimmedBase || null,
        ai_offline: offline,
        ai_failover: failover.map((fb) => ({
          provider: fb.provider,
          model: (fb.model ?? "").trim() || null,
          base_url: (fb.base_url ?? "").trim() || null,
        })),
        prompt_templates: {
          commit: tplCommit.trim() || null,
          conflict: tplConflict.trim() || null,
          pr: tplPr.trim() || null,
          reflog: tplReflog.trim() || null,
          health: tplHealth.trim() || null,
        },
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

  if (!workspaceId) return null;

  return (
    <Modal
      open={open}
      onOpenChange={(next) => {
        onOpenChange(next);
        if (!next) {
          setError(null);
          setNotice(null);
          setApiKey("");
        }
      }}
      title={workspaceName ? `AI provider · ${workspaceName}` : "AI provider"}
      description="Workspace-scoped. Cloud keys use OS keychain (BYOK). AI never auto-commits."
      size="md"
      footer={
        <>
          <Button variant="secondary" size="sm" onClick={() => onOpenChange(false)}>
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
        </>
      }
    >
      <div className="flex flex-col gap-3">
        {repoRules ? (
          <p className="rounded-md bg-bg-secondary px-2 py-1.5 text-[11px] text-text-secondary">
            Per-repo AI rules active — <code>.gitwave/AI.md</code> ({repoRules.length} chars) is
            appended to every AI prompt for the active repo.
          </p>
        ) : null}
        <Label className="text-xs text-text-secondary">
          Provider
          <Select
            aria-label="Provider"
            className="mt-1 w-full"
            value={provider}
            onChange={(next) => {
              const id = next as ProviderId;
              setProvider(id);
              setModel(defaultModel(id));
              setBaseUrl(defaultBaseUrl(id));
            }}
            options={PROVIDERS.map((p) => ({ value: p.id, label: p.label }))}
          />
        </Label>

        <Input placeholder="Model" value={model} onChange={setModel} />

        <div className="flex flex-col gap-1">
          <Input placeholder="API base URL" value={baseUrl} onChange={setBaseUrl} />
          <p className="text-[11px] text-text-muted">{baseUrlHint(provider)}</p>
        </div>

        {provider === "ollama" ? (
          <>
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

        <Checkbox checked={offline} onChange={setOffline} className="text-xs">
          Offline mode — disable all cloud AI calls (Ollama still allowed)
        </Checkbox>

        <div className="flex flex-col gap-1">
          <Label className="text-xs text-text-secondary">Fallback providers</Label>
          <p className="text-[11px] text-text-muted">
            Tried in order when the provider above fails with a network error. Cloud entries need
            their own API key (stored per provider in the OS keychain).
          </p>
          {failover.map((fb, i) => (
            <div key={i} className="flex items-center gap-1">
              <Select
                aria-label={`Fallback provider ${i + 1}`}
                className="w-36 shrink-0"
                value={
                  (PROVIDERS.some((p) => p.id === fb.provider)
                    ? fb.provider
                    : "openai") as ProviderId
                }
                onChange={(next) => updateFallback(i, { provider: next })}
                options={PROVIDERS.map((p) => ({ value: p.id, label: p.label }))}
              />
              <Input
                placeholder="Model (default)"
                value={fb.model ?? ""}
                onChange={(v) => updateFallback(i, { model: v })}
                className="flex-1"
              />
              <Button
                variant="ghost"
                size="sm"
                disabled={i === 0}
                onClick={() => moveFallback(i, -1)}
                title="Move up"
              >
                <ArrowUp size={14} />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                disabled={i === failover.length - 1}
                onClick={() => moveFallback(i, 1)}
                title="Move down"
              >
                <ArrowDown size={14} />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="text-danger"
                onClick={() => removeFallback(i)}
                title="Remove"
              >
                <X size={14} />
              </Button>
            </div>
          ))}
          <Button variant="ghost" size="sm" className="self-start" onClick={addFallback}>
            Add fallback
          </Button>
        </div>

        <Button
          variant="secondary"
          size="sm"
          className="self-start"
          onClick={() => setTemplatesOpen((v) => !v)}
        >
          {templatesOpen ? "Hide prompt templates" : "Prompt templates (commit / conflict / PR)"}
        </Button>
        {templatesOpen ? (
          <div className="flex flex-col gap-2 rounded-md border border-border-subtle p-2">
            <p className="text-[11px] text-text-muted">
              Each field replaces the built-in system prompt for that task — the diff, conflict
              sides, and repo context are appended automatically. Leave empty to use the built-in
              default. Per-repo rules (<code>.gitwave/AI.md</code>) are added on top of whichever
              prompt runs.
            </p>
            <PromptTemplateField
              label="Commit message"
              value={tplCommit}
              onChange={setTplCommit}
              placeholder="Override the built-in commit-message system prompt"
            />
            <PromptTemplateField
              label="Conflict explanation"
              value={tplConflict}
              onChange={setTplConflict}
              placeholder="Override the built-in conflict-explanation system prompt"
            />
            <PromptTemplateField
              label="PR description"
              value={tplPr}
              onChange={setTplPr}
              placeholder="System prompt for AI PR description generation"
            />
            <PromptTemplateField
              label="Recovery assistant (reflog)"
              value={tplReflog}
              onChange={setTplReflog}
              placeholder="Override the recovery-assistant system prompt"
            />
            <PromptTemplateField
              label="Health summarizer"
              value={tplHealth}
              onChange={setTplHealth}
              placeholder="Override the health-report system prompt"
            />
          </div>
        ) : null}

        {error ? <p className="text-xs text-danger">{error}</p> : null}
        {notice ? <p className="text-xs text-text-secondary">{notice}</p> : null}
      </div>
    </Modal>
  );
}

function PromptTemplateField({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
}): React.JSX.Element {
  return (
    <label className="flex flex-col gap-1 text-xs text-text-secondary">
      {label}
      <Textarea
        value={value}
        onChange={onChange}
        placeholder={placeholder}
        rows={3}
        spellCheck={false}
        className="rounded-md px-2 py-1.5 font-mono text-[11px] leading-5"
      />
    </label>
  );
}
