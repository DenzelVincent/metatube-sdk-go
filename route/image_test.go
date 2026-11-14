package route

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/metatube-community/metatube-sdk-go/database"
	"github.com/metatube-community/metatube-sdk-go/engine"
	"github.com/metatube-community/metatube-sdk-go/model"
)

func newImageRouteEngine(t *testing.T) (*engine.Engine, *gorm.DB) {
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
	return app, db
}

// stripedImage 生成三等分色带图，用裁切结果的主色判断裁切窗落在左/中/右。
// horizontal=false 按 x 分红绿蓝（横图），true 按 y 分红绿蓝（竖图）。
func stripedImage(width, height int, horizontal bool) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			band := x * 3 / width
			if horizontal {
				band = y * 3 / height
			}
			var c color.RGBA
			switch band {
			case 0:
				c = color.RGBA{R: 255, A: 255}
			case 1:
				c = color.RGBA{G: 255, A: 255}
			default:
				c = color.RGBA{B: 255, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func dominantColor(img image.Image) string {
	r, g, b, _ := img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2).RGBA()
	switch {
	case r > g && r > b:
		return "red"
	case g > r && g > b:
		return "green"
	default:
		return "blue"
	}
}

// 指定 url 的图片请求必须与各自的非指定 url 路径同口径补默认裁切位置：
// 影片主图右缘、演员主图/缩略图居中。pos=-1 是插件未手动指定位置时的传参，
// 之前被原样当成裁切中心，导致主图裁成左半（DMM 高清封面开关打开后暴露）。
func TestImageURLPositionDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app, db := newImageRouteEngine(t)

	landscape := stripedImage(600, 300, false)
	portrait := stripedImage(300, 600, true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		img := landscape
		if strings.HasSuffix(r.URL.Path, "portrait.jpg") {
			img = portrait
		}
		w.Header().Set("Content-Type", "image/jpeg")
		require.NoError(t, jpeg.Encode(w, img, &jpeg.Options{Quality: 90}))
	}))
	t.Cleanup(srv.Close)

	seed := func(id, number string) {
		t.Helper()
		require.NoError(t, db.Create(&model.MovieInfo{
			ID:       id,
			Number:   number,
			Title:    "test movie",
			Provider: "JavBus",
			Homepage: "https://www.javbus.com/" + id,
			CoverURL: srv.URL + "/landscape.jpg",
		}).Error)
	}
	seed("ABC-123", "ABC-123")
	seed("FC2-123456", "FC2-123456")

	r := gin.New()
	r.GET("/v1/images/primary/:provider/:id", getImage(app, primaryImageType))
	r.GET("/v1/images/thumb/:provider/:id", getImage(app, thumbImageType))

	get := func(t *testing.T, path, query string) image.Image {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path+"?"+query, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		img, err := jpeg.Decode(w.Body)
		require.NoError(t, err)
		return img
	}

	t.Run("movie primary with url defaults to right", func(t *testing.T) {
		img := get(t, "/v1/images/primary/JavBus/ABC-123", "url="+srv.URL+"/landscape.jpg&pos=-1")
		assert.Equal(t, "blue", dominantColor(img))
	})

	t.Run("movie primary with url keeps manual pos", func(t *testing.T) {
		img := get(t, "/v1/images/primary/JavBus/ABC-123", "url="+srv.URL+"/landscape.jpg&pos=0")
		assert.Equal(t, "red", dominantColor(img))
	})

	t.Run("movie primary without url defaults to right", func(t *testing.T) {
		img := get(t, "/v1/images/primary/JavBus/ABC-123", "pos=-1")
		assert.Equal(t, "blue", dominantColor(img))
	})

	t.Run("movie primary with url auto face detection falls back to default", func(t *testing.T) {
		// FC2 番号启用人脸检测；纯色图检不到脸，应回退默认右缘而不是左缘。
		img := get(t, "/v1/images/primary/JavBus/FC2-123456", "url="+srv.URL+"/landscape.jpg&pos=-1")
		assert.Equal(t, "blue", dominantColor(img))
	})

	t.Run("movie thumb with url defaults to center", func(t *testing.T) {
		img := get(t, "/v1/images/thumb/JavBus/ABC-123", "url="+srv.URL+"/portrait.jpg&pos=-1")
		assert.Equal(t, "green", dominantColor(img))
	})

	t.Run("actor primary with url defaults to center", func(t *testing.T) {
		img := get(t, "/v1/images/primary/Gfriends/test-actor", "url="+srv.URL+"/landscape.jpg&pos=-1")
		assert.Equal(t, "green", dominantColor(img))
	})
}
