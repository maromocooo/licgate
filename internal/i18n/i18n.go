// Package i18n is a deliberately tiny message table (English / Japanese).
package i18n

import (
	"fmt"
	"os"
	"strings"
)

// Lang is "en" or "ja".
var Lang = detect()

var table = map[string][2]string{}

func detect() string {
	for _, k := range []string{"LICGATE_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		v := strings.ToLower(os.Getenv(k))
		if v == "" {
			continue
		}
		if strings.HasPrefix(v, "ja") {
			return "ja"
		}
		return "en"
	}
	return "en"
}

// Set sets the language; anything starting with "ja" selects Japanese.
func Set(l string) {
	if strings.HasPrefix(strings.ToLower(l), "ja") {
		Lang = "ja"
	} else if l != "" {
		Lang = "en"
	}
}

// Add registers messages: key -> {english, japanese}.
func Add(m map[string][2]string) {
	for k, v := range m {
		table[k] = v
	}
}

// T formats message key with args in the current language.
func T(key string, args ...any) string {
	v, ok := table[key]
	if !ok {
		if len(args) == 0 {
			return key
		}
		return key + fmt.Sprint(args...)
	}
	f := v[0]
	if Lang == "ja" && v[1] != "" {
		f = v[1]
	}
	if len(args) == 0 {
		return f
	}
	return fmt.Sprintf(f, args...)
}
