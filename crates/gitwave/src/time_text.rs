//! How the interface words a time: as how long ago, and as a date once that
//! says little.

use std::time::SystemTime;

use chrono::DateTime;
use rust_i18n::t;

/// After a week, a date is quicker to read than a number of days.
const A_WEEK: i64 = 7 * 24 * 60 * 60;

/// How long before `now` the Unix time `seconds` is: just now, then minutes,
/// hours and days, and a date after a week.
pub fn ago(now: SystemTime, seconds: i64) -> String {
    let now = unix_seconds(now);
    let elapsed = (now - seconds).max(0);
    if elapsed < 60 {
        t!("branches.time.justNow").to_string()
    } else if elapsed < 60 * 60 {
        t!("branches.time.minutesAgo", n = elapsed / 60).to_string()
    } else if elapsed < 24 * 60 * 60 {
        t!("branches.time.hoursAgo", n = elapsed / 3600).to_string()
    } else if elapsed < A_WEEK {
        t!("branches.time.daysAgo", n = elapsed / 86400).to_string()
    } else {
        date(seconds)
    }
}

/// The local date of a Unix time, as `2026-11-26`.
pub fn date(seconds: i64) -> String {
    DateTime::from_timestamp(seconds, 0)
        .map(|time| {
            time.with_timezone(&chrono::Local)
                .format("%Y-%m-%d")
                .to_string()
        })
        .unwrap_or_default()
}

fn unix_seconds(time: SystemTime) -> i64 {
    time.duration_since(SystemTime::UNIX_EPOCH)
        .map_or(0, |since| since.as_secs() as i64)
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use super::*;

    fn at(seconds: i64) -> SystemTime {
        SystemTime::UNIX_EPOCH + Duration::from_secs(seconds as u64)
    }

    fn english() -> impl Drop {
        let _turn = crate::locales::lock_language();
        rust_i18n::set_locale("en");
        _turn
    }

    #[test]
    fn the_words_follow_the_old_thresholds() {
        let _english = english();
        let now = at(1_000_000);
        assert_eq!(ago(now, 1_000_000), "just now");
        assert_eq!(ago(now, 1_000_000 - 59), "just now");
        assert_eq!(ago(now, 1_000_000 - 60), "1m ago");
        assert_eq!(ago(now, 1_000_000 - 3599), "59m ago");
        assert_eq!(ago(now, 1_000_000 - 3600), "1h ago");
        assert_eq!(ago(now, 1_000_000 - 86_399), "23h ago");
        assert_eq!(ago(now, 1_000_000 - 86_400), "1d ago");
        assert_eq!(ago(now, 1_000_000 - A_WEEK + 1), "6d ago");
    }

    #[test]
    fn a_week_back_is_a_date() {
        let _english = english();
        let now = at(1_800_000_000);
        assert_eq!(
            ago(now, 1_800_000_000 - A_WEEK),
            date(1_800_000_000 - A_WEEK)
        );
        assert_eq!(
            date(0).len(),
            10,
            "a date has four digits, two and two: {}",
            date(0)
        );
        assert_eq!(date(i64::MAX), "", "a time out of range has no date");
    }

    #[test]
    fn a_time_ahead_of_now_is_just_now() {
        let _english = english();
        assert_eq!(ago(at(1_000), 5_000), "just now");
    }

    #[test]
    fn the_words_follow_the_language() {
        {
            let _turn = crate::locales::lock_language();
            rust_i18n::set_locale("zh-CN");
            assert_eq!(ago(at(10_000), 10_000 - 120), "2 分钟前");
        }
        let _english = english();
        assert_eq!(ago(at(10_000), 10_000 - 120), "2m ago");
    }
}
