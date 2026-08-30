import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
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
  { id: "openai", labelKey: "ai.provider.openai" },
  { id: "anthropic", labelKey: "ai.provider.anthropic" },
  { id: "ollama", labelKey: "ai.provider.ollama" },
] as const;

type ProviderId = (typeof PROVIDERS)[number]["id"];

const BASE_URL_HINT_KEY: Record<ProviderId, string> = {
  openai: "ai.baseUrlHint.openai",
  anthropic: "ai.baseUrlHint.anthropic",
  ollama: "ai.baseUrlHint.ollama",
};

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
  const { t } = useTranslation();
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
      setNotice(t("ai.saved"));
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
      title={workspaceName ? t("ai.titleWithWorkspace", { name: workspaceName }) : t("ai.title")}
      description={t("ai.description")}
      size="md"
      footer={
        <>
          <Button variant="secondary" size="sm" onClick={() => onOpenChange(false)}>
            {t("ai.actions.close")}
          </Button>
          <Button
            variant="primary"
            size="sm"
            disabled={saveMut.isPending}
            onClick={() => saveMut.mutate()}
          >
            {t("ai.actions.save")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        {repoRules ? (
          <p className="rounded-md bg-bg-secondary px-2 py-1.5 text-[11px] text-text-secondary">
            {t("ai.repoRules.prefix")}
            <code>.gitwave/AI.md</code>
            {t("ai.repoRules.suffix", { chars: repoRules.length })}
          </p>
        ) : null}
        <Label className="text-xs text-text-secondary">
          {t("ai.provider.label")}
          <Select
            aria-label={t("ai.provider.label")}
            className="mt-1 w-full"
            value={provider}
            onChange={(next) => {
              const id = next as ProviderId;
              setProvider(id);
              setModel(defaultModel(id));
              setBaseUrl(defaultBaseUrl(id));
            }}
            options={PROVIDERS.map((p) => ({ value: p.id, label: t(p.labelKey) }))}
          />
        </Label>

        <Input placeholder={t("ai.modelPlaceholder")} value={model} onChange={setModel} />

        <div className="flex flex-col gap-1">
          <Input placeholder={t("ai.baseUrlPlaceholder")} value={baseUrl} onChange={setBaseUrl} />
          <p className="text-[11px] text-text-muted">{t(BASE_URL_HINT_KEY[provider])}</p>
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
                        ? t("ai.ollamaFound", { num: models.length })
                        : t("ai.ollamaNoModels"),
                    );
                  })
                  .catch((e) => setError(formatAppError(e)));
              }}
            >
              {t("ai.detectOllama")}
            </Button>
            {ollamaModels.length > 0 ? (
              <p className="text-xs text-text-muted truncate">{ollamaModels.join(", ")}</p>
            ) : null}
          </>
        ) : (
          <>
            <Input
              type="password"
              placeholder={keyStatus?.has_key ? t("ai.apiKeyKeep") : t("ai.apiKey")}
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
                      setNotice(t("ai.apiKeyCleared"));
                      void queryClient.invalidateQueries({
                        queryKey: ["ai-key", workspaceId, provider],
                      });
                    })
                    .catch((e) => setError(formatAppError(e)));
                }}
              >
                {t("ai.clearStoredKey")}
              </Button>
            ) : null}
          </>
        )}

        <Checkbox checked={offline} onChange={setOffline} className="text-xs">
          {t("ai.offlineMode")}
        </Checkbox>

        <div className="flex flex-col gap-1">
          <Label className="text-xs text-text-secondary">{t("ai.fallback.label")}</Label>
          <p className="text-[11px] text-text-muted">{t("ai.fallback.hint")}</p>
          {failover.map((fb, i) => (
            <div key={i} className="flex items-center gap-1">
              <Select
                aria-label={t("ai.fallback.ariaLabel", { num: i + 1 })}
                className="w-36 shrink-0"
                value={
                  (PROVIDERS.some((p) => p.id === fb.provider)
                    ? fb.provider
                    : "openai") as ProviderId
                }
                onChange={(next) => updateFallback(i, { provider: next })}
                options={PROVIDERS.map((p) => ({ value: p.id, label: t(p.labelKey) }))}
              />
              <Input
                placeholder={t("ai.fallback.modelPlaceholder")}
                value={fb.model ?? ""}
                onChange={(v) => updateFallback(i, { model: v })}
                className="flex-1"
              />
              <Button
                variant="ghost"
                size="sm"
                disabled={i === 0}
                onClick={() => moveFallback(i, -1)}
                title={t("ai.fallback.moveUp")}
              >
                <ArrowUp size={14} />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                disabled={i === failover.length - 1}
                onClick={() => moveFallback(i, 1)}
                title={t("ai.fallback.moveDown")}
              >
                <ArrowDown size={14} />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="text-danger"
                onClick={() => removeFallback(i)}
                title={t("ai.fallback.remove")}
              >
                <X size={14} />
              </Button>
            </div>
          ))}
          <Button variant="ghost" size="sm" className="self-start" onClick={addFallback}>
            {t("ai.fallback.add")}
          </Button>
        </div>

        <Button
          variant="secondary"
          size="sm"
          className="self-start"
          onClick={() => setTemplatesOpen((v) => !v)}
        >
          {templatesOpen ? t("ai.templates.hide") : t("ai.templates.show")}
        </Button>
        {templatesOpen ? (
          <div className="flex flex-col gap-2 rounded-md border border-border-subtle p-2">
            <p className="text-[11px] text-text-muted">
              {t("ai.templates.hint")} <code>.gitwave/AI.md</code>
              {t("ai.templates.hintTail")}
            </p>
            <PromptTemplateField
              label={t("ai.templates.commitLabel")}
              value={tplCommit}
              onChange={setTplCommit}
              placeholder={t("ai.templates.commitPlaceholder")}
            />
            <PromptTemplateField
              label={t("ai.templates.conflictLabel")}
              value={tplConflict}
              onChange={setTplConflict}
              placeholder={t("ai.templates.conflictPlaceholder")}
            />
            <PromptTemplateField
              label={t("ai.templates.prLabel")}
              value={tplPr}
              onChange={setTplPr}
              placeholder={t("ai.templates.prPlaceholder")}
            />
            <PromptTemplateField
              label={t("ai.templates.reflogLabel")}
              value={tplReflog}
              onChange={setTplReflog}
              placeholder={t("ai.templates.reflogPlaceholder")}
            />
            <PromptTemplateField
              label={t("ai.templates.healthLabel")}
              value={tplHealth}
              onChange={setTplHealth}
              placeholder={t("ai.templates.healthPlaceholder")}
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
