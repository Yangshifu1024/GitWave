//! Stable, user-visible error codes (F010).
//!
//! One constant per error surface, grouped by source area so ownership stays
//! clear. Values are dot.case strings (`"workspace.name_empty"`) that mirror
//! the `errors.*` subtrees in `src/i18n/locales/{en,zh-CN}/errors-*.json`;
//! the frontend translates them and falls back to the English `message`
//! field when a code is missing. Reference as `codes::<area>::<CONST>`.

pub mod cmds;
pub mod git;
pub mod infra;
pub mod usecases;
