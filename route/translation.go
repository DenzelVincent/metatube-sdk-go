package route

import (
	goerr "errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/metatube-community/metatube-sdk-go/engine"
	"github.com/metatube-community/metatube-sdk-go/model"
)

const maxTranslationTermLength = 512

type translationTermQuery struct {
	Kind       string `form:"kind" binding:"required"`
	TargetLang string `form:"target_lang" binding:"required"`
	Source     string `form:"source" binding:"required"`
}

type translationTermRequest struct {
	Kind           string `json:"kind" binding:"required"`
	TargetLang     string `json:"target_lang" binding:"required"`
	Source         string `json:"source" binding:"required"`
	TranslatedText string `json:"translated_text" binding:"required"`
	Engine         string `json:"engine"`
}

type translationTermResponse struct {
	Found          bool   `json:"found"`
	TranslatedText string `json:"translated_text,omitempty"`
}

func getTranslationTerm(app *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var query translationTermQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			abortWithStatusMessage(c, http.StatusBadRequest, err)
			return
		}
		if err := validateTranslationTerm(query.Kind, query.TargetLang, query.Source, "", false); err != nil {
			abortWithStatusMessage(c, http.StatusBadRequest, err)
			return
		}

		info, err := app.GetTranslationTerm(query.Kind, query.TargetLang, query.Source)
		switch {
		case goerr.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusOK, &responseMessage{
				Data: &translationTermResponse{Found: false},
			})
		case err != nil:
			abortWithStatusMessage(c, http.StatusInternalServerError, err)
		default:
			c.JSON(http.StatusOK, &responseMessage{
				Data: &translationTermResponse{
					Found:          true,
					TranslatedText: info.TranslatedText,
				},
			})
		}
	}
}

func putTranslationTerm(app *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body translationTermRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			abortWithStatusMessage(c, http.StatusBadRequest, err)
			return
		}
		if err := validateTranslationTerm(body.Kind, body.TargetLang, body.Source, body.TranslatedText, true); err != nil {
			abortWithStatusMessage(c, http.StatusBadRequest, err)
			return
		}

		info, err := app.SaveTranslationTerm(body.Kind, body.TargetLang, body.Source, body.TranslatedText, body.Engine)
		if err != nil {
			abortWithStatusMessage(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, &responseMessage{
			Data: &translationTermResponse{
				Found:          true,
				TranslatedText: info.TranslatedText,
			},
		})
	}
}

func validateTranslationTerm(kind, targetLang, source, translated string, requireTranslated bool) error {
	if !model.IsValidTranslationKind(kind) {
		return fmt.Errorf("invalid translation kind: %q", kind)
	}
	if strings.TrimSpace(targetLang) == "" || utf8.RuneCountInString(strings.TrimSpace(targetLang)) > 16 {
		return fmt.Errorf("invalid target language")
	}
	if strings.TrimSpace(source) == "" || utf8.RuneCountInString(strings.TrimSpace(source)) > maxTranslationTermLength {
		return fmt.Errorf("invalid source")
	}
	if !requireTranslated {
		return nil
	}

	value := strings.TrimSpace(translated)
	if value == "" || utf8.RuneCountInString(value) > maxTranslationTermLength {
		return fmt.Errorf("invalid translated text")
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("translated text contains line breaks")
	}
	return nil
}
