package engine

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm/clause"

	"github.com/metatube-community/metatube-sdk-go/model"
)

// maxTranslationTermLength mirrors the varchar(512) columns in
// translation_metadata. Postgres enforces the limit, SQLite does not, so the
// check must live in code to keep both drivers behaving the same.
const maxTranslationTermLength = 512

func normalizeTranslationTerm(kind, targetLang, source string) (normalizedKind, normalizedTarget, sourceKey string, err error) {
	normalizedKind = strings.ToLower(strings.TrimSpace(kind))
	normalizedTarget = strings.ToLower(strings.TrimSpace(targetLang))
	sourceKey = strings.ToLower(strings.TrimSpace(source))

	if !model.IsValidTranslationKind(normalizedKind) {
		return "", "", "", fmt.Errorf("invalid translation kind: %q", kind)
	}
	if normalizedTarget == "" {
		return "", "", "", fmt.Errorf("target language is empty")
	}
	if utf8.RuneCountInString(normalizedTarget) > 16 {
		return "", "", "", fmt.Errorf("target language is too long")
	}
	if sourceKey == "" {
		return "", "", "", fmt.Errorf("source is empty")
	}
	if utf8.RuneCountInString(sourceKey) > maxTranslationTermLength {
		return "", "", "", fmt.Errorf("source is too long")
	}
	return normalizedKind, normalizedTarget, sourceKey, nil
}

// GetTranslationTerm looks up a term by kind, target language and source text.
// It returns gorm.ErrRecordNotFound when the term is not in the dictionary.
func (e *Engine) GetTranslationTerm(kind, targetLang, source string) (*model.TranslationInfo, error) {
	normalizedKind, normalizedTarget, sourceKey, err := normalizeTranslationTerm(kind, targetLang, source)
	if err != nil {
		return nil, err
	}

	info := &model.TranslationInfo{}
	err = e.db.
		Where("kind = ? AND target_lang = ? AND source_key = ?",
			normalizedKind, normalizedTarget, sourceKey).
		First(info).Error
	return info, err
}

// SaveTranslationTerm inserts a term translation using first-write-wins
// semantics: if the unique key already exists, the existing translation is
// kept and returned. The returned record is the value that callers must use.
func (e *Engine) SaveTranslationTerm(kind, targetLang, source, translated, engineName string) (*model.TranslationInfo, error) {
	normalizedKind, normalizedTarget, sourceKey, err := normalizeTranslationTerm(kind, targetLang, source)
	if err != nil {
		return nil, err
	}

	sourceText := strings.TrimSpace(source)
	translatedText := strings.TrimSpace(translated)
	engineName = strings.TrimSpace(engineName)

	if utf8.RuneCountInString(sourceText) > maxTranslationTermLength {
		return nil, fmt.Errorf("source is too long")
	}
	if translatedText == "" {
		return nil, fmt.Errorf("translated text is empty")
	}
	if utf8.RuneCountInString(translatedText) > maxTranslationTermLength {
		return nil, fmt.Errorf("translated text is too long")
	}
	// Terms become collection names, tags and actor names, so an embedded
	// line break would corrupt those values.
	if strings.ContainsAny(translatedText, "\r\n") {
		return nil, fmt.Errorf("translated text contains line breaks")
	}
	engineName = truncateRunes(engineName, 32)

	info := &model.TranslationInfo{
		Kind:           normalizedKind,
		TargetLang:     normalizedTarget,
		SourceKey:      sourceKey,
		SourceText:     sourceText,
		TranslatedText: translatedText,
		Engine:         engineName,
	}
	if err := e.db.Clauses(clause.OnConflict{DoNothing: true}).Create(info).Error; err != nil {
		return nil, err
	}

	// Always return the stored row: a concurrent writer may have won the
	// first-write race, and every caller must use the same translation.
	return e.GetTranslationTerm(normalizedKind, normalizedTarget, sourceText)
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}
