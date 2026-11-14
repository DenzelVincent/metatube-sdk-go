package route

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/metatube-community/metatube-sdk-go/database"
	"github.com/metatube-community/metatube-sdk-go/engine"
)

type translationTermEnvelope struct {
	Data  translationTermResponse `json:"data"`
	Error json.RawMessage         `json:"error"`
}

func newTranslationRouteEngine(t *testing.T) *engine.Engine {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "-", " ", "-").Replace(t.Name()))
	db, err := database.Open(&database.Config{
		DSN:                  dsn,
		DisableAutomaticPing: true,
	})
	require.NoError(t, err)

	app := engine.New(db)
	require.NoError(t, app.DBAutoMigrate(true))
	return app
}

func TestTranslationTermRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := newTranslationRouteEngine(t)

	r := gin.New()
	r.GET("/v1/translations/term", getTranslationTerm(app))
	r.PUT("/v1/translations/term", putTranslationTerm(app))

	put := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/v1/translations/term", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	get := func(query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/translations/term?"+query, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	decode := func(t *testing.T, w *httptest.ResponseRecorder) translationTermEnvelope {
		t.Helper()
		var envelope translationTermEnvelope
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
		return envelope
	}

	t.Run("put then get", func(t *testing.T) {
		w := put(`{"kind":"series","target_lang":"zh-CN","source":"First Love",
			"translated_text":"初恋","engine":"Baidu"}`)
		require.Equal(t, http.StatusOK, w.Code)
		envelope := decode(t, w)
		assert.True(t, envelope.Data.Found)
		assert.Equal(t, "初恋", envelope.Data.TranslatedText)

		w = get("kind=series&target_lang=zh-CN&source=First+Love")
		require.Equal(t, http.StatusOK, w.Code)
		envelope = decode(t, w)
		assert.True(t, envelope.Data.Found)
		assert.Equal(t, "初恋", envelope.Data.TranslatedText)
	})

	t.Run("put is first-write-wins", func(t *testing.T) {
		w := put(`{"kind":"series","target_lang":"zh-CN","source":"First Love",
			"translated_text":"初恋","engine":"Google"}`)
		require.Equal(t, http.StatusOK, w.Code)

		w = put(`{"kind":"series","target_lang":"zh-CN","source":"First Love",
			"translated_text":"初次恋爱","engine":"Baidu"}`)
		require.Equal(t, http.StatusOK, w.Code)
		envelope := decode(t, w)
		assert.Equal(t, "初恋", envelope.Data.TranslatedText)
	})

	t.Run("get miss returns found false", func(t *testing.T) {
		w := get("kind=series&target_lang=zh-CN&source=Nobody")
		require.Equal(t, http.StatusOK, w.Code)
		envelope := decode(t, w)
		assert.False(t, envelope.Data.Found)
		assert.Empty(t, envelope.Data.TranslatedText)
	})

	t.Run("actor kind is rejected", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, get("kind=actor&target_lang=zh-CN&source=x").Code)
		assert.Equal(t, http.StatusBadRequest,
			put(`{"kind":"actor","target_lang":"zh-CN","source":"x","translated_text":"y"}`).Code)
	})

	t.Run("invalid input returns 400", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, get("kind=unknown&target_lang=zh-CN&source=x").Code)
		assert.Equal(t, http.StatusBadRequest, get("kind=series&target_lang=zh-CN&source=").Code)
		assert.Equal(t, http.StatusBadRequest,
			put(`{"kind":"series","target_lang":"zh-CN","source":"x","translated_text":"第一行\n第二行"}`).Code)
		assert.Equal(t, http.StatusBadRequest,
			put(`{"kind":"series","target_lang":"zh-CN","source":"x","translated_text":"   "}`).Code)
	})
}
