package providers

import (
	"sort"
	"sync"

	"github.com/op/go-logging"
	"github.com/scakemyer/quasar/bittorrent"
	"github.com/scakemyer/quasar/config"
	"github.com/scakemyer/quasar/tmdb"
)

const (
	SortMovies = iota
	SortShows
)

const (
	SortBySeeders = iota
	SortByResolution
	SortBalanced
)

const (
	Sort1080p720p480p = iota
	Sort720p1080p480p
	Sort720p480p1080p
	Sort480p720p1080p
)

var log = logging.MustGetLogger("linkssearch")

func Search(searchers []Searcher, query string) []*bittorrent.Torrent {
	torrentsChan := make(chan *bittorrent.Torrent)
	go func() {
		wg := sync.WaitGroup{}
		for _, searcher := range searchers {
			wg.Add(1)
			go func(searcher Searcher) {
				defer wg.Done()
				for _, torrent := range searcher.SearchLinks(query) {
					torrentsChan <- torrent
				}
			}(searcher)
		}
		wg.Wait()
		close(torrentsChan)
	}()

	return processLinks(torrentsChan, SortMovies)
}

func SearchMovie(searchers []MovieSearcher, movie *tmdb.Movie) []*bittorrent.Torrent {
	torrentsChan := make(chan *bittorrent.Torrent)
	go func() {
		wg := sync.WaitGroup{}
		for _, searcher := range searchers {
			wg.Add(1)
			go func(searcher MovieSearcher) {
				defer wg.Done()
				for _, torrent := range searcher.SearchMovieLinks(movie) {
					torrentsChan <- torrent
				}
			}(searcher)
		}
		wg.Wait()
		close(torrentsChan)
	}()

	return processLinks(torrentsChan, SortMovies)
}

func SearchSeason(searchers []SeasonSearcher, show *tmdb.Show, season *tmdb.Season) []*bittorrent.Torrent {
	torrentsChan := make(chan *bittorrent.Torrent)
	go func() {
		wg := sync.WaitGroup{}
		for _, searcher := range searchers {
			wg.Add(1)
			go func(searcher SeasonSearcher) {
				defer wg.Done()
				for _, torrent := range searcher.SearchSeasonLinks(show, season) {
					torrentsChan <- torrent
				}
			}(searcher)
		}
		wg.Wait()
		close(torrentsChan)
	}()

	return processLinks(torrentsChan, SortShows)
}

func SearchEpisode(searchers []EpisodeSearcher, show *tmdb.Show, episode *tmdb.Episode) []*bittorrent.Torrent {
	torrentsChan := make(chan *bittorrent.Torrent)
	go func() {
		wg := sync.WaitGroup{}
		for _, searcher := range searchers {
			wg.Add(1)
			go func(searcher EpisodeSearcher) {
				defer wg.Done()
				for _, torrent := range searcher.SearchEpisodeLinks(show, episode) {
					torrentsChan <- torrent
				}
			}(searcher)
		}
		wg.Wait()
		close(torrentsChan)
	}()

	return processLinks(torrentsChan, SortShows)
}

func processLinks(torrentsChan chan *bittorrent.Torrent, sortType int) []*bittorrent.Torrent {
	trackers := map[string]*bittorrent.Tracker{}
	torrentsMap := map[string]*bittorrent.Torrent{}

	torrents := make([]*bittorrent.Torrent, 0)

	log.Info("Resolving torrent files...")
	wg := sync.WaitGroup{}
	for torrent := range torrentsChan {
		torrents = append(torrents, torrent)
		wg.Add(1)
		go func(torrent *bittorrent.Torrent) {
			defer wg.Done()
			if err := torrent.Resolve(); err != nil {
				log.Errorf("Unable to resolve .torrent file at: %s", torrent.URI)
			}
		}(torrent)
	}
	wg.Wait()

	for _, torrent := range torrents {
		if torrent.InfoHash == "" { // ignore torrents whose infohash is empty
			log.Errorf("InfoHash is empty for %s", torrent.URI)
			continue
		}
		if existingTorrent, exists := torrentsMap[torrent.InfoHash]; exists {
			existingTorrent.Trackers = append(existingTorrent.Trackers, torrent.Trackers...)
			if torrent.Resolution > existingTorrent.Resolution {
				existingTorrent.Resolution = torrent.Resolution
			}
			if torrent.VideoCodec > existingTorrent.VideoCodec {
				existingTorrent.VideoCodec = torrent.VideoCodec
			}
			if torrent.AudioCodec > existingTorrent.AudioCodec {
				existingTorrent.AudioCodec = torrent.AudioCodec
			}
			if torrent.RipType > existingTorrent.RipType {
				existingTorrent.RipType = torrent.RipType
			}
			if torrent.SceneRating > existingTorrent.SceneRating {
				existingTorrent.SceneRating = torrent.SceneRating
			}
			existingTorrent.Multi = true
		} else {
			torrentsMap[torrent.InfoHash] = torrent
		}
		for _, tracker := range torrent.Trackers {
			bTracker, err := bittorrent.NewTracker(tracker)
			if err != nil {
				continue
			}
			trackers[bTracker.URL.Host] = bTracker
		}
	}

	for _, trackerUrl := range bittorrent.DefaultTrackers {
		tracker, _ := bittorrent.NewTracker(trackerUrl)
		trackers[tracker.URL.Host] = tracker
	}

	torrents = make([]*bittorrent.Torrent, 0, len(torrentsMap))
	for _, torrent := range torrentsMap {
		torrents = append(torrents, torrent)
	}

	log.Infof("Received %d links.\n", len(torrents))

	if len(torrents) == 0 {
		return torrents
	}

	log.Infof("Scraping torrent metrics from %d trackers...\n", len(trackers))
	scrapeResults := make(chan []bittorrent.ScrapeResponseEntry)
	go func() {
		wg := sync.WaitGroup{}
		for _, tracker := range trackers {
			wg.Add(1)
			go func(tracker *bittorrent.Tracker) {
				defer wg.Done()
				if err := tracker.Connect(); err != nil {
					log.Errorf("Tracker %s is not available because: %s", tracker, err)
					return
				}
				scrapeResults <- tracker.Scrape(torrents)
			}(tracker)
		}
		wg.Wait()
		close(scrapeResults)
	}()

	for results := range scrapeResults {
		for i, result := range results {
			if int64(result.Seeders) > torrents[i].Seeds {
				torrents[i].Seeds = int64(result.Seeders)
			}
			if int64(result.Leechers) > torrents[i].Peers {
				torrents[i].Peers = int64(result.Leechers)
			}
		}
	}

	conf := config.Get()
	sortMode := conf.SortingModeMovies
	resolutionPreference := conf.ResolutionPreferenceMovies

	if sortType == SortShows {
		sortMode = conf.SortingModeShows
		resolutionPreference = conf.ResolutionPreferenceShows
	}

	seeds := func(c1, c2 *bittorrent.Torrent) bool { return c1.Seeds > c2.Seeds }
	resolutionUp := func(c1, c2 *bittorrent.Torrent) bool { return c1.Resolution < c2.Resolution }
	resolutionDown := func(c1, c2 *bittorrent.Torrent) bool { return c1.Resolution > c2.Resolution }
	resolution720p1080p := func(c1, c2 *bittorrent.Torrent) bool { return Resolution720p1080p(c1) < Resolution720p1080p(c2) }
	resolution720p480p := func(c1, c2 *bittorrent.Torrent) bool { return Resolution720p480p(c1) < Resolution720p480p(c2) }
	balanced := func(c1, c2 *bittorrent.Torrent) bool { return float64(c1.Seeds) > Balanced(c2) }

	if sortMode == SortBySeeders {
		sort.Sort(sort.Reverse(BySeeds(torrents)))
	} else {
		switch resolutionPreference {
		case Sort1080p720p480p:
			if sortMode == SortBalanced {
				SortBy(balanced, resolutionDown).Sort(torrents)
			} else {
				SortBy(resolutionDown, seeds).Sort(torrents)
			}
			break
		case Sort480p720p1080p:
			if sortMode == SortBalanced {
				SortBy(balanced, resolutionUp).Sort(torrents)
			} else {
				SortBy(resolutionUp, seeds).Sort(torrents)
			}
			break
		case Sort720p1080p480p:
			if sortMode == SortBalanced {
				SortBy(balanced, resolution720p1080p).Sort(torrents)
			} else {
				SortBy(resolution720p1080p, seeds).Sort(torrents)
			}
			break
		case Sort720p480p1080p:
			if sortMode == SortBalanced {
				SortBy(balanced, resolution720p480p).Sort(torrents)
			} else {
				SortBy(resolution720p480p, seeds).Sort(torrents)
			}
			break
		}
	}

	log.Info("Sorted torrent candidates:")
	for _, torrent := range torrents {
		log.Infof("S:%d P:%d %s - %s", torrent.Seeds, torrent.Peers, torrent.Name, torrent.Provider)
	}

	return torrents
}
