package publicapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aihot-server/internal/daily"
)

type fakeDailyStore struct {
	latest  *daily.Report
	byDate  map[string]*daily.Report
	entries []daily.Entry
}

func (f *fakeDailyStore) GetLatest(ctx context.Context) (*daily.Report, error) {
	if f.latest == nil {
		return nil, daily.ErrNotFound
	}
	return f.latest, nil
}
func (f *fakeDailyStore) GetByDate(ctx context.Context, date string) (*daily.Report, error) {
	if r, ok := f.byDate[date]; ok {
		return r, nil
	}
	return nil, daily.ErrNotFound
}
func (f *fakeDailyStore) ListRecent(ctx context.Context, take int) ([]daily.Entry, error) {
	if take < len(f.entries) {
		return f.entries[:take], nil
	}
	return f.entries, nil
}

func mkReport(date string) *daily.Report {
	return &daily.Report{
		Date: date, GeneratedAt: time.Date(2026, 5, 8, 1, 0, 0, 0, time.UTC),
		WindowStart: time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC),
		Lead:        &daily.Lead{Title: "导语标题", LeadParagraph: "导语段落"},
		Sections:    []daily.Section{{Label: "模型发布/更新", Items: []daily.SectionItem{{Title: "t1", Summary: "s1", SourceURL: "https://x/1", SourceName: "Src"}}}},
		Flashes:     []daily.Flash{},
	}
}

func TestLatestDaily200And404(t *testing.T) {
	f := &fakeDailyStore{latest: mkReport("2026-05-07")}
	h := NewLatestDailyHandler(f)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/daily", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	var rep daily.Report
	if err := json.Unmarshal(rr.Body.Bytes(), &rep); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rep.Date != "2026-05-07" || rep.Lead == nil || rep.Lead.Title != "导语标题" {
		t.Fatalf("report: %+v", rep)
	}

	empty := NewLatestDailyHandler(&fakeDailyStore{})
	rr2 := httptest.NewRecorder()
	empty.ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/api/public/daily", nil))
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr2.Code)
	}

	// nil Sections/Flashes must serialize as [] (openapi arrays), not null.
	nilRep := mkReport("2026-05-06")
	nilRep.Sections = nil
	nilRep.Flashes = nil
	nh := NewLatestDailyHandler(&fakeDailyStore{latest: nilRep})
	rr3 := httptest.NewRecorder()
	nh.ServeHTTP(rr3, httptest.NewRequest(http.MethodGet, "/api/public/daily", nil))
	if rr3.Code != http.StatusOK {
		t.Fatalf("nil slices code: %d", rr3.Code)
	}
	body := rr3.Body.String()
	if !strings.Contains(body, `"sections":[]`) || !strings.Contains(body, `"flashes":[]`) {
		t.Fatalf("nil slices must serialize as empty arrays, got body: %s", body)
	}
}

func TestDailyByDateValidations(t *testing.T) {
	f := &fakeDailyStore{byDate: map[string]*daily.Report{"2026-05-07": mkReport("2026-05-07")}}
	h := NewDailyByDateHandler(f)

	ok := httptest.NewRecorder()
	h.ServeHTTP(ok, httptest.NewRequest(http.MethodGet, "/api/public/daily/2026-05-07", nil))
	if ok.Code != http.StatusOK {
		t.Fatalf("valid date: %d", ok.Code)
	}

	bad := httptest.NewRecorder()
	h.ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/api/public/daily/07-05-2026", nil))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad format want 400, got %d", bad.Code)
	}

	// Right shape but not a real calendar date must also be 400.
	notADate := httptest.NewRecorder()
	h.ServeHTTP(notADate, httptest.NewRequest(http.MethodGet, "/api/public/daily/1999-13-99", nil))
	if notADate.Code != http.StatusBadRequest {
		t.Fatalf("non-calendar date want 400, got %d", notADate.Code)
	}

	missing := httptest.NewRecorder()
	h.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/public/daily/2026-01-01", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing want 404, got %d", missing.Code)
	}
}

func TestDailiesIndexAndTakeValidation(t *testing.T) {
	lt := "L1"
	f := &fakeDailyStore{entries: []daily.Entry{
		{Date: "2026-05-07", GeneratedAt: time.Now().UTC(), LeadTitle: &lt},
		{Date: "2026-05-06", GeneratedAt: time.Now().UTC()},
	}}
	h := NewDailiesHandler(f)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/dailies", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	var env struct {
		Count int           `json:"count"`
		Items []daily.Entry `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Count != 2 || len(env.Items) != 2 || env.Items[0].Date != "2026-05-07" {
		t.Fatalf("envelope: %+v", env)
	}

	one := httptest.NewRecorder()
	h.ServeHTTP(one, httptest.NewRequest(http.MethodGet, "/api/public/dailies?take=1", nil))
	var env1 struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(one.Body.Bytes(), &env1)
	if env1.Count != 1 {
		t.Fatalf("take=1: %+v", env1)
	}

	for _, bad := range []string{"0", "181", "1.5", "abc"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/dailies?take="+bad, nil))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("take=%s want 400, got %d", bad, rr.Code)
		}
	}
}
