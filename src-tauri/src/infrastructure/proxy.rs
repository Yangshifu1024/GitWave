//! System proxy detection and the process env bridge (F013).
//!
//! One mechanism covers all four outbound paths — AI requests (reqwest
//! singleton), git fetch/push/clone (libgit2), LFS + credential helper
//! subprocesses, and update checks (updater plugin's reqwest): resolve the
//! configured proxy (manual URL or OS system proxy) into `HTTP_PROXY` /
//! `HTTPS_PROXY` / `NO_PROXY` and set them on the process environment.
//! reqwest reads env at client build time, libgit2 reads env per operation
//! (keeping per-host `no_proxy` granularity, which `ProxyOptions::url()`
//! would lose), and spawned git subprocesses inherit the environment.
//!
//! Precedence: manual in-app setting > pre-existing env vars (user intent
//! from the launch environment — only filled in, never overwritten in
//! `System` mode) > detected system proxy.
//!
//! `NO_PROXY` always contains the loopback hosts so local Ollama / LAN
//! services are never proxied, no matter what the OS bypass list says.

use std::sync::Mutex;

use crate::domain::app_settings::{ProxyMode, ProxySettings};

pub const ENV_HTTP_PROXY: &str = "HTTP_PROXY";
pub const ENV_HTTPS_PROXY: &str = "HTTPS_PROXY";
pub const ENV_NO_PROXY: &str = "NO_PROXY";

const LOOPBACK_HOSTS: [&str; 3] = ["localhost", "127.0.0.1", "::1"];

/// System proxy as reported by the OS (Windows Internet Settings / macOS
/// system network proxies). URLs carry an explicit scheme so both reqwest
/// and libgit2 pick the right transport.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SystemProxy {
    /// Proxy for plain-HTTP targets.
    pub http: Option<String>,
    /// Proxy for HTTPS targets (normally the same HTTP proxy, used via CONNECT).
    pub https: Option<String>,
    /// Hosts that bypass the proxy, normalized for `NO_PROXY` semantics.
    pub bypass: Vec<String>,
}

/// The proxy triplet to inject into the environment.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ResolvedProxy {
    pub http: Option<String>,
    pub https: Option<String>,
    pub no_proxy: String,
}

/// One injected env var: name plus the pre-injection value (`None` = the
/// var did not exist), so leaving the proxy modes restores the launch
/// environment exactly instead of leaving ours behind or deleting a
/// user-set value.
type InjectedVar = (&'static str, Option<std::ffi::OsString>);

static INJECTED_VARS: Mutex<Vec<InjectedVar>> = Mutex::new(Vec::new());

/// Environment mutation surface. The real process env in production; an
/// in-memory fake in tests — the env is process-global and tests run in
/// parallel threads, so they must never touch it.
trait EnvAccess {
    fn get(&self, name: &str) -> Option<std::ffi::OsString>;
    fn set(&mut self, name: &'static str, value: &str);
    /// Put back the pre-injection value, removing the var if there was none.
    fn restore(&mut self, name: &'static str, original: Option<std::ffi::OsString>);
}

struct ProcessEnv;

impl EnvAccess for ProcessEnv {
    fn get(&self, name: &str) -> Option<std::ffi::OsString> {
        std::env::var_os(name)
    }

    fn set(&mut self, name: &'static str, value: &str) {
        std::env::set_var(name, value);
    }

    fn restore(&mut self, name: &'static str, original: Option<std::ffi::OsString>) {
        match original {
            Some(v) => std::env::set_var(name, v),
            None => std::env::remove_var(name),
        }
    }
}

/// Detect the OS-level proxy. `None` when no usable proxy is configured —
/// in particular Linux, where env vars *are* the system convention and
/// already apply without bridging.
#[cfg(windows)]
pub fn detect_system_proxy() -> Option<SystemProxy> {
    let settings = windows_registry::CURRENT_USER
        .open("Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings")
        .ok()?;
    // ProxyEnable == 0 means the "system proxy" switch is off, even when
    // stale ProxyServer entries remain.
    if settings.get_u32("ProxyEnable").unwrap_or(0) == 0 {
        return None;
    }
    let server = settings.get_string("ProxyServer").ok()?;
    let (http, https) = parse_proxy_server(&server);
    let bypass = settings
        .get_string("ProxyOverride")
        .map(|raw| parse_bypass_entries(&raw))
        .unwrap_or_default();
    Some(SystemProxy {
        http,
        https,
        bypass,
    })
}

#[cfg(target_os = "macos")]
pub fn detect_system_proxy() -> Option<SystemProxy> {
    use system_configuration::core_foundation::base::CFType;
    use system_configuration::core_foundation::dictionary::CFDictionary;
    use system_configuration::core_foundation::number::CFNumber;
    use system_configuration::core_foundation::string::{CFString, CFStringRef};
    use system_configuration::dynamic_store::SCDynamicStoreBuilder;
    use system_configuration::sys::schema_definitions::{
        kSCPropNetProxiesHTTPEnable, kSCPropNetProxiesHTTPPort, kSCPropNetProxiesHTTPProxy,
        kSCPropNetProxiesHTTPSEnable, kSCPropNetProxiesHTTPSPort, kSCPropNetProxiesHTTPSProxy,
    };

    fn entry(
        proxies: &CFDictionary<CFString, CFType>,
        enabled_key: CFStringRef,
        host_key: CFStringRef,
        port_key: CFStringRef,
    ) -> Option<String> {
        let enabled = proxies
            .find(enabled_key)
            .and_then(|v| v.downcast::<CFNumber>())
            .and_then(|v| v.to_i32())
            .unwrap_or(0)
            == 1;
        if !enabled {
            return None;
        }
        let host = proxies
            .find(host_key)
            .and_then(|v| v.downcast::<CFString>())
            .map(|v| v.to_string())?;
        let port = proxies
            .find(port_key)
            .and_then(|v| v.downcast::<CFNumber>())
            .and_then(|v| v.to_i32());
        Some(match port {
            Some(port) => format!("http://{host}:{port}"),
            None => format!("http://{host}"),
        })
    }

    let store = SCDynamicStoreBuilder::new("GitWave-proxy").build()?;
    let proxies = store.get_proxies()?;
    Some(SystemProxy {
        http: entry(
            &proxies,
            unsafe { kSCPropNetProxiesHTTPEnable },
            unsafe { kSCPropNetProxiesHTTPProxy },
            unsafe { kSCPropNetProxiesHTTPPort },
        ),
        https: entry(
            &proxies,
            unsafe { kSCPropNetProxiesHTTPSEnable },
            unsafe { kSCPropNetProxiesHTTPSProxy },
            unsafe { kSCPropNetProxiesHTTPSPort },
        ),
        // macOS "exceptions list" is skipped: CFArray downcasting for a
        // best-effort nicety is not worth the unsafe surface; loopback is
        // always injected anyway.
        bypass: Vec::new(),
    })
    .filter(|p| p.http.is_some() || p.https.is_some())
}

#[cfg(all(unix, not(target_os = "macos")))]
pub fn detect_system_proxy() -> Option<SystemProxy> {
    None
}

/// Normalize a manual proxy URL: default the scheme to `http://`, require a
/// host, allow http/https proxies only (SOCKS is not supported by libgit2,
/// so a manual SOCKS URL would silently break git operations).
pub fn normalize_manual_url(raw: &str) -> Option<String> {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return None;
    }
    let with_scheme = if trimmed.contains("://") {
        trimmed.to_string()
    } else {
        format!("http://{trimmed}")
    };
    let url = reqwest::Url::parse(&with_scheme).ok()?;
    if url.host_str()?.is_empty() {
        return None;
    }
    match url.scheme() {
        "http" | "https" => {}
        _ => return None,
    }
    let normalized = url.to_string();
    Some(normalized.trim_end_matches('/').to_string())
}

/// Map the app proxy settings to the env triplet. `None` = no proxy from
/// the app (mode Off, or nothing detectable/configured).
pub fn resolve(settings: &ProxySettings) -> Option<ResolvedProxy> {
    match settings.mode {
        ProxyMode::Off => None,
        ProxyMode::Manual => {
            let url = normalize_manual_url(settings.manual_url.as_deref()?)?;
            Some(ResolvedProxy {
                http: Some(url.clone()),
                https: Some(url),
                no_proxy: build_no_proxy(&[]),
            })
        }
        ProxyMode::System => {
            let sys = detect_system_proxy()?;
            Some(ResolvedProxy {
                http: sys.http,
                https: sys.https,
                no_proxy: build_no_proxy(&sys.bypass),
            })
        }
    }
}

/// Apply `settings` to the process environment. Called once at startup and
/// again whenever the user saves proxy settings (changes take effect
/// immediately: libgit2 reads env per operation and the AI client is
/// rebuilt; reqwest reads env when a client is constructed).
///
/// Startup runs single-threaded before the Tauri builder. Runtime saves
/// mutate the environment from a command thread — the same pattern dotenv
/// loaders use; libgit2/reqwest read the env from other unsynchronized
/// threads, a theoretical POSIX race accepted here because saves are rare
/// user actions.
pub fn apply_to_env(settings: &ProxySettings) {
    let mut injected = INJECTED_VARS.lock().expect("proxy env lock");
    let resolved = resolve(settings);
    let force = matches!(settings.mode, ProxyMode::Manual);
    let mut env = ProcessEnv;
    bridge_apply(&mut injected, resolved.as_ref(), force, &mut env);
    match resolved.as_ref() {
        None => tracing::info!(
            mode = ?settings.mode,
            "proxy bridge: no proxy, injected env vars restored"
        ),
        Some(rp) => tracing::info!(
            mode = ?settings.mode,
            http = rp.http.is_some(),
            https = rp.https.is_some(),
            "proxy bridge applied"
        ),
    }
}

/// The mode-switching state machine, parameterized over env access:
///
/// - `force` (Manual) is an explicit in-app choice: it overwrites anything,
///   including values the launch environment provided.
/// - otherwise (System) only blanks are filled — but values WE injected
///   earlier count as ours and are replaced, so a stale manual proxy never
///   outlives its mode.
/// - a cleared resolution (Off / nothing configured) restores the recorded
///   pre-injection values; user-set variables come back untouched.
fn bridge_apply(
    injected: &mut Vec<InjectedVar>,
    resolved: Option<&ResolvedProxy>,
    force: bool,
    env: &mut impl EnvAccess,
) {
    match resolved {
        None => {
            for (name, original) in injected.drain(..) {
                env.restore(name, original);
            }
        }
        Some(rp) => {
            let any_proxy = set_injected(injected, env, ENV_HTTP_PROXY, rp.http.as_deref(), force)
                | set_injected(injected, env, ENV_HTTPS_PROXY, rp.https.as_deref(), force);
            // NO_PROXY only matters when a proxy is in play — and it would
            // otherwise clobber semantics of an env proxy we did not set.
            if any_proxy || force {
                set_injected(injected, env, ENV_NO_PROXY, Some(&rp.no_proxy), force);
            }
        }
    }
}

/// Set one env var, recording the pre-injection value. Returns whether it
/// was set.
fn set_injected(
    injected: &mut Vec<InjectedVar>,
    env: &mut impl EnvAccess,
    name: &'static str,
    value: Option<&str>,
    force: bool,
) -> bool {
    let Some(value) = value else {
        return false;
    };
    let ours = injected.iter().any(|(n, _)| *n == name);
    if !force && !ours && env.get(name).is_some_and(|v| !v.is_empty()) {
        return false;
    }
    if !ours {
        injected.push((name, env.get(name)));
    }
    env.set(name, value);
    true
}

/// Windows `ProxyServer` → (http, https) proxy URLs.
///
/// Two registry formats: bare `host:port` (applies to every protocol) and
/// per-scheme `http=a:80;https=b:443;socks=c:1080`. SOCKS-only entries
/// resolve to no proxy here — libgit2 has no SOCKS transport, and routing
/// only the AI traffic through it would surprise more than it helps.
fn parse_proxy_server(raw: &str) -> (Option<String>, Option<String>) {
    let raw = raw.trim();
    if raw.is_empty() {
        return (None, None);
    }
    if !raw.contains('=') {
        let url = with_proxy_scheme(raw, "http");
        return (url.clone(), url);
    }
    let mut http = None;
    let mut https = None;
    for part in raw.split(';') {
        let Some((scheme, value)) = part.split_once('=') else {
            continue;
        };
        let (scheme, value) = (scheme.trim().to_ascii_lowercase(), value.trim());
        if value.is_empty() {
            continue;
        }
        match scheme.as_str() {
            "http" => http = with_proxy_scheme(value, "http"),
            "https" | "secure" => https = with_proxy_scheme(value, "http"),
            // socks entries (and anything unknown) fall through unused.
            _ => {}
        }
    }
    (http, https)
}

/// A bare `host:port` registry entry means an HTTP proxy reached over plain
/// HTTP (HTTPS targets go through it via CONNECT) — only keep an explicit
/// scheme when it is a usable one (http/https; SOCKS would fail reqwest's
/// client build since the `socks` feature is off, so drop it).
fn with_proxy_scheme(entry: &str, default_scheme: &str) -> Option<String> {
    if let Some((scheme, _)) = entry.split_once("://") {
        match scheme.to_ascii_lowercase().as_str() {
            "http" | "https" => Some(entry.to_string()),
            _ => None,
        }
    } else {
        Some(format!("{default_scheme}://{entry}"))
    }
}

/// Windows `ProxyOverride` / generic bypass list → `NO_PROXY` entries.
///
/// `<local>` has no `NO_PROXY` equivalent and IP wildcards (`127.*`) match
/// nothing in either hyper-util or libgit2, so they are dropped — loopback
/// is always injected by [`build_no_proxy`] anyway.
fn parse_bypass_entries(raw: &str) -> Vec<String> {
    let mut out: Vec<String> = Vec::new();
    for entry in raw.split([';', ',']) {
        let entry = entry.trim();
        if entry.is_empty() || entry.eq_ignore_ascii_case("<local>") {
            continue;
        }
        let entry = entry
            .strip_prefix("*.")
            .or_else(|| entry.strip_prefix('.'))
            .unwrap_or(entry);
        if !out.iter().any(|e: &String| e.eq_ignore_ascii_case(entry)) {
            out.push(entry.to_string());
        }
    }
    out
}

/// `NO_PROXY` value: the bypass list plus loopback hosts (deduped,
/// case-insensitive) so local services are never proxied.
fn build_no_proxy(bypass: &[String]) -> String {
    let mut out: Vec<String> = LOOPBACK_HOSTS.iter().map(|h| (*h).to_string()).collect();
    for entry in bypass {
        if !out.iter().any(|e| e.eq_ignore_ascii_case(entry)) {
            out.push(entry.clone());
        }
    }
    out.join(",")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_proxy_server_bare_host_applies_to_all_schemes() {
        let (http, https) = parse_proxy_server("127.0.0.1:7890");
        assert_eq!(http.as_deref(), Some("http://127.0.0.1:7890"));
        assert_eq!(https.as_deref(), Some("http://127.0.0.1:7890"));
    }

    #[test]
    fn parse_proxy_server_per_scheme_form() {
        let (http, https) = parse_proxy_server("http=10.0.0.1:8080;https=10.0.0.2:8443");
        assert_eq!(http.as_deref(), Some("http://10.0.0.1:8080"));
        assert_eq!(https.as_deref(), Some("http://10.0.0.2:8443"));
    }

    #[test]
    fn parse_proxy_server_socks_only_is_not_usable() {
        let (http, https) = parse_proxy_server("socks=127.0.0.1:1080");
        assert_eq!(http, None);
        assert_eq!(https, None);
        // Bare-form entries carrying a socks scheme directly.
        let (http, https) = parse_proxy_server("socks5://127.0.0.1:1080");
        assert_eq!(http, None);
        assert_eq!(https, None);
    }

    #[test]
    fn parse_proxy_server_blank_and_garbage() {
        assert_eq!(parse_proxy_server(""), (None, None));
        assert_eq!(parse_proxy_server("   "), (None, None));
        let (http, https) = parse_proxy_server("http=");
        assert_eq!(http, None);
        assert_eq!(https, None);
    }

    #[test]
    fn parse_proxy_server_explicit_scheme_is_kept() {
        let (http, _) = parse_proxy_server("https://secure.proxy:8443");
        assert_eq!(http.as_deref(), Some("https://secure.proxy:8443"));
    }

    #[test]
    fn bypass_entries_drop_local_marker_and_wildcard_prefix() {
        let entries = parse_bypass_entries("localhost;<local>;*.corp.example.com;foo.com;foo.COM");
        assert_eq!(entries, vec!["localhost", "corp.example.com", "foo.com"]);
    }

    #[test]
    fn no_proxy_always_contains_loopback_once() {
        let value = build_no_proxy(&parse_bypass_entries(
            "localhost;127.0.0.1;*.corp.example.com",
        ));
        let parts: Vec<&str> = value.split(',').collect();
        assert_eq!(
            parts,
            vec!["localhost", "127.0.0.1", "::1", "corp.example.com"]
        );
        assert_eq!(build_no_proxy(&[]), "localhost,127.0.0.1,::1");
    }

    #[test]
    fn manual_url_defaults_scheme_and_requires_host() {
        assert_eq!(
            normalize_manual_url("127.0.0.1:7890").as_deref(),
            Some("http://127.0.0.1:7890")
        );
        assert_eq!(
            normalize_manual_url(" http://proxy.lan:8080/ ").as_deref(),
            Some("http://proxy.lan:8080")
        );
        assert_eq!(normalize_manual_url(""), None);
        assert_eq!(normalize_manual_url("   "), None);
        assert_eq!(normalize_manual_url("http://"), None);
        assert_eq!(normalize_manual_url("ftp://proxy:21"), None);
    }

    #[test]
    fn resolve_by_mode() {
        // Off always wins, even with a manual URL configured.
        let off = ProxySettings {
            mode: ProxyMode::Off,
            manual_url: Some("http://127.0.0.1:7890".to_string()),
        };
        assert_eq!(resolve(&off), None);

        // Manual mode uses the URL for both schemes.
        let manual = ProxySettings {
            mode: ProxyMode::Manual,
            manual_url: Some("127.0.0.1:7890".to_string()),
        };
        let resolved = resolve(&manual).expect("manual resolves");
        assert_eq!(resolved.http.as_deref(), Some("http://127.0.0.1:7890"));
        assert_eq!(resolved.https, resolved.http);
        assert_eq!(resolved.no_proxy, "localhost,127.0.0.1,::1");

        // System mode follows whatever the OS reports on this machine
        // (environment-dependent by design): None together with the
        // detector, and loopback-exempt no_proxy when detected.
        let resolved = resolve(&ProxySettings::default());
        match detect_system_proxy() {
            None => assert_eq!(resolved, None),
            Some(sys) => {
                let resolved = resolved.expect("detector found a proxy but resolve missed it");
                assert_eq!(resolved.http, sys.http);
                assert_eq!(resolved.https, sys.https);
                assert!(resolved.no_proxy.starts_with("localhost,127.0.0.1,::1"));
            }
        }
    }

    // ─── bridge_apply state machine (no real env access) ────────────────────

    #[derive(Default)]
    struct FakeEnv {
        vars: std::collections::HashMap<&'static str, std::ffi::OsString>,
    }

    impl EnvAccess for FakeEnv {
        fn get(&self, name: &str) -> Option<std::ffi::OsString> {
            self.vars.get(name).cloned()
        }

        fn set(&mut self, name: &'static str, value: &str) {
            self.vars.insert(name, value.into());
        }

        fn restore(&mut self, name: &'static str, original: Option<std::ffi::OsString>) {
            match original {
                Some(v) => {
                    self.vars.insert(name, v);
                }
                None => {
                    self.vars.remove(name);
                }
            }
        }
    }

    fn resolved_fixed(url: &str) -> ResolvedProxy {
        ResolvedProxy {
            http: Some(url.to_string()),
            https: Some(url.to_string()),
            no_proxy: "localhost,127.0.0.1,::1".to_string(),
        }
    }

    fn env_value(env: &FakeEnv, name: &str) -> Option<String> {
        env.get(name).map(|v| v.to_string_lossy().into_owned())
    }

    #[test]
    fn bridge_manual_overrides_then_off_restores_user_env() {
        let mut env = FakeEnv::default();
        env.set(ENV_HTTP_PROXY, "http://user-env:1");
        let mut injected: Vec<InjectedVar> = Vec::new();

        // Manual (force) overwrites the user's value and records it.
        let manual = resolved_fixed("http://manual:2");
        bridge_apply(&mut injected, Some(&manual), true, &mut env);
        assert_eq!(
            env_value(&env, ENV_HTTP_PROXY).as_deref(),
            Some("http://manual:2")
        );
        assert!(injected
            .iter()
            .any(|(n, o)| *n == ENV_HTTP_PROXY && o.is_some()));

        // Off restores the recorded pre-injection value instead of deleting.
        bridge_apply(&mut injected, None, false, &mut env);
        assert_eq!(
            env_value(&env, ENV_HTTP_PROXY).as_deref(),
            Some("http://user-env:1")
        );
        assert!(injected.is_empty());
    }

    #[test]
    fn bridge_manual_to_system_replaces_stale_manual_proxy() {
        let mut env = FakeEnv::default();
        let mut injected: Vec<InjectedVar> = Vec::new();

        let manual = resolved_fixed("http://old-manual:1");
        bridge_apply(&mut injected, Some(&manual), true, &mut env);

        // Switching to System must replace the stale manual proxy — the
        // value is ours even though the env slot is occupied.
        let system = resolved_fixed("http://system:2");
        bridge_apply(&mut injected, Some(&system), false, &mut env);
        assert_eq!(
            env_value(&env, ENV_HTTP_PROXY).as_deref(),
            Some("http://system:2")
        );
        assert_eq!(
            env_value(&env, ENV_HTTPS_PROXY).as_deref(),
            Some("http://system:2")
        );
    }

    #[test]
    fn bridge_system_keeps_user_env_and_fills_blanks() {
        let mut env = FakeEnv::default();
        env.set(ENV_HTTPS_PROXY, "http://user-env:1");
        let mut injected: Vec<InjectedVar> = Vec::new();

        let system = resolved_fixed("http://system:2");
        bridge_apply(&mut injected, Some(&system), false, &mut env);
        // Blank slot filled, user value untouched, NO_PROXY set because a
        // proxy variable was set by us.
        assert_eq!(
            env_value(&env, ENV_HTTP_PROXY).as_deref(),
            Some("http://system:2")
        );
        assert_eq!(
            env_value(&env, ENV_HTTPS_PROXY).as_deref(),
            Some("http://user-env:1")
        );
        assert_eq!(
            env_value(&env, ENV_NO_PROXY).as_deref(),
            Some("localhost,127.0.0.1,::1")
        );
    }

    #[test]
    fn bridge_off_without_injection_is_a_noop() {
        let mut env = FakeEnv::default();
        env.set(ENV_HTTP_PROXY, "http://user-env:1");
        let mut injected: Vec<InjectedVar> = Vec::new();

        bridge_apply(&mut injected, None, false, &mut env);
        assert_eq!(
            env_value(&env, ENV_HTTP_PROXY).as_deref(),
            Some("http://user-env:1")
        );
        assert!(injected.is_empty());
    }
}
