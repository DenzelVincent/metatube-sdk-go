package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/metatube-community/metatube-sdk-go/model"
)

func TestPreferredActorProviders(t *testing.T) {
	for _, name := range []string{"MINNANO", "avbase", "Gfriends"} {
		assert.True(t, isPreferredActorProvider(name), "name: %s", name)
	}
	for _, name := range []string{"Wikipedia", "AV-LEAGUE", "ThePornDB"} {
		assert.False(t, isPreferredActorProvider(name), "name: %s", name)
	}
}

func TestNormalizeActorNameKey(t *testing.T) {
	cases := map[string]string{
		"柚井初美（ゆい はつみ）":    "柚井初美(ゆいはつみ)",
		"柚井初美(ゆい　はつみ)":    "柚井初美(ゆいはつみ)",
		"  Yui Hatsumi  ": "yuihatsumi",
		"YUＩ":             "yui",
	}
	for input, want := range cases {
		assert.Equal(t, want, NormalizeActorNameKey(input), "input: %q", input)
	}
}

func TestActorPersonSaveAndLookup(t *testing.T) {
	eng := newTranslationTestEngine(t)

	person, err := eng.SaveActorAlias(ActorAliasInput{
		StageName:         "ゆい はつみ",
		TranslatedName:    "柚井初美",
		TranslationSource: "source",
		Provider:          "MINNANO",
		ActorID:           "42",
		Source:            "JavBus",
	})
	require.NoError(t, err)
	require.NotNil(t, person)
	assert.Equal(t, "ゆい はつみ", person.StageName)
	assert.Equal(t, "柚井初美", person.TranslatedName)
	assert.Equal(t, "source", person.TranslationSource)

	for _, query := range []string{"ゆい はつみ", "柚井初美"} {
		persons, err := eng.FindActorPersonsByName(query)
		require.NoError(t, err, "query: %q", query)
		require.Len(t, persons, 1, "query: %q", query)
		assert.Equal(t, person.ID, persons[0].ID)
	}
}

func TestActorPersonSaveIsIdempotent(t *testing.T) {
	eng := newTranslationTestEngine(t)

	input := ActorAliasInput{
		StageName: "ゆい はつみ",
		Provider:  "MINNANO",
		ActorID:   "42",
	}
	first, err := eng.SaveActorAlias(input)
	require.NoError(t, err)
	second, err := eng.SaveActorAlias(input)
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID)
	var count int64
	require.NoError(t, eng.db.Model(&model.ActorPerson{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestActorTranslationPriority(t *testing.T) {
	eng := newTranslationTestEngine(t)

	// 引擎译文先写入；另一个引擎结果不能覆盖。
	_, err := eng.SaveActorAlias(ActorAliasInput{
		StageName: "ゆい はつみ", TranslatedName: "引擎译名", TranslationSource: "engine",
		Provider: "MINNANO", ActorID: "42",
	})
	require.NoError(t, err)
	person, err := eng.SaveActorAlias(ActorAliasInput{
		StageName: "ゆい はつみ", TranslatedName: "另一个引擎结果", TranslationSource: "engine",
		Provider: "MINNANO", ActorID: "42",
	})
	require.NoError(t, err)
	assert.Equal(t, "引擎译名", person.TranslatedName)

	// 数据源给的汉字名（A）可以覆盖引擎译文。
	person, err = eng.SaveActorAlias(ActorAliasInput{
		StageName: "ゆい はつみ", TranslatedName: "柚井初美", TranslationSource: "source",
		Provider: "MINNANO", ActorID: "42",
	})
	require.NoError(t, err)
	assert.Equal(t, "柚井初美", person.TranslatedName)

	// 空译名后续补写。
	_, err = eng.SaveActorAlias(ActorAliasInput{
		StageName: "あい", Provider: "MINNANO", ActorID: "43",
	})
	require.NoError(t, err)
	other, err := eng.SaveActorAlias(ActorAliasInput{
		StageName: "あい", TranslatedName: "爱", TranslationSource: "engine",
		Provider: "MINNANO", ActorID: "43",
	})
	require.NoError(t, err)
	assert.Equal(t, "爱", other.TranslatedName)
}

func TestFindActorPersonsByNameAmbiguous(t *testing.T) {
	eng := newTranslationTestEngine(t)

	for _, id := range []string{"1", "2"} {
		_, err := eng.SaveActorAlias(ActorAliasInput{
			StageName: "同名", Provider: "MINNANO", ActorID: id,
		})
		require.NoError(t, err)
	}

	persons, err := eng.FindActorPersonsByName("同名")
	require.NoError(t, err)
	assert.Len(t, persons, 2)
}

func TestMergeActorPersons(t *testing.T) {
	eng := newTranslationTestEngine(t)

	first, err := eng.SaveActorAlias(ActorAliasInput{
		StageName: "ゆい はつみ", TranslatedName: "柚井初美",
		Provider: "MINNANO", ActorID: "1",
	})
	require.NoError(t, err)
	second, err := eng.SaveActorAlias(ActorAliasInput{
		StageName: "ゆい初美", Provider: "AVBASE", ActorID: "2",
	})
	require.NoError(t, err)

	merged, err := eng.MergeActorPersons([]uint{first.ID, second.ID}, "ゆい はつみ", "柚井初美")
	require.NoError(t, err)
	assert.Equal(t, first.ID, merged.ID)
	assert.Equal(t, "ゆい はつみ", merged.StageName)
	assert.Equal(t, "柚井初美", merged.TranslatedName)

	var count int64
	require.NoError(t, eng.db.Model(&model.ActorPerson{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)

	persons, err := eng.FindActorPersonsByName("柚井初美")
	require.NoError(t, err)
	require.Len(t, persons, 1)
	assert.Equal(t, first.ID, persons[0].ID)
}

func TestActorAliasRejectsInvalidInput(t *testing.T) {
	eng := newTranslationTestEngine(t)

	_, err := eng.SaveActorAlias(ActorAliasInput{ActorID: "1"})
	assert.ErrorIs(t, err, ErrActorAliasInvalid)

	_, err = eng.MergeActorPersons([]uint{1}, "", "")
	assert.ErrorIs(t, err, ErrActorAliasMergeInvalid)
}
