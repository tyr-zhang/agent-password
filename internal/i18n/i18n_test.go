package i18n

import (
	"regexp"
	"strings"
	"testing"
)

var (
	placeholder = regexp.MustCompile(`%[a-zA-Z]`)
	backtick    = regexp.MustCompile("`[^`]*`")
	flagToken   = regexp.MustCompile(`--[a-zA-Z0-9-]+`)
)

func TestParseLang(t *testing.T) {
	cases := map[string]Lang{
		"en": EN, "EN": EN, "zh": ZHCN, "zh-CN": ZHCN,
		"zh_Hans": ZHCN, "fr": EN, "": EN,
	}
	for in, want := range cases {
		if got := ParseLang(in); got != want {
			t.Errorf("ParseLang(%q)=%s want %s", in, got, want)
		}
	}
}

func TestDetectPrecedence(t *testing.T) {
	t.Setenv("AGENT_PWD_LANG", "zh")
	if got := Detect(""); got != ZHCN {
		t.Fatalf("env ignored: %s", got)
	}
	if got := Detect("en"); got != EN {
		t.Fatalf("explicit flag did not win: %s", got)
	}
	if got := Detect("bogus"); got != EN {
		t.Fatalf("invalid flag did not fall back to English: %s", got)
	}
}

func TestTFallback(t *testing.T) {
	if got := T(EN, "ordinary text"); got != "ordinary text" {
		t.Fatalf("English identity failed: %q", got)
	}
	if got := T(ZHCN, "key-with-no-translation"); got != "key-with-no-translation" {
		t.Fatalf("missing translation fallback failed: %q", got)
	}
}

// TestCatalogIntegrity guards against drift: translations must preserve
// placeholders, command/flag names and backticked spans exactly.
func TestCatalogIntegrity(t *testing.T) {
	if len(catalogZHCN) == 0 {
		t.Fatal("zh-CN catalog is empty")
	}
	for key, value := range catalogZHCN {
		if strings.TrimSpace(value) == "" {
			t.Errorf("empty translation for %q", key)
			continue
		}
		if placeholder.FindAllString(key, -1) == nil && placeholder.FindAllString(value, -1) != nil {
			t.Errorf("%q: translation added a placeholder", key)
		}
		if placeholders(key) != placeholders(value) {
			t.Errorf("%q: placeholder mismatch %d vs %d", key, placeholders(key), placeholders(value))
		}
		if tokens(backtick, key) != tokens(backtick, value) {
			t.Errorf("%q: backticked spans changed: %q", key, value)
		}
		if tokens(flagToken, key) != tokens(flagToken, value) {
			t.Errorf("%q: flag tokens changed: %q", key, value)
		}
	}
}

func placeholders(s string) int { return len(placeholder.FindAllString(s, -1)) }

func tokens(re *regexp.Regexp, s string) string {
	return strings.Join(re.FindAllString(s, -1), "|")
}
