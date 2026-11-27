// Package i18n provides a tiny translation layer for human-facing CLI text.
// Machine-readable values (JSON keys, enums, exit codes, error codes) are
// deliberately never translated.
package i18n

import (
	"fmt"
	"os"
	"strings"
)

type Lang string

const (
	EN    Lang = "en"
	ZHCN  Lang = "zh-CN"
)

// ParseLang normalizes user input. Unsupported values fall back to English so
// generated documentation is deterministic and never depends on OS locale.
func ParseLang(v string) Lang {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "zh", "zh-cn", "zh_hans", "zh-hans", "zh_cn":
		return ZHCN
	default:
		return EN
	}
}

// Detect applies the documented precedence: explicit flag, environment, en.
func Detect(flagValue string) Lang {
	if flagValue != "" {
		return ParseLang(flagValue)
	}
	return ParseLang(os.Getenv("AGENT_PWD_LANG"))
}

// T translates a canonical English source string. English is the identity
// locale; a missing translation returns the key itself.
func T(lang Lang, key string, args ...any) string {
	if lang == ZHCN {
		if translated, ok := catalogZHCN[key]; ok {
			key = translated
		}
	}
	if len(args) == 0 {
		return key
	}
	return fmt.Sprintf(key, args...)
}

// Supported returns the canonical languages, in documentation order.
func Supported() []Lang { return []Lang{EN, ZHCN} }
