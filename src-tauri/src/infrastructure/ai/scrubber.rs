//! Lightweight secret scrubber before sending diffs to cloud AI.

/// Redact common secret shapes from text destined for AI providers.
pub fn scrub_secrets(input: &str) -> String {
    let mut out = String::with_capacity(input.len());
    for line in input.lines() {
        let lower = line.to_ascii_lowercase();
        let redacted = if lower.contains("api_key")
            || lower.contains("apikey")
            || lower.contains("secret")
            || lower.contains("password")
            || lower.contains("-----begin")
            || line.contains("ghp_")
            || line.contains("sk-")
        {
            "[REDACTED]"
        } else {
            line
        };
        out.push_str(redacted);
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
}
