package tmdb

import (
	"NUMParser/db/models"
	"encoding/json"
	"testing"
)

func TestImageSize(t *testing.T) {
	got := ImageSize("http://image.tmdb.org/t/p/w780/abc.jpg", "w1280")
	if got != "http://image.tmdb.org/t/p/w1280/abc.jpg" {
		t.Fatal(got)
	}
	if ImageSize("/abc.jpg", "w1280") != "/abc.jpg" {
		t.Fatal("не TMDB-ссылка должна остаться как есть")
	}
}

func TestPickLogo(t *testing.T) {
	var imgs models.Images
	json.Unmarshal([]byte(`{"logos":[
		{"file_path":"/en.png","iso_639_1":"en","vote_average":9,"width":500},
		{"file_path":"/ru.svg","iso_639_1":"ru","vote_average":9,"width":900},
		{"file_path":"/ru_low.png","iso_639_1":"ru","vote_average":1,"width":300},
		{"file_path":"/ru_hi.png","iso_639_1":"ru","vote_average":5,"width":300},
		{"file_path":"/null.png","iso_639_1":null,"vote_average":10}
	]}`), &imgs)
	l := pickLogo(&imgs)
	if l == nil || l.FilePath != "/ru_hi.png" || l.Lang != "ru" {
		t.Fatalf("%+v", l)
	}
	var only models.Images
	json.Unmarshal([]byte(`{"logos":[{"file_path":"/n.png","iso_639_1":null}]}`), &only)
	if l := pickLogo(&only); l == nil || l.FilePath != "/n.png" || l.Lang != "" {
		t.Fatalf("%+v", l)
	}
	if pickLogo(&models.Images{}) != nil {
		t.Fatal("без логотипов — nil")
	}
}
