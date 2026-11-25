//! The interface languages, and the checks that keep their message files aligned.
//!
//! `locales/<domain>.<locale>.json` holds the interface text, one tree per domain
//! and language. rust-i18n takes the language from the last dot-separated part
//! of the file name and flattens every tree into dotted keys, so a message reads
//! `t!("app.sidebar.health")` and an error message `t!("errors.git.raw")`. The
//! files are registered in `main` and embedded when the crate is compiled.
//! `build.rs` puts a digest of them into the crate's fingerprint, so editing
//! one triggers a rebuild.
//!
//! The files were carried over from the previous interface, which read them with
//! i18next. Only the placeholders changed, from `{{name}}` to `%{name}`. rust-i18n
//! has neither plurals nor contexts: the keys that end in `_one` and `_other`
//! have to be chosen in code.

#[cfg(test)]
mod tests {
    use std::collections::BTreeSet;
    use std::fs;
    use std::path::{Path, PathBuf};

    use serde_json::Value;

    /// The languages the interface ships with. English is the fallback, so a
    /// message missing from another language still shows.
    const LANGUAGES: [&str; 2] = ["en", "zh-CN"];

    /// The message files live in the crate; the error codes they name live in
    /// gitwave-core.
    const ERROR_CODES: &str = "../gitwave-core/src/domain/error_codes";

    fn crate_dir() -> PathBuf {
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
    }

    /// Every `*.json` in `locales/`, in file name order.
    fn message_files() -> Vec<PathBuf> {
        let mut files: Vec<PathBuf> = fs::read_dir(crate_dir().join("locales"))
            .expect("read the locales directory")
            .map(|entry| entry.expect("a locales entry").path())
            .filter(|path| {
                path.extension()
                    .is_some_and(|extension| extension == "json")
            })
            .collect();
        files.sort();
        files
    }

    /// `<domain>.<locale>` of a message file. rust-i18n reads the locale the
    /// same way, from the last dot-separated part of the file name.
    fn split(path: &Path) -> (String, String) {
        let stem = path
            .file_stem()
            .and_then(|stem| stem.to_str())
            .expect("a file stem");
        let (domain, locale) = stem.rsplit_once('.').expect("a domain and a locale");
        (domain.to_string(), locale.to_string())
    }

    /// Every message file of one language, as (domain, tree), in file name
    /// order.
    fn domains(locale: &str) -> Vec<(String, Value)> {
        let files: Vec<PathBuf> = message_files()
            .into_iter()
            .filter(|path| split(path).1 == locale)
            .collect();
        assert!(!files.is_empty(), "no message files for {locale}");
        files
            .into_iter()
            .map(|path| {
                let (domain, _) = split(&path);
                let tree = serde_json::from_slice(&fs::read(&path).expect("read a message file"))
                    .expect("a message file holds JSON");
                (domain, tree)
            })
            .collect()
    }

    /// Dotted key paths of every message in a tree.
    fn leaves(value: &Value, prefix: &str, out: &mut BTreeSet<String>) {
        match value {
            Value::Object(map) => {
                for (key, child) in map {
                    let path = if prefix.is_empty() {
                        key.clone()
                    } else {
                        format!("{prefix}.{key}")
                    };
                    leaves(child, &path, out);
                }
            }
            Value::String(text) => {
                assert!(!text.trim().is_empty(), "{prefix} is an empty message");
                out.insert(prefix.to_string());
            }
            other => panic!("{prefix} is not a string: {other}"),
        }
    }

    fn messages(tree: &Value) -> BTreeSet<String> {
        let mut paths = BTreeSet::new();
        leaves(tree, "", &mut paths);
        paths
    }

    /// The node a dotted path points at.
    fn node<'a>(tree: &'a Value, path: &str) -> Option<&'a Value> {
        path.split('.').try_fold(tree, |node, key| node.get(key))
    }

    fn message<'a>(tree: &'a Value, path: &str) -> &'a str {
        node(tree, path)
            .and_then(Value::as_str)
            .unwrap_or_else(|| panic!("{path} is not a message"))
    }

    /// The `%{name}` placeholders a message uses. rust-i18n interpolates a
    /// placeholder only when code passes the very same name, so anything other
    /// than an identifier between the braces would never be replaced.
    fn placeholders(text: &str) -> BTreeSet<String> {
        let mut found = BTreeSet::new();
        let mut rest = text;
        while let Some(start) = rest.find("%{") {
            let after = &rest[start + 2..];
            let end = after
                .find('}')
                .unwrap_or_else(|| panic!("{text:?} opens a placeholder it never closes"));
            let name = &after[..end];
            assert!(
                !name.is_empty()
                    && name
                        .chars()
                        .all(|character| character.is_ascii_alphanumeric() || character == '_'),
                "{text:?} has the placeholder `%{{{name}}}`; a placeholder name is letters, digits and underscores"
            );
            found.insert(name.to_string());
            rest = &after[end + 1..];
        }
        found
    }

    /// The `errors` trees of every `errors-*` file, merged into one. Only
    /// `errors-*` files may carry an `errors` tree, and each of them must.
    fn errors(locale: &str) -> Value {
        let mut merged = Value::Object(Default::default());
        for (domain, tree) in domains(locale) {
            match (domain.starts_with("errors"), tree.get("errors")) {
                (true, Some(errors)) => merge(&mut merged, errors, ""),
                (true, None) => panic!("{domain}.{locale}.json has no `errors` tree"),
                (false, Some(_)) => panic!(
                    "{domain}.{locale}.json carries an `errors` tree; error messages belong in errors-* files"
                ),
                (false, None) => {}
            }
        }
        merged
    }

    /// Merges `source` into `target`. rust-i18n would settle two files that
    /// define the same message by load order, so it fails here instead.
    fn merge(target: &mut Value, source: &Value, prefix: &str) {
        if let (Value::Object(target), Value::Object(source)) = (&mut *target, source) {
            for (key, value) in source {
                let path = if prefix.is_empty() {
                    key.clone()
                } else {
                    format!("{prefix}.{key}")
                };
                match target.get_mut(key) {
                    Some(slot) if slot.is_object() && value.is_object() => {
                        merge(slot, value, &path);
                    }
                    Some(_) => panic!("two message files define `errors.{path}`"),
                    None => {
                        target.insert(key.clone(), value.clone());
                    }
                }
            }
        }
    }

    /// The value of every `pub const NAME: &str = "a.b.c";` in the error_codes
    /// module, read from its source the way the previous interface did.
    fn error_codes() -> BTreeSet<String> {
        let mut codes = BTreeSet::new();
        for path in fs::read_dir(crate_dir().join(ERROR_CODES)).expect("read error_codes") {
            let path = path.expect("an error_codes entry").path();
            if path.extension().is_none_or(|extension| extension != "rs") {
                continue;
            }
            let source = fs::read_to_string(&path).expect("read an error_codes file");
            let constants = source
                .lines()
                .map(str::trim_start)
                .filter(|line| line.starts_with("pub const "));
            for line in constants {
                let (_, value) = line
                    .strip_prefix("pub const ")
                    .and_then(|rest| rest.split_once(": &str = \""))
                    .unwrap_or_else(|| panic!("not a string constant: {line}"));
                codes.insert(value.split('"').next().expect("a value").to_string());
            }
        }
        codes
    }

    #[test]
    fn the_languages_on_disk_are_exactly_the_shipped_ones() {
        let on_disk: BTreeSet<String> = message_files().iter().map(|path| split(path).1).collect();
        assert_eq!(
            on_disk,
            LANGUAGES
                .iter()
                .map(|language| language.to_string())
                .collect(),
            "a message file names a language that is not in LANGUAGES, or the other way round"
        );
    }

    #[test]
    fn every_language_ships_the_same_domains_with_the_same_messages() {
        let english = domains("en");
        for language in LANGUAGES {
            let other = domains(language);
            assert_eq!(
                other.iter().map(|(domain, _)| domain).collect::<Vec<_>>(),
                english.iter().map(|(domain, _)| domain).collect::<Vec<_>>(),
                "{language} ships different domains"
            );
            for ((_, en_tree), (_, tree)) in english.iter().zip(&other) {
                assert_eq!(
                    messages(tree),
                    messages(en_tree),
                    "{language} ships different messages"
                );
            }
        }
    }

    #[test]
    fn every_message_uses_the_same_placeholders_in_every_language() {
        let english = domains("en");
        for language in LANGUAGES.into_iter().skip(1) {
            for ((_, en_tree), (_, tree)) in english.iter().zip(domains(language)) {
                for path in messages(en_tree) {
                    assert_eq!(
                        placeholders(message(&tree, &path)),
                        placeholders(message(en_tree, &path)),
                        "{language}/{path} uses different placeholders"
                    );
                }
            }
        }
    }

    /// Reads every message back through rust-i18n and compares it with its
    /// file. The catalogues are embedded at compile time, so a binary built
    /// before a file was edited fails here as well.
    #[test]
    fn every_message_loads_through_rust_i18n_as_written() {
        for language in LANGUAGES {
            rust_i18n::set_locale(language);
            for (domain, tree) in domains(language) {
                for path in messages(&tree) {
                    assert_eq!(
                        rust_i18n::t!(&path),
                        message(&tree, &path),
                        "{language}/{path} does not load the way {domain}.{language}.json spells it"
                    );
                }
            }
        }
        rust_i18n::set_locale("en");
    }

    #[test]
    fn every_error_code_has_a_message_and_every_error_message_matches_a_code() {
        let codes = error_codes();
        assert!(!codes.is_empty(), "no error codes found");
        for language in LANGUAGES {
            let errors = errors(language);
            for code in &codes {
                assert!(
                    node(&errors, code).is_some_and(Value::is_string),
                    "{language}: errors.{code} has no message"
                );
            }
            for path in messages(&errors) {
                assert!(
                    codes.contains(&path),
                    "{language}: errors.{path} matches no error code"
                );
            }
        }
    }
}
