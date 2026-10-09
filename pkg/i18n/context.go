package i18n

import "context"

// languageKey is a private type for context keys to avoid collisions.
type languageKey struct{}

// WithLanguage injects a language into the context.
func WithLanguage(ctx context.Context, lang LanguageCode) context.Context {
	return context.WithValue(ctx, languageKey{}, lang)
}

// LanguageFromContext gets the language from the context.
// Returns English as fallback if not found.
func LanguageFromContext(ctx context.Context) LanguageCode {
	if lang, ok := ctx.Value(languageKey{}).(LanguageCode); ok {
		return lang
	}

	return English
}
