package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/chriscasey/watchwhat/internal/tmdb/tmdbtest"
)

const searchJSON = `{"results":[
 {"id":100,"media_type":"tv","name":"Shōgun","first_air_date":"2024-02-27","poster_path":"/s.jpg"},
 {"id":7,"media_type":"movie","title":"Shogun Assassin","release_date":"1980-11-11"}]}`

// ---- 4.4 search ----

func TestSearchEmptyQueryNoTMDBCall(t *testing.T) {
	h := newHarness(t)
	for _, q := range []string{"", "   "} {
		rec := h.hx(http.MethodGet, "/search/results", url.Values{"q": {q}})
		if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "" {
			t.Fatalf("q=%q: %d %q", q, rec.Code, rec.Body.String())
		}
	}
	if h.fake.Calls() != 0 {
		t.Fatalf("TMDB called %d times", h.fake.Calls())
	}
}

func TestSearchShowsStatusAndDoesNotPersist(t *testing.T) {
	h := newHarness(t)
	h.fake.SearchJSON = searchJSON
	rec := h.hx(http.MethodGet, "/search/results", url.Values{"q": {"shogun"}})
	body := rec.Body.String()
	if rec.Code != 200 || isDocument(body) {
		t.Fatalf("%d doc=%v", rec.Code, isDocument(body))
	}
	mustContain(t, body, "Shōgun", "2024", "Shogun Assassin", `hx-post="/media"`, "https://image.tmdb.org/t/p/w92/s.jpg")
	if h.count(`SELECT COUNT(*) FROM media`) != 0 {
		t.Fatal("search persisted results")
	}

	h.addShow(100, "Shōgun", "Ended", "watching")
	body = h.hx(http.MethodGet, "/search/results", url.Values{"q": {"shogun"}}).Body.String()
	if strings.Count(body, `hx-post="/media"`) != 1 || !strings.Contains(body, ">Watching</a>") {
		t.Fatalf("already-added row should show status: %s", body)
	}
}

func TestAddFromSearchSwapsRow(t *testing.T) {
	h := newHarness(t)
	h.fake.Shows[100] = &tmdbtest.Show{ID: 100, Name: "Shōgun", Status: "Ended", FirstAir: "2024-02-27", Episodes: eps(1, 3, 1000, "2024-02-27")}
	form := url.Values{"tmdb_id": {"100"}, "media_type": {"tv"}}
	rec := h.hx(http.MethodPost, "/media", form)
	body := rec.Body.String()
	if rec.Code != 200 || !strings.HasPrefix(strings.TrimSpace(body), "<li") || !strings.Contains(body, ">Backlog</a>") {
		t.Fatalf("%d %s", rec.Code, body)
	}
	if h.count(`SELECT COUNT(*) FROM episodes`) != 3 {
		t.Fatal("episodes not stored")
	}
	// Movie as watching
	h.fake.Movies[7] = &tmdbtest.Movie{ID: 7, Title: "Heat"}
	body = h.hx(http.MethodPost, "/media", url.Values{"tmdb_id": {"7"}, "media_type": {"movie"}, "status": {"watching"}}).Body.String()
	mustContain(t, body, ">Watching</a>")
}

func TestAddTMDBOutageToast(t *testing.T) {
	h := newHarness(t)
	h.fake.Down = true
	rec := h.hx(http.MethodPost, "/media", url.Values{"tmdb_id": {"100"}, "media_type": {"tv"}})
	if rec.Code != http.StatusBadGateway || rec.Header().Get("HX-Retarget") != "#toasts" {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
}

func TestCachedViewsWorkWhenTMDBDown(t *testing.T) {
	h := newHarness(t)
	id := h.addShow(1, "Show", "Returning Series", "watching", eps(1, 2, 10, "2024-01-01")...)
	h.app.Sched.Assign(t.Context(), 2, id)
	h.fake.Set(func(f *tmdbtest.Fake) { f.Down = true })
	for _, p := range []string{"/", "/watching", "/backlog", "/schedule", "/media/" + itoa(id)} {
		if rec := h.get(p); rec.Code != 200 {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
}

// ---- 4.6 detail ----

func TestDetailRefreshUpdatesFragment(t *testing.T) {
	h := newHarness(t)
	id := h.addShow(1, "Show", "Returning Series", "watching", eps(1, 2, 10, "2024-01-01")...)
	body := h.get("/media/" + itoa(id)).Body.String()
	mustContain(t, body, `id="detail"`, "Refresh from TMDB", `hx-delete="/media/`+itoa(id)+`"`, "hx-confirm", `id="progress-`+itoa(id)+`"`)
	h.fake.Set(func(f *tmdbtest.Fake) {
		f.Shows[1].Episodes = append(f.Shows[1].Episodes, tmdbtest.Episode{ID: 99, Season: 1, Number: 3, Name: "Brand New", AirDate: "2024-06-01"})
	})
	rec := h.hx(http.MethodPost, "/media/"+itoa(id)+"/refresh", nil)
	body = rec.Body.String()
	if rec.Code != 200 || isDocument(body) || !strings.HasPrefix(strings.TrimSpace(body), `<div id="detail"`) {
		t.Fatalf("%d %.200s", rec.Code, body)
	}
	mustContain(t, body, "Brand New", "0/3")
}

func TestRemove(t *testing.T) {
	h := newHarness(t)
	h.fake.SearchJSON = searchJSON
	id := h.addShow(100, "Shōgun", "Ended", "watching", eps(1, 2, 10, "2024-01-01")...)
	h.app.Sched.Assign(t.Context(), 2, id)
	rec := h.hx(http.MethodDelete, "/media/"+itoa(id), nil)
	if rec.Code != 200 || rec.Header().Get("HX-Redirect") != "/watching" {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	mustNotContain(t, h.get("/watching").Body.String(), "Shōgun")
	mustNotContain(t, h.get("/schedule").Body.String(), "Shōgun</a>")
	body := h.hx(http.MethodGet, "/search/results", url.Values{"q": {"shogun"}}).Body.String()
	if strings.Count(body, `hx-post="/media"`) != 2 {
		t.Fatal("removed show should be addable again")
	}
}

// ---- 5.3 cards / mark next ----

func TestMarkNextTwiceAdvancesOnce(t *testing.T) {
	h := newHarness(t)
	h.addShow(1, "Show", "Returning Series", "watching", eps(1, 5, 10, "2024-01-01")...)
	e1 := h.episodeID(11)
	body := h.get("/watching").Body.String()
	mustContain(t, body, `hx-put="/episodes/`+itoa(e1)+`/watched"`, `hx-disabled-elt="this"`, `hx-sync="closest article:drop"`, "S01E01", "0/5")

	var last string
	for range 2 {
		rec := h.hx(http.MethodPut, "/episodes/"+itoa(e1)+"/watched", url.Values{"ctx": {"watching"}})
		if rec.Code != 200 {
			t.Fatalf("%d", rec.Code)
		}
		last = rec.Body.String()
	}
	if n := h.count(`SELECT COUNT(*) FROM episode_watches`); n != 1 {
		t.Fatalf("watch rows=%d", n)
	}
	if !strings.HasPrefix(strings.TrimSpace(last), "<article") || isDocument(last) {
		t.Fatalf("not a card: %.100s", last)
	}
	mustContain(t, last, "S01E02", "1/5", `hx-delete="/episodes/`+itoa(e1)+`/watched"`, "Undo")
	mustNotContain(t, last, "S01E03")

	// Undo returns to E1 and the undo button disappears.
	undo := h.hx(http.MethodDelete, "/episodes/"+itoa(e1)+"/watched", url.Values{"ctx": {"watching"}}).Body.String()
	mustContain(t, undo, "S01E01", "0/5")
	mustNotContain(t, undo, "Undo")
}

func TestStaleBadgeClearedByProgress(t *testing.T) {
	h := newHarness(t)
	h.now = tuesday.AddDate(0, 0, -21)
	h.addShow(1, "Show", "Returning Series", "watching", eps(1, 3, 10, "2024-01-01")...)
	h.now = tuesday
	body := h.get("/watching").Body.String()
	mustContain(t, body, "Stale · 21 days")
	card := h.hx(http.MethodPut, "/episodes/"+itoa(h.episodeID(11))+"/watched", url.Values{"ctx": {"watching"}}).Body.String()
	mustNotContain(t, card, "Stale")
}

// ---- 5.4 completion prompt ----

func TestFinaleOffersCompletion(t *testing.T) {
	h := newHarness(t)
	id := h.addShow(1, "Show", "Ended", "watching", eps(1, 2, 10, "2024-01-01")...)
	h.hx(http.MethodPut, "/episodes/"+itoa(h.episodeID(11))+"/watched", url.Values{"ctx": {"watching"}})
	card := h.hx(http.MethodPut, "/episodes/"+itoa(h.episodeID(12))+"/watched", url.Values{"ctx": {"watching"}}).Body.String()
	mustContain(t, card, "Mark completed", "Caught up")
	if st := h.count(`SELECT COUNT(*) FROM tracking WHERE media_id = ? AND status = 'watching'`, id); st != 1 {
		t.Fatal("status changed automatically")
	}
	rec := h.hx(http.MethodPost, "/media/"+itoa(id)+"/status", url.Values{"status": {"completed"}, "ctx": {"watching"}})
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "" {
		t.Fatalf("completed card should leave Watching: %d %q", rec.Code, rec.Body.String())
	}
	if h.count(`SELECT COUNT(*) FROM tracking WHERE media_id = ? AND status = 'completed'`, id) != 1 {
		t.Fatal("not completed")
	}
}

func TestReturningShowNoCompletionPrompt(t *testing.T) {
	h := newHarness(t)
	h.addShow(1, "Show", "Returning Series", "watching", eps(1, 1, 10, "2024-01-01")...)
	card := h.hx(http.MethodPut, "/episodes/"+itoa(h.episodeID(11))+"/watched", url.Values{"ctx": {"watching"}}).Body.String()
	mustNotContain(t, card, "Mark completed")
}

// ---- 5.5 list pages ----

func TestListPages(t *testing.T) {
	h := newHarness(t)
	h.now = tuesday.AddDate(0, 0, -30)
	h.addShow(1, "Old Stale", "Returning Series", "watching", eps(1, 2, 10, "2024-01-01")...)
	h.now = tuesday.AddDate(0, 0, -20)
	h.addShow(2, "Less Stale", "Returning Series", "watching", eps(1, 2, 20, "2024-01-01")...)
	h.now = tuesday
	h.addShow(3, "Fresh", "Returning Series", "watching", eps(1, 2, 30, "2024-01-01")...)
	h.addMovie(7, "A Movie", "watching")
	h.addShow(4, "Queued", "Ended", "backlog")
	h.addMovie(8, "Queued Movie", "backlog")

	order := func(body string, titles ...string) bool {
		last := -1
		for _, s := range titles {
			i := strings.Index(body, `title="`+s+`"`)
			if i < 0 || i < last {
				return false
			}
			last = i
		}
		return true
	}

	body := h.get("/watching").Body.String()
	if !order(body, "Fresh", "Less Stale", "Old Stale") {
		t.Error("default sort should be most recent activity first")
	}
	mustNotContain(t, body, "Queued")

	body = h.get("/watching?sort=stale").Body.String()
	if !order(body, "Old Stale", "Less Stale", "Fresh") {
		t.Error("most stale first")
	}
	mustContain(t, body, `href="/watching?sort=stale&amp;stale=1"`)

	body = h.get("/watching?stale=1").Body.String()
	mustContain(t, body, "Old Stale", "Less Stale")
	mustNotContain(t, body, `title="Fresh"`, `title="A Movie"`)

	body = h.get("/backlog?type=movie").Body.String()
	mustContain(t, body, `title="Queued Movie"`)
	mustNotContain(t, body, `title="Queued"`)
	body = h.get("/backlog?type=tv").Body.String()
	mustContain(t, body, `title="Queued"`)
	mustNotContain(t, body, `title="Queued Movie"`)

	// stale/sort ignored outside Watching; bogus type ignored
	body = h.get("/backlog?stale=1&type=bogus").Body.String()
	mustContain(t, body, `title="Queued"`, `title="Queued Movie"`)

	for _, p := range []string{"/completed", "/dropped"} {
		if rec := h.get(p); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Nothing here") {
			t.Errorf("%s empty state", p)
		}
	}
}

// ---- 5.6 ctx-aware status change ----

func TestStatusChangeCtx(t *testing.T) {
	h := newHarness(t)
	id := h.addShow(1, "Show", "Ended", "backlog", eps(1, 2, 10, "2024-01-01")...)
	rec := h.hx(http.MethodPost, "/media/"+itoa(id)+"/status", url.Values{"status": {"dropped"}, "ctx": {"backlog"}})
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "" {
		t.Fatalf("leaving list: %d %q", rec.Code, rec.Body.String())
	}
	rec = h.hx(http.MethodPost, "/media/"+itoa(id)+"/status", url.Values{"status": {"watching"}, "ctx": {"lineup"}})
	if !strings.HasPrefix(strings.TrimSpace(rec.Body.String()), "<article") {
		t.Fatalf("staying in lineup should return card: %q", rec.Body.String())
	}
	mustContain(t, h.get("/watching").Body.String(), `title="Show"`)
	rec = h.hx(http.MethodPost, "/media/"+itoa(id)+"/status", url.Values{"status": {"watching"}, "ctx": {"detail"}})
	mustContain(t, rec.Body.String(), `id="detail"`)
	rec = h.hx(http.MethodPost, "/media/"+itoa(id)+"/status", url.Values{"status": {"bogus"}, "ctx": {"watching"}})
	if rec.Code != 400 || rec.Header().Get("HX-Retarget") != "#toasts" {
		t.Fatalf("invalid status: %d", rec.Code)
	}
}

func TestMovieWatched(t *testing.T) {
	h := newHarness(t)
	id := h.addMovie(7, "Heat", "watching")
	rec := h.hx(http.MethodPost, "/media/"+itoa(id)+"/watched", url.Values{"ctx": {"watching"}})
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "" {
		t.Fatalf("%d %q", rec.Code, rec.Body.String())
	}
	mustContain(t, h.get("/completed").Body.String(), `title="Heat"`, "Watched Tue, Jun 18")
}

// ---- 5.7 detail episode toggles ----

func TestEpisodeToggleOOBCounter(t *testing.T) {
	h := newHarness(t)
	id := h.addShow(1, "Show", "Returning Series", "watching", eps(1, 3, 10, "2024-01-01")...)
	e2 := h.episodeID(12)
	rec := h.hx(http.MethodPut, "/episodes/"+itoa(e2)+"/watched", url.Values{"ctx": {"detail"}})
	body := rec.Body.String()
	if rec.Code != 200 || !strings.HasPrefix(strings.TrimSpace(body), "<li") {
		t.Fatalf("%d %.100s", rec.Code, body)
	}
	mustContain(t, body, `hx-delete="/episodes/`+itoa(e2)+`/watched"`, `id="progress-`+itoa(id)+`"`, `hx-swap-oob="true"`, "1/3")
	body = h.hx(http.MethodDelete, "/episodes/"+itoa(e2)+"/watched", url.Values{"ctx": {"detail"}}).Body.String()
	mustContain(t, body, `hx-put="/episodes/`+itoa(e2)+`/watched"`, "0/3", `hx-swap-oob="true"`)
}

func TestSeasonMarkAiredOnly(t *testing.T) {
	h := newHarness(t)
	e := eps(1, 2, 10, "2024-01-01")
	e = append(e, tmdbtest.Episode{ID: 13, Season: 1, Number: 3, AirDate: "2024-07-01"})
	id := h.addShow(1, "Show", "Returning Series", "watching", e...)
	rec := h.hx(http.MethodPut, "/media/"+itoa(id)+"/seasons/1/watched", nil)
	body := rec.Body.String()
	if rec.Code != 200 || !strings.HasPrefix(strings.TrimSpace(body), "<section") {
		t.Fatalf("%d %.100s", rec.Code, body)
	}
	mustContain(t, body, `hx-swap-oob="true"`, "2/2")
	if h.count(`SELECT COUNT(*) FROM episode_watches`) != 2 {
		t.Fatal("unaired episode marked")
	}
}

// ---- 6.2 schedule ----

func TestScheduleActionsReturnOneDay(t *testing.T) {
	h := newHarness(t)
	a := h.addShow(1, "Alpha", "Returning Series", "watching")
	b := h.addShow(2, "Beta", "Returning Series", "backlog")
	page := h.get("/schedule").Body.String()
	mon := strings.Index(page, `aria-label="Monday"`)
	sun := strings.Index(page, `aria-label="Sunday"`)
	if mon < 0 || sun < mon || !strings.Contains(page, "Today") {
		t.Fatal("planner should start Monday and mark today")
	}

	for _, step := range []struct {
		method, path string
		form         url.Values
	}{
		{http.MethodPost, "/schedule/5", url.Values{"media_id": {itoa(a)}}},
		{http.MethodPost, "/schedule/5", url.Values{"media_id": {itoa(b)}}},
		{http.MethodPost, "/schedule/5", url.Values{"media_id": {itoa(b)}}}, // duplicate
		{http.MethodPost, "/schedule/5/" + itoa(b) + "/move", url.Values{"dir": {"up"}}},
	} {
		rec := h.hx(step.method, step.path, step.form)
		body := rec.Body.String()
		if rec.Code != 200 || strings.Count(body, "<section") != 1 || !strings.Contains(body, `aria-label="Friday"`) || isDocument(body) {
			t.Fatalf("%s %s: %d %.120s", step.method, step.path, rec.Code, body)
		}
	}
	fri := h.hx(http.MethodPost, "/schedule/5/"+itoa(a)+"/move", url.Values{"dir": {"down"}}).Body.String()
	if strings.Index(fri, ">Beta</a>") > strings.Index(fri, ">Alpha</a>") || strings.Count(fri, ">Beta</a>") != 1 {
		t.Fatal("order/dup wrong")
	}
	rec := h.hx(http.MethodDelete, "/schedule/5/"+itoa(a), nil)
	mustNotContain(t, rec.Body.String(), ">Alpha</a>")
	mustContain(t, rec.Body.String(), ">Beta</a>")

	// Rejections
	m := h.addMovie(7, "Movie", "watching")
	if rec := h.hx(http.MethodPost, "/schedule/5", url.Values{"media_id": {itoa(m)}}); rec.Code != 422 || rec.Header().Get("HX-Retarget") != "#toasts" {
		t.Fatalf("movie: %d", rec.Code)
	}
	if rec := h.hx(http.MethodPost, "/schedule/9", url.Values{"media_id": {itoa(a)}}); rec.Code != 404 {
		t.Fatalf("bad day: %d", rec.Code)
	}
	if rec := h.hx(http.MethodPost, "/schedule/5/"+itoa(b)+"/move", url.Values{"dir": {"sideways"}}); rec.Code != 400 {
		t.Fatalf("bad dir: %d", rec.Code)
	}
}

func TestScheduleDroppedInactive(t *testing.T) {
	h := newHarness(t)
	a := h.addShow(1, "Alpha", "Returning Series", "watching")
	h.app.Sched.Assign(t.Context(), 2, a) // Tuesday = today
	h.hx(http.MethodPost, "/media/"+itoa(a)+"/status", url.Values{"status": {"dropped"}, "ctx": {"watching"}})
	mustContain(t, h.get("/schedule").Body.String(), ">Alpha</a>", "inactive")
	mustNotContain(t, h.get("/").Body.String(), `title="Alpha"`)
}

// ---- 6.3 home ----

func TestHomeTodaysLineup(t *testing.T) {
	h := newHarness(t)
	sh := h.addShow(1, "Shōgun", "Returning Series", "watching", eps(1, 3, 10, "2024-01-01")...)
	other := h.addShow(2, "Wednesday Show", "Returning Series", "watching", eps(1, 1, 20, "2024-01-01")...)
	h.app.Sched.Assign(t.Context(), 2, sh)    // Tuesday
	h.app.Sched.Assign(t.Context(), 3, other) // Wednesday

	body := h.get("/").Body.String()
	lineup := strings.Index(body, "Today's Lineup")
	cont := strings.Index(body, "Continue watching")
	if lineup < 0 || cont < lineup {
		t.Fatal("Lineup must appear above Continue watching")
	}
	mustContain(t, body[lineup:cont], "Tuesday", `title="Shōgun"`, `hx-put="/episodes/`+itoa(h.episodeID(11))+`/watched"`, `&#34;ctx&#34;:&#34;lineup&#34;`)
	mustNotContain(t, body[lineup:cont], `title="Wednesday Show"`)
	mustContain(t, body[cont:], `title="Shōgun"`, `title="Wednesday Show"`)

	// Marking from the lineup updates the card in place like in Watching.
	card := h.hx(http.MethodPut, "/episodes/"+itoa(h.episodeID(11))+"/watched", url.Values{"ctx": {"lineup"}}).Body.String()
	mustContain(t, card, "S01E02", "1/3", "Undo")

	// Wednesday: different lineup.
	h.now = tuesday.AddDate(0, 0, 1)
	body = h.get("/").Body.String()
	mustContain(t, body[:strings.Index(body, "Continue watching")], "Wednesday", `title="Wednesday Show"`)
}

func TestHomeEmptyDay(t *testing.T) {
	h := newHarness(t)
	body := h.get("/").Body.String()
	mustContain(t, body, "Nothing scheduled for Tuesday", `href="/schedule"`)
}

func TestHomeTodayUsesServerTZ(t *testing.T) {
	h := newHarness(t)
	sh := h.addShow(1, "Late", "Returning Series", "watching", eps(1, 1, 10, "2024-01-01")...)
	h.app.Sched.Assign(t.Context(), 2, sh)
	// 23:30 Tuesday in Chicago is already Wednesday in UTC.
	h.now = tuesday.Add(3*time.Hour + 30*time.Minute)
	body := h.get("/").Body.String()
	mustContain(t, body[:strings.Index(body, "Continue watching")], `title="Late"`)
}

func TestContinueWatchingLimit(t *testing.T) {
	h := newHarness(t)
	for i := int64(1); i <= 10; i++ {
		h.now = tuesday.Add(time.Duration(i) * time.Minute)
		h.addShow(i, "Show"+itoa(i), "Returning Series", "watching")
	}
	body := h.get("/").Body.String()
	cont := body[strings.Index(body, "Continue watching"):]
	if n := strings.Count(cont, "<article"); n != 8 {
		t.Fatalf("continue watching shows %d", n)
	}
	mustContain(t, cont, `title="Show10"`)
	mustNotContain(t, cont, `title="Show1"`, `title="Show2"`)
}
