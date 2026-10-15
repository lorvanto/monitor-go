package app

import (
	"fmt"
	"github.com/lorvanto/monitor-go/locales"
	"net/http"
)

type formattedError struct {
	key  string
	args []any
}

func (e *formattedError) Error() string  { return fmt.Sprintf(e.key, e.args...) }
func badf(key string, args ...any) error { return &formattedError{key, args} }

func requestLocale(w http.ResponseWriter, r *http.Request) string {
	code := locales.Negotiate(r.Header.Get("Accept-Language"))
	// Links and image requests cannot set Accept-Language to the UI preference.
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		if selected := locales.Match(r.URL.Query().Get("lang")); selected != "" {
			code = selected
		}
	}
	w.Header().Set("Content-Language", code)
	w.Header().Add("Vary", "Accept-Language")
	return code
}

func menuNameKey(m Menu, builtinKey string) string {
	if !m.Builtin {
		return ""
	}
	expected := ""
	if m.Kind == "page" {
		for _, page := range pages {
			if page.Key == m.PageKey {
				expected = page.Name
				break
			}
		}
	} else if m.Kind == "group" {
		if builtinKey == "application-tools" {
			expected = "应用与工具"
		}
		if m.ID == 1 {
			expected = "观测与运维"
		}
		if m.ID == 2 {
			expected = "文件与资源"
		}
		if m.ID == 3 {
			expected = "系统管理"
		}
	}
	if m.Name == expected {
		return expected
	}
	return ""
}
