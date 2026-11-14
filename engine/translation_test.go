package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/metatube-community/metatube-sdk-go/database"
)

func newTranslationTestEngine(t *testing.T) *Engine {
	t.Helper()

	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", name)
	db, err := database.Open(&database.Config{
		DSN:                  dsn,
		DisableAutomaticPing: true,
	})
	require.NoError(t, err)

	eng := New(db)
	require.NoError(t, eng.DBAutoMigrate(true))
	return eng
}

func TestTranslationTerms(t *testing.T) {
	eng := newTranslationTestEngine(t)

	t.Run("save and lookup roundtrip", func(t *testing.T) {
		saved, err := eng.SaveTranslationTerm("series", "zh-CN", "First Love", "初恋", "Baidu")
		require.NoError(t, err)
		require.NotNil(t, saved)
		assert.Equal(t, "初恋", saved.TranslatedText)
		assert.Equal(t, "First Love", saved.SourceText)

		got, err := eng.GetTranslationTerm("series", "zh-cn", "First Love")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "初恋", got.TranslatedText)
	})

	t.Run("normalizes case and surrounding whitespace", func(t *testing.T) {
		_, err := eng.SaveTranslationTerm("series", "zh-CN", "  First Love  ", " 初恋 ", "Google")
		require.NoError(t, err)

		got, err := eng.GetTranslationTerm("series", "ZH-CN", " first love ")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "初恋", got.TranslatedText)
		assert.Equal(t, "First Love", got.SourceText)
	})

	t.Run("first write wins", func(t *testing.T) {
		first, err := eng.SaveTranslationTerm("genre", "zh-CN", "巨乳", "大胸", "Baidu")
		require.NoError(t, err)
		assert.Equal(t, "大胸", first.TranslatedText)

		second, err := eng.SaveTranslationTerm("genre", "zh-CN", "巨乳", "美胸", "Google")
		require.NoError(t, err)
		require.NotNil(t, second)
		assert.Equal(t, "大胸", second.TranslatedText)

		got, err := eng.GetTranslationTerm("genre", "zh-CN", "巨乳")
		require.NoError(t, err)
		assert.Equal(t, "大胸", got.TranslatedText)
	})

	t.Run("isolates kind and target language", func(t *testing.T) {
		_, err := eng.SaveTranslationTerm("maker", "zh-CN", "Same Name", "片商译名", "Baidu")
		require.NoError(t, err)
		_, err = eng.SaveTranslationTerm("label", "zh-CN", "Same Name", "厂牌译名", "Baidu")
		require.NoError(t, err)
		_, err = eng.SaveTranslationTerm("maker", "zh-TW", "Same Name", "片商譯名", "Baidu")
		require.NoError(t, err)

		makerCN, err := eng.GetTranslationTerm("maker", "zh-CN", "Same Name")
		require.NoError(t, err)
		assert.Equal(t, "片商译名", makerCN.TranslatedText)

		labelCN, err := eng.GetTranslationTerm("label", "zh-CN", "Same Name")
		require.NoError(t, err)
		assert.Equal(t, "厂牌译名", labelCN.TranslatedText)

		makerTW, err := eng.GetTranslationTerm("maker", "zh-TW", "Same Name")
		require.NoError(t, err)
		assert.Equal(t, "片商譯名", makerTW.TranslatedText)
	})

	t.Run("returns record not found for missing term", func(t *testing.T) {
		_, err := eng.GetTranslationTerm("series", "zh-CN", "Nobody")
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	})

	t.Run("rejects actor kind", func(t *testing.T) {
		_, err := eng.SaveTranslationTerm("actor", "zh-CN", "x", "y", "")
		assert.Error(t, err)

		_, err = eng.GetTranslationTerm("actor", "zh-CN", "x")
		assert.Error(t, err)
	})

	t.Run("rejects blank or invalid input", func(t *testing.T) {
		_, err := eng.SaveTranslationTerm("", "zh-CN", "x", "y", "")
		assert.Error(t, err)

		_, err = eng.SaveTranslationTerm("series", "zh-CN", "   ", "y", "")
		assert.Error(t, err)

		_, err = eng.SaveTranslationTerm("series", "zh-CN", "x", "   ", "")
		assert.Error(t, err)

		_, err = eng.SaveTranslationTerm("series", "zh-CN", "x", "第一行\n第二行", "")
		assert.Error(t, err)

		_, err = eng.SaveTranslationTerm("series", "zh-CN", strings.Repeat("a", 513), "y", "")
		assert.Error(t, err)
	})
}
