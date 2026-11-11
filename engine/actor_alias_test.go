package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/metatube-community/metatube-sdk-go/model"
)

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

func TestActorAliasSaveAndLookup(t *testing.T) {
	eng := newTranslationTestEngine(t)

	person, names, conflicts, err := eng.SaveActorAlias(ActorAliasInput{
		DisplayName: "柚井初美",
		Provider:    "MINNANO",
		ActorID:     "12345",
		Source:      "JavBus",
		Names: []ActorAliasNameInput{
			{Name: "柚井初美", Role: model.ActorNameRoleStage},
			{Name: "柚井初美（ゆい はつみ）", Role: model.ActorNameRoleAlias},
			{Name: "ゆい はつみ", Role: model.ActorNameRoleKana},
			{Name: "Yui Hatsumi", Role: model.ActorNameRoleRomaji},
			{Name: "柚井初美", Role: model.ActorNameRoleTranslation, Lang: "zh-CN"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, person)
	assert.Empty(t, conflicts)
	assert.Equal(t, "柚井初美", person.DisplayName)
	assert.Len(t, names, 5)

	for _, query := range []string{
		"柚井初美",
		"柚井初美（ゆい はつみ）",
		"ゆい はつみ",
		"yui hatsumi",
	} {
		persons, err := eng.FindActorPersonsByName(query)
		require.NoError(t, err, "query: %q", query)
		require.Len(t, persons, 1, "query: %q", query)
		assert.Equal(t, person.ID, persons[0].ID)
	}
}

func TestActorAliasSaveIsIdempotent(t *testing.T) {
	eng := newTranslationTestEngine(t)

	input := ActorAliasInput{
		DisplayName: "柚井初美",
		Provider:    "MINNANO",
		ActorID:     "12345",
		Names: []ActorAliasNameInput{
			{Name: "柚井初美", Role: model.ActorNameRoleStage},
			{Name: "ゆい はつみ", Role: model.ActorNameRoleKana},
		},
	}

	first, _, _, err := eng.SaveActorAlias(input)
	require.NoError(t, err)
	second, names, conflicts, err := eng.SaveActorAlias(input)
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID)
	assert.Empty(t, conflicts)
	assert.Len(t, names, 2)
}

func TestActorAliasRejectsInvalidInput(t *testing.T) {
	eng := newTranslationTestEngine(t)

	_, _, _, err := eng.SaveActorAlias(ActorAliasInput{ActorID: "1"})
	assert.ErrorIs(t, err, ErrActorAliasInvalid)

	_, _, _, err = eng.SaveActorAlias(ActorAliasInput{
		Provider: "MINNANO",
		ActorID:  "1",
		Names:    []ActorAliasNameInput{{Name: "x", Role: "unknown"}},
	})
	assert.ErrorIs(t, err, ErrActorAliasInvalidRole)
}

func TestActorAliasSameNameConflictsAcrossPersons(t *testing.T) {
	eng := newTranslationTestEngine(t)

	first, _, _, err := eng.SaveActorAlias(ActorAliasInput{
		DisplayName: "A",
		Provider:    "MINNANO",
		ActorID:     "1",
		Names:       []ActorAliasNameInput{{Name: "同名", Role: model.ActorNameRoleStage}},
	})
	require.NoError(t, err)

	second, _, conflicts, err := eng.SaveActorAlias(ActorAliasInput{
		DisplayName: "B",
		Provider:    "MINNANO",
		ActorID:     "2",
		Names:       []ActorAliasNameInput{{Name: "同名", Role: model.ActorNameRoleStage}},
	})
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, second.ID)
	assert.Equal(t, []string{"同名"}, conflicts)

	// 冲突的名字不会挂到第二个身份上，查询仍然只命中第一个。
	persons, err := eng.FindActorPersonsByName("同名")
	require.NoError(t, err)
	require.Len(t, persons, 1)
	assert.Equal(t, first.ID, persons[0].ID)
}

func TestActorAliasAmbiguousLookup(t *testing.T) {
	eng := newTranslationTestEngine(t)

	first := model.ActorPerson{DisplayName: "A", Provider: "MINNANO", ActorID: "1"}
	second := model.ActorPerson{DisplayName: "B", Provider: "MINNANO", ActorID: "2"}
	require.NoError(t, eng.db.Create(&first).Error)
	require.NoError(t, eng.db.Create(&second).Error)
	for _, person := range []model.ActorPerson{first, second} {
		require.NoError(t, eng.db.Create(&model.ActorName{
			PersonID: person.ID,
			Name:     "同名",
			NameKey:  NormalizeActorNameKey("同名"),
			Role:     model.ActorNameRoleStage,
		}).Error)
	}

	persons, err := eng.FindActorPersonsByName("同名")
	require.NoError(t, err)
	assert.Len(t, persons, 2)
}

func TestMergeActorPersons(t *testing.T) {
	eng := newTranslationTestEngine(t)

	first, _, _, err := eng.SaveActorAlias(ActorAliasInput{
		DisplayName: "柚井初美",
		Provider:    "MINNANO",
		ActorID:     "1",
		Names:       []ActorAliasNameInput{{Name: "柚井初美", Role: model.ActorNameRoleStage}},
	})
	require.NoError(t, err)
	second, _, _, err := eng.SaveActorAlias(ActorAliasInput{
		DisplayName: "ゆい初美",
		Provider:    "AVBASE",
		ActorID:     "2",
		Names: []ActorAliasNameInput{
			{Name: "ゆい初美", Role: model.ActorNameRoleStage},
			{Name: "ゆい はつみ", Role: model.ActorNameRoleKana},
		},
	})
	require.NoError(t, err)

	merged, names, err := eng.MergeActorPersons([]uint{first.ID, second.ID}, "柚井初美")
	require.NoError(t, err)
	assert.Equal(t, first.ID, merged.ID)
	assert.Equal(t, "柚井初美", merged.DisplayName)
	assert.Len(t, names, 3)

	// 另一个身份已被删除，旧名字也能查到合并后的身份。
	persons, err := eng.FindActorPersonsByName("ゆい はつみ")
	require.NoError(t, err)
	require.Len(t, persons, 1)
	assert.Equal(t, first.ID, persons[0].ID)
}
