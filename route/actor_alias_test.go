package route

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type actorAliasEnvelope struct {
	Data  actorAliasResponse `json:"data"`
	Error json.RawMessage    `json:"error"`
}

func TestActorAliasRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := newTranslationRouteEngine(t)

	r := gin.New()
	r.GET("/v1/actor-aliases", getActorAlias(app))
	r.PUT("/v1/actor-aliases", putActorAlias(app))
	r.POST("/v1/actor-aliases/merge", mergeActorAliases(app))

	put := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/v1/actor-aliases", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	get := func(query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/actor-aliases?"+query, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	merge := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/actor-aliases/merge", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	decode := func(t *testing.T, w *httptest.ResponseRecorder) actorAliasEnvelope {
		t.Helper()
		var envelope actorAliasEnvelope
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
		return envelope
	}

	t.Run("put then get", func(t *testing.T) {
		w := put(`{
			"stage_name":"ゆい はつみ",
			"translated_name":"柚井初美",
			"translation_source":"source",
			"provider":"MINNANO",
			"actor_id":"42",
			"source":"JavBus"
		}`)
		require.Equal(t, http.StatusOK, w.Code)
		saved := decode(t, w)
		require.True(t, saved.Data.Found)
		require.NotNil(t, saved.Data.Person)
		assert.Equal(t, "ゆい はつみ", saved.Data.Person.StageName)
		assert.Equal(t, "柚井初美", saved.Data.Person.TranslatedName)

		for _, name := range []string{"ゆい+はつみ", "柚井初美"} {
			w = get("name=" + name)
			require.Equal(t, http.StatusOK, w.Code)
			got := decode(t, w)
			require.True(t, got.Data.Found)
			assert.False(t, got.Data.Ambiguous)
			assert.Equal(t, saved.Data.Person.ID, got.Data.Person.ID)
			assert.Equal(t, "柚井初美", got.Data.Person.TranslatedName)
		}

		// 全半角与空白不影响匹配。
		w = get("name=柚井初美（ゆい　はつみ）")
		require.Equal(t, http.StatusOK, w.Code)
		assert.False(t, decode(t, w).Data.Found)
	})

	t.Run("get miss returns found false", func(t *testing.T) {
		w := get("name=Nobody")
		require.Equal(t, http.StatusOK, w.Code)
		assert.False(t, decode(t, w).Data.Found)
	})

	t.Run("same stage name on another person is ambiguous", func(t *testing.T) {
		w := put(`{"stage_name":"ゆい はつみ","provider":"AVBASE","actor_id":"99"}`)
		require.Equal(t, http.StatusOK, w.Code)

		w = get("name=ゆい+はつみ")
		require.Equal(t, http.StatusOK, w.Code)
		got := decode(t, w)
		assert.True(t, got.Data.Ambiguous)
		assert.Len(t, got.Data.Matches, 2)
	})

	t.Run("invalid input returns 400", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, put(`{"provider":"MINNANO","actor_id":"1"}`).Code)
		assert.Equal(t, http.StatusBadRequest, put(`{"stage_name":"x","actor_id":"1"}`).Code)
		assert.Equal(t, http.StatusBadRequest, merge(`{"person_ids":[1]}`).Code)
	})

	t.Run("merge persons", func(t *testing.T) {
		w := put(`{"stage_name":"乙","provider":"MINNANO","actor_id":"200",
			"translated_name":"乙女","translation_source":"source"}`)
		require.Equal(t, http.StatusOK, w.Code)
		second := decode(t, w)

		w = put(`{"stage_name":"おとめ","provider":"AVBASE","actor_id":"201"}`)
		require.Equal(t, http.StatusOK, w.Code)
		third := decode(t, w)

		w = merge(`{"person_ids":[` +
			jsonNumber(second.Data.Person.ID) + `,` + jsonNumber(third.Data.Person.ID) +
			`],"stage_name":"おとめ","translated_name":"乙女"}`)
		require.Equal(t, http.StatusOK, w.Code)
		merged := decode(t, w)
		assert.Equal(t, second.Data.Person.ID, merged.Data.Person.ID)
		assert.Equal(t, "乙女", merged.Data.Person.TranslatedName)

		w = get("name=乙女")
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, second.Data.Person.ID, decode(t, w).Data.Person.ID)
	})
}

func TestActorAliasRoutesRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := newTranslationRouteEngine(t)

	r := New(app, nil)
	found := false
	for _, route := range r.Routes() {
		if route.Path == "/v1/actor-aliases" {
			found = true
			assert.Contains(t, []string{http.MethodGet, http.MethodPut}, route.Method)
		}
	}
	assert.True(t, found, "GET/PUT /v1/actor-aliases should be registered")
}

func jsonNumber(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
