package engine

import (
	"strings"
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

func TestCountKanji(t *testing.T) {
	cases := map[string]int{
		"柚井初美":        4,
		"ゆい はつみ":      0,
		"Yui Hatsumi": 0,
		"ゆい初美":        2,
		"㐀":           1, // U+3400 扩展 A
		"豈":           1, // U+F900 兼容表意文字
		"ｱｲｳ":         0, // 半角假名
	}
	for input, want := range cases {
		assert.Equal(t, want, countKanji(input), "input: %q", input)
	}
}

func TestSelectStageName(t *testing.T) {
	// 汉字最多者胜出。
	assert.Equal(t, "柚井初美", selectStageName([]string{"Yui Hatsumi", "ゆい はつみ", "柚井初美"}))
	// 平手取池内顺序第一个。
	assert.Equal(t, "柚井初美", selectStageName([]string{"柚井初美", "初美柚井"}))
	// 按匹配键去重（全半角/空白/大小写），保留首次出现的写法。
	assert.Equal(t, "Yui Hatsumi", selectStageName([]string{"Yui Hatsumi", "yui  hatsumi", "Ｙｕｉ Ｈａｔｓｕｍｉ"}))
	// 全假名池取第一个。
	assert.Equal(t, "ゆい はつみ", selectStageName([]string{"ゆい はつみ", "ユイ ハツミ"}))
	// 空白项剔除；空池返回空。
	assert.Equal(t, "柚井初美", selectStageName([]string{"  ", "柚井初美"}))
	assert.Equal(t, "", selectStageName(nil))
	assert.Equal(t, "", selectStageName([]string{"  ", ""}))
}

func TestActorPersonSaveAndLookup(t *testing.T) {
	eng := newTranslationTestEngine(t)

	person, err := eng.SaveActorAlias(ActorAliasInput{
		Names:    []string{"ゆい はつみ", "柚井初美"},
		Provider: "MINNANO",
		ActorID:  "42",
	})
	require.NoError(t, err)
	require.NotNil(t, person)
	assert.Equal(t, "柚井初美", person.StageName)
	assert.Equal(t, "", person.StageSource)

	persons, err := eng.FindActorPersonsByName("柚井初美")
	require.NoError(t, err)
	require.Len(t, persons, 1)
	assert.Equal(t, person.ID, persons[0].ID)

	// 全半角与空白不影响匹配。
	persons, err = eng.FindActorPersonsByName("柚井　初美")
	require.NoError(t, err)
	require.Len(t, persons, 1)

	// 只按艺名键查：落选的假名写法不再命中。
	_, err = eng.FindActorPersonsByName("ゆい はつみ")
	assert.Error(t, err)
}

func TestActorPersonSaveIsIdempotent(t *testing.T) {
	eng := newTranslationTestEngine(t)

	input := ActorAliasInput{
		Names:    []string{"ゆい はつみ", "柚井初美"},
		Provider: "MINNANO",
		ActorID:  "42",
	}
	first, err := eng.SaveActorAlias(input)
	require.NoError(t, err)
	second, err := eng.SaveActorAlias(input)
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, first.StageName, second.StageName)
	var count int64
	require.NoError(t, eng.db.Model(&model.ActorPerson{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestActorStageAutoUpgrade(t *testing.T) {
	eng := newTranslationTestEngine(t)

	_, err := eng.SaveActorAlias(ActorAliasInput{
		Names: []string{"ゆい はつみ"}, Provider: "MINNANO", ActorID: "42",
	})
	require.NoError(t, err)

	// 汉字严格更多 → 升级。
	person, err := eng.SaveActorAlias(ActorAliasInput{
		Names: []string{"ゆい はつみ", "柚井初美"}, Provider: "MINNANO", ActorID: "42",
	})
	require.NoError(t, err)
	assert.Equal(t, "柚井初美", person.StageName)

	// 平手 → 保留现有。
	person, err = eng.SaveActorAlias(ActorAliasInput{
		Names: []string{"初美柚井"}, Provider: "MINNANO", ActorID: "42",
	})
	require.NoError(t, err)
	assert.Equal(t, "柚井初美", person.StageName)

	// 更少 → 保留现有。
	person, err = eng.SaveActorAlias(ActorAliasInput{
		Names: []string{"初美"}, Provider: "MINNANO", ActorID: "42",
	})
	require.NoError(t, err)
	assert.Equal(t, "柚井初美", person.StageName)
}

func TestActorStageManualLock(t *testing.T) {
	eng := newTranslationTestEngine(t)

	// person-edit 写入并标 manual。
	person, err := eng.SaveActorAlias(ActorAliasInput{
		Names: []string{"ゆい"}, Provider: "MINNANO", ActorID: "42", Source: "person-edit",
	})
	require.NoError(t, err)
	assert.Equal(t, "ゆい", person.StageName)
	assert.Equal(t, "manual", person.StageSource)

	// auto 即使汉字更多也不覆盖。
	person, err = eng.SaveActorAlias(ActorAliasInput{
		Names: []string{"柚井初美"}, Provider: "MINNANO", ActorID: "42",
	})
	require.NoError(t, err)
	assert.Equal(t, "ゆい", person.StageName)
	assert.Equal(t, "manual", person.StageSource)

	// person-edit 可以再次覆盖，仍标 manual。
	person, err = eng.SaveActorAlias(ActorAliasInput{
		Names: []string{"新名字"}, Provider: "MINNANO", ActorID: "42", Source: "person-edit",
	})
	require.NoError(t, err)
	assert.Equal(t, "新名字", person.StageName)
	assert.Equal(t, "manual", person.StageSource)
}

func TestActorStageCompositeGuard(t *testing.T) {
	eng := newTranslationTestEngine(t)

	_, err := eng.SaveActorAlias(ActorAliasInput{
		Names: []string{"ゆい はつみ"}, Provider: "MINNANO", ActorID: "42",
	})
	require.NoError(t, err)

	// person-edit 提交「X（当前艺名）」复合名（全角/半角括号）→ 不覆盖。
	for _, composite := range []string{"柚井初美（ゆい はつみ）", "柚井初美(ゆい はつみ)"} {
		person, err := eng.SaveActorAlias(ActorAliasInput{
			Names: []string{composite}, Provider: "MINNANO", ActorID: "42", Source: "person-edit",
		})
		require.NoError(t, err)
		assert.Equal(t, "ゆい はつみ", person.StageName, "composite: %q", composite)
		assert.Equal(t, "", person.StageSource)
	}

	// 括号内不是当前艺名 → 正常覆盖。
	person, err := eng.SaveActorAlias(ActorAliasInput{
		Names: []string{"测试（别的名字）"}, Provider: "MINNANO", ActorID: "42", Source: "person-edit",
	})
	require.NoError(t, err)
	assert.Equal(t, "测试（别的名字）", person.StageName)

	// 普通新名可以覆盖。
	person, err = eng.SaveActorAlias(ActorAliasInput{
		Names: []string{"柚井初美"}, Provider: "MINNANO", ActorID: "42", Source: "person-edit",
	})
	require.NoError(t, err)
	assert.Equal(t, "柚井初美", person.StageName)
	assert.Equal(t, "manual", person.StageSource)
}

func TestFindActorPersonsByNameAmbiguous(t *testing.T) {
	eng := newTranslationTestEngine(t)

	for _, id := range []string{"1", "2"} {
		_, err := eng.SaveActorAlias(ActorAliasInput{
			Names: []string{"同名"}, Provider: "MINNANO", ActorID: id,
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
		Names: []string{"ゆい はつみ"}, Provider: "MINNANO", ActorID: "1",
	})
	require.NoError(t, err)
	second, err := eng.SaveActorAlias(ActorAliasInput{
		Names: []string{"ゆい初美"}, Provider: "AVBASE", ActorID: "2",
	})
	require.NoError(t, err)

	merged, err := eng.MergeActorPersons([]uint{first.ID, second.ID}, "柚井初美")
	require.NoError(t, err)
	assert.Equal(t, first.ID, merged.ID)
	assert.Equal(t, "柚井初美", merged.StageName)
	assert.Equal(t, "manual", merged.StageSource)

	var count int64
	require.NoError(t, eng.db.Model(&model.ActorPerson{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)

	persons, err := eng.FindActorPersonsByName("柚井初美")
	require.NoError(t, err)
	require.Len(t, persons, 1)
	assert.Equal(t, first.ID, persons[0].ID)

	// 合并后手动名不被 auto 升级覆盖。
	again, err := eng.SaveActorAlias(ActorAliasInput{
		Names: []string{"汉字更多的艺名"}, Provider: "MINNANO", ActorID: "1",
	})
	require.NoError(t, err)
	assert.Equal(t, "柚井初美", again.StageName)
}

func TestActorAliasRejectsInvalidInput(t *testing.T) {
	eng := newTranslationTestEngine(t)

	_, err := eng.SaveActorAlias(ActorAliasInput{ActorID: "1"})
	assert.ErrorIs(t, err, ErrActorAliasInvalid)

	_, err = eng.SaveActorAlias(ActorAliasInput{
		Names: []string{"  ", ""}, Provider: "MINNANO", ActorID: "1",
	})
	assert.ErrorIs(t, err, ErrActorAliasInvalid)

	_, err = eng.SaveActorAlias(ActorAliasInput{
		Names: []string{strings.Repeat("柚", 256)}, Provider: "MINNANO", ActorID: "1",
	})
	assert.ErrorIs(t, err, ErrActorAliasNameTooLong)

	_, err = eng.MergeActorPersons([]uint{1}, "")
	assert.ErrorIs(t, err, ErrActorAliasMergeInvalid)
}
