package engine

import (
	"fmt"

	"github.com/metatube-community/metatube-sdk-go/database"
	"github.com/metatube-community/metatube-sdk-go/model"
)

func (e *Engine) DBAutoMigrate(v bool) error {
	if !v {
		return nil
	}
	// Create Case-Insensitive Collation for Postgres.
	if e.DBDriver() == database.Postgres {
		e.db.Exec(`CREATE COLLATION IF NOT EXISTS NOCASE (
		provider = icu,
		locale = 'und-u-ks-level2',
		deterministic = FALSE)`)
	}
	if err := e.db.AutoMigrate(
		&model.MovieInfo{},
		&model.ActorInfo{},
		&model.MovieReviewInfo{},
		&model.TranslationInfo{},
		&model.ActorPerson{},
	); err != nil {
		return err
	}

	// v1.8.5 的 actor_name 明细表已被「person 存艺名」取代，数据可重新学习。
	_ = e.db.Migrator().DropTable("actor_name")
	if err := e.dropActorTranslationColumns(); err != nil {
		return err
	}
	e.backfillActorPersonKeys()
	return nil
}

// dropActorTranslationColumns 物理删除 v1.8.9 的三个译名列并清空演员术语：
// 演员不再翻译，只留优选艺名，不留脏数据。幂等，可重复执行。
func (e *Engine) dropActorTranslationColumns() error {
	migrator := e.db.Migrator()
	// translated_key 上有索引，SQLite 删列前必须先删索引。
	if migrator.HasIndex(&model.ActorPerson{}, "idx_actor_person_translated_key") {
		if err := migrator.DropIndex(&model.ActorPerson{}, "idx_actor_person_translated_key"); err != nil {
			return err
		}
	}
	for _, column := range []string{"translated_name", "translated_key", "translation_source"} {
		if !migrator.HasColumn(&model.ActorPerson{}, column) {
			continue
		}
		if err := migrator.DropColumn(&model.ActorPerson{}, column); err != nil {
			return err
		}
	}
	return e.db.Where("kind = ?", "actor").Delete(&model.TranslationInfo{}).Error
}

// backfillActorPersonKeys 给升级前写入的 actor_person 补匹配键。
func (e *Engine) backfillActorPersonKeys() {
	var persons []model.ActorPerson
	if err := e.db.Where("stage_key = '' OR stage_key IS NULL").Find(&persons).Error; err != nil {
		return
	}
	for _, person := range persons {
		e.db.Model(&model.ActorPerson{}).Where("id = ?", person.ID).
			Update("stage_key", NormalizeActorNameKey(person.StageName))
	}
}

func (e *Engine) DBDriver() string {
	return e.db.Name()
}

func (e *Engine) DBVersion() (version string, err error) {
	switch dbType := e.DBDriver(); dbType {
	case database.Postgres:
		err = e.db.Raw("SELECT version();").Scan(&version).Error
	case database.Sqlite:
		err = e.db.Raw("SELECT sqlite_version();").Scan(&version).Error
	default:
		err = fmt.Errorf("unsupported DB type: %s", dbType)
	}
	return
}
