package releases

import (
	"NUMParser/db"
	"NUMParser/db/models"
	"NUMParser/utils"
	"log"
	"sort"
)

func GetNewTVs() {
	torrs := db.GetTorrs()
	var list []*models.TorrentDetails

	for _, torr := range torrs {
		if torr.Categories == models.CatSeries /*&& (utils.Abs(torr.Year-time.Now().Year()) < 10)*/ {
			list = append(list, torr)
		}
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].CreateDate == list[j].CreateDate {
			return list[i].Title > list[j].Title
		}
		return list[i].CreateDate.After(list[j].CreateDate)
	})

	list = utils.UniqueTorrList(list)

	if len(list) > tvTorrMax {
		list = list[:tvTorrMax]
	}

	// 1000 подходящих по году и оценке (tvFits), а не 1000 найденных
	ents := FillTMDBKeep("TV", false, list, tvReserve, tvFits)
	ents = firstEnts(ents, tvLimit)

	log.Println("Found torrents:", len(ents))
	log.Println("All torrents:", len(list))

	save("tv_id.json", ents)
	utils.FreeOSMemGC()
}
