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
	Name       string `form:"name" binding:"required"`
	TargetLang string `form:"target_lang"`
}

type actorAliasNameDTO struct {
	Name string `json:"name"`
	Role string `json:"role"`
	Lang string `json:"lang,omitempty"`
}

type actorAliasPersonDTO struct {
	ID          uint   `json:"id"`
	DisplayName string `json:"display_name"`
	Provider    string `json:"provider"`
	ActorID     string `json:"actor_id"`
}

type actorAliasResponse struct {
	Found          bool                  `json:"found"`
	Ambiguous      bool                  `json:"ambiguous,omitempty"`
	Person         *actorAliasPersonDTO  `json:"person,omitempty"`
	Names          []actorAliasNameDTO   `json:"names,omitempty"`
	TranslatedName string                `json:"translated_name,omitempty"`
	Matches        []actorAliasPersonDTO `json:"matches,omitempty"`
	Conflicts      []string              `json:"conflicts,omitempty"`
}

type actorAliasTranslation struct {
	Lang string `json:"lang" binding:"required"`
	Text string `json:"text" binding:"required"`
}

type actorAliasSaveRequest struct {
	DisplayName string                 `json:"display_name"`
	Provider    string                 `json:"provider" binding:"required"`
	ActorID     string                 `json:"actor_id" binding:"required"`
	Source      string                 `json:"source"`
	Names       []actorAliasNameDTO    `json:"names"`
	Translation *actorAliasTranslation `json:"translation"`
}

type actorAliasMergeRequest struct {
	PersonIDs   []uint `json:"person_ids" binding:"required"`
	DisplayName string `json:"display_name"`
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

		person := persons[0]
		names, err := app.GetActorNames(person.ID)
		if err != nil {
			abortWithStatusMessage(c, http.StatusInternalServerError, err)
			return
		}

		targetLang := strings.ToLower(strings.TrimSpace(query.TargetLang))
		response := &actorAliasResponse{
			Found:  true,
			Person: toActorAliasPersonDTOPtr(person),
			Names:  toActorAliasNameDTOs(names),
		}
		for _, name := range names {
			if name.Role == model.ActorNameRoleTranslation && name.Lang == targetLang {
				response.TranslatedName = name.Name
				break
			}
		}

		c.JSON(http.StatusOK, &responseMessage{Data: response})
	}
}

func putActorAlias(app *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body actorAliasSaveRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			abortWithStatusMessage(c, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(body.Provider) == "" || strings.TrimSpace(body.ActorID) == "" {
			abortWithStatusMessage(c, http.StatusBadRequest, engine.ErrActorAliasInvalid)
			return
		}

		input := engine.ActorAliasInput{
			DisplayName: body.DisplayName,
			Provider:    body.Provider,
			ActorID:     body.ActorID,
			Source:      body.Source,
		}
		for _, name := range body.Names {
			input.Names = append(input.Names, engine.ActorAliasNameInput{
				Name: name.Name,
				Role: name.Role,
				Lang: name.Lang,
			})
		}
		if body.Translation != nil {
			input.Names = append(input.Names, engine.ActorAliasNameInput{
				Name: body.Translation.Text,
				Role: model.ActorNameRoleTranslation,
				Lang: body.Translation.Lang,
			})
		}

		person, names, conflicts, err := app.SaveActorAlias(input)
		switch {
		case goerr.Is(err, engine.ErrActorAliasInvalid),
			goerr.Is(err, engine.ErrActorAliasInvalidRole),
			goerr.Is(err, engine.ErrActorAliasNameTooLong):
			abortWithStatusMessage(c, http.StatusBadRequest, err)
			return
		case err != nil:
			abortWithStatusMessage(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, &responseMessage{
			Data: &actorAliasResponse{
				Found:     true,
				Person:    toActorAliasPersonDTOPtr(*person),
				Names:     toActorAliasNameDTOs(names),
				Conflicts: conflicts,
			},
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

		person, names, err := app.MergeActorPersons(body.PersonIDs, body.DisplayName)
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

		c.JSON(http.StatusOK, &responseMessage{
			Data: &actorAliasResponse{
				Found:  true,
				Person: toActorAliasPersonDTOPtr(*person),
				Names:  toActorAliasNameDTOs(names),
			},
		})
	}
}

func toActorAliasPersonDTO(person model.ActorPerson) actorAliasPersonDTO {
	return actorAliasPersonDTO{
		ID:          person.ID,
		DisplayName: person.DisplayName,
		Provider:    person.Provider,
		ActorID:     person.ActorID,
	}
}

func toActorAliasPersonDTOPtr(person model.ActorPerson) *actorAliasPersonDTO {
	dto := toActorAliasPersonDTO(person)
	return &dto
}

func toActorAliasNameDTOs(names []model.ActorName) []actorAliasNameDTO {
	if len(names) == 0 {
		return nil
	}
	dtos := make([]actorAliasNameDTO, 0, len(names))
	for _, name := range names {
		dtos = append(dtos, actorAliasNameDTO{
			Name: name.Name,
			Role: name.Role,
			Lang: name.Lang,
		})
	}
	return dtos
}
