// Package i18n provides internationalization utilities for language detection and normalization.
package i18n

import (
	"strings"
)

// LanguageCode represents a standardized language code.
type LanguageCode string

// Common language codes.
const (
	English    LanguageCode = "en"
	ChineseCN  LanguageCode = "zh-cn" // Simplified Chinese
	ChineseTW  LanguageCode = "zh-tw" // Traditional Chinese
	Japanese   LanguageCode = "ja"
	Korean     LanguageCode = "ko"
	Spanish    LanguageCode = "es"
	French     LanguageCode = "fr"
	German     LanguageCode = "de"
	Italian    LanguageCode = "it"
	Portuguese LanguageCode = "pt"
	Russian    LanguageCode = "ru"
	Arabic     LanguageCode = "ar"
	Hindi      LanguageCode = "hi"
)

// LanguageInfo contains information about a language.
type LanguageInfo struct {
	Code        LanguageCode `json:"code"`
	Name        string       `json:"name"`
	NativeName  string       `json:"nativeName"`
	Fallback    LanguageCode `json:"fallback"`
	IsSupported bool         `json:"isSupported"`
}

// SupportedLanguages defines the languages supported by the application.
var SupportedLanguages = map[LanguageCode]LanguageInfo{
	English: {
		Code:        English,
		Name:        "English",
		NativeName:  "English",
		Fallback:    English,
		IsSupported: true,
	},
	ChineseCN: {
		Code:        ChineseCN,
		Name:        "Chinese (Simplified)",
		NativeName:  "简体中文",
		Fallback:    English,
		IsSupported: true,
	},
	ChineseTW: {
		Code:        ChineseTW,
		Name:        "Chinese (Traditional)",
		NativeName:  "繁體中文",
		Fallback:    English,
		IsSupported: true,
	},
	Japanese: {
		Code:        Japanese,
		Name:        "Japanese",
		NativeName:  "日本語",
		Fallback:    English,
		IsSupported: true,
	},
	Korean: {
		Code:        Korean,
		Name:        "Korean",
		NativeName:  "한국어",
		Fallback:    English,
		IsSupported: true,
	},
	Spanish: {
		Code:        Spanish,
		Name:        "Spanish",
		NativeName:  "Español",
		Fallback:    English,
		IsSupported: true,
	},
	French: {
		Code:        French,
		Name:        "French",
		NativeName:  "Français",
		Fallback:    English,
		IsSupported: true,
	},
	German: {
		Code:        German,
		Name:        "German",
		NativeName:  "Deutsch",
		Fallback:    English,
		IsSupported: true,
	},
	Italian: {
		Code:        Italian,
		Name:        "Italian",
		NativeName:  "Italiano",
		Fallback:    English,
		IsSupported: true,
	},
	Portuguese: {
		Code:        Portuguese,
		Name:        "Portuguese",
		NativeName:  "Português",
		Fallback:    English,
		IsSupported: true,
	},
	Russian: {
		Code:        Russian,
		Name:        "Russian",
		NativeName:  "Русский",
		Fallback:    English,
		IsSupported: true,
	},
	Arabic: {
		Code:        Arabic,
		Name:        "Arabic",
		NativeName:  "العربية",
		Fallback:    English,
		IsSupported: true,
	},
	Hindi: {
		Code:        Hindi,
		Name:        "Hindi",
		NativeName:  "हिन्दी",
		Fallback:    English,
		IsSupported: true,
	},
}

// NormalizeLanguageCode normalizes various language code formats to standard codes.
// It handles common variations from browsers and operating systems.
func NormalizeLanguageCode(input string) LanguageCode {
	if input == "" {
		return English
	}

	// Convert to lowercase and trim
	code := strings.ToLower(strings.TrimSpace(input))

	// Handle common variations
	switch code {
	case "en", "en-us", "en-gb", "en-au", "en-ca":
		return English
	case "zh", "zh-cn", "zh-hans", "zh-hans-cn":
		return ChineseCN
	case "zh-tw", "zh-hant", "zh-hant-tw", "zh-hk", "zh-hant-hk":
		return ChineseTW
	case "ja", "ja-jp":
		return Japanese
	case "ko", "ko-kr":
		return Korean
	case "es", "es-es", "es-mx", "es-ar":
		return Spanish
	case "fr", "fr-fr", "fr-ca", "fr-be":
		return French
	case "de", "de-de", "de-at", "de-ch":
		return German
	case "it", "it-it", "it-ch":
		return Italian
	case "pt", "pt-pt", "pt-br":
		return Portuguese
	case "ru", "ru-ru":
		return Russian
	case "ar", "ar-sa", "ar-ae":
		return Arabic
	case "hi", "hi-in":
		return Hindi
	}

	// Try to extract base language code (first part before dash)
	if idx := strings.Index(code, "-"); idx > 0 {
		baseCode := code[:idx]

		return NormalizeLanguageCode(baseCode)
	}

	return English
}

// GetSupportedLanguageCode returns a supported language code, falling back to English if not supported.
func GetSupportedLanguageCode(input string) LanguageCode {
	normalized := NormalizeLanguageCode(input)

	if info, exists := SupportedLanguages[normalized]; exists && info.IsSupported {
		return normalized
	}

	return English
}

// GetFallbackLanguage returns the fallback language for a given language code.
func GetFallbackLanguage(code LanguageCode) LanguageCode {
	if info, exists := SupportedLanguages[code]; exists {
		return info.Fallback
	}

	return English
}

// IsLanguageSupported checks if a language code is supported.
func IsLanguageSupported(code LanguageCode) bool {
	if info, exists := SupportedLanguages[code]; exists {
		return info.IsSupported
	}

	return false
}

// GetLanguageInfo returns language information for a given code.
func GetLanguageInfo(code LanguageCode) (LanguageInfo, bool) {
	info, exists := SupportedLanguages[code]

	return info, exists
}

// GetSupportedLanguageCodes returns a slice of all supported language codes.
func GetSupportedLanguageCodes() []LanguageCode {
	var codes []LanguageCode
	for code, info := range SupportedLanguages {
		if info.IsSupported {
			codes = append(codes, code)
		}
	}

	return codes
}

// ParseAcceptLanguage parses the Accept-Language header and returns the best match.
// This is commonly used in web applications to detect user language preference.
func ParseAcceptLanguage(acceptLanguage string) LanguageCode {
	if acceptLanguage == "" {
		return English
	}

	// Split by comma and parse each language tag
	languages := strings.Split(acceptLanguage, ",")

	for _, lang := range languages {
		// Parse language tag (e.g., "en-US;q=0.9")
		parts := strings.Split(strings.TrimSpace(lang), ";")
		langCode := strings.TrimSpace(parts[0])

		// Skip if no quality value or quality is 0
		if len(parts) > 1 {
			qPart := strings.TrimSpace(parts[1])
			if strings.HasPrefix(qPart, "q=") {
				qValue := strings.TrimPrefix(qPart, "q=")
				if qValue == "0" || qValue == "0.0" {
					continue
				}
			}
		}

		// Try to normalize and check if supported
		normalized := NormalizeLanguageCode(langCode)
		if IsLanguageSupported(normalized) {
			return normalized
		}
	}

	return English
}
