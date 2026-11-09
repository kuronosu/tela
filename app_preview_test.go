package tela

import "testing"

// A Preview as the root of an App keeps its Theme and ColorScheme for Run
// and Main, as telatest and the CLI apply them (ADR-256).
func TestAppRootPreviewThemeAndScheme(t *testing.T) {
	root := C(func() Node { return nil })
	appTheme, previewTheme := DefaultTheme(), DefaultTheme()
	previewTheme.Radius = 2
	appTheme.Radius = 9
	for _, c := range []struct {
		name       string
		app        App
		wantTheme  *Theme
		wantScheme ColorScheme
	}{
		{"plain root", App{Root: root, Theme: &appTheme, ColorScheme: Dark}, &appTheme, Dark},
		{"preview without settings", App{Root: Preview{Root: root}, Theme: &appTheme, ColorScheme: Dark}, &appTheme, Dark},
		{"preview theme wins", App{Root: Preview{Root: root, Theme: &previewTheme}, Theme: &appTheme}, &previewTheme, System},
		{"preview scheme wins", App{Root: Preview{Root: root, ColorScheme: Light}, ColorScheme: Dark}, nil, Light},
		{"preview dark", App{Root: Preview{Root: root, ColorScheme: Dark}}, nil, Dark},
		{"preview theme and scheme", App{Root: Preview{Root: root, Theme: &previewTheme, ColorScheme: Dark}, Theme: &appTheme, ColorScheme: Light}, &previewTheme, Dark},
		// Only the root itself counts (ADR-263): a component that returns a
		// Preview, or a Preview inside another node, leaves the App's.
		{"preview below the root", App{Root: C(func() Node { return Preview{Root: root, Theme: &previewTheme, ColorScheme: Dark}.Build() }), Theme: &appTheme, ColorScheme: Light}, &appTheme, Light},
		{"keyed preview", App{Root: WithKey(Preview{Root: root, ColorScheme: Dark}, 1), ColorScheme: Light}, nil, Light},
	} {
		cfg := c.app.options().Config
		if (*Theme)(cfg.Theme) != c.wantTheme {
			t.Errorf("%s: theme %p, want %p", c.name, cfg.Theme, c.wantTheme)
		}
		if cfg.ColorScheme != c.wantScheme {
			t.Errorf("%s: scheme %v, want %v", c.name, cfg.ColorScheme, c.wantScheme)
		}
	}
}
