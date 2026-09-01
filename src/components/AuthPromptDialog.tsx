// F012: in-app credential recovery. Mounted once in App; opens whenever a
// sync operation fails on authentication. Submitting hands the entered
// credentials back to the operation's retry closure — on success with
// "remember" checked the backend persists them into the system credential
// helper (shared with the git CLI).

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/Button";
import { Checkbox } from "@/components/ui/Checkbox";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { useAuthPromptStore } from "@/stores/authPromptStore";

export function AuthPromptDialog(): React.JSX.Element | null {
  const { t } = useTranslation();
  const remoteName = useAuthPromptStore((s) => s.remoteName);
  const retry = useAuthPromptStore((s) => s.retry);
  const close = useAuthPromptStore((s) => s.close);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [remember, setRemember] = useState(true);

  // Fresh fields on every open (the component stays mounted): stale input
  // from a previous attempt must not survive, least of all the password.
  useEffect(() => {
    if (remoteName) {
      setUsername("");
      setPassword("");
      setRemember(true);
    }
  }, [remoteName]);

  if (!remoteName || !retry) return null;

  const submit = (): void => {
    if (!username.trim() || !password) return;
    const auth = { username: username.trim(), password, remember };
    close();
    retry(auth);
  };

  return (
    <Modal
      open
      onOpenChange={(open) => {
        if (!open) close();
      }}
      title={t("common.authPrompt.title")}
      description={t("common.authPrompt.description", { remote: remoteName })}
      size="sm"
      footer={
        <>
          <Button variant="secondary" size="sm" className="min-w-0 flex-[3]" onClick={close}>
            {t("common.cancel")}
          </Button>
          <Button
            variant="primary"
            size="sm"
            className="min-w-0 flex-[7]"
            disabled={!username.trim() || !password}
            onClick={submit}
          >
            {t("common.authPrompt.submit")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-2 rounded-xl bg-bg-primary p-3">
        <Input
          autoFocus
          value={username}
          onChange={setUsername}
          placeholder={t("common.authPrompt.username")}
        />
        <Input
          value={password}
          onChange={setPassword}
          placeholder={t("common.authPrompt.token")}
          type="password"
        />
        <Checkbox checked={remember} onChange={setRemember}>
          {t("common.authPrompt.remember")}
        </Checkbox>
      </div>
    </Modal>
  );
}
