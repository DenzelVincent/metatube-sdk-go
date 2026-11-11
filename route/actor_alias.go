package route

import (
	goerr "errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/metatube-community/metatube-sdk-go/engine"
	"github.com/metatube-community/metatube-sdk-go/model"
)

type actorAliasQuery struct {
	Name string `form:"name" binding:"required"`
}

type actorAliasPersonDTO struct {
	ID             uint   `json:"id"`
	StageName      string `json:"stage_name"`
	TranslatedName string `json:"translated_name,omitempty"`
	Provider       string `json:"provider"`
	ActorID        string `json:"actor_id"`
}

type actorAliasResponse struct {
	Found     bool                  `json:"found"`
	Ambiguous bool                  `json:"ambiguous,omitempty"`
	Person    *actorAliasPersonDTO  `json:"person,omitempty"`
	Matches   []actorAliasPersonDTO `json:"matches,omitempty"`
}

type actorAliasSaveRequest struct {
	StageName         string `json:"stage_name" binding:"required"`
	TranslatedName    string `json:"translated_name"`
	TranslationSource string `json:"translation_source"`
	Provider          string `json:"provider" binding:"required"`
	ActorID           string `json:"actor_id" binding:"required"`
	Source            string `json:"source"`
}

type actorAliasMergeRequest struct {
	PersonIDs      []uint `json:"person_ids" binding:"required"`
	StageName      string `json:"stage_name"`
	TranslatedName string `json:"translated_name"`
}

type actorAliasLookupRequest struct {
	Names []string `json:"names" binding:"required"`
}

type actorAliasLookupItem struct {
	Name      string                `json:"name"`
	Found     bool                  `json:"found"`
	Ambiguous bool                  `json:"ambiguous,omitempty"`
	Person    *actorAliasPersonDTO  `json:"person,omitempty"`
	Matches   []actorAliasPersonDTO `json:"matches,omitempty"`
}

type actorAliasLookupResponse struct {
	Items []actorAliasLookupItem `json:"items"`
}

// lookupActorAliases 批量按名字查身份，一次请求覆盖整部影片的演员，
// 避免每位演员最多 3 次单条查询。
func lookupActorAliases(app *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body actorAliasLookupRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			abortWithStatusMessage(c, http.StatusBadRequest, err)
			return
		}
		if len(body.Names) == 0 || len(body.Names) > 200 {
			abortWithStatusMessage(c, http.StatusBadRequest, goerr.New("names must contain 1-200 entries"))
			return
		}

		items := make([]actorAliasLookupItem, 0, len(body.Names))
		seen := make(map[string]struct{}, len(body.Names))
		for _, rawName := range body.Names {
			name := strings.TrimSpace(rawName)
			if name == "" {
				continue
			}
			key := strings.ToLower(name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}

			item := actorAliasLookupItem{Name: name}
			persons, err := app.FindActorPersonsByName(name)
			switch {
			case goerr.Is(err, gorm.ErrRecordNotFound):
				items = append(items, item)
				continue
			case err != nil:
				abortWithStatusMessage(c, http.StatusInternalServerError, err)
				return
			case len(persons) > 1:
				item.Found = true
				item.Ambiguous = true
				for _, person := range persons {
					item.Matches = append(item.Matches, toActorAliasPersonDTO(person))
				}
			default:
				item.Found = true
				person := toActorAliasPersonDTO(persons[0])
				item.Person = &person
			}
			items = append(items, item)
		}

		if len(items) == 0 {
			abortWithStatusMessage(c, http.StatusBadRequest, goerr.New("names are all blank"))
			return
		}

		c.JSON(http.StatusOK, &responseMessage{
			Data: &actorAliasLookupResponse{Items: items},
		})
	}
}

func getActorAlias(app *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var query actorAliasQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			abortWithStatusMessage(c, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(query.Name) == "" {
			abortWithStatusMessage(c, http.StatusBadRequest, goerr.New("name is required"))
			return
		}

		persons, err := app.FindActorPersonsByName(query.Name)
		switch {
		case goerr.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusOK, &responseMessage{
				Data: &actorAliasResponse{Found: false},
			})
			return
		case err != nil:
			abortWithStatusMessage(c, http.StatusInternalServerError, err)
			return
		}

		if len(persons) > 1 {
			matches := make([]actorAliasPersonDTO, 0, len(persons))
			for _, person := range persons {
				matches = append(matches, toActorAliasPersonDTO(person))
			}
			c.JSON(http.StatusOK, &responseMessage{
				Data: &actorAliasResponse{
					Found:     true,
					Ambiguous: true,
					Matches:   matches,
				},
			})
			return
		}

		person := toActorAliasPersonDTO(persons[0])
		c.JSON(http.StatusOK, &responseMessage{
			Data: &actorAliasResponse{Found: true, Person: &person},
		})
	}
}

func putActorAlias(app *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body actorAliasSaveRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			abortWithStatusMessage(c, http.StatusBadRequest, err)
			return
		}

		person, err := app.SaveActorAlias(engine.ActorAliasInput{
			StageName:         body.StageName,
			TranslatedName:    body.TranslatedName,
			TranslationSource: body.TranslationSource,
			Provider:          body.Provider,
			ActorID:           body.ActorID,
			Source:            body.Source,
		})
		switch {
		case goerr.Is(err, engine.ErrActorAliasInvalid),
			goerr.Is(err, engine.ErrActorAliasNameTooLong):
			abortWithStatusMessage(c, http.StatusBadRequest, err)
			return
		case err != nil:
			abortWithStatusMessage(c, http.StatusInternalServerError, err)
			return
		}

		dto := toActorAliasPersonDTO(*person)
		c.JSON(http.StatusOK, &responseMessage{
			Data: &actorAliasResponse{Found: true, Person: &dto},
		})
	}
}

func mergeActorAliases(app *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body actorAliasMergeRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			abortWithStatusMessage(c, http.StatusBadRequest, err)
			return
		}

		person, err := app.MergeActorPersons(body.PersonIDs, body.StageName, body.TranslatedName)
		switch {
		case goerr.Is(err, engine.ErrActorAliasMergeInvalid):
			abortWithStatusMessage(c, http.StatusBadRequest, err)
			return
		case goerr.Is(err, gorm.ErrRecordNotFound):
			abortWithStatusMessage(c, http.StatusNotFound, err)
			return
		case err != nil:
			abortWithStatusMessage(c, http.StatusInternalServerError, err)
			return
		}

		dto := toActorAliasPersonDTO(*person)
		c.JSON(http.StatusOK, &responseMessage{
			Data: &actorAliasResponse{Found: true, Person: &dto},
		})
	}
}

func toActorAliasPersonDTO(person model.ActorPerson) actorAliasPersonDTO {
	return actorAliasPersonDTO{
		ID:             person.ID,
		StageName:      person.StageName,
		TranslatedName: person.TranslatedName,
		Provider:       person.Provider,
		ActorID:        person.ActorID,
	}
}
