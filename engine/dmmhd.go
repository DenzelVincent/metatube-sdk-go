package engine

import (
	"regexp"

	"github.com/metatube-community/metatube-sdk-go/model"
)

// dmmCIDPattern extracts a work's cid from either DMM CDN. Only the host is
// case-insensitive: cids on DMM are always lowercase, and a mixed-case value
// would produce a URL that does not exist.
var dmmCIDPattern = regexp.MustCompile(`(?i:https?://(?:pics\.dmm\.co\.jp/digital/video|awsimgsrc\.dmm\.co\.jp/pics_dig/digital/video))/([a-z0-9_]+)/`)

// dmmHDCoverURL returns the high-definition cover URL for a movie, or "" when
// the work's cid cannot be resolved (callers keep the original URL then).
//
// JavBus/JAV321/FANZA return covers from the old DMM CDN (pics.dmm.co.jp,
// 800x538). The same work is available as 2184x1468 on the new CDN
// (awsimgsrc.dmm.co.jp, same "pl" file name), which is what Jellyfin's 2:3
// poster crop needs. Some sources serve their own CDN for the cover but still
// return DMM-hosted sample images (which carry the cid), so preview images are
// used as a fallback.
func dmmHDCoverURL(info *model.MovieInfo) string {
	if info == nil {
		return ""
	}
	if cid := dmmCID(info.CoverURL); cid != "" {
		return dmmHDURL(cid)
	}
	for _, preview := range info.PreviewImages {
		if cid := dmmCID(preview); cid != "" {
			return dmmHDURL(cid)
		}
	}
	return ""
}

func dmmCID(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	if m := dmmCIDPattern.FindStringSubmatch(rawURL); m != nil {
		return m[1]
	}
	return ""
}

func dmmHDURL(cid string) string {
	return "https://awsimgsrc.dmm.co.jp/pics_dig/digital/video/" + cid + "/" + cid + "pl.jpg"
}

// applyDmmHDCover swaps CoverURL for the HD version when the feature is on.
// The rewrite is applied on read only (never persisted): turning the switch
// back off restores the original URL for cached entries instead of leaving
// them upgraded, and the stored 800px URL stays available as a fallback.
func (e *Engine) applyDmmHDCover(info *model.MovieInfo) {
	if !e.dmmHDCover || info == nil {
		return
	}
	if hd := dmmHDCoverURL(info); hd != "" && hd != info.CoverURL {
		info.CoverURL = hd
	}
}
