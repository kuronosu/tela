# ADR-239: `uxtheme.dll` for dark menus on Windows

- **Date:** 2026-11-07
- **Status:** Accepted (amends §2, ADR-234)

## Decision

The owner allowed `uxtheme.dll` on Windows (§2's DLL list), for dark popup menus only: the drop-down menus of a native menu bar, tray menus and context menus follow the system's dark mode as Explorer's do. The calls are uxtheme's unnamed exports used by Explorer, Notepad++ and win32-darkmode (by ordinal: `SetPreferredAppMode` 135, `AllowDarkModeForWindow` 133, `FlushMenuThemes` 136, `RefreshImmersiveColorPolicyState` 104), looked up at run time and skipped when missing or when the build is unknown to have them (before Windows 10 1903), so menus stay light there. The menu bar itself stays drawn by ADR-234's `WM_UAHDRAWMENU` code.

## Why

ADR-234 left the drop-down menus light under a dark bar; there is no documented API for dark popup menus, and owner-drawing them would lose access keys, screen reader text and system metrics.
