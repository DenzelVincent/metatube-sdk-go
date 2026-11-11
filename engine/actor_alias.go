package engine

import (
	"errors"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"

	"github.com/metatube-community/metatube-sdk-go/model"
)

const maxActorNameLength = 255

var (
	ErrActorAliasInvalid      = errors.New("actor alias: provider, actor id and stage name are required")
	ErrActorAliasNameTooLong  = errors.New("actor alias: name is too long")
	ErrActorAliasMergeInvalid = errors.New("actor alias: at least two person ids are required")
)

// NormalizeActorNameKey 生成名字匹配键：全半角统一（NFKC）、去首尾空白、
// 忽略大小写与所有空白。原始名字单独保存，键只用于匹配。
func NormalizeActorNameKey(name string) string {
	normalized := norm.NFKC.String(strings.TrimSpace(name))
	normalized = strings.ToLower(normalized)
	return strings.Join(strings.Fields(normalized), "")
}

// FindActorPersonsByName 按艺名或译名查演员身份。
// 同名不同人时返回多条，调用方必须按「歧义」处理。
func (e *Engine) FindActorPersonsByName(name string) ([]model.ActorPerson, error) {
	key := NormalizeActorNameKey(name)
	if key == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var persons []model.ActorPerson
	if err := e.db.
		Where("stage_key = ? OR translated_key = ?", key, key).
		Order("id").
		Find(&persons).Error; err != nil {
		return nil, err
	}
	if len(persons) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return persons, nil
}

// ActorAliasInput 是按来源 pid 回写演员身份的入参。
// StageName 是搜索用的艺名（B）；TranslatedName 是显示名（A 或引擎译文）。
// TranslationSource 为 "source"（A）时允许覆盖已有引擎译文。
type ActorAliasInput struct {
	StageName         string
	TranslatedName    string
	TranslationSource string
	Provider          string
	ActorID           string
	Source            string
}

// SaveActorAlias 按 (provider, actor_id) 找到或创建演员身份。
// 艺名稳定不覆盖；译名为空时写入，来自数据源（source）的译名可以覆盖引擎译文。
func (e *Engine) SaveActorAlias(in ActorAliasInput) (*model.ActorPerson, error) {
	stage := strings.TrimSpace(in.StageName)
	provider := strings.TrimSpace(in.Provider)
	actorID := strings.TrimSpace(in.ActorID)
	if stage == "" || provider == "" || actorID == "" {
		return nil, ErrActorAliasInvalid
	}
	if utf8.RuneCountInString(stage) > maxActorNameLength {
		return nil, ErrActorAliasNameTooLong
	}

	translated := strings.TrimSpace(in.TranslatedName)
	if utf8.RuneCountInString(translated) > maxActorNameLength {
		return nil, ErrActorAliasNameTooLong
	}
	source := strings.ToLower(strings.TrimSpace(in.TranslationSource))
	if source != "source" {
		source = "engine"
	}

	var person model.ActorPerson
	err := e.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("provider = ? AND actor_id = ?", provider, actorID).First(&person).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			person = model.ActorPerson{
				StageName: stage,
				StageKey:  NormalizeActorNameKey(stage),
				Provider:  provider,
				ActorID:   actorID,
			}
			if translated != "" {
				person.TranslatedName = translated
				person.TranslatedKey = NormalizeActorNameKey(translated)
				person.TranslationSource = source
			}
			return tx.Create(&person).Error
		}
		if err != nil {
			return err
		}

		updates := map[string]any{}
		if person.StageName == "" {
			person.StageName = stage
			person.StageKey = NormalizeActorNameKey(stage)
			updates["display_name"] = person.StageName
			updates["stage_key"] = person.StageKey
		}
		// 译名只在为空时补写；数据源给的译名（A）优先于引擎译文。
		if translated != "" && person.TranslatedName != translated &&
			(person.TranslatedName == "" || source == "source") {
			person.TranslatedName = translated
			person.TranslatedKey = NormalizeActorNameKey(translated)
			person.TranslationSource = source
			updates["translated_name"] = translated
			updates["translated_key"] = person.TranslatedKey
			updates["translation_source"] = source
		}
		if len(updates) == 0 {
			return nil
		}
		return tx.Model(&model.ActorPerson{}).Where("id = ?", person.ID).Updates(updates).Error
	})
	if err != nil {
		return nil, err
	}
	return &person, nil
}

// MergeActorPersons 手工把多个身份合并到 id 最小的那个，可同时更新艺名/译名。
func (e *Engine) MergeActorPersons(personIDs []uint, stageName, translatedName string) (*model.ActorPerson, error) {
	if len(personIDs) < 2 {
		return nil, ErrActorAliasMergeInvalid
	}

	var target model.ActorPerson
	err := e.db.Transaction(func(tx *gorm.DB) error {
		var persons []model.ActorPerson
		if err := tx.Where("id IN ?", personIDs).Order("id").Find(&persons).Error; err != nil {
			return err
		}
		if len(persons) < 2 {
			return gorm.ErrRecordNotFound
		}
		target = persons[0]

		for _, source := range persons[1:] {
			if err := tx.Delete(&model.ActorPerson{}, source.ID).Error; err != nil {
				return err
			}
		}

		updates := map[string]any{}
		if name := strings.TrimSpace(stageName); name != "" {
			target.StageName = name
			target.StageKey = NormalizeActorNameKey(name)
			updates["display_name"] = name
			updates["stage_key"] = target.StageKey
		}
		if translated := strings.TrimSpace(translatedName); translated != "" {
			target.TranslatedName = translated
			target.TranslatedKey = NormalizeActorNameKey(translated)
			target.TranslationSource = "source"
			updates["translated_name"] = translated
			updates["translated_key"] = target.TranslatedKey
			updates["translation_source"] = "source"
		}
		if len(updates) == 0 {
			return nil
		}
		return tx.Model(&model.ActorPerson{}).Where("id = ?", target.ID).Updates(updates).Error
	})
	if err != nil {
		return nil, err
	}
	return &target, nil
}
