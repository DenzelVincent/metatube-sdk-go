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
	ErrActorAliasInvalid      = errors.New("actor alias: provider, actor id and names are required")
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

// countKanji 统计汉字数：CJK 统一表意文字（U+4E00–U+9FFF）、扩展 A
// （U+3400–U+4DBF）、兼容表意文字（U+F900–U+FAFF）；假名/罗马字计 0。
func countKanji(s string) int {
	n := 0
	for _, r := range s {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF,
			r >= 0x3400 && r <= 0x4DBF,
			r >= 0xF900 && r <= 0xFAFF:
			n++
		}
	}
	return n
}

// selectStageName 从候选池优选艺名：汉字最多者胜出，平手取池内第一个。
// 入池前按匹配键去重（保留首次出现的写法），空白项剔除；空池返回空串。
// 插件按固定顺序入池（详情名→别名→影片汉字名→搜索名），服务端不重排。
func selectStageName(names []string) string {
	best := ""
	bestKanji := -1
	seen := make(map[string]struct{}, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		key := NormalizeActorNameKey(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if n := countKanji(name); n > bestKanji {
			best, bestKanji = name, n
		}
	}
	return best
}

// isCompositeStageName 识别「X（当前艺名）」遗留显示名（全角/半角括号都认）。
// 手动改名写回遇到这种复合名要跳过：旧「译名（艺名）」Person 同步回来时，
// 不能把复合体写进字典当艺名。
func isCompositeStageName(candidate, current string) bool {
	if current == "" {
		return false
	}
	runes := []rune(candidate)
	if len(runes) < 3 {
		return false
	}
	if last := runes[len(runes)-1]; last != '）' && last != ')' {
		return false
	}
	// i > 0 保证括号前还有非空内容 X。
	for i := len(runes) - 2; i > 0; i-- {
		if runes[i] != '（' && runes[i] != '(' {
			continue
		}
		inner := string(runes[i+1 : len(runes)-1])
		prefix := strings.TrimSpace(string(runes[:i]))
		return prefix != "" && NormalizeActorNameKey(inner) == NormalizeActorNameKey(current)
	}
	return false
}

// FindActorPersonsByName 按艺名键查演员身份。
// 同名不同人时返回多条，调用方必须按「歧义」处理。
func (e *Engine) FindActorPersonsByName(name string) ([]model.ActorPerson, error) {
	key := NormalizeActorNameKey(name)
	if key == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var persons []model.ActorPerson
	if err := e.db.
		Where("stage_key = ?", key).
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
// Names 是候选艺名池，服务端按汉字数优选；Source 为 "person-edit"
// （手动改名写回）时标 manual 并允许覆盖，其余值按 auto 处理。
type ActorAliasInput struct {
	Names    []string
	Provider string
	ActorID  string
	Source   string
}

// SaveActorAlias 按 (provider, actor_id) 找到或创建演员身份并优选艺名。
// 覆盖策略：新身份直接写；manual 只认 person-edit；auto 仅汉字严格更多才升级；
// person-edit 提交「X（当前艺名）」复合名时跳过。无变化时不写库（幂等）。
func (e *Engine) SaveActorAlias(in ActorAliasInput) (*model.ActorPerson, error) {
	provider := strings.TrimSpace(in.Provider)
	actorID := strings.TrimSpace(in.ActorID)
	stage := selectStageName(in.Names)
	if stage == "" || provider == "" || actorID == "" {
		return nil, ErrActorAliasInvalid
	}
	if utf8.RuneCountInString(stage) > maxActorNameLength {
		return nil, ErrActorAliasNameTooLong
	}
	personEdit := strings.EqualFold(strings.TrimSpace(in.Source), "person-edit")

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
			if personEdit {
				person.StageSource = model.ActorStageSourceManual
			}
			return tx.Create(&person).Error
		}
		if err != nil {
			return err
		}

		if personEdit && isCompositeStageName(stage, person.StageName) {
			return nil
		}
		switch {
		case person.StageName == "":
			// 历史空名兜底补写。
		case person.StageSource == model.ActorStageSourceManual && !personEdit:
			return nil
		case personEdit:
			// 手动改名直接覆盖，保持 manual。
		case countKanji(stage) > countKanji(person.StageName):
			// 自动升级：汉字严格更多。
		default:
			return nil
		}

		person.StageName = stage
		person.StageKey = NormalizeActorNameKey(stage)
		if personEdit {
			person.StageSource = model.ActorStageSourceManual
		}
		return tx.Model(&model.ActorPerson{}).Where("id = ?", person.ID).Updates(map[string]any{
			"display_name": person.StageName,
			"stage_key":    person.StageKey,
			"stage_source": person.StageSource,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return &person, nil
}

// MergeActorPersons 手工把多个身份合并到 id 最小的那个；给了艺名就标 manual。
func (e *Engine) MergeActorPersons(personIDs []uint, stageName string) (*model.ActorPerson, error) {
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

		name := strings.TrimSpace(stageName)
		if name == "" {
			return nil
		}
		target.StageName = name
		target.StageKey = NormalizeActorNameKey(name)
		target.StageSource = model.ActorStageSourceManual
		return tx.Model(&model.ActorPerson{}).Where("id = ?", target.ID).Updates(map[string]any{
			"display_name": name,
			"stage_key":    target.StageKey,
			"stage_source": target.StageSource,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return &target, nil
}
