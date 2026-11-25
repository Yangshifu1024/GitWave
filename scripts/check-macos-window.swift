import CoreGraphics
import Foundation

// Window geometry and the owner PID are available without screen recording permission.
guard CommandLine.arguments.count == 2,
      let pid = Int32(CommandLine.arguments[1]),
      let windows = CGWindowListCopyWindowInfo([.optionOnScreenOnly, .excludeDesktopElements], kCGNullWindowID) as? [[String: Any]] else {
    exit(1)
}
let hasWindow = windows.contains { window in
    guard window[kCGWindowOwnerPID as String] as? Int32 == pid,
          window[kCGWindowLayer as String] as? Int == 0,
          let bounds = window[kCGWindowBounds as String] as? [String: CGFloat] else {
        return false
    }
    return (bounds["Width"] ?? 0) >= 160 && (bounds["Height"] ?? 0) >= 90
}
exit(hasWindow ? 0 : 1)
