package engine

import (
	"errors"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/metatube-community/metatube-sdk-go/model"
)

const maxActorNameLength = 255

var (
	ErrActorAliasInvalid      = errors.New("actor alias: provider and actor id are required")
	ErrActorAliasInvalidRole  = errors.New("actor alias: invalid name role")
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

// FindActorPersonsByName 按名字（任意角色、任意语言）查演员身份。
// 同名不同人时会返回多条，调用方必须按「歧义」处理，不能自动挑一个。
func (e *Engine) FindActorPersonsByName(name string) ([]model.ActorPerson, error) {
	key := NormalizeActorNameKey(name)
	if key == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var personIDs []uint
	if err := e.db.Model(&model.ActorName{}).
		Where("name_key = ?", key).
		Distinct().
		Pluck("person_id", &personIDs).Error; err != nil {
		return nil, err
	}
	if len(personIDs) == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	var persons []model.ActorPerson
	if err := e.db.Where("id IN ?", personIDs).Order("id").Find(&persons).Error; err != nil {
		return nil, err
	}
	return persons, nil
}

// GetActorNames 返回某个演员身份下的全部名字（含译名行）。
func (e *Engine) GetActorNames(personID uint) ([]model.ActorName, error) {
	var names []model.ActorName
	err := e.db.Where("person_id = ?", personID).Order("id").Find(&names).Error
	return names, err
}

// ActorAliasNameInput 是写入的一条名字；Role 取 model.ActorNameRole* 之一，
// Lang 只在译名行使用（如 zh-CN）。
type ActorAliasNameInput struct {
	Name string
	Role string
	Lang string
}

// ActorAliasInput 是按来源 pid 回写演员身份的入参。
// Provider + ActorID 是身份主键；DisplayName 是首选显示名（汉字/艺名）。
type ActorAliasInput struct {
	DisplayName string
	Provider    string
	ActorID     string
	Source      string
	Names       []ActorAliasNameInput
}

// SaveActorAlias 按 (provider, actor_id) 找到或创建演员身份，再挂上名字。
// 同一个名字键已经属于另一个身份时不自动合并，跳过并作为 conflict 返回。
func (e *Engine) SaveActorAlias(in ActorAliasInput) (*model.ActorPerson, []model.ActorName, []string, error) {
	provider := strings.TrimSpace(in.Provider)
	actorID := strings.TrimSpace(in.ActorID)
	if provider == "" || actorID == "" {
		return nil, nil, nil, ErrActorAliasInvalid
	}
	displayName := strings.TrimSpace(in.DisplayName)

	var person model.ActorPerson
	var conflicts []string

	err := e.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("provider = ? AND actor_id = ?", provider, actorID).First(&person).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			person = model.ActorPerson{
				DisplayName: displayName,
				Provider:    provider,
				ActorID:     actorID,
			}
			if person.DisplayName == "" {
				person.DisplayName = actorID
			}
			if err := tx.Create(&person).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		for _, item := range in.Names {
			name := strings.TrimSpace(item.Name)
			if name == "" {
				continue
			}
			if utf8.RuneCountInString(name) > maxActorNameLength {
				return ErrActorAliasNameTooLong
			}

			role := strings.ToLower(strings.TrimSpace(item.Role))
			if !model.IsValidActorNameRole(role) {
				return ErrActorAliasInvalidRole
			}
			lang := strings.ToLower(strings.TrimSpace(item.Lang))
			if role != model.ActorNameRoleTranslation {
				lang = ""
			}
			key := NormalizeActorNameKey(name)
			if key == "" {
				continue
			}

			// 同名不同人：保留歧义，不把这条名字挂到当前身份上。
			var owners []uint
			if err := tx.Model(&model.ActorName{}).
				Where("name_key = ? AND person_id <> ?", key, person.ID).
				Limit(1).
				Pluck("person_id", &owners).Error; err != nil {
				return err
			}
			if len(owners) > 0 {
				conflicts = append(conflicts, name)
				continue
			}

			row := model.ActorName{
				PersonID: person.ID,
				Name:     name,
				NameKey:  key,
				Role:     role,
				Lang:     lang,
				Source:   strings.TrimSpace(in.Source),
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	}

	names, err := e.GetActorNames(person.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	return &person, names, conflicts, nil
}

// MergeActorPersons 手工把多个身份合并到 id 最小的那个，可同时改显示名。
// 名字只移动不删除；两边存在同名同角色的行时保留目标身份的那一行。
func (e *Engine) MergeActorPersons(personIDs []uint, displayName string) (*model.ActorPerson, []model.ActorName, error) {
	if len(personIDs) < 2 {
		return nil, nil, ErrActorAliasMergeInvalid
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
			var rows []model.ActorName
			if err := tx.Where("person_id = ?", source.ID).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				var duplicated int64
				if err := tx.Model(&model.ActorName{}).
					Where("person_id = ? AND name_key = ? AND role = ? AND lang = ?",
						target.ID, row.NameKey, row.Role, row.Lang).
					Count(&duplicated).Error; err != nil {
					return err
				}
				if duplicated > 0 {
					continue
				}
				if err := tx.Model(&model.ActorName{}).
					Where("id = ?", row.ID).
					Update("person_id", target.ID).Error; err != nil {
					return err
				}
			}
			if err := tx.Delete(&model.ActorPerson{}, source.ID).Error; err != nil {
				return err
			}
		}

		if name := strings.TrimSpace(displayName); name != "" {
			target.DisplayName = name
			if err := tx.Model(&model.ActorPerson{}).
				Where("id = ?", target.ID).
				Update("display_name", name).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	names, err := e.GetActorNames(target.ID)
	if err != nil {
		return nil, nil, err
	}
	return &target, names, nil
}
