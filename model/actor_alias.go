package model

const (
	ActorPersonTableName = "actor_person"
	ActorNameTableName   = "actor_name"
)

// 演员名字的角色。同一个人的艺名、假名读音、英文拼音、译名与改名后的旧名
// 都挂在同一个 ActorPerson 下，按角色区分用途。
const (
	ActorNameRoleStage       = "stage"
	ActorNameRoleKana        = "kana"
	ActorNameRoleRomaji      = "romaji"
	ActorNameRoleAlias       = "alias"
	ActorNameRoleTranslation = "translation"
)

// ActorPerson 是一个演员身份。Provider + ActorID 是首选来源里的 pid，
// 同一身份可以有多个来源的 pid（见 ActorName 的 Provider/ActorID）。
type ActorPerson struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	DisplayName string `json:"display_name" gorm:"size:255;not null"`
	Provider    string `json:"provider" gorm:"size:32;not null;uniqueIndex:idx_actor_person_identity,priority:1"`
	ActorID     string `json:"actor_id" gorm:"size:255;not null;uniqueIndex:idx_actor_person_identity,priority:2"`
	TimeTracker `json:"-"`
}

// ActorName 是挂在演员身份下的一条名字。Name 原样保存（可能带括号读音），
// NameKey 是匹配用的归一化键；Role 决定它是艺名、读音、拼音、译名还是旧名。
// 同一个 NameKey 允许出现在多个身份下（同名不同人），解析时视为歧义。
type ActorName struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	PersonID    uint   `json:"person_id" gorm:"not null;index;uniqueIndex:idx_actor_name_unique,priority:1"`
	Name        string `json:"name" gorm:"size:255;not null"`
	NameKey     string `json:"name_key" gorm:"size:255;not null;index;uniqueIndex:idx_actor_name_unique,priority:2"`
	Role        string `json:"role" gorm:"size:16;not null;uniqueIndex:idx_actor_name_unique,priority:3"`
	Lang        string `json:"lang" gorm:"size:16;uniqueIndex:idx_actor_name_unique,priority:4"`
	Provider    string `json:"provider" gorm:"size:32"`
	ActorID     string `json:"actor_id" gorm:"size:255"`
	Source      string `json:"source" gorm:"size:32"`
	TimeTracker `json:"-"`
}

func (*ActorPerson) TableName() string {
	return ActorPersonTableName
}

func (*ActorName) TableName() string {
	return ActorNameTableName
}

func IsValidActorNameRole(role string) bool {
	switch role {
	case ActorNameRoleStage,
		ActorNameRoleKana,
		ActorNameRoleRomaji,
		ActorNameRoleAlias,
		ActorNameRoleTranslation:
		return true
	default:
		return false
	}
}
