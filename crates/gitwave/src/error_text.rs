//! What an error says in the interface.

use gitwave_core::domain::error::AppError;
use rust_i18n::t;

/// The message of the error's code, in the language in force, with the
/// error's details filled in. The English message when the code has none.
pub fn show(error: &AppError) -> String {
    let key = format!("errors.{}", error.code());
    let template = t!(&key);
    if template == key {
        return error.message();
    }
    let (patterns, values): (Vec<&str>, Vec<String>) = error
        .params()
        .iter()
        .map(|(name, value)| (*name, value.clone()))
        .unzip();
    rust_i18n::replace_patterns(&template, &patterns, &values)
}

#[cfg(test)]
mod tests {
    use super::*;
    use gitwave_core::domain::error_codes as codes;

    #[test]
    fn the_message_of_the_code_is_filled_in_in_the_language_in_force() {
        let error = AppError::permission_with(
            codes::infra::DATA_DIR_CREATE,
            "create /nope: full",
            &[("dir", "/nope".to_string()), ("error", "full".to_string())],
        );
        {
            let _turn = crate::locales::lock_language();
            rust_i18n::set_locale("en");
            assert_eq!(show(&error), "Could not create directory /nope: full");
        }
        {
            let _turn = crate::locales::lock_language();
            rust_i18n::set_locale("zh-CN");
            assert_eq!(show(&error), "无法创建目录 /nope：full");
        }
    }

    #[test]
    fn a_code_without_a_message_shows_the_english_one() {
        let _turn = crate::locales::lock_language();
        rust_i18n::set_locale("zh-CN");
        let error = AppError::unknown("nope.not_a_code", "the English message");

        assert_eq!(show(&error), "unknown error: the English message");
    }
}
