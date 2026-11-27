//! Lightweight secret scrubber before sending diffs to cloud AI.
//!
//! Line-granular and biased to over-redact: a lost context line costs less
//! than a leaked key. This is the only defence between staged diffs and
//! third-party LLM APIs, so new secret shapes get added here (with a test).

use regex::Regex;
use serde::Serialize;
use std::sync::LazyLock;

#[derive(Clone, Default, Serialize)]
pub struct RedactionCounts {
    pub secret_lines: usize,
    pub emails: usize,
    pub phones: usize,
}

/// The only prompt type accepted by the provider adapter. It can be
/// serialized for display, but the webview cannot deserialize or supply one.
#[derive(Clone, Serialize)]
pub struct PreparedPrompt {
    pub system: String,
    pub user: String,
    pub counts: RedactionCounts,
}

impl PreparedPrompt {
    pub fn new(system: &str, user: &str) -> Self {
        let mut counts = RedactionCounts::default();
        Self {
            system: scrub(system, &mut counts),
            user: scrub(user, &mut counts),
            counts,
        }
    }
}

static EMAIL: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r"(?i)[a-z0-9_][a-z0-9._%+'-]*@[a-z0-9-]+(?:[.][a-z0-9-]+)*[.][a-z]{2,}").unwrap()
});
static PHONE: LazyLock<Regex> = LazyLock::new(|| {
    let international = r"\+[1-9](?:[\x20().-]*[0-9]){6,14}";
    let area_code = r"\([2-9][0-9]{2}\)[\x20]?";
    let local_number = r"[0-9]{3}[\x20.-][0-9]{4}";
    // Prefer two complete numbers before the international branch can consume the next area code.
    Regex::new(&format!(
        r"(?x)
        {international}(?P<gap>\x20+){area_code}{local_number}(?-u:\b)
        |{international}
        |1[3-9][0-9][\x20-]?[0-9]{{4}}[\x20-]?[0-9]{{4}}
        |(?:{area_code}|[2-9][0-9]{{2}}[\x20.-]){local_number}
        ",
    ))
    .unwrap()
});

fn mask_contacts(line: &str, counts: &mut RedactionCounts) -> String {
    let email_masked = EMAIL.replace_all(line, |m: &regex::Captures<'_>| {
        let matched = m.get(0).unwrap();
        let suffix = &line[matched.end()..];
        // Only exempt an identifiable Git remote. A colon also commonly
        // follows an ordinary contact address in prose and configuration.
        let path = suffix.strip_prefix(':').and_then(|s| {
            // split_whitespace would also accept a label followed by a space.
            s.split(char::is_whitespace).next()
        });
        let remote = path.is_some_and(|path| {
            let path = path.trim_end_matches(['"', '\'', ')', ']', '>', '.', ',', ';']);
            !path.contains(':')
                && (path.ends_with(".git")
                    || (matched.as_str().starts_with("git@")
                        && (path.contains('/')
                            || path.starts_with(|c: char| c.is_ascii_alphabetic()))))
        });
        if remote {
            matched.as_str().to_owned()
        } else {
            counts.emails += 1;
            "[EMAIL]".to_owned()
        }
    });
    PHONE
        .replace_all(&email_masked, |m: &regex::Captures<'_>| {
            let matched = m.get(0).unwrap();
            let before = email_masked[..matched.start()].chars().next_back();
            let after = email_masked[matched.end()..].chars().next();
            let in_identifier = |c: char| c.is_ascii_alphanumeric() || c == '_';
            let prefix = &email_masked[..matched.start()];
            let version_component = prefix
                .strip_suffix('.')
                .and_then(|s| s.chars().next_back())
                .is_some_and(|c| c.is_ascii_digit());
            let starts_in_identifier = version_component || before.is_some_and(in_identifier);
            if let Some(gap) = m.name("gap") {
                counts.phones += 1;
                let first = if starts_in_identifier {
                    &email_masked[matched.start()..gap.start()]
                } else {
                    counts.phones += 1;
                    "[PHONE]"
                };
                format!("{first}{}[PHONE]", gap.as_str())
            } else if starts_in_identifier || after.is_some_and(in_identifier) {
                matched.as_str().to_owned()
            } else {
                counts.phones += 1;
                "[PHONE]".to_owned()
            }
        })
        .into_owned()
}

/// Whole-line keywords, matched case-insensitively. `token` is deliberately
/// NOT a bare keyword (`tokenCount` etc. are common identifiers) — only
/// assignment / affixed forms count.
const LINE_KEYWORDS: &[&str] = &[
    "api_key",
    "apikey",
    "secret",
    "password",
    "passwd",
    "private_key",
    "privatekey",
    "aws_",
    "bearer",
    "auth_token",
    "access_token",
    "refresh_token",
    "client_secret",
    "credential",
    "token=",
    "token:",
    "_token",
    "token_",
    "sessiontoken",
    "accesskeyid",
    "-----begin",
];

/// Known secret token prefixes, matched case-insensitively
/// (`GHP_…` / `SK-ant-…` must hit as well as their lowercase forms).
/// Note: no bare `asia` — it over-matches prose and timezones
/// (`Asia/Shanghai`); STS `ASIA…` keys realistically travel with `aws_` /
/// `secret` context lines that already hit, or with `sessiontoken` /
/// `accesskeyid` (both listed above).
const TOKEN_PREFIXES: &[&str] = &[
    "ghp_",
    "gho_",
    "ghu_",
    "ghs_",
    "ghr_",
    "github_pat_",
    "glpat-",
    "sk-",
    "xoxb-",
    "xoxp-",
    "xoxa-",
    "xoxr-",
    "xoxs-",
    "xoxo-",
    "xoxd-",
    "akia",
    "aiza",
    "npm_",
];

/// Inspect complete Git blobs before selecting diff hunks: a hunk can
/// contain a single short key-body line with no recognisable delimiters.
pub fn contains_private_key(input: &[u8]) -> bool {
    input
        .windows(b"PRIVATE KEY-----".len())
        .any(|part| part.eq_ignore_ascii_case(b"PRIVATE KEY-----"))
}

/// Redact common secret shapes from text destined for AI providers.
#[cfg(test)]
fn scrub_secrets(input: &str) -> String {
    scrub(input, &mut RedactionCounts::default())
}

fn scrub(input: &str, counts: &mut RedactionCounts) -> String {
    let lines: Vec<_> = input.lines().collect();
    let mut sensitive = vec![false; lines.len()];
    let mut in_key = false;
    let mut run_start = 0;
    let mut encoded_run = false;
    // Diffs may omit the BEGIN/END lines entirely. Conservatively remove
    // complete runs of base64-looking text once a long encoded line is seen,
    // including short final lines. This can also remove encoded assets.
    for (i, line) in lines.iter().enumerate() {
        let payload = if line.starts_with("-----") {
            // A raw PEM delimiter already starts with '-' without a diff
            // prefix. Both five and six dashes still contain the marker.
            *line
        } else {
            line.strip_prefix(['+', '-', ' ']).unwrap_or(line)
        }
        .trim();
        if line.starts_with("diff --git ") {
            in_key = false;
        }
        let upper = payload.to_ascii_uppercase();
        let begins = upper.contains("-----BEGIN ") && upper.contains("PRIVATE KEY-----");
        let ends = upper.contains("-----END ") && upper.contains("PRIVATE KEY-----");
        sensitive[i] = in_key || begins || ends;
        in_key = (in_key || begins) && !ends;
        let base64 = !payload.is_empty()
            && payload
                .bytes()
                .all(|b| b.is_ascii_alphanumeric() || b"+/=".contains(&b));
        if base64 {
            encoded_run |= payload.len() >= 32;
        } else if !line.starts_with("@@") {
            if encoded_run {
                sensitive[run_start..i].fill(true);
            }
            run_start = i + 1;
            encoded_run = false;
        }
    }
    if encoded_run {
        sensitive[run_start..].fill(true);
    }
    let mut out = String::with_capacity(input.len());
    for (i, line) in lines.iter().enumerate() {
        let lower = line.to_ascii_lowercase();
        // Spaceless copy catches `token = "…"` / `token : "…"` while plain
        // `tokenCount` still passes (no `=`/`:` directly after `token`).
        let nospace: String = lower.chars().filter(|c| *c != ' ' && *c != '\t').collect();
        let redacted = sensitive[i]
            || LINE_KEYWORDS.iter().any(|k| lower.contains(k))
            || nospace.contains("token=")
            || nospace.contains("token:")
            || TOKEN_PREFIXES.iter().any(|p| lower.contains(p));
        if redacted {
            counts.secret_lines += 1;
            out.push_str("[REDACTED]");
        } else {
            out.push_str(&mask_contacts(line, counts));
        }
        out.push('\n');
    }
    if !input.ends_with('\n') && out.ends_with('\n') {
        out.pop();
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn redacts_private_key_blocks_with_all_diff_prefixes() {
        for prefix in ["", "+", "-", " "] {
            for kind in ["RSA", "EC", "OPENSSH", "ENCRYPTED"] {
                let input = format!("{prefix}-----BEGIN {kind} PRIVATE KEY-----\n{prefix}shortPayload\n{prefix}-----END {kind} PRIVATE KEY-----\n+let x = 1;");
                let scrubbed = scrub_secrets(&input);
                assert!(!scrubbed.contains("shortPayload"));
                assert!(!scrubbed.contains("PRIVATE KEY"));
                assert!(scrubbed.contains("let x = 1;"));
            }
        }
    }

    #[test]
    fn redacts_body_only_hunks_including_short_tail() {
        let input = "@@ -20,2 +20,2 @@\n-ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcd\n+abcdefghijklmnopqrstuvwxyz0123456789ABCD\n@@ -30 +30 @@\n+tiny==\n+let x = 1;";
        let scrubbed = scrub_secrets(input);
        assert!(!scrubbed.contains("ABCDEFGHIJKLMNOPQRSTUVWXYZ"));
        assert!(!scrubbed.contains("abcdefghijklmnopqrstuvwxyz"));
        assert!(!scrubbed.contains("tiny"));
        assert!(scrubbed.contains("let x = 1;"));
    }

    #[test]
    fn truncated_key_stays_redacted_until_next_file() {
        let input = "+-----BEGIN PRIVATE KEY-----\n+shortBody\n@@ -50 +50 @@\n+tail\ndiff --git a/code b/code\n+let x = 1;";
        let scrubbed = scrub_secrets(input);
        assert!(!scrubbed.contains("shortBody"));
        assert!(!scrubbed.contains("tail"));
        assert!(scrubbed.contains("let x = 1;"));
    }

    #[test]
    fn scrubs_openai_style_key_line() {
        let s = scrub_secrets("Authorization: Bearer sk-abcdefghijklmnopqrstuvwxyz");
        assert!(s.contains("[REDACTED]"));
        assert!(!s.contains("sk-abc"));
    }

    #[test]
    fn scrubs_uppercase_vendor_prefixes() {
        for line in [
            "+GITHUB_TOKEN=GHP_ABCDEFGHIJKLMNOP",
            "+key = \"SK-ant-api03-xyz\"",
            "+AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE",
            "+const google = \"AIzaSyD-EXAMPLE\";",
        ] {
            let s = scrub_secrets(line);
            assert!(s.contains("[REDACTED]"), "missed: {line}");
        }
    }

    #[test]
    fn scrubs_collab_and_package_tokens() {
        for line in [
            "+token: xoxb-1234-abcdef",
            "+password: glpat-AbCdEfGhIjKlMnOpQrSt",
            "+// registry=https://npm.pkg.github.com/:_authToken=npm_abc123",
            "+aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCY",
            "+-----BEGIN RSA PRIVATE KEY-----",
        ] {
            let s = scrub_secrets(line);
            assert!(s.contains("[REDACTED]"), "missed: {line}");
        }
    }

    #[test]
    fn keeps_ordinary_code_and_identifiers() {
        let s =
            scrub_secrets("+const tokenCount = tokens.length;\n+// refresh the view\n+let x = 1;");
        assert!(!s.contains("[REDACTED]"), "over-redacted: {s}");
        assert!(s.contains("tokenCount"));
    }

    #[test]
    fn scrubs_spaced_token_assignment() {
        let s = scrub_secrets("+token = \"abc123\"");
        assert!(s.contains("[REDACTED]"), "missed spaced assignment: {s}");
    }

    #[test]
    fn keeps_prose_and_timezones_despite_asia() {
        let s = scrub_secrets("+// schedule for Asia/Shanghai\n+const region = \"asia-pacific\";");
        assert!(!s.contains("[REDACTED]"), "over-redacted: {s}");
    }
}

#[cfg(test)]
mod contact_tests {
    use super::PreparedPrompt;

    #[test]
    fn counts_contacts_in_both_messages_after_removing_secret_lines() {
        let prompt = PreparedPrompt::new(
            "Owner: A.User+build@dev.example.org.\npassword = hidden@example.org (+14155552671)",
            "Email <help@example.net>, dial +44 20 7946 0958 or (415) 555-2671.\nCN: 13800138000\n",
        );
        assert_eq!(prompt.system, "Owner: [EMAIL].\n[REDACTED]");
        assert_eq!(
            prompt.user,
            "Email <[EMAIL]>, dial [PHONE] or [PHONE].\nCN: [PHONE]\n"
        );
        assert_eq!(prompt.counts.secret_lines, 1);
        assert_eq!(prompt.counts.emails, 2);
        assert_eq!(prompt.counts.phones, 3);
    }

    #[test]
    fn distinguishes_contact_labels_from_git_remotes() {
        let prompt = PreparedPrompt::new("", "help@example.net:443 user@example.net:来电 git@host.example:repo user@host.example:org/repo.git");
        assert_eq!(
            prompt.user,
            "[EMAIL]:443 [EMAIL]:来电 git@host.example:repo user@host.example:org/repo.git"
        );
    }

    #[test]
    fn masks_email_followed_by_a_url_or_contact_path() {
        let prompt = PreparedPrompt::new(
            "",
            "alice@example.org:https://example.org owner@example.org:team/support git@example.org:demo/app.git",
        );
        assert_eq!(
            prompt.user,
            "[EMAIL]:https://example.org [EMAIL]:team/support git@example.org:demo/app.git"
        );
        assert_eq!(prompt.counts.emails, 2);
    }

    #[test]
    fn separately_masks_adjacent_international_and_parenthesized_phones() {
        for input in [
            "+44 1632 960123 (202) 555-0142",
            "+1 (415) 555-2671 (202) 555-0142",
        ] {
            let prompt = PreparedPrompt::new("", input);
            assert_eq!(prompt.user, "[PHONE] [PHONE]");
            assert_eq!(prompt.counts.phones, 2);
        }
    }

    #[test]
    fn keeps_masking_international_phones_with_parenthesized_groups() {
        for input in ["+1 ( 415 ) 555-2671", "+44 20 (7946) 0958"] {
            let prompt = PreparedPrompt::new("", input);
            assert_eq!(prompt.user, "[PHONE]");
            assert_eq!(prompt.counts.phones, 1);
        }
    }

    #[test]
    fn masks_parenthesized_phone_after_an_identifier() {
        let prompt = PreparedPrompt::new("", "id+123456789012345 (202) 555-0142");
        assert_eq!(prompt.user, "id+123456789012345 [PHONE]");
        assert_eq!(prompt.counts.phones, 1);
    }

    #[test]
    fn keeps_unlabelled_hexadecimal_material_in_diff_hunks_redacted() {
        let key = "b2a18c9f".repeat(8);
        let prompt = PreparedPrompt::new("", &format!("+{key}"));
        assert_eq!(prompt.user, "[REDACTED]");
    }

    #[test]
    fn keeps_git_identifiers_dates_versions_and_ssh_remote_addresses() {
        let context = "Remote git@github.com:owner/project.git\n\
            Commit: a13800138000bbbeefed1234, timestamp: 1735689600000\n\
            Version: 1.23.456.7890, v2.202.555.0142; dated 2026-11-27T15:30:45+08:00";
        let prompt = PreparedPrompt::new("", context);
        assert_eq!(prompt.user, context);
        assert_eq!(prompt.counts.emails, 0);
        assert_eq!(prompt.counts.phones, 0);
    }
}
