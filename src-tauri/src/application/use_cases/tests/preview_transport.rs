use super::*;
use crate::application::use_cases::ai_preview::{preview, send_preview, AiTask};
use serde_json::{json, Value};
use std::time::Duration;
use tokio::io::{AsyncBufReadExt, AsyncReadExt, AsyncWriteExt, BufReader};
use tokio::net::TcpListener;
use tokio::sync::mpsc;

// Recording the actual loopback HTTP bodies catches drift between preview,
// provider-specific serialization, same-provider retry, and chain failover.
async fn mock_ollama() -> (String, mpsc::UnboundedReceiver<Value>) {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let endpoint = format!("http://{}", listener.local_addr().unwrap());
    let (seen, received) = mpsc::unbounded_channel();
    tokio::spawn(async move {
        while let Ok((mut socket, _)) = listener.accept().await {
            let mut stream = BufReader::new(&mut socket);
            let mut length = None;
            let mut header = String::new();
            loop {
                header.clear();
                if stream.read_line(&mut header).await.unwrap() == 0 || header == "\r\n" {
                    break;
                }
                if let Some((name, value)) = header.split_once(':') {
                    if name.eq_ignore_ascii_case("content-length") {
                        length = Some(value.trim().parse::<usize>().unwrap());
                    }
                }
            }
            let mut body = vec![0; length.expect("reqwest JSON has a content length")];
            stream.read_exact(&mut body).await.unwrap();
            let payload: Value = serde_json::from_slice(&body).unwrap();
            let (status, reply) = if payload["model"] == "retry" {
                ("503 Service Unavailable", "{}")
            } else {
                ("200 OK", r#"{"message":{"content":"local mock response"}}"#)
            };
            seen.send(payload).unwrap();
            let response = format!(
                "HTTP/1.1 {status}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{reply}",
                reply.len()
            );
            socket.write_all(response.as_bytes()).await.unwrap();
        }
    });
    (endpoint, received)
}

#[tokio::test]
async fn preview_shows_exact_retry_and_fallback_payloads_and_direct_send_uses_current_context() {
    let (endpoint, mut seen) = mock_ollama().await;
    let ctx = fresh_ctx();
    let ws = create_workspace(&ctx, "Preview".into()).unwrap();
    let (dir, _) = build_linear_repo(1);
    let repo_ref = add_local_repo(&ctx, ws.id.clone(), dir.to_string_lossy().into()).unwrap();
    set_active_repo(&ctx, ws.id.clone(), Some(repo_ref.id)).unwrap();

    let mut settings = ws.settings;
    settings.ai_offline = true;
    settings.ai_provider = Some("ollama".into());
    settings.ai_model = Some("retry".into());
    settings.ai_base_url = Some(endpoint.clone());
    settings.ai_failover = vec![AiProviderConfig {
        provider: "ollama".into(),
        model: Some("ok".into()),
        base_url: Some(endpoint),
    }];
    settings.prompt_templates.commit = Some("Ask team+git@example.org.".into());
    update_workspace_settings(&ctx, ws.id.clone(), settings.clone()).unwrap();
    let stage = |content: &str| {
        fs::write(dir.join("support.txt"), content).unwrap();
        stage_files(&ctx, &ws.id, None, vec!["support.txt".into()]).unwrap();
    };
    stage("Call (415) 555-2671\npassword=alice@example.org\n");
    let task = AiTask::CommitMessage { repo_id: None };
    let snapshot = preview(&ctx, &ws.id, &task, None, None).unwrap();
    assert!(
        seen.try_recv().is_err(),
        "local preview contacted a provider"
    );
    assert_eq!(snapshot.prompt.counts.secret_lines, 1);
    assert_eq!(snapshot.prompt.counts.emails, 1);
    assert_eq!(snapshot.prompt.counts.phones, 1);
    assert!(snapshot.prompt.system.contains("Ask [EMAIL]."));
    assert!(snapshot.prompt.user.contains("Call [PHONE]\n[REDACTED]"));
    let expected = json!([
        {"role": "system", "content": snapshot.prompt.system},
        {"role": "user", "content": snapshot.prompt.user},
    ]);

    stage("Updated contact new@example.org.\n");
    settings.prompt_templates.commit = Some("Updated admin@example.org.".into());
    update_workspace_settings(&ctx, ws.id.clone(), settings.clone()).unwrap();
    let sent = tokio::time::timeout(
        Duration::from_secs(10),
        send_preview(&ctx, &ws.id, &task, None, snapshot.id),
    )
    .await
    .expect("mock provider timed out");
    assert!(sent.is_ok(), "preview confirmation failed");
    for model in ["retry", "retry", "ok"] {
        let payload = seen.try_recv().expect("expected provider attempt");
        assert_eq!(payload["model"], model);
        assert_eq!(payload["messages"], expected);
    }
    assert!(send_preview(&ctx, &ws.id, &task, None, snapshot.id)
        .await
        .is_err());
    assert!(
        seen.try_recv().is_err(),
        "confirmation replay made a request"
    );

    settings.ai_model = Some("ok".into());
    update_workspace_settings(&ctx, ws.id.clone(), settings).unwrap();
    let direct = generate_commit_message(&ctx, ws.id, None, None)
        .await
        .unwrap();
    assert_eq!(direct.text, "local mock response");
    let payload = seen.try_recv().expect("expected direct generation");
    assert!(payload["messages"][0]["content"]
        .as_str()
        .unwrap()
        .contains("Updated [EMAIL]."));
    assert!(payload["messages"][1]["content"]
        .as_str()
        .unwrap()
        .contains("+Updated contact [EMAIL]."));
    assert!(seen.try_recv().is_err());
    cleanup(&dir);
}

#[test]
fn every_task_prepares_a_redacted_preview_without_configuring_a_provider() {
    use crate::infrastructure::git::test_helpers::build_conflicted_merge;

    let ctx = fresh_ctx();
    let ws = create_workspace(&ctx, "Preview".into()).unwrap();
    let (dir, repo, _) = build_conflicted_merge();
    let head = repo.head().unwrap().peel_to_commit().unwrap();
    let base = head.parent_id(0).unwrap().to_string();
    let sha = head.id().to_string();
    drop(head);
    drop(repo);
    let repo_ref = add_local_repo(&ctx, ws.id.clone(), dir.to_string_lossy().into()).unwrap();
    set_active_repo(&ctx, ws.id.clone(), Some(repo_ref.id)).unwrap();
    let rules = "Contact team+review@example.org.";
    fs::create_dir_all(dir.join(".gitwave")).unwrap();
    fs::write(dir.join(".gitwave/AI.md"), rules).unwrap();
    fs::write(dir.join("extra.txt"), "Updated info\n").unwrap();
    stage_files(&ctx, &ws.id, None, vec!["extra.txt".into()]).unwrap();
    let mut settings = ws.settings;
    settings.prompt_templates.health = Some(rules.into());
    settings.prompt_templates.reflog = Some(rules.into());
    update_workspace_settings(&ctx, ws.id.clone(), settings).unwrap();

    let tasks = [
        AiTask::CommitMessage { repo_id: None },
        AiTask::Pr {
            base: Some(base.clone()),
        },
        AiTask::Commit { sha: sha.clone() },
        AiTask::Conflict {
            path: "file0.txt".into(),
        },
        AiTask::Palette {
            query: "Show branches".into(),
        },
        AiTask::Health,
        AiTask::Reflog {
            old_oid: base,
            new_oid: sha,
            action: "commit".into(),
            message: "Main edit".into(),
        },
    ];
    for task in tasks {
        let result = preview(&ctx, &ws.id, &task, None, None).unwrap();
        assert!(result.prompt.system.contains("Contact [EMAIL]."));
        assert!(!result.prompt.user.is_empty());
        assert_eq!(result.prompt.counts.emails, 1);
    }
    cleanup(&dir);
}

#[tokio::test]
async fn changed_object_or_repository_and_discarded_handle_cannot_be_sent() {
    use crate::application::use_cases::ai_preview::discard;

    let ctx = fresh_ctx();
    let ws = create_workspace(&ctx, "Preview".into()).unwrap();
    let (first, repo) = build_linear_repo(1);
    let (second, _) = build_linear_repo(1);
    let a = add_local_repo(&ctx, ws.id.clone(), first.to_string_lossy().into()).unwrap();
    let b = add_local_repo(&ctx, ws.id.clone(), second.to_string_lossy().into()).unwrap();
    set_active_repo(&ctx, ws.id.clone(), Some(a.id)).unwrap();
    let task = AiTask::Palette {
        query: "show tags".into(),
    };
    let other = AiTask::Palette {
        query: "show branches".into(),
    };
    let prepared = || preview(&ctx, &ws.id, &task, None, None).unwrap().id;

    assert_eq!(
        preview(&ctx, &ws.id, &task, None, Some(&b.id))
            .err()
            .unwrap()
            .code(),
        codes::usecases::AI_PREVIEW_EXPIRED
    );
    let id = prepared();
    let err = send_preview(&ctx, &ws.id, &other, None, id)
        .await
        .err()
        .unwrap();
    assert_eq!(err.code(), codes::usecases::AI_PREVIEW_EXPIRED);
    let id = prepared();
    discard(&ctx, id);
    let err = send_preview(&ctx, &ws.id, &task, None, id)
        .await
        .err()
        .unwrap();
    assert_eq!(err.code(), codes::usecases::AI_PREVIEW_EXPIRED);
    fs::write(first.join("new.txt"), "staged content").unwrap();
    stage_files(&ctx, &ws.id, None, vec!["new.txt".into()]).unwrap();
    let message = AiTask::CommitMessage { repo_id: None };
    let id = preview(&ctx, &ws.id, &message, None, None).unwrap().id;
    repo.branch(
        "other",
        &repo.head().unwrap().peel_to_commit().unwrap(),
        false,
    )
    .unwrap();
    repo.set_head("refs/heads/other").unwrap();
    let err = send_preview(&ctx, &ws.id, &message, None, id)
        .await
        .err()
        .unwrap();
    assert_eq!(err.code(), codes::usecases::AI_PREVIEW_EXPIRED);
    drop(repo);
    let id = prepared();
    set_active_repo(&ctx, ws.id.clone(), Some(b.id)).unwrap();
    let err = send_preview(&ctx, &ws.id, &task, None, id)
        .await
        .err()
        .unwrap();
    assert_eq!(err.code(), codes::usecases::AI_PREVIEW_EXPIRED);
    cleanup(&first);
    cleanup(&second);
}
