//! Lightweight secret scrubber before sending diffs to cloud AI.
//!
//! Line-granular and biased to over-redact: a lost context line costs less
//! than a leaked key. This is the only defence between staged diffs and
//! third-party LLM APIs, so new secret shapes get added here (with a test).

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

/// Redact common secret shapes from text destined for AI providers.
pub fn scrub_secrets(input: &str) -> String {
    let mut out = String::with_capacity(input.len());
    for line in input.lines() {
        let lower = line.to_ascii_lowercase();
        // Spaceless copy catches `token = "…"` / `token : "…"` while plain
        // `tokenCount` still passes (no `=`/`:` directly after `token`).
        let nospace: String = lower.chars().filter(|c| *c != ' ' && *c != '\t').collect();
        let redacted = LINE_KEYWORDS.iter().any(|k| lower.contains(k))
            || nospace.contains("token=")
            || nospace.contains("token:")
            || TOKEN_PREFIXES.iter().any(|p| lower.contains(p));
        out.push_str(if redacted { "[REDACTED]" } else { line });
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
