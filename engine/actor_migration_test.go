package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/metatube-community/metatube-sdk-go/database"
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
