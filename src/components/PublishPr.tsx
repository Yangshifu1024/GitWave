import { useState } from "react";
import { invoke } from "@tauri-apps/api/core";
import { openUrl } from "@tauri-apps/plugin-opener";
import { useQuery, useMutation } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { formatAppError } from "@/lib/api";
type Target = { repository: string; head: string; oid: string; base: string };
export function PublishPr({
  workspaceId,
  repoId,
  title,
  body,
  disabled,
}: {
  workspaceId: string;
  repoId: string;
  title: string;
  body: string;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const [review, setReview] = useState(false);
  const [openError, setOpenError] = useState<string | null>(null);
  const [base, setBase] = useState<string | null>(null);
  const target = useQuery({
    queryKey: ["github-pr-target", workspaceId, repoId],
    queryFn: () => invoke<Target>("cmd_github_pr_target", { workspaceId, repoId }),
    enabled: review,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const publish = useMutation({
    mutationFn: (draft: { target: Target; base: string; title: string; body: string }) =>
      invoke<string>("cmd_create_github_pr", { workspaceId, repoId, draft }),
    retry: false,
  });
  const url = publish.data;
  if (url)
    return (
      <div>
        <Button
          onClick={() => {
            setOpenError(null);
            void openUrl(url).catch((error: unknown) => setOpenError(formatAppError(error)));
          }}
        >
          {url}
        </Button>
        {openError && <p role="alert">{openError}</p>}
      </div>
    );
  if (!review)
    return (
      <Button disabled={!title.trim() || disabled} onClick={() => setReview(true)}>
        {t("commits.pr.review")}
      </Button>
    );
  const choice = base ?? target.data?.base ?? "";
  return (
    <section className="space-y-2">
      {target.isLoading && <p>{t("diff.preview.loading")}</p>}
      {(target.error || publish.error) && (
        <p role="alert">
          {formatAppError(target.error ?? publish.error)}{" "}
          <Button
            disabled={publish.isPending}
            onClick={() => {
              publish.reset();
              void target.refetch();
            }}
          >
            {t("common.retry")}
          </Button>
        </p>
      )}
      {target.data && (
        <>
          <p>
            {target.data.repository} · {target.data.head} ({target.data.oid.slice(0, 7)}) → {choice}
          </p>
          <Input
            label={t("commits.pr.base")}
            value={choice}
            onChange={setBase}
            disabled={publish.isPending}
          />
          <Button
            disabled={
              disabled ||
              target.isFetching ||
              !!target.error ||
              publish.isPending ||
              !title.trim() ||
              !choice ||
              choice === target.data.head
            }
            onClick={() => {
              if (target.data) publish.mutate({ target: target.data, base: choice, title, body });
            }}
          >
            {t("commits.pr.publish")}
          </Button>
        </>
      )}
    </section>
  );
}
