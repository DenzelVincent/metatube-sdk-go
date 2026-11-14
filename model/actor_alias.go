package model

const ActorPersonTableName = "actor_person"

// ActorStageSourceManual 标记手动改过的艺名：自动汉字升级不再覆盖它。
const ActorStageSourceManual = "manual"

// ActorPerson 是演员身份：一个优选艺名 + 来源 pid。
//
// StageName 由服务端在候选名字池里按汉字数优选，显示/合集/搜索都用它；
// 身份以 provider + actor_id 为准。StageName 复用 v1.8.5 的 display_name 列，
// 避免已有数据迁移。v1.8.9 的三个译名列已在 DBAutoMigrate 里物理删除。
type ActorPerson struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	StageName   string `json:"stage_name" gorm:"column:display_name;size:255;not null"`
	StageKey    string `json:"-" gorm:"size:255;not null;default:'';index"`
	StageSource string `json:"-" gorm:"size:16;not null;default:''"`
	Provider    string `json:"provider" gorm:"size:32;not null;uniqueIndex:idx_actor_person_identity,priority:1"`
	ActorID     string `json:"actor_id" gorm:"size:255;not null;uniqueIndex:idx_actor_person_identity,priority:2"`
	TimeTracker `json:"-"`
}

func (*ActorPerson) TableName() string {
	return ActorPersonTableName
}
