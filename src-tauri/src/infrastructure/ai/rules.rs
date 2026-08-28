//! Per-repo AI rules — an optional `<repo>/.gitwave/AI.md` whose content is
//! appended to every AI system prompt, letting a repository pin
//! project-specific conventions (language, style, forbidden topics).

use std::path::Path;

/// Hard cap so a huge rules file cannot blow the prompt budget.
const MAX_RULES_CHARS: usize = 8_000;

/// Read `<workdir>/.gitwave/AI.md`. Returns `None` when the file is
/// absent, unreadable, or blank — rules are advisory and must never block
/// a request.
pub fn read_ai_rules(workdir: &Path) -> Option<String> {
    let path = workdir.join(".gitwave").join("AI.md");
    let content = std::fs::read_to_string(path).ok()?;
    let trimmed = content.trim().to_string();
    if trimmed.is_empty() {
        return None;
    }
    if trimmed.chars().count() > MAX_RULES_CHARS {
        let cut: String = trimmed.chars().take(MAX_RULES_CHARS).collect();
        return Some(format!("{cut}\n[rules truncated due to size]"));
    }
    Some(trimmed)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    fn temp_dir(label: &str) -> std::path::PathBuf {
        let dir = std::env::temp_dir().join(format!(
            "gitwave-rules-{label}-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .expect("clock")
                .as_nanos()
        ));
        fs::create_dir_all(&dir).expect("mkdir");
        dir
    }

    #[test]
    fn absent_file_is_no_rules() {
        let dir = temp_dir("absent");
        assert_eq!(read_ai_rules(&dir), None);
        fs::remove_dir_all(&dir).expect("cleanup");
    }

    #[test]
    fn blank_file_is_no_rules() {
        let dir = temp_dir("blank");
        fs::create_dir_all(dir.join(".gitwave")).expect("mkdir");
        fs::write(dir.join(".gitwave").join("AI.md"), "  \n\n  ").expect("write");
        assert_eq!(read_ai_rules(&dir), None);
        fs::remove_dir_all(&dir).expect("cleanup");
    }

    #[test]
    fn rules_content_is_trimmed() {
        let dir = temp_dir("content");
        fs::create_dir_all(dir.join(".gitwave")).expect("mkdir");
        fs::write(
            dir.join(".gitwave").join("AI.md"),
            "\nWrite subjects in English.\n",
        )
        .expect("write");
        assert_eq!(
            read_ai_rules(&dir).as_deref(),
            Some("Write subjects in English.")
        );
        fs::remove_dir_all(&dir).expect("cleanup");
    }

    #[test]
    fn oversized_rules_are_truncated() {
        let dir = temp_dir("huge");
        fs::create_dir_all(dir.join(".gitwave")).expect("mkdir");
        fs::write(
            dir.join(".gitwave").join("AI.md"),
            "x".repeat(MAX_RULES_CHARS + 500),
        )
        .expect("write");
        let rules = read_ai_rules(&dir).expect("rules");
        assert!(rules.contains("[rules truncated due to size]"));
        assert!(rules.chars().count() < MAX_RULES_CHARS + 100);
        fs::remove_dir_all(&dir).expect("cleanup");
    }
}
