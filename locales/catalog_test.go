package locales

import (
	"reflect"
	"regexp"
	"sort"
	"testing"
)

func TestCatalogContract(t *testing.T) {
	tokens := regexp.MustCompile(`\{\w+\}|%[ds]`)
	extract := func(s string) []string { v := tokens.FindAllString(s, -1); sort.Strings(v); return v }
	for _, code := range Supported {
		if len(catalogs[code]) != len(catalogs["zh-CN"]) {
			t.Fatalf("%s key count differs", code)
		}
		for key := range catalogs["zh-CN"] {
			value, ok := catalogs[code][key]
			if !ok || value == "" || !reflect.DeepEqual(extract(key), extract(value)) {
				t.Errorf("%s: missing text or inconsistent placeholders for %q", code, key)
			}
		}
	}
}

func TestNegotiate(t *testing.T) {
	for _, v := range []struct{ input, want string }{
		{"", "zh-CN"}, {"fr-FR", "en-US"}, {"ja;q=0.5,en;q=0.9", "en-US"},
		{"ko;q=0,ja;q=0.7", "ja-JP"}, {"zh-Hant-HK", "zh-TW"}, {"zh-SG", "zh-CN"},
		{"ko;q=NaN,en", "en-US"}, {"ja;q=2,ko;q=0.5", "ko-KR"}, {"ja;q=bad,en", "en-US"},
		{"fr-FR,ko-KR;q=0.8", "ko-KR"},
	} {
		if got := Negotiate(v.input); got != v.want {
			t.Errorf("%q: got %s, want %s", v.input, got, v.want)
		}
	}
}
