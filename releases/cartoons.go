package releases

import (
	"NUMParser/db"
	"NUMParser/db/models"
	"NUMParser/utils"
	"log"
	"sort"
	"time"
)

const cartoonYears = 5

func GetNewCartoons() {
	torrs := db.GetTorrs()
	now := time.Now().Year()
	var list []*models.TorrentDetails

	for _, torr := range torrs {
		// Окно — пять лет: полнометражных мультфильмов на Rutor выходит мало, и
		// за ±1 год набиралось около сотни. Свежие раздачи всё равно первыми
		if torr.Categories == models.CatCartoonMovie && torr.Year >= now-cartoonYears && torr.Year <= now+1 {
			list = append(list, torr)
		}
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

	list = utils.UniqueTorrList(list)

	ents := FillTMDB("Cartoons", true, list, 1000)

	log.Println("Found torrents:", len(ents))
	log.Println("All torrents:", len(list))

	save("cartoons_id.json", ents)
	utils.FreeOSMemGC()
}
