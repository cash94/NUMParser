package releases

// Подборки Кинопоиска (kinopoiskapiunofficial.tech) — категории каталога
// TorrStream kp_*_id.json. Элементы подборки сопоставляются с TMDB (по IMDb,
// иначе по названию и году) и с раздачей Rutor из базы парсера; без раздачи
// элемент не попадает в файл. Формат файла — как у остальных каталогов (save).
//
// Восстановлено 2026-11-25 по рабочему бинарнику (исходник был утерян).

import (
	"NUMParser/config"
	"NUMParser/db"
	"NUMParser/db/models"
	"NUMParser/movies/tmdb"
	"NUMParser/utils"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type kpCollection struct {
	File  string // имя файла в public/releases
	Type  string // type= в API Кинопоиска
	Name  string // для журнала
	Movie bool   // искать в TMDB фильмы (true) или сериалы (false)
	Limit int    // сколько элементов брать из подборки
}

var kpCollections = []kpCollection{
	{"kp_popular_id.json", "TOP_POPULAR_ALL", "Популярное", true, 700},
	{"kp_pop_movies_id.json", "TOP_POPULAR_MOVIES", "Популярные фильмы", true, 700},
	{"kp_pop_series_id.json", "POPULAR_SERIES", "Популярные сериалы", false, 700},
	{"kp_top250_id.json", "TOP_250_MOVIES", "Топ 250 фильмов", true, 250},
	{"kp_top250_tv_id.json", "TOP_250_TV_SHOWS", "Топ 250 сериалов", false, 250},
	{"kp_family_id.json", "FAMILY", "Семейное", true, 230},
	{"kp_comics_id.json", "COMICS_THEME", "Комиксы", true, 100},
	{"kp_love_id.json", "LOVE_THEME", "Про любовь", true, 70},
}


const kpAPI = "https://kinopoiskapiunofficial.tech/api/v2.2/films/collections?type="

type kpResponse struct {
	Total      int       `json:"total"`
	TotalPages int       `json:"totalPages"`
	Items      []*kpItem `json:"items"`
}

type kpItem struct {
	KinopoiskID     int     `json:"kinopoiskId"`
	ImdbID          string  `json:"imdbId"`
	NameRu          string  `json:"nameRu"`
	NameEn          string  `json:"nameEn"`
	NameOriginal    string  `json:"nameOriginal"`
	Year            int     `json:"year"`
	Type            string  `json:"type"`
	RatingKinopoisk float64 `json:"ratingKinopoisk"`
}

func GetKPCollections() {
	token := kpToken()
	if token == "" {
		log.Println("KP: пустой ключ Кинопоиска, подборки пропущены")
		return
	}

	log.Println("Update kinopoisk collections")

	idx := newTorrIndex(db.GetTorrs())
	log.Println("KP: торрентов с kinopoiskId:", len(idx.byKP), "с imdbId:", len(idx.byIMDB))

	for _, c := range kpCollections {
		getKPCollection(c, token, idx)
		utils.FreeOSMemGC()
	}
}

func kpToken() string {
	if t, err := config.ReadConfigParser("KpToken"); err == nil {
		if t = strings.TrimSpace(t); t != "" {
			return t
		}
	}
	return "" // ключ только в config.yml (kptoken), в исходник его не класть
}

func getKPCollection(c kpCollection, token string, idx *torrIndex) {
	log.Println("KP:", c.Name, "— загрузка подборки")

	items := fetchKPCollection(c, token)
	if len(items) == 0 {
		log.Println("KP:", c.Name, "— Кинопоиск ничего не отдал, файл не трогаем")
		return
	}
	log.Println("KP:", c.Name, "— элементов в подборке:", len(items))

	seen := map[int64]bool{}
	var list []*models.Entity
	for i, it := range items {
		log.Println("KP:", c.Name, i+1, "/", len(items), "—", it.title())
		ent := findTmdbKP(c, it)
		// Подборка сериалов — только сериалы, фильмов — только фильмы
		// (sameMediaType, findTMDB.go)
		if ent != nil && !sameMediaType(c.Movie, ent) {
			log.Println("KP: не тот тип в TMDB:", it.title(), ent.MediaType, ent.ID)
			ent = nil
		}
		if ent == nil {
			log.Println("KP: не найден в TMDB:", it.title(), it.KinopoiskID)
			continue
		}
		if seen[ent.ID] {
			continue
		}
		torr := idx.find(c, it, ent)
		if torr == nil {
			log.Println("KP: нет раздачи:", it.title(), it.KinopoiskID)
			continue
		}
		seen[ent.ID] = true
		ent.SetTorrent(torr)
		list = append(list, ent)
		utils.FreeOSMemGC()
	}

	log.Println("KP:", c.Name, "— собрано:", len(list), "из", len(items))
	save(c.File, list)
}

func (it *kpItem) title() string {
	if it.NameRu != "" {
		return it.NameRu
	}
	if it.NameOriginal != "" {
		return it.NameOriginal
	}
	return it.NameEn
}

// Страницы подборки по 20 элементов, пока не наберётся c.Limit
func fetchKPCollection(c kpCollection, token string) []*kpItem {
	pages := c.Limit / 20
	if c.Limit%20 != 0 {
		pages++
	}
	var items []*kpItem
	for page := 1; page <= pages; page++ {
		resp := fetchKPPage(c.Type, page, token)
		if resp == nil {
			break
		}
		items = append(items, resp.Items...)
		if len(resp.Items) == 0 || page >= resp.TotalPages || len(items) >= c.Limit {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if len(items) > c.Limit {
		items = items[:c.Limit]
	}
	return items
}

func fetchKPPage(kind string, page int, token string) *kpResponse {
	u := kpAPI + url.QueryEscape(kind) + "&page=" + strconv.Itoa(page)

	client := &http.Client{Timeout: 20 * time.Second}

	for try := 0; try < 3; try++ {
		if try > 0 {
			time.Sleep(time.Duration(try) * time.Second)
		}

		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			log.Println("KP: ошибка запроса:", err)
			return nil
		}
		req.Header.Set("X-API-KEY", token)
		req.Header.Set("accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			log.Println("KP: страница", page, kind, "—", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if resp.StatusCode == http.StatusPaymentRequired {
				log.Println("KP: исчерпана квота ключа Кинопоиска")
				return nil
			}
			log.Println("KP: страница", page, kind, "— HTTP", resp.StatusCode)
			continue
		}

		var r kpResponse
		err = json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()
		if err != nil {
			log.Println("KP: страница", page, kind, "— разбор ответа:", err)
			continue
		}
		return &r
	}
	return nil
}

// TMDB: сначала по IMDb, потом по названиям (русское, оригинальное,
// английское) с проверкой года
func findTmdbKP(c kpCollection, it *kpItem) *models.Entity {
	if it.ImdbID != "" {
		if e := tmdb.FindByID(c.Movie, it.ImdbID, "imdb_id"); e != nil {
			return e
		}
	}

	for _, name := range [...]string{it.NameRu, it.NameOriginal, it.NameEn} {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if e := pickTmdbByYear(tmdb.Search(c.Movie, name), it); e != nil {
			return e
		}
	}
	return nil
}

// Первый результат с годом ±1 от года Кинопоиска; года нет — первый
func pickTmdbByYear(list []*models.Entity, it *kpItem) *models.Entity {
	if len(list) == 0 {
		return nil
	}
	if it.Year <= 0 {
		return list[0]
	}
	for _, e := range list {
		y, err := strconv.Atoi(e.Year)
		if err != nil {
			continue
		}
		if utils.Abs(y-it.Year) <= 1 {
			return e
		}
	}
	return nil
}

// Раздачи Rutor по kinopoiskId и imdbId из описаний раздач
type torrIndex struct {
	byKP   map[string][]*models.TorrentDetails
	byIMDB map[string][]*models.TorrentDetails
}

func newTorrIndex(torrs []*models.TorrentDetails) *torrIndex {
	idx := &torrIndex{
		byKP:   map[string][]*models.TorrentDetails{},
		byIMDB: map[string][]*models.TorrentDetails{},
	}
	for _, t := range torrs {
		if t == nil {
			continue
		}
		if t.KPID != "" {
			idx.byKP[t.KPID] = append(idx.byKP[t.KPID], t)
		}
		if t.IMDBID != "" {
			idx.byIMDB[t.IMDBID] = append(idx.byIMDB[t.IMDBID], t)
		}
	}
	return idx
}

func (idx *torrIndex) find(c kpCollection, it *kpItem, ent *models.Entity) *models.TorrentDetails {
	if it.KinopoiskID > 0 {
		if t := bestTorr(c, idx.byKP[strconv.Itoa(it.KinopoiskID)]); t != nil {
			return t
		}
	}
	if it.ImdbID != "" {
		if t := bestTorr(c, idx.byIMDB[it.ImdbID]); t != nil {
			return t
		}
	}
	if ent.ImdbID != "" && ent.ImdbID != it.ImdbID {
		if t := bestTorr(c, idx.byIMDB[ent.ImdbID]); t != nil {
			return t
		}
	}
	return searchTorrKP(c, it, ent)
}

// Поиск раздачи по названию из TMDB: «название оригинальное год», потом
// английские альтернативные названия
func searchTorrKP(c kpCollection, it *kpItem, ent *models.Entity) *models.TorrentDetails {
	year := ent.Year
	if year == "" && it.Year > 0 {
		year = strconv.Itoa(it.Year)
	}
	if t := bestTorr(c, db.SearchTorr(ent.Title+" "+ent.OriginalTitle+" "+year)); t != nil {
		return t
	}
	for _, name := range getEnNames(ent.Titles) {
		if t := bestTorr(c, db.SearchTorr(ent.Title+" "+name+" "+year)); t != nil {
			return t
		}
	}
	return nil
}

// Раздача нужного вида (фильм или сериал): самая свежая, при равной дате —
// с лучшим видео, потом звуком
func bestTorr(c kpCollection, list []*models.TorrentDetails) *models.TorrentDetails {
	list = utils.Filter(list, func(i int, t *models.TorrentDetails) bool {
		return t == nil || !kpCatFits(t.Categories, c.Movie)
	})
	if len(list) == 0 {
		return nil
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].CreateDate == list[j].CreateDate {
			if list[i].VideoQuality == list[j].VideoQuality {
				return list[i].AudioQuality > list[j].AudioQuality
			}
			return list[i].VideoQuality > list[j].VideoQuality
		}
		return list[i].CreateDate.After(list[j].CreateDate)
	})
	return list[0]
}

func kpCatFits(cat string, movie bool) bool {
	switch cat {
	case models.CatMovie, models.CatDocMovie, models.CatCartoonMovie:
		return movie
	case models.CatSeries, models.CatDocSeries, models.CatCartoonSeries, models.CatTVShow, models.CatAnime:
		return !movie
	}
	return true
}
