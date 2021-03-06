package tmdb

import (
	"fmt"
	"path"
	"sync"
	"time"
	"errors"
	"strconv"
	"strings"
	"math/rand"

	"github.com/jmcvetta/napping"
	"github.com/scakemyer/quasar/cache"
	"github.com/scakemyer/quasar/config"
	"github.com/scakemyer/quasar/xbmc"
)

const (
	moviesPerPage          = 20
	popularMoviesMaxPages  = 20
	popularMoviesStartPage = 1
)

func GetMovie(tmdbId int, language string) *Movie {
	return GetMovieById(strconv.Itoa(tmdbId), language)
}

func GetMovieById(movieId string, language string) *Movie {
	var movie *Movie
	cacheStore := cache.NewFileStore(path.Join(config.Get().ProfilePath, "cache"))
	key := fmt.Sprintf("com.tmdb.movie.%s.%s", movieId, language)
	if err := cacheStore.Get(key, &movie); err != nil {
		rateLimiter.Call(func() {
      urlValues := napping.Params{
				"api_key": apiKey,
				"append_to_response": "credits,images,alternative_titles,translations,external_ids,trailers",
				"language": language,
			}.AsUrlValues()
			resp, err := napping.Get(
				tmdbEndpoint + "movie/" + movieId,
				&urlValues,
				&movie,
				nil,
			)
			if err != nil {
				panic(err)
			}
			if resp.Status() != 200 {
				panic(errors.New(fmt.Sprintf("Bad status: %d", resp.Status())))
			}
			if movie != nil {
				cacheStore.Set(key, movie, cacheTime)
			}
		})
	}
	if movie == nil {
		return nil
	}
	switch t := movie.RawPopularity.(type) {
	case string:
		popularity, _ := strconv.ParseFloat(t, 64)
		movie.Popularity = popularity
	case float64:
		movie.Popularity = t
	}
	return movie
}

func GetMovies(tmdbIds []int, language string) Movies {
	var wg sync.WaitGroup
	movies := make(Movies, len(tmdbIds))
	wg.Add(len(tmdbIds))
	for i, tmdbId := range tmdbIds {
		go func(i int, tmdbId int) {
			defer wg.Done()
			movies[i] = GetMovie(tmdbId, language)
		}(i, tmdbId)
	}
	wg.Wait()
	return movies
}

func GetMovieGenres(language string) []*Genre {
	genres := GenreList{}
	rateLimiter.Call(func() {
    urlValues := napping.Params{
			"api_key": apiKey,
			"language": language,
		}.AsUrlValues()
		resp, err := napping.Get(
			tmdbEndpoint+"genre/movie/list",
			&urlValues,
			&genres,
			nil,
		)
		if err != nil {
			panic(err)
		}
		if resp.Status() != 200 {
			panic(errors.New(fmt.Sprintf("Bad status: %d", resp.Status())))
		}
	})
	return genres.Genres
}

func SearchMovies(query string, language string) Movies {
	var results EntityList
	rateLimiter.Call(func() {
    urlValues := napping.Params{
			"api_key": apiKey,
			"query": query,
		}.AsUrlValues()
		resp, err := napping.Get(
			tmdbEndpoint + "search/movie",
			&urlValues,
			&results,
			nil,
		)
		if err != nil {
			panic(err)
		}
		if resp.Status() != 200 {
			panic(errors.New(fmt.Sprintf("Bad status: %d", resp.Status())))
		}
	})
	tmdbIds := make([]int, 0, len(results.Results))
	for _, movie := range results.Results {
		tmdbIds = append(tmdbIds, movie.Id)
	}
	return GetMovies(tmdbIds, language)
}

func GetList(listId string, language string) Movies {
	var results *List
	rateLimiter.Call(func() {
    urlValues := napping.Params{
			"api_key": apiKey,
		}.AsUrlValues()
		resp, err := napping.Get(
			tmdbEndpoint + "list/" + listId,
			&urlValues,
			&results,
			nil,
		)
		if err != nil {
			panic(err)
		}
		if resp.Status() != 200 {
			panic(errors.New(fmt.Sprintf("Bad status: %d", resp.Status())))
		}
	})
	tmdbIds := make([]int, 0, len(results.Items))
	for _, movie := range results.Items {
		tmdbIds = append(tmdbIds, movie.Id)
	}
	return GetMovies(tmdbIds, language)
}

type ByPopularity Movies

func (a ByPopularity) Len() int           { return len(a) }
func (a ByPopularity) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByPopularity) Less(i, j int) bool { return a[i].Popularity < a[j].Popularity }

func ListMoviesComplete(endpoint string, params napping.Params, page int) Movies {
	MaxPages := popularMoviesMaxPages
	if page >= 0 {
		MaxPages = 1
	}
	movies := make(Movies, MaxPages * moviesPerPage)
	params["api_key"] = apiKey

	wg := sync.WaitGroup{}
	for i := 0; i < MaxPages; i++ {
		wg.Add(1)
		currentpage := i
		startMoviesIndex := i * moviesPerPage
		if page >= 0 {
			currentpage = page
		}
		go func(page int) {
			defer wg.Done()
			var tmp *EntityList
			tmpParams := napping.Params{
				"page": strconv.Itoa(popularMoviesStartPage + page),
			}
			for k, v := range params {
				tmpParams[k] = v
			}
      urlValues := tmpParams.AsUrlValues()
			rateLimiter.Call(func() {
				resp, err := napping.Get(
					tmdbEndpoint + endpoint,
					&urlValues,
					&tmp,
					nil,
				)
				if err != nil {
					panic(err)
				}
				if resp.Status() != 200 {
					panic(errors.New(fmt.Sprintf("Bad status: %d", resp.Status())))
				}
			})
			for i, movie := range tmp.Results {
				movies[startMoviesIndex + i] = GetMovie(movie.Id, params["language"])
			}
		}(currentpage)
	}
	wg.Wait()
	return movies
}

func PopularMoviesComplete(genre string, language string, page int) Movies {
	var p napping.Params
	if genre == "" {
		p = napping.Params{
			"language":                 language,
			"sort_by":                  "popularity.desc",
			"primary_release_date.lte": time.Now().UTC().Format("2006-01-02"),
		}
	} else {
		p = napping.Params{
			"language":                 language,
			"sort_by":                  "popularity.desc",
			"primary_release_date.lte": time.Now().UTC().Format("2006-01-02"),
			"with_genres":              genre,
		}
	}
	return ListMoviesComplete("discover/movie", p, page)
}

func RecentMoviesComplete(genre string, language string, page int) Movies {
	var p napping.Params
	if genre == "" {
		p = napping.Params{
			"language":                 language,
			"sort_by":                  "primary_release_date.desc",
			"vote_count.gte":           "10",
			"primary_release_date.lte": time.Now().UTC().Format("2006-01-02"),
		}
	} else {
		p = napping.Params{
			"language":                 language,
			"sort_by":                  "primary_release_date.desc",
			"vote_count.gte":           "10",
			"primary_release_date.lte": time.Now().UTC().Format("2006-01-02"),
			"with_genres":              genre,
		}
	}
	return ListMoviesComplete("discover/movie", p, page)
}

func TopRatedMoviesComplete(genre string, language string, page int) Movies {
	return ListMoviesComplete("movie/top_rated", napping.Params{"language": language}, page)
}

func MostVotedMoviesComplete(genre string, language string, page int) Movies {
	var p napping.Params
	if genre == "" {
		p = napping.Params{
			"language":                 language,
			"sort_by":                  "vote_count.desc",
			"primary_release_date.lte": time.Now().UTC().Format("2006-01-02"),
		}
	} else {
		p = napping.Params{
			"language":                 language,
			"sort_by":                  "vote_count.desc",
			"primary_release_date.lte": time.Now().UTC().Format("2006-01-02"),
			"with_genres":              genre,
		}
	}
	return ListMoviesComplete("discover/movie", p, page)
}

func (movie *Movie) ToListItem() *xbmc.ListItem {
	year, _ := strconv.Atoi(strings.Split(movie.ReleaseDate, "-")[0])

	title := movie.Title
	if config.Get().UseOriginalTitle && movie.OriginalTitle != "" {
		title = movie.OriginalTitle
	}

	item := &xbmc.ListItem{
		Label: title,
		Info: &xbmc.ListItemInfo{
			Year:          year,
			Count:         rand.Int(),
			Title:         title,
			OriginalTitle: movie.OriginalTitle,
			Plot:          movie.Overview,
			PlotOutline:   movie.Overview,
			TagLine:       movie.TagLine,
			Duration:      movie.Runtime * 60,
			Code:          movie.IMDBId,
			Date:          movie.ReleaseDate,
			Votes:         strconv.Itoa(movie.VoteCount),
			Rating:        movie.VoteAverage,
		},
		Art: &xbmc.ListItemArt{
			FanArt: ImageURL(movie.BackdropPath, "w1280"),
			Poster: ImageURL(movie.PosterPath, "w500"),
		},
	}
	item.Thumbnail = item.Art.Poster
	item.Art.Thumbnail = item.Art.Poster
	genres := make([]string, 0, len(movie.Genres))
	for _, genre := range movie.Genres {
		genres = append(genres, genre.Name)
	}
	item.Info.Genre = strings.Join(genres, " / ")

	if movie.Trailers != nil {
		for _, trailer := range movie.Trailers.Youtube {
			item.Info.Trailer = trailer.Source
			break
		}
	}

	if item.Info.Trailer == "" && config.Get().Language != "en" {
		enMovie := GetMovie(movie.Id, "en")
		if enMovie.Trailers != nil {
			for _, trailer := range enMovie.Trailers.Youtube {
				item.Info.Trailer = trailer.Source
				break
			}
		}
	}

	for _, language := range movie.SpokenLanguages {
		item.StreamInfo = &xbmc.StreamInfo{
			Audio: &xbmc.StreamInfoEntry{
				Language: language.ISO_639_1,
			},
		}
		break
	}

	for _, company := range movie.ProductionCompanies {
		item.Info.Studio = company.Name
		break
	}
	if movie.Credits != nil {
		item.Info.CastAndRole = make([][]string, 0)
		for _, cast := range movie.Credits.Cast {
			item.Info.CastAndRole = append(item.Info.CastAndRole, []string{cast.Name, cast.Character})
		}
		directors := make([]string, 0)
		writers := make([]string, 0)
		for _, crew := range movie.Credits.Crew {
			switch crew.Job {
			case "Director":
				directors = append(directors, crew.Name)
			case "Writer":
				writers = append(writers, crew.Name)
			}
		}
		item.Info.Director = strings.Join(directors, " / ")
		item.Info.Writer = strings.Join(writers, " / ")
	}
	return item
}
