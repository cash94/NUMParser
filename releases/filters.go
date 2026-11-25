package releases

// Отбор найденного в TMDB для «Фильмов» и «Сериалов» TorrStream.

import (
	"NUMParser/db/models"
	"NUMParser/utils"
	"log"
	"strconv"
	"time"
)

// Год из даты TMDB после fixEntity («02.01.2006») или исходной («2006-01-02»);
// 0 — даты нет
func dateYear(date string) int {
	if len(date) < 4 {
		return 0
	}
	if y, err := strconv.Atoi(date[len(date)-4:]); err == nil && y > 1800 {
		return y
	}
	if y, err := strconv.Atoi(date[:4]); err == nil && y > 1800 {
		return y
	}
	return 0
}

// Сериал выходит сейчас: последний эпизод — в этом или прошлом году. Свежая
// раздача давно закончившегося сериала («Кремниевая долина», «Грань») — не
// новый сезон, в «Сериалы» она не идёт. Даты последнего эпизода нет — по
// дате выхода.
const tvRecentYears = 1

func tvIsRecent(e *models.Entity) bool {
	y := dateYear(e.LastAirDate)
	if y == 0 {
		y = dateYear(e.FirstAirDate)
	}
	if y == 0 {
		y = dateYear(e.ReleaseDate)
	}
	return y == 0 || y >= time.Now().Year()-tvRecentYears
}

// Фильм для «Фильмов»: оценка TMDB выше 6 или оценки ещё нет (свежий релиз
// без голосов), и вышел не раньше чем два года назад — раздача может быть
// новой, а фильм старым
const (
	movieMinRating   = 6.0
	movieRecentYears = 2
)

func movieFits(e *models.Entity) bool {
	rated := e.VoteCount > 0 && e.VoteAverage > 0
	if rated && e.VoteAverage <= movieMinRating {
		return false
	}
	y := dateYear(e.ReleaseDate)
	return y == 0 || y >= time.Now().Year()-movieRecentYears
}

func filterEnts(label string, ents []*models.Entity, keep func(e *models.Entity) bool) []*models.Entity {
	before := 0
	out := utils.Filter(ents, func(i int, e *models.Entity) bool {
		if e == nil {
			return true
		}
		before++
		return !keep(e)
	})
	log.Println(label+":", "после отбора", len(out), "из", before)
	return out
}
