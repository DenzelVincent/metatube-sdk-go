package engine

import (
	"testing"

	"github.com/metatube-community/metatube-sdk-go/model"
)

func TestDmmHDCoverURL(t *testing.T) {
	tests := []struct {
		name string
		info *model.MovieInfo
		want string
	}{
		{
			name: "old CDN cover",
			info: &model.MovieInfo{CoverURL: "https://pics.dmm.co.jp/digital/video/abc01234/abc01234pl.jpg"},
			want: "https://awsimgsrc.dmm.co.jp/pics_dig/digital/video/abc01234/abc01234pl.jpg",
		},
		{
			name: "new CDN cover is idempotent",
			info: &model.MovieInfo{CoverURL: "https://awsimgsrc.dmm.co.jp/pics_dig/digital/video/abc01234/abc01234pl.jpg"},
			want: "https://awsimgsrc.dmm.co.jp/pics_dig/digital/video/abc01234/abc01234pl.jpg",
		},
		{
			name: "cid from preview images when cover is not DMM",
			info: &model.MovieInfo{
				CoverURL:      "https://www.javbus.com/pics/cover/abc01234_b.jpg",
				PreviewImages: []string{"https://pics.dmm.co.jp/digital/video/abc01234/abc01234jp-1.jpg"},
			},
			want: "https://awsimgsrc.dmm.co.jp/pics_dig/digital/video/abc01234/abc01234pl.jpg",
		},
		{
			name: "no cid anywhere",
			info: &model.MovieInfo{
				CoverURL:      "https://duga.jp/cover.jpg",
				PreviewImages: []string{"https://duga.jp/sample1.jpg"},
			},
			want: "",
		},
		{
			name: "nil info",
			info: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dmmHDCoverURL(tt.info); got != tt.want {
				t.Fatalf("dmmHDCoverURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyDmmHDCover(t *testing.T) {
	const (
		original = "https://pics.dmm.co.jp/digital/video/abc01234/abc01234pl.jpg"
		hdc      = "https://awsimgsrc.dmm.co.jp/pics_dig/digital/video/abc01234/abc01234pl.jpg"
	)

	info := &model.MovieInfo{CoverURL: original}
	new(Engine).applyDmmHDCover(info) // switch off
	if info.CoverURL != original {
		t.Fatalf("switch off should keep cover, got %q", info.CoverURL)
	}

	(&Engine{dmmHDCover: true}).applyDmmHDCover(info)
	if info.CoverURL != hdc {
		t.Fatalf("switch on should rewrite cover, got %q", info.CoverURL)
	}
}
