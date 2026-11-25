package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const desktopPreferencesKey = "native.desktop-preferences"

type DesktopPreference string

const (
	UILanguagePreference DesktopPreference = "ui_language"
	AILanguagePreference DesktopPreference = "ai_language"
	ThemePreference      DesktopPreference = "theme"
	PalettePreference    DesktopPreference = "palette"
	UIFontPreference     DesktopPreference = "ui_font"
	UIFontSizePreference DesktopPreference = "ui_font_size"
	CodeFontPreference   DesktopPreference = "code_font"
	CodeSizePreference   DesktopPreference = "code_font_size"
)

const (
	SystemTheme = "system"
	LightTheme  = "light"
	DarkTheme   = "dark"
	NativeBlue  = "native-blue"
	TideStudio  = "tide"
)

type DesktopPreferences struct {
	UILanguage string `json:"ui_language"`
	AILanguage string `json:"ai_language"`
	Theme      string `json:"theme"`
	Palette    string `json:"palette"`
	Fonts      DesktopFonts
}

type DesktopFonts struct {
	UI, UISize, Code, CodeSize string
}

func DefaultDesktopPreferences() DesktopPreferences {
	return DesktopPreferences{AILanguage: "en", Theme: SystemTheme, Palette: NativeBlue}
}

func (p DesktopPreference) accepts(value string) bool {
	switch p {
	case UILanguagePreference:
		return slices.Contains([]string{"en", "zh-CN"}, value)
	case AILanguagePreference:
		return slices.Contains([]string{"en", "zh", "ja", "ko"}, value)
	case ThemePreference:
		return slices.Contains([]string{SystemTheme, LightTheme, DarkTheme}, value)
	case PalettePreference:
		return value == NativeBlue || value == TideStudio
	case UIFontPreference, CodeFontPreference:
		return value == NormalizeFontFamilies(value)
	case UIFontSizePreference:
		return value == NormalizeFontSize(value, 12)
	case CodeSizePreference:
		return value == NormalizeFontSize(value, 10)
	default:
		return false
	}
}

func (p DesktopPreferences) With(key DesktopPreference, value string) (DesktopPreferences, error) {
	if !key.accepts(value) {
		return p, fmt.Errorf("invalid desktop preference %q", key)
	}
	switch key {
	case UILanguagePreference:
		p.UILanguage = value
	case AILanguagePreference:
		p.AILanguage = value
	case ThemePreference:
		p.Theme = value
	case PalettePreference:
		p.Palette = value
	case UIFontPreference:
		p.Fonts.UI = value
	case UIFontSizePreference:
		p.Fonts.UISize = value
	case CodeFontPreference:
		p.Fonts.Code = value
	case CodeSizePreference:
		p.Fonts.CodeSize = value
	}
	return p, nil
}

func NormalizeFontFamilies(input string) string {
	var names []string
	for _, name := range strings.Split(input, ",") {
		name = strings.Map(func(r rune) rune {
			if strings.ContainsRune("\"'\\;{}<>", r) || unicode.Is(unicode.C, r) || r == '\u2028' || r == '\u2029' {
				return -1
			}
			return r
		}, name)
		if normalized := strings.Join(strings.Fields(name), " "); normalized != "" {
			names = append(names, normalized)
		}
	}
	return strings.Join(names, ", ")
}

func NormalizeFontSize(input string, minimum int) string {
	input = strings.TrimSpace(input)
	digits := strings.TrimLeft(input, "+-")
	if len(input)-len(digits) > 1 {
		return ""
	}
	i := strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' })
	if i == 0 || digits == "" {
		return ""
	}
	if i > 0 {
		input = input[:len(input)-len(digits)+i]
	}
	size, _ := strconv.ParseInt(input, 10, 64)
	return strconv.FormatInt(max(int64(minimum), min(size, 20)), 10)
}

func readDesktopPreferences(raw string) (DesktopPreferences, error) {
	defaults := DefaultDesktopPreferences()
	var saved map[DesktopPreference]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &saved); err != nil || saved == nil {
		return defaults, fmt.Errorf("invalid saved desktop preferences")
	}
	prefs := defaults
	for key, field := range saved {
		var value string
		if json.Unmarshal(field, &value) == nil && key.accepts(value) {
			prefs, _ = prefs.With(key, value)
		}
	}
	return prefs, nil
}

func (s *Store) DesktopPreferences(ctx context.Context) (DesktopPreferences, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT value_json FROM app_settings WHERE key = ?", desktopPreferencesKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultDesktopPreferences(), nil
	}
	if err != nil {
		return DefaultDesktopPreferences(), err
	}
	return readDesktopPreferences(raw)
}

// SaveDesktopPreference replaces one field so other windows and later versions
// can keep independent preferences in the same object.
func (s *Store) SaveDesktopPreference(ctx context.Context, key DesktopPreference, value string) (DesktopPreferences, error) {
	return s.SaveDesktopPreferences(ctx, map[DesktopPreference]string{key: value})
}

// SaveDesktopPreferences updates the changed fields in one SQLite statement.
func (s *Store) SaveDesktopPreferences(ctx context.Context, changes map[DesktopPreference]string) (DesktopPreferences, error) {
	if len(changes) == 0 {
		return s.DesktopPreferences(ctx)
	}
	for key, value := range changes {
		if !key.accepts(value) {
			return DesktopPreferences{}, fmt.Errorf("invalid desktop preference %q", key)
		}
	}
	patch, err := json.Marshal(changes)
	if err != nil {
		return DesktopPreferences{}, err
	}
	var raw string
	err = s.db.QueryRowContext(ctx, `INSERT INTO app_settings (key, value_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json = json_patch(app_settings.value_json, excluded.value_json), updated_at = excluded.updated_at
		WHERE json_valid(app_settings.value_json) AND json_type(app_settings.value_json) = 'object'
		RETURNING value_json`, desktopPreferencesKey, string(patch), time.Now().Unix()).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return DesktopPreferences{}, fmt.Errorf("invalid saved desktop preferences")
	}
	if err != nil {
		return DesktopPreferences{}, err
	}
	return readDesktopPreferences(raw)
}
