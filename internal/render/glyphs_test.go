package render

import "testing"

func envFrom(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestGlyphsFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want Glyphs
	}{
		{"utf-8 locale", map[string]string{"LANG": "en_US.UTF-8"}, unicodeGlyphs},
		{"utf8 spelling, lower case", map[string]string{"LANG": "de_CH.utf8"}, unicodeGlyphs},
		{"LC_ALL wins over LANG", map[string]string{"LC_ALL": "C", "LANG": "en_US.UTF-8"}, asciiGlyphs},
		{"LC_CTYPE wins over LANG", map[string]string{"LC_CTYPE": "en_US.UTF-8", "LANG": "C"}, unicodeGlyphs},
		{"dumb terminal", map[string]string{"TERM": "dumb", "LANG": "en_US.UTF-8"}, asciiGlyphs},
		{"nothing set", map[string]string{}, asciiGlyphs},
		{"POSIX locale", map[string]string{"LANG": "POSIX"}, asciiGlyphs},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := glyphsFromEnv(envFrom(tt.env)); got != tt.want {
				t.Errorf("glyphsFromEnv = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestGlyphSets_AsciiHasNoMultiByteRunes(t *testing.T) {
	for name, s := range map[string]string{
		"Info": asciiGlyphs.Info, "OK": asciiGlyphs.OK, "Warn": asciiGlyphs.Warn, "Go": asciiGlyphs.Go,
		"From": asciiGlyphs.From, "Dot": asciiGlyphs.Dot, "Ellipsis": asciiGlyphs.Ellipsis,
		"H": asciiGlyphs.H, "V": asciiGlyphs.V, "TL": asciiGlyphs.TL, "TR": asciiGlyphs.TR,
		"BL": asciiGlyphs.BL, "BR": asciiGlyphs.BR,
	} {
		for _, r := range s {
			if r > 127 {
				t.Errorf("ASCII glyph %s = %q contains non-ASCII rune %q", name, s, r)
			}
		}
		if s == "" {
			t.Errorf("ASCII glyph %s is empty", name)
		}
	}
}
