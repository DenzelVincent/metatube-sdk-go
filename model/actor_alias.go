package model

const ActorPersonTableName = "actor_person"

// ActorPerson 是演员身份：一个艺名（搜索名）+ 一个译名。
//
// StageName 是影片接口里用于搜索的 B（含假名的艺名）；TranslatedName 是显示/合集名
// 用的译名（A 或引擎译文）。改名/多别名不再单独建表，身份以 provider + actor_id 为准。
// StageName 复用 v1.8.5 的 display_name 列，避免已有数据迁移。
type ActorPerson struct {
	ID                uint   `json:"id" gorm:"primaryKey"`
	StageName         string `json:"stage_name" gorm:"column:display_name;size:255;not null"`
	StageKey          string `json:"-" gorm:"size:255;not null;index"`
	TranslatedName    string `json:"translated_name" gorm:"size:255"`
	TranslatedKey     string `json:"-" gorm:"size:255;index"`
	TranslationSource string `json:"-" gorm:"size:16"`
	Provider          string `json:"provider" gorm:"size:32;not null;uniqueIndex:idx_actor_person_identity,priority:1"`
	ActorID           string `json:"actor_id" gorm:"size:255;not null;uniqueIndex:idx_actor_person_identity,priority:2"`
	TimeTracker       `json:"-"`
}

func (*ActorPerson) TableName() string {
	return ActorPersonTableName
}
