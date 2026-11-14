package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/metatube-community/metatube-sdk-go/database"
	"github.com/metatube-community/metatube-sdk-go/model"
)

// 复现 v1.8.5/v1.8.7 的 actor_person 表结构：只有 display_name/provider/actor_id。
// 新版本 AutoMigrate 必须能在已有数据上补出 stage_key/translated_key 并回填，不能失败。
func TestDBAutoMigrateUpgradesActorPersonFromV1(t *testing.T) {
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	db, err := database.Open(&database.Config{
		DSN:                  fmt.Sprintf("file:%s?mode=memory&cache=shared", name),
		DisableAutomaticPing: true,
	})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE actor_person (
		id integer PRIMARY KEY AUTOINCREMENT,
		display_name text NOT NULL,
		provider text NOT NULL,
		actor_id text NOT NULL,
		created_at datetime,
		updated_at datetime
	)`).Error)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX idx_actor_person_identity ON actor_person(provider, actor_id)`).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO actor_person (display_name, provider, actor_id) VALUES ('ゆい はつみ', 'MINNANO', '42')`).Error)

	eng := New(db)
	require.NoError(t, eng.DBAutoMigrate(true))

	persons, err := eng.FindActorPersonsByName("ゆい はつみ")
	require.NoError(t, err)
	require.Len(t, persons, 1)
	assert.Equal(t, "ゆい はつみ", persons[0].StageName)
}

// 复现 v1.8.9 的 actor_person 结构（含三个译名列）与演员术语行。
// 新迁移必须物理删除三列、补出 stage_source、清空 actor 术语，且艺名数据完好。
func TestDBAutoMigrateDropsActorTranslationColumns(t *testing.T) {
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	db, err := database.Open(&database.Config{
		DSN:                  fmt.Sprintf("file:%s?mode=memory&cache=shared", name),
		DisableAutomaticPing: true,
	})
	require.NoError(t, err)

	// 与 GORM AutoMigrate 生成的真实 v1.8.9 结构一致（列名带反引号）：
	// glebarez/sqlite 的 removeColumn 只识别带引号的列定义。
	require.NoError(t, db.Exec("CREATE TABLE actor_person ("+
		"`id` integer PRIMARY KEY AUTOINCREMENT,"+
		"`display_name` text NOT NULL,"+
		"`stage_key` text NOT NULL DEFAULT '',"+
		"`translated_name` text,"+
		"`translated_key` text,"+
		"`translation_source` text,"+
		"`provider` text NOT NULL,"+
		"`actor_id` text NOT NULL,"+
		"`created_at` datetime,"+
		"`updated_at` datetime)").Error)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX idx_actor_person_identity ON actor_person(provider, actor_id)`).Error)
	require.NoError(t, db.Exec(
		`CREATE INDEX idx_actor_person_stage_key ON actor_person(stage_key)`).Error)
	require.NoError(t, db.Exec(
		`CREATE INDEX idx_actor_person_translated_key ON actor_person(translated_key)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO actor_person
		(display_name, stage_key, translated_name, translated_key, translation_source, provider, actor_id)
		VALUES ('ゆい はつみ', 'ゆいはつみ', '柚井初美', '柚井初美', 'source', 'MINNANO', '42')`).Error)

	require.NoError(t, db.Exec(`CREATE TABLE translation_metadata (
		id integer PRIMARY KEY AUTOINCREMENT,
		kind text NOT NULL,
		target_lang text NOT NULL,
		source_key text NOT NULL,
		source_text text NOT NULL,
		translated_text text NOT NULL,
		engine text,
		created_at datetime,
		updated_at datetime
	)`).Error)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX idx_translation_metadata_term ON translation_metadata(kind, target_lang, source_key)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO translation_metadata
		(kind, target_lang, source_key, source_text, translated_text)
		VALUES ('actor', 'zh-cn', 'yui hatsumi', 'Yui Hatsumi', '柚井初美')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO translation_metadata
		(kind, target_lang, source_key, source_text, translated_text)
		VALUES ('series', 'zh-cn', 'first love', 'First Love', '初恋')`).Error)

	eng := New(db)
	require.NoError(t, eng.DBAutoMigrate(true))

	var cols []struct{ Name string }
	require.NoError(t, db.Raw(`SELECT name FROM pragma_table_info('actor_person')`).Scan(&cols).Error)
	colNames := make(map[string]bool, len(cols))
	for _, c := range cols {
		colNames[c.Name] = true
	}
	assert.False(t, colNames["translated_name"], "translated_name 应被物理删除")
	assert.False(t, colNames["translated_key"], "translated_key 应被物理删除")
	assert.False(t, colNames["translation_source"], "translation_source 应被物理删除")
	assert.True(t, colNames["stage_source"], "stage_source 应被补出")

	persons, err := eng.FindActorPersonsByName("ゆい はつみ")
	require.NoError(t, err)
	require.Len(t, persons, 1)
	assert.Equal(t, "ゆい はつみ", persons[0].StageName)

	var actorTerms, seriesTerms int64
	require.NoError(t, db.Model(&model.TranslationInfo{}).Where("kind = ?", "actor").Count(&actorTerms).Error)
	require.NoError(t, db.Model(&model.TranslationInfo{}).Where("kind = ?", "series").Count(&seriesTerms).Error)
	assert.Zero(t, actorTerms, "actor 术语行应被清空")
	assert.Equal(t, int64(1), seriesTerms, "series 术语行应保留")

	// 幂等：重复迁移不报错。
	require.NoError(t, eng.DBAutoMigrate(true))
}
