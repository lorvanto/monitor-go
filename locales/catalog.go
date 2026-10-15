// Package locales shares display-only message catalogs with the web client.
// Persisted names, audit records and diagnostic payloads are never translated here.
package locales

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.json
var files embed.FS

var Supported = []string{"zh-CN", "en-US", "ja-JP", "ko-KR", "zh-TW"}
var catalogs = load()

func load() map[string]map[string]string {
	result := make(map[string]map[string]string)
	for _, code := range Supported {
		raw, err := files.ReadFile(code + ".json")
		if err != nil {
			panic(err)
		}
		var messages map[string]string
		if err = json.Unmarshal(raw, &messages); err != nil {
			panic(err)
		}
		result[code] = messages
	}
	return result
}

func Match(tag string) string {
	tag = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(tag), "_", "-"))
	parts := strings.Split(tag, "-")
	switch parts[0] {
	case "en":
		return "en-US"
	case "ja":
		return "ja-JP"
	case "ko":
		return "ko-KR"
	case "zh":
		for _, part := range parts[1:] {
			if part == "hant" || part == "tw" || part == "hk" || part == "mo" {
				return "zh-TW"
			}
		}
		return "zh-CN"
	}
	return ""
}

// Negotiate respects quality values and skips unsupported or excluded languages.
// Clients omitting the header retain the original Chinese API behavior.
func Negotiate(header string) string {
	if strings.TrimSpace(header) == "" {
		return "zh-CN"
	}
	type preference struct {
		code    string
		quality float64
	}
	choices := []preference{}
	for _, item := range strings.Split(header, ",") {
		parts := strings.Split(strings.TrimSpace(item), ";")
		code, quality := Match(parts[0]), 1.0
		for _, parameter := range parts[1:] {
			key, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if ok && strings.EqualFold(key, "q") {
				n, err := strconv.ParseFloat(value, 64)
				if err != nil || !(n > 0 && n <= 1) {
					quality = 0
				} else {
					quality = n
				}
			}
		}
		if code != "" && quality > 0 {
			choices = append(choices, preference{code, quality})
		}
	}
	sort.SliceStable(choices, func(i, j int) bool { return choices[i].quality > choices[j].quality })
	if len(choices) > 0 {
		return choices[0].code
	}
	return "en-US"
}

func Text(code, source string) string {
	if value := catalogs[code][source]; value != "" {
		return value
	}
	if value := catalogs["en-US"][source]; value != "" {
		return value
	}
	return source
}

func Format(code, source string, args ...any) string {
	return fmt.Sprintf(Text(code, source), args...)
}
