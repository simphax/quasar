package bittorrent

import (
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	"github.com/steeve/pulsar/xbmc"
)

type Torrent struct {
	URI      string   `json:"uri"`
	InfoHash string   `json:"info_hash"`
	Name     string   `json:"name"`
	Trackers []string `json:"trackers"`
	Size     int      `json:"size"`
	Seeds    int      `json:"seeds"`
	Peers    int      `json:"peers"`

	Resolution  int    `json:"resolution"`
	VideoCodec  int    `json:"video_codec"`
	AudioCodec  int    `json:"audio_codec"`
	Language    string `json:"language"`
	RipType     int    `json:"rip_type"`
	SceneRating int    `json:"scene_rating"`
}

const (
	ResolutionUnkown = iota
	Resolution480p
	Resolution720p
	Resolution1080p
)

var Resolutions = []string{"", "480p", "720p", "1080p"}

const (
	RipUnknown = iota
	RipCam
	RipTS
	RipTC
	RipScr
	RipDVDScr
	RipDVD
	RipHDTV
	RipWeb
	RipBluRay
)

var Rips = []string{"", "Cam", "TeleSync", "TeleCine", "Screener", "DVD Screener", "DVDRip", "HDTV", "WebDL", "Blu-Ray"}

const (
	RatingOk = iota
	RatingProper
	RatingNuked
)

const (
	CodecUnknown = iota

	CodecXVid
	CodecH264

	CodecMp3
	CodecAAC
	CodecAC3
	CodecDTS
	CodecDTSHD
	CodecDTSHDMA
)

var Codecs = []string{"", "Xvid", "h264", "MP3", "AAC", "AC3", "DTS", "DTS HD", "DTS HD Master Audio"}

// Used to avoid infinite recursion in UnmarshalJSON
type torrent Torrent

func (t *Torrent) UnmarshalJSON(b []byte) error {
	tmp := torrent{}
	if err := json.Unmarshal(b, &tmp); err != nil {
		return err
	}
	*t = Torrent(tmp)
	t.initialize()
	return nil
}

func (t *Torrent) initialize() {
	if strings.HasPrefix(t.URI, "magnet:") {
		magnetURI, _ := url.Parse(t.URI)
		vals := magnetURI.Query()
		hash := strings.ToUpper(strings.TrimPrefix(vals.Get("xt"), "urn:btih:"))

		// for backward compatibility
		if unBase32Hash, err := base32.StdEncoding.DecodeString(hash); err == nil {
			hash = hex.EncodeToString(unBase32Hash)
		}

		t.InfoHash = strings.ToLower(hash)
		t.Name = vals.Get("dn")

		t.Trackers = make([]string, 0, len(vals.Get("tr")))
		for _, tracker := range vals.Get("tr") {
			t.Trackers = append(t.Trackers, strings.Replace(string(tracker), "\\", "", -1))
		}
	}

	if t.Resolution == ResolutionUnkown {
		t.Resolution = matchTags(t, map[string]int{
			`(480p|xvid|dvd)`: Resolution480p,
			`(720p|hdrip)`:    Resolution720p,
			`1080p`:           Resolution1080p,
		})
	}
	if t.VideoCodec == CodecUnknown {
		t.VideoCodec = matchTags(t, map[string]int{
			`([hx]264|1080p|hdrip)`: CodecH264,
			`xvid`:                  CodecXVid,
		})
	}
	if t.AudioCodec == CodecUnknown {
		t.AudioCodec = matchTags(t, map[string]int{
			`mp3`:           CodecMp3,
			`aac`:           CodecAAC,
			`(ac3|5\W+1)`:   CodecAC3,
			`dts`:           CodecDTS,
			`dts\W+hd`:      CodecDTSHD,
			`dts\W+hd\W+ma`: CodecDTSHDMA,
		})
	}
	if t.RipType == RipUnknown {
		t.RipType = matchTags(t, map[string]int{
			`(cam|camrip|hdcam)`:      RipCam,
			`(ts|telesync)`:           RipTS,
			`(tc|telecine)`:           RipTC,
			`(scr|screener)`:          RipScr,
			`dvd\W*scr`:               RipDVDScr,
			`dvd\W*rip`:               RipDVD,
			`hdtv`:                    RipHDTV,
			`(web\W*dl|web\W*rip)`:    RipWeb,
			`(bluray|b[rd]rip|hdrip)`: RipBluRay,
		})
	}
	if t.SceneRating == RatingOk {
		t.SceneRating = matchTags(t, map[string]int{
			`nuked`:  RatingNuked,
			`proper`: RatingProper,
			``:       RatingOk,
		})
	}
}

func NewTorrent(uri string) *Torrent {
	t := &Torrent{
		URI: uri,
	}
	t.initialize()
	return t
}

func matchTags(t *Torrent, tokens map[string]int) int {
	lowName := strings.ToLower(t.Name)
	codec := 0
	for key, value := range tokens {
		if regexp.MustCompile(`\W+` + key + `\W+`).MatchString(lowName) {
			codec = value
		}
	}
	return codec
}

func (t *Torrent) StreamInfo() *xbmc.StreamInfo {
	sie := &xbmc.StreamInfo{
		Video: &xbmc.StreamInfoEntry{
			Codec: Codecs[t.VideoCodec],
		},
		Audio: &xbmc.StreamInfoEntry{
			Codec: Codecs[t.AudioCodec],
		},
	}

	switch t.Resolution {
	case Resolution480p:
		sie.Video.Width = 853
		sie.Video.Height = 480
		break
	case Resolution720p:
		sie.Video.Width = 1280
		sie.Video.Height = 720
		break
	case Resolution1080p:
		sie.Video.Width = 1920
		sie.Video.Height = 1080
		break
	}

	return sie
}