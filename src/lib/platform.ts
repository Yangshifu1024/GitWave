// Synchronous platform detection helpers for the WebView runtime.
//
// Use these in render code; for one-shot async checks at startup prefer
// @tauri-apps/api/os's platform().

export function isMacOS(): boolean {
  if (typeof navigator === "undefined") return false;
  const ua = navigator.userAgent;
  return /Mac/i.test(ua);
}

export function isWindows(): boolean {
  if (typeof navigator === "undefined") return false;
  const ua = navigator.userAgent;
  return /Windows/i.test(ua);
}

export function isLinux(): boolean {
  if (typeof navigator === "undefined") return false;
  const ua = navigator.userAgent;
  return /Linux/i.test(ua);
}
