//! The interface languages: which ones GitWave ships, how one is chosen, and the
//! checks that keep their message files aligned.
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
use std::sync::{Mutex, MutexGuard, PoisonError};

use gitwave_core::application::use_cases::{
    AppContext, get_language_settings, set_language_settings,
};
use gitwave_core::domain::LanguageSettings;
#[cfg(test)]
use gpui_kit::TestAppContext;
use gpui_kit::{App, Global};

use crate::app_state;

/// A language the interface ships.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Language {
    /// Its tag, the way rust-i18n names the files: `app.zh-CN.json`.
    pub tag: &'static str,
    /// The key of its name in the message files. Each language names itself, so
    /// a user who cannot read the current one can still find theirs.
    pub name: &'static str,
}

/// English. `main` registers it as rust-i18n's fallback, so a message missing
/// from another language still shows, and it is what a system that reports
/// nothing GitWave ships gets.
pub const ENGLISH: Language = Language {
    tag: "en",
    name: "common.language.english",
};

/// The one Chinese language GitWave ships; every Chinese variant of the system
/// maps onto it.
pub const SIMPLIFIED_CHINESE: Language = Language {
    tag: "zh-CN",
    name: "common.language.chinese",
};

/// The languages the interface ships, the fallback first.
pub const LANGUAGES: [Language; 2] = [ENGLISH, SIMPLIFIED_CHINESE];

/// The locale of [`ENGLISH`]. `main` names it a second time, as the literal of
/// its `i18n!` call; a test keeps the two from drifting apart.
pub const FALLBACK: &str = ENGLISH.tag;

/// What the user asked for: the system's language, or a fixed one.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LanguageChoice {
    /// The shipped language closest to the first one the system reports.
    System,
    /// One of `LANGUAGES`. A reference to it, so that there is no such thing
    /// as the fixed choice of a language GitWave does not ship.
    Fixed(&'static Language),
}

impl LanguageChoice {
    /// The fixed choice for a stored tag when it names a shipped language. Tags
    /// compare without regard to case, the way language tags do.
    #[must_use]
    pub fn showing(tag: &str) -> Option<Self> {
        LANGUAGES
            .iter()
            .find(|language| language.tag.eq_ignore_ascii_case(tag))
            .map(Self::Fixed)
    }

    /// What gitwave-core stores for the choice.
    fn settings(self) -> LanguageSettings {
        LanguageSettings {
            locale: match self {
                Self::System => None,
                Self::Fixed(language) => Some(language.tag.to_string()),
            },
        }
    }

    /// The locale the choice puts in force.
    fn locale(self, system_languages: &[String]) -> &'static str {
        match self {
            Self::System => language_for(system_languages),
            Self::Fixed(language) => language.tag,
        }
    }
}

/// The user's language choice, and the languages of the system it is resolved
/// against.
pub(crate) struct InterfaceLanguage {
    choice: LanguageChoice,
    /// The languages the system reports, most preferred first, as read when the
    /// application started.
    system_languages: Vec<String>,
}

impl Global for InterfaceLanguage {}

/// The choices on offer, in the order every list shows them: following the
/// system first, then each shipped language.
pub fn choices() -> impl Iterator<Item = LanguageChoice> {
    std::iter::once(LanguageChoice::System).chain(LANGUAGES.iter().map(LanguageChoice::Fixed))
}

/// The languages the system reports, most preferred first.
#[must_use]
pub fn system_languages() -> Vec<String> {
    sys_locale::get_locales().collect()
}

/// Maps the languages the system reports, in order of preference, onto one
/// GitWave ships: every Chinese variant onto the single simplified-Chinese
/// language, English onto English, and anything else onto the fallback.
#[must_use]
pub fn language_for(system_languages: &[String]) -> &'static str {
    for language in system_languages {
        // Only the language part decides: `en-GB` is English, `zh-Hant-TW` is Chinese.
        let primary = language
            .split(['-', '_'])
            .next()
            .unwrap_or_default()
            .to_lowercase();
        if primary == "zh" {
            return SIMPLIFIED_CHINESE.tag;
        }
        if primary == "en" {
            return FALLBACK;
        }
    }
    FALLBACK
}

/// The choice the interface starts with: the stored one when it names a shipped
/// language, otherwise the system's.
#[must_use]
pub fn initial_choice(stored: Option<&str>) -> LanguageChoice {
    stored
        .and_then(LanguageChoice::showing)
        .unwrap_or(LanguageChoice::System)
}

/// Reads the stored choice and starts the interface in that language. Call it
/// once, before the first window opens and before anything builds a menu. It
/// reads synchronously: a window that showed in the wrong language first would
/// be worse than one small read.
pub fn init(context: &AppContext, system_languages: Vec<String>, cx: &mut App) {
    let stored = get_language_settings(context).unwrap_or_else(|error| {
        tracing::warn!("the stored language could not be read, following the system: {error}");
        LanguageSettings::default()
    });
    let choice = initial_choice(stored.locale.as_deref());
    rust_i18n::set_locale(choice.locale(&system_languages));
    cx.set_global(InterfaceLanguage {
        choice,
        system_languages,
    });
}

/// The choice behind the language in force.
#[must_use]
pub fn choice(cx: &App) -> LanguageChoice {
    cx.global::<InterfaceLanguage>().choice
}

/// Switches the interface to `choice` and stores it through gitwave-core, so
/// that the next start begins in the same language. Views and menus observe the
/// language and update themselves.
///
/// The store runs on the main thread and the interface waits for it. It is one
/// small write to the local database, far too short to be worth a background
/// task, and doing it here is also what keeps two quick switches from racing.
///
/// Choosing the language already in force, which is what picking the checked
/// menu item does, changes nothing: nothing is stored and nothing is redrawn.
pub fn switch_and_store(choice: LanguageChoice, cx: &mut App) {
    if cx.global::<InterfaceLanguage>().choice == choice {
        return;
    }
    {
        // Going through `global_mut` is what tells the observers that the language changed.
        let state = cx.global_mut::<InterfaceLanguage>();
        rust_i18n::set_locale(choice.locale(&state.system_languages));
        state.choice = choice;
    }
    // A failed store only means that the next start falls back to what was
    // stored before; the choice is showing already.
    if let Err(error) = set_language_settings(&app_state::context(cx), choice.settings()) {
        tracing::warn!("the language choice could not be stored: {error}");
    }
}

/// rust-i18n keeps the language for the whole process, so the tests that depend
/// on it take turns.
#[cfg(test)]
static LANGUAGE_LOCK: Mutex<()> = Mutex::new(());

/// The turn, held by the application of a test for as long as it exists. The
/// guard is never read: holding it is the whole point.
#[cfg(test)]
struct LanguageTurn {
    _turn: MutexGuard<'static, ()>,
}

#[cfg(test)]
impl Global for LanguageTurn {}

/// Takes the turn for the rest of a test that renders a window. The lock is
/// held until the application of the test is dropped, which is later than the
/// end of its body: the framework draws a frame or two after the body returns,
/// and a dialog that is still open reads the language then.
#[cfg(test)]
pub(crate) fn take_language_turn(cx: &mut TestAppContext) {
    let turn = lock_language();
    cx.update(|cx| cx.set_global(LanguageTurn { _turn: turn }));
}

/// Takes the turn for the rest of a test that reads the language without a
/// window: hold the guard until the body of the test ends.
#[cfg(test)]
pub(crate) fn lock_language() -> MutexGuard<'static, ()> {
    LANGUAGE_LOCK.lock().unwrap_or_else(PoisonError::into_inner)
}

/// Whether some test holds the turn. For checks that the lock is where it has
/// to be.
#[cfg(test)]
pub(crate) fn language_is_locked() -> bool {
    LANGUAGE_LOCK.try_lock().is_err()
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::collections::BTreeSet;
    use std::fs;
    use std::path::{Path, PathBuf};
    use std::rc::Rc;

    use gpui_kit::TestAppContext;
    use serde_json::Value;

    use super::{
        ENGLISH, FALLBACK, LANGUAGES, SIMPLIFIED_CHINESE, choice, init, initial_choice,
        language_for, lock_language, switch_and_store, take_language_turn,
    };
    use crate::app_state;
    use gitwave_core::application::use_cases::{get_language_settings, set_language_settings};
    use gitwave_core::domain::LanguageSettings;

    use super::LanguageChoice;

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

    fn languages(list: &[&str]) -> Vec<String> {
        list.iter().map(ToString::to_string).collect()
    }

    #[test]
    fn the_languages_on_disk_are_exactly_the_shipped_ones() {
        let on_disk: BTreeSet<String> = message_files().iter().map(|path| split(path).1).collect();
        assert_eq!(
            on_disk,
            LANGUAGES
                .iter()
                .map(|language| language.tag.to_string())
                .collect(),
            "a message file names a language that is not in LANGUAGES, or the other way round"
        );
    }

    #[test]
    fn every_language_ships_the_same_domains_with_the_same_messages() {
        let english = domains(FALLBACK);
        for language in LANGUAGES {
            let other = domains(language.tag);
            assert_eq!(
                other.iter().map(|(domain, _)| domain).collect::<Vec<_>>(),
                english.iter().map(|(domain, _)| domain).collect::<Vec<_>>(),
                "{} ships different domains",
                language.tag
            );
            for ((_, en_tree), (_, tree)) in english.iter().zip(&other) {
                assert_eq!(
                    messages(tree),
                    messages(en_tree),
                    "{} ships different messages",
                    language.tag
                );
            }
        }
    }

    #[test]
    fn every_message_uses_the_same_placeholders_in_every_language() {
        let english = domains(FALLBACK);
        for language in LANGUAGES.iter().skip(1) {
            for ((_, en_tree), (_, tree)) in english.iter().zip(domains(language.tag)) {
                for path in messages(en_tree) {
                    assert_eq!(
                        placeholders(message(&tree, &path)),
                        placeholders(message(en_tree, &path)),
                        "{}/{path} uses different placeholders",
                        language.tag
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
        let _turn = lock_language();
        for language in LANGUAGES {
            rust_i18n::set_locale(language.tag);
            for (domain, tree) in domains(language.tag) {
                for path in messages(&tree) {
                    assert_eq!(
                        rust_i18n::t!(&path),
                        message(&tree, &path),
                        "{}/{path} does not load the way {domain}.{}.json spells it",
                        language.tag,
                        language.tag
                    );
                }
            }
            // The name of every language has to be in every catalogue.
            for named in LANGUAGES {
                assert_ne!(
                    rust_i18n::t!(named.name),
                    named.name,
                    "{}/{} has no name",
                    language.tag,
                    named.tag
                );
            }
        }
        rust_i18n::set_locale(FALLBACK);
    }

    #[test]
    fn every_error_code_has_a_message_and_every_error_message_matches_a_code() {
        let codes = error_codes();
        assert!(!codes.is_empty(), "no error codes found");
        for language in LANGUAGES {
            let errors = errors(language.tag);
            for code in &codes {
                assert!(
                    node(&errors, code).is_some_and(Value::is_string),
                    "{}: errors.{code} has no message",
                    language.tag
                );
            }
            for path in messages(&errors) {
                assert!(
                    codes.contains(&path),
                    "{}: errors.{path} matches no error code",
                    language.tag
                );
            }
        }
    }

    #[test]
    fn the_languages_the_rules_name_are_shipped() {
        let tags: Vec<&str> = LANGUAGES.iter().map(|language| language.tag).collect();
        assert!(
            tags.contains(&FALLBACK),
            "the fallback has to be a shipped language"
        );
        assert!(
            tags.contains(&SIMPLIFIED_CHINESE.tag),
            "the Chinese every Chinese variant maps onto has to be shipped"
        );
        assert_eq!(tags.first(), Some(&FALLBACK), "English is the fallback");
        assert!(
            include_str!("main.rs").contains(&format!("fallback = \"{FALLBACK}\"")),
            "main registers another fallback than FALLBACK"
        );
    }

    #[test]
    fn system_languages_map_to_the_closest_shipped_language() {
        let system = |list: &[&str]| language_for(&languages(list));

        assert_eq!(system(&["zh-TW", "en"]), SIMPLIFIED_CHINESE.tag);
        assert_eq!(system(&["fr", "zh-Hans-CN"]), SIMPLIFIED_CHINESE.tag);
        assert_eq!(
            system(&["zh_CN"]),
            SIMPLIFIED_CHINESE.tag,
            "an underscore is not a hyphen, but still Chinese"
        );
        // Only the first language the user prefers decides.
        assert_eq!(system(&["en-GB", "zh-Hans-CN"]), FALLBACK);
        assert_eq!(system(&["fr", "en-US"]), FALLBACK);
        assert_eq!(system(&["fr"]), FALLBACK);
        assert_eq!(system(&[]), FALLBACK);
    }

    #[test]
    fn a_stored_choice_wins_when_it_names_a_shipped_language() {
        assert_eq!(
            initial_choice(Some("zh-CN")),
            LanguageChoice::Fixed(&SIMPLIFIED_CHINESE)
        );
        assert_eq!(
            initial_choice(Some("ZH-cn")),
            LanguageChoice::Fixed(&SIMPLIFIED_CHINESE),
            "language tags do not care about case"
        );
        assert_eq!(
            initial_choice(Some("ja")),
            LanguageChoice::System,
            "a language GitWave no longer ships falls back to the system"
        );
        assert_eq!(initial_choice(Some("")), LanguageChoice::System);
        assert_eq!(initial_choice(None), LanguageChoice::System);
    }

    /// What gitwave-core has stored.
    fn stored(cx: &mut TestAppContext) -> Option<String> {
        cx.read(|cx| {
            get_language_settings(&app_state::context(cx))
                .expect("read the stored language")
                .locale
        })
    }

    /// Starts in the language of the stored choice, or of the system, and
    /// switching changes the language in force, tells the observers and has
    /// stored the choice by the time it returns.
    #[gpui_kit::test]
    fn starting_and_switching_follow_the_stored_choice_and_the_system(cx: &mut TestAppContext) {
        take_language_turn(cx);
        assert!(super::language_is_locked());
        let started = Rc::new(Cell::new(0));

        // Nothing stored: the system decides.
        cx.update(|cx| {
            gpui_kit::init(cx);
            app_state::init(cx);
            init(
                &app_state::context(cx),
                languages(&["zh-Hans-CN", "en-GB"]),
                cx,
            );
            let started = started.clone();
            cx.observe_global::<super::InterfaceLanguage>(move |_| {
                started.set(started.get() + 1);
            })
            .detach();
        });
        assert_eq!(&*rust_i18n::locale(), SIMPLIFIED_CHINESE.tag);
        assert_eq!(cx.read(choice), LanguageChoice::System);

        cx.update(|cx| switch_and_store(LanguageChoice::Fixed(&ENGLISH), cx));
        assert_eq!(&*rust_i18n::locale(), FALLBACK);
        assert_eq!(cx.read(choice), LanguageChoice::Fixed(&ENGLISH));
        assert_eq!(started.get(), 1, "the observers hear about a switch");
        assert_eq!(
            stored(cx).as_deref(),
            Some(FALLBACK),
            "the choice is stored by the time the switch returns"
        );

        // Choosing the language in force again changes nothing.
        cx.update(|cx| switch_and_store(LanguageChoice::Fixed(&ENGLISH), cx));
        assert_eq!(
            started.get(),
            1,
            "the observers hear nothing the second time"
        );

        // Back to the system, which reports Chinese first.
        cx.update(|cx| switch_and_store(LanguageChoice::System, cx));
        assert_eq!(&*rust_i18n::locale(), SIMPLIFIED_CHINESE.tag);
        assert_eq!(started.get(), 2);
        assert_eq!(
            stored(cx),
            None,
            "following the system is stored as no choice"
        );

        // Two switches in a row leave the second one stored: nothing runs in
        // the background that could finish out of order.
        cx.update(|cx| {
            switch_and_store(LanguageChoice::Fixed(&ENGLISH), cx);
            switch_and_store(LanguageChoice::Fixed(&SIMPLIFIED_CHINESE), cx);
        });
        assert_eq!(stored(cx).as_deref(), Some(SIMPLIFIED_CHINESE.tag));

        // What a later start finds stored is what it shows: store a choice
        // behind the application's back, then start again.
        cx.update(|cx| {
            set_language_settings(
                &app_state::context(cx),
                LanguageSettings {
                    locale: Some(SIMPLIFIED_CHINESE.tag.to_string()),
                },
            )
            .expect("store a choice");
            init(&app_state::context(cx), languages(&["en-GB"]), cx);
        });
        assert_eq!(cx.read(choice), LanguageChoice::Fixed(&SIMPLIFIED_CHINESE));
        assert_eq!(&*rust_i18n::locale(), SIMPLIFIED_CHINESE.tag);
        rust_i18n::set_locale(FALLBACK);
    }
}
