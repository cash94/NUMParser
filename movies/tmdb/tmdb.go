package tmdb

import (
	"NUMParser/config"
	"NUMParser/db/models"
	"NUMParser/db/tmdb"
	"NUMParser/utils"
	"github.com/jmcvetta/napping"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	tmdbEndpoint  = "https://api.themoviedb.org/3/"
	imageEndpoint = "http://image.tmdb.org/t/p/"
)

var (
	genres      []*models.Genre
	TMDBAuthKey string
)

func Init() {
	log.Println("Init tmdb")

	//	dir := filepath.Dir(os.Args[0])
	//	buf, err := os.ReadFile(filepath.Join(dir, "tmdb.key"))
	//	if err != nil || strings.TrimSpace(string(buf)) == "" {
	//		log.Println("Fatal error read tmdb auth key:", err)
	//		os.Exit(1)
	//	}
	//	TMDBAuthKey = strings.TrimSpace(string(buf))

	buf, err := config.ReadConfigParser("TmdbToken")
	if err == nil && buf != "" {
		TMDBAuthKey = strings.TrimSpace(buf)
	} else {
		log.Println("Fatal error read tmdb auth key:", err)
		os.Exit(1)
	}

	lstmg := GetGenres("movie")
	lsttvg := GetGenres("tv")

	if lstmg == nil && lsttvg == nil {
		return
	}

	genres = append(lstmg, lsttvg...)

	sort.Slice(genres, func(i, j int) bool {
		return genres[i].Name < genres[j].Name
	})

}

// Сериал в базе обновляется раз в неделю: у него выходят новые сезоны, а по
// дате последнего эпизода «Сериалы» отличают новый сезон от перезалитой
// раздачи старого сериала (releases/tv.go). Фильм не меняется — не обновляем
const tvRefreshAge = 7 * 24 * time.Hour

func GetVideoDetails(isMovie bool, id int64) *models.Entity {
	var ent, old *models.Entity
	if isMovie {
		ent = tmdb.GetMovie(id)
	} else {
		ent = tmdb.GetTV(id)
	}
	if ent != nil {
		if isMovie || time.Since(ent.UpdateDate) < tvRefreshAge {
			return ent
		}
		old, ent = ent, nil
	}

	params := map[string]string{}
	//params["api_key"] = apiKey

	if _, ok := params["language"]; !ok {
		params["language"] = "ru"
	}

	ids := strconv.FormatInt(id, 10)

	endpoint := ""
	if isMovie {
		endpoint = "movie/" + ids
	} else {
		endpoint = "tv/" + ids
	}

	err := readPageTmdb(endpoint, params, &ent)
	if err != nil || ent == nil {
		// Обновить не вышло — прежняя запись лучше, чем ничего
		return old
	}
	fixEntity(ent)

	titles := alternativeTitles(isMovie, id)
	ent.Titles = titles

	if old != nil {
		ent.Logo, ent.LogoChecked = old.Logo, old.LogoChecked
	}

	tmdb.AddTMDB(ent)

	return ent
}

// Логотип названия — по тем же правилам, что у сервера TorrStream
// (services/tmdb.js pickLogo), чтобы в каталоге и в карточке он был один:
// язык ru → en → без языка, внутри — PNG раньше SVG, затем голоса и ширина.
// Спрашивается один раз на сущность, результат лежит в базе.
var logoLangs = []string{"ru", "en", ""}

func EnsureLogo(e *models.Entity) {
	if e == nil || e.LogoChecked {
		return
	}
	st := "movie"
	if e.MediaType == "tv" {
		st = "tv"
	}
	var imgs *models.Images
	err := readPageTmdb(st+"/"+strconv.FormatInt(e.ID, 10)+"/images",
		map[string]string{"include_image_language": "ru,en,null"}, &imgs)
	if err != nil {
		return // спросим в следующий раз
	}
	e.Logo = pickLogo(imgs)
	e.LogoChecked = true
	tmdb.AddTMDB(e)
}

func pickLogo(imgs *models.Images) *models.Logo {
	if imgs == nil {
		return nil
	}
	for _, lang := range logoLangs {
		var best *models.Logo
		var bestSvg bool
		var bestVote float64
		var bestWidth int
		for _, l := range imgs.Logos {
			if l.FilePath == "" || l.Iso6391 != lang {
				continue
			}
			svg := strings.HasSuffix(strings.ToLower(l.FilePath), ".svg")
			better := best == nil ||
				(bestSvg && !svg) ||
				(bestSvg == svg && (l.VoteAverage > bestVote || (l.VoteAverage == bestVote && l.Width > bestWidth)))
			if better {
				best = &models.Logo{FilePath: l.FilePath, AspectRatio: l.AspectRatio, Lang: lang}
				bestSvg, bestVote, bestWidth = svg, l.VoteAverage, l.Width
			}
		}
		if best != nil {
			return best
		}
	}
	return nil
}

// Картинка TMDB другого размера: «…/t/p/w780/x.jpg» → «…/t/p/w1280/x.jpg»
func ImageSize(uri string, size string) string {
	i := strings.Index(uri, "/t/p/")
	if i < 0 {
		return uri
	}
	rest := uri[i+len("/t/p/"):]
	j := strings.Index(rest, "/")
	if j < 0 {
		return uri
	}
	return uri[:i+len("/t/p/")] + size + rest[j:]
}

func Search(isMovie bool, query string) []*models.Entity {
	var st = "movie"
	if !isMovie {
		st = "tv"
	}

	params := map[string]string{}
	params["query"] = query

	return listVideoPages("search/"+st, params)
}

func FindByID(isMovie bool, id string, idType string) *models.Entity {
	// В локальной базе IMDb id ищется сразу в сериалах и фильмах — берём, только
	// если тип совпал, иначе спрашиваем TMDB (find/ отдаёт movie_ и tv_results)
	if ent := tmdb.FindIMDB(id); ent != nil && (ent.MediaType == "movie") == isMovie {
		return ent
	}

	params := napping.Params{}

	//params["api_key"] = apiKey
	params["external_source"] = idType
	params["language"] = "ru"

	var results *models.FindResult

	err := readPageTmdb("find/"+id, params, &results)
	if err != nil {
		return nil
	}

	if results == nil {
		return nil
	}

	var ent *models.Entity
	if isMovie {
		if len(results.MovieResults) > 0 {
			ent = results.MovieResults[0]
		}
	} else {
		if len(results.TVResults) > 0 {
			ent = results.TVResults[0]
		}
	}

	if ent == nil {
		return nil
	}

	ent = GetVideoDetails(isMovie, ent.ID)
	if ent == nil {
		return nil
	}
	return ent
}

func Legends() []*models.Entity {
	list := listVideoPages("movie/top_rated", map[string]string{})
	for _, e := range list {
		names := alternativeTitles(true, e.ID)
		e.Titles = names
	}
	return list
}

func alternativeTitles(isMovie bool, id int64) []string {
	params := napping.Params{}
	//params["api_key"] = apiKey

	var st = "movie"
	if !isMovie {
		st = "tv"
	}
	var results *models.AlternativeTitles

	err := readPageTmdb(st+"/"+strconv.FormatInt(id, 10)+"/alternative_titles", params, &results)
	if err != nil {
		return nil
	}

	if results == nil {
		return nil
	}

	var list []string

	for _, title := range results.Titles {
		if title.Title != "" {
			list = append(list, title.Title)
		}
	}

	return list
}

func listVideoPages(endpoint string, params napping.Params) []*models.Entity {
	p := map[string]string{}
	for k, v := range params {
		p[k] = v
	}
	p["page"] = "1"
	lst, pages := listVideo(endpoint, p)
	if pages > 10 {
		pages = 10
	}
	if pages > 1 {
		lsts := make([][]*models.Entity, pages-1)
		utils.ParallelFor(2, pages+1, func(i int) {
			p := map[string]string{}
			for k, v := range params {
				p[k] = v
			}
			p["page"] = strconv.Itoa(i)
			lsts[i-2], _ = listVideo(endpoint, p)
		})
		for _, l := range lsts {
			lst = append(lst, l...)
		}
	}

	return lst
}

func listVideo(endpoint string, params napping.Params) ([]*models.Entity, int) {
	//params["api_key"] = apiKey

	if _, ok := params["language"]; !ok {
		params["language"] = "ru"
	}

	var results *models.EntityRequest
	pageParams := napping.Params{}
	for k, v := range params {
		pageParams[k] = v
	}

	err := readPageTmdb(endpoint, params, &results)
	if err != nil {
		return nil, 0
	}

	if results == nil {
		return nil, 0
	}

	for _, v := range results.Results {
		fixEntity(v)
	}

	return results.Results, results.TotalPages
}
