package trakt

import (
	"fmt"
  "errors"
	"strconv"
	"strings"
	"math/rand"

	"github.com/jmcvetta/napping"
	"github.com/scakemyer/quasar/xbmc"
)

func GetShow(Id string) (show *Show) {
	endPoint := fmt.Sprintf("shows/%s", Id)

	params := napping.Params{
		"extended": "full,images",
	}.AsUrlValues()

	resp, err := Get(endPoint, params)

	if err != nil {
		panic(err)
	}

	resp.Unmarshal(&show)
	return show
}

func SearchShows(query string, page string) (shows []*Shows) {
	endPoint := "search"

	params := napping.Params{
		"page": page,
		"limit": Limit,
		"query": query,
		"extended": "full,images",
	}.AsUrlValues()

	resp, err := Get(endPoint, params)

	if err != nil {
		panic(err)
	}
	if resp.Status() != 200 {
		panic(errors.New(fmt.Sprintf("Bad status: %d", resp.Status())))
	}

	resp.Unmarshal(&shows)
	return shows
}

func TopShows(topCategory string, page string) (shows []*Shows) {
	endPoint := "shows/" + topCategory

	params := napping.Params{
		"page": page,
		"limit": Limit,
		"extended": "full,images",
	}.AsUrlValues()

	resp, err := Get(endPoint, params)

	if err != nil {
		panic(err)
	}
	if resp.Status() != 200 {
		panic(errors.New(fmt.Sprintf("Bad status: %d", resp.Status())))
	}

  if topCategory == "popular" {
  	var showList []*Show
  	resp.Unmarshal(&showList)

    showListing := make([]*Shows, 0)
    for _, show := range showList {
  		showItem := Shows{
        Show: show,
      }
      showListing = append(showListing, &showItem)
    }
    shows = showListing
  } else {
  	resp.Unmarshal(&shows)
  }
	return shows
}

func (show *Show) ToListItem() *xbmc.ListItem {
	return &xbmc.ListItem{
		Label: show.Title,
		Info: &xbmc.ListItemInfo{
			Count:       rand.Int(),
			Title:       show.Title,
      Year:        show.Year,
			Genre:       strings.Title(strings.Join(show.Genres, " / ")),
			Plot:        show.Overview,
			PlotOutline: show.Overview,
			Rating:      show.Rating,
      Votes:       strconv.Itoa(show.Votes),
			Duration:    show.Runtime * 60,
			MPAA:        show.Certification,
			Code:        show.IDs.IMDB,
			Trailer:     show.Trailer,
		},
		Art: &xbmc.ListItemArt{
			Poster: show.Images.Poster.Full,
			FanArt: show.Images.FanArt.Full,
			Banner: show.Images.Banner.Full,
			Thumbnail: show.Images.Thumbnail.Full,
		},
	}
}

func (season *Season) ToListItem() *xbmc.ListItem {
	seasonLabel := fmt.Sprintf("Season %d", season.Number)
	return &xbmc.ListItem{
		Label: seasonLabel,
		Info: &xbmc.ListItemInfo{
			Count:  rand.Int(),
			Title:  seasonLabel,
			Season: season.Number,
			Rating: season.Rating,
      Votes:  strconv.Itoa(season.Votes),
		},
		Art: &xbmc.ListItemArt{
			Poster: season.Images.Poster.Full,
      Thumbnail: season.Images.Thumbnail.Full,
			// FanArt: season.Images.FanArt.Full,
		},
	}
}

func (episode *Episode) ToListItem() *xbmc.ListItem {
	title := fmt.Sprintf("%dx%02d %s", episode.Season, episode.Number, episode.Title)
	return &xbmc.ListItem{
		Label:     title,
		Thumbnail: episode.Images.ScreenShot.Full,
		Info: &xbmc.ListItemInfo{
			Count:       rand.Int(),
			Title:       title,
			Plot:        episode.Overview,
			PlotOutline: episode.Overview,
			Rating:      episode.Rating,
      Votes:       strconv.Itoa(episode.Votes),
			Episode:     episode.Number,
			Season:      episode.Season,
		},
		Art: &xbmc.ListItemArt{
			Thumbnail: episode.Images.ScreenShot.Full,
			// FanArt:    episode.Season.Show.Images.FanArt.Full,
			// Banner:    episode.Season.Show.Images.Banner.Full,
		},
	}
}
