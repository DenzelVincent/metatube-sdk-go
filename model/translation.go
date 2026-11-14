package model

import "strings"

const TranslationMetadataTableName = "translation_metadata"

const (
	TranslationKindSeries = "series"
	TranslationKindGenre  = "genre"
	TranslationKindMaker  = "maker"
	TranslationKindLabel  = "label"
)

// IsValidTranslationKind reports whether kind is a supported term category.
// Matching is case-insensitive because callers may pass user input.
// 演员不再翻译（只留优选艺名），kind=actor 已移除。
func IsValidTranslationKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case TranslationKindSeries,
		TranslationKindGenre,
		TranslationKindMaker,
		TranslationKindLabel:
		return true
	default:
		return false
	}
}

// TranslationInfo is a term-level translation cached on the server.
// Titles and summaries are intentionally not stored here: they are translated
// on every scrape and the engine/CDN handles caching.
type TranslationInfo struct {
	ID uint `json:"id" gorm:"primaryKey"`
	// Kind is one of series/genre/maker/label.
	Kind string `json:"kind" gorm:"size:16;not null;uniqueIndex:idx_translation_metadata_term,priority:1"`
	// TargetLang is normalized to lower case (e.g. zh-cn).
	TargetLang string `json:"target_lang" gorm:"size:16;not null;uniqueIndex:idx_translation_metadata_term,priority:2"`
	// SourceKey is TrimSpace+lower of SourceText and is the lookup key.
	SourceKey string `json:"source_key" gorm:"size:512;not null;uniqueIndex:idx_translation_metadata_term,priority:3"`
	// SourceText keeps the original spelling for diagnostics.
	SourceText     string `json:"source_text" gorm:"size:512;not null"`
	TranslatedText string `json:"translated_text" gorm:"size:512;not null"`
	// Engine records which engine first produced this translation.
	Engine      string `json:"engine" gorm:"size:32"`
	TimeTracker `json:"-"`
}

func (*TranslationInfo) TableName() string {
	return TranslationMetadataTableName
}
