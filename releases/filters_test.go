package releases

import (
	"NUMParser/db/models"
	"strconv"
	"testing"
	"time"
)

func d(y int) string { return "01.06." + strconv.Itoa(y) }

func TestTvIsRecent(t *testing.T) {
	now := time.Now().Year()
	cases := []struct {
		e    models.Entity
		want bool
	}{
		{models.Entity{LastAirDate: d(2019), FirstAirDate: d(2014)}, false}, // «Кремниевая долина»
		{models.Entity{LastAirDate: d(now), FirstAirDate: d(2014)}, true},   // старый сериал, новый сезон
		{models.Entity{LastAirDate: d(now - 1)}, true},
		{models.Entity{LastAirDate: d(now - 2)}, false},
		{models.Entity{FirstAirDate: d(now)}, true},
		{models.Entity{}, true},
	}
	for i, c := range cases {
		if got := tvIsRecent(&c.e); got != c.want {
			t.Errorf("%d: %v, ждали %v", i, got, c.want)
		}
	}
}

func TestMovieFits(t *testing.T) {
	now := time.Now().Year()
	cases := []struct {
		e    models.Entity
		want bool
	}{
		{models.Entity{VoteAverage: 7.1, VoteCount: 50, ReleaseDate: d(now)}, true},
		{models.Entity{VoteAverage: 6.0, VoteCount: 50, ReleaseDate: d(now)}, false},
		{models.Entity{VoteAverage: 5.2, VoteCount: 3, ReleaseDate: d(now)}, false},
		{models.Entity{VoteCount: 0, ReleaseDate: d(now)}, true}, // оценки нет
		{models.Entity{VoteAverage: 8, VoteCount: 900, ReleaseDate: d(1999)}, false},
		{models.Entity{VoteAverage: 8, VoteCount: 900}, true},
	}
	for i, c := range cases {
		if got := movieFits(&c.e); got != c.want {
			t.Errorf("%d: %v, ждали %v", i, got, c.want)
		}
	}
	if dateYear("2025-03-30") != 2025 || dateYear("30.03.2025") != 2025 || dateYear("") != 0 {
		t.Error("dateYear")
	}
}

func TestFirstEnts(t *testing.T) {
	var ents []*models.Entity
	for i := 0; i < 10; i++ {
		ents = append(ents, &models.Entity{ID: int64(i % 7)}, nil)
	}
	out := firstEnts(ents, 5)
	if len(out) != 5 || out[0].ID != 0 || out[4].ID != 4 {
		t.Fatalf("%d", len(out))
	}
	if len(firstEnts(ents, 100)) != 7 {
		t.Fatal("повторы по id должны отсеиваться")
	}
}
