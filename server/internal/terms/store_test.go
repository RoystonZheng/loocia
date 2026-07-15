package terms

import (
	"context"
	"os"
	"testing"
	"time"

	"aihot-server/internal/db"
	"aihot-server/internal/items"
)

// newTestStores brings up items (FK prerequisite) + terms schemas on the test DB.
func newTestStores(t *testing.T) (*items.Store, *Store) {
	t.Helper()
	dsn := os.Getenv("AIHOT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AIHOT_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	t.Cleanup(pool.Close)
	is := items.New(pool)
	if err := is.EnsureSchema(ctx); err != nil {
		t.Fatalf("items EnsureSchema: %v", err)
	}
	ts := NewStore(pool)
	if err := ts.EnsureSchema(ctx); err != nil {
		t.Fatalf("terms EnsureSchema: %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE items CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return is, ts
}

func seedItem(t *testing.T, is *items.Store, id string, published time.Time) {
	t.Helper()
	if err := is.Upsert(context.Background(), items.Item{
		ID: id, Title: "title-" + id, URL: "https://example.com/" + id,
		Permalink: "/items/" + id, Source: "Example", PublishedAt: &published, Present: true,
	}); err != nil {
		t.Fatalf("upsert %s: %v", id, err)
	}
}

func TestReplaceForItemIdempotent(t *testing.T) {
	is, ts := newTestStores(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedItem(t, is, "a", now)

	if err := ts.ReplaceForItem(ctx, "a", []Term{{Term: "OpenAI", Kind: "entity"}, {Term: "开源", Kind: "topic"}}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	// 重跑换一套词：旧词必须被清掉
	if err := ts.ReplaceForItem(ctx, "a", []Term{{Term: "英伟达", Kind: "entity"}}); err != nil {
		t.Fatalf("replace2: %v", err)
	}
	cloud, err := ts.Cloud(ctx, nil, nil, 10)
	if err != nil {
		t.Fatalf("cloud: %v", err)
	}
	if len(cloud) != 1 || cloud[0].Term != "英伟达" || cloud[0].Count != 1 {
		t.Fatalf("cloud after replace: %+v", cloud)
	}
}

func TestCloudCountsWindowAndKind(t *testing.T) {
	is, ts := newTestStores(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedItem(t, is, "new1", now.Add(-1*time.Hour))
	seedItem(t, is, "new2", now.Add(-2*time.Hour))
	seedItem(t, is, "old", now.Add(-40*24*time.Hour))

	must := func(id string, ts2 []Term) {
		if err := ts.ReplaceForItem(ctx, id, ts2); err != nil {
			t.Fatalf("replace %s: %v", id, err)
		}
	}
	must("new1", []Term{{Term: "OpenAI", Kind: "entity"}, {Term: "开源", Kind: "topic"}})
	must("new2", []Term{{Term: "OpenAI", Kind: "entity"}})
	must("old", []Term{{Term: "OpenAI", Kind: "entity"}, {Term: "远古话题", Kind: "topic"}})

	// all（since=nil）：OpenAI=3 居首
	all, err := ts.Cloud(ctx, nil, nil, 10)
	if err != nil {
		t.Fatalf("cloud all: %v", err)
	}
	if len(all) != 3 || all[0].Term != "OpenAI" || all[0].Count != 3 || all[0].Kind != "entity" {
		t.Fatalf("cloud all: %+v", all)
	}
	// 7d 窗口：old 被滤掉
	since := now.Add(-7 * 24 * time.Hour)
	recent, err := ts.Cloud(ctx, &since, nil, 10)
	if err != nil {
		t.Fatalf("cloud 7d: %v", err)
	}
	if len(recent) != 2 || recent[0].Count != 2 {
		t.Fatalf("cloud 7d: %+v", recent)
	}
	for _, c := range recent {
		if c.Term == "远古话题" {
			t.Fatalf("window leak: %+v", recent)
		}
	}
}

func TestNeighborsCooccurrenceAndClusterBoost(t *testing.T) {
	is, ts := newTestStores(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedItem(t, is, "plain", now.Add(-1*time.Hour))
	seedItem(t, is, "clustered", now.Add(-2*time.Hour))
	// clustered 属于一个多源事件 cluster → 该条里的共现权重 ×2
	if err := is.AssignCluster(ctx, "clustered", "clustered", true); err != nil {
		t.Fatalf("assign cluster: %v", err)
	}

	if err := ts.ReplaceForItem(ctx, "plain", []Term{{Term: "OpenAI", Kind: "entity"}, {Term: "开源", Kind: "topic"}}); err != nil {
		t.Fatalf("replace plain: %v", err)
	}
	if err := ts.ReplaceForItem(ctx, "clustered", []Term{{Term: "OpenAI", Kind: "entity"}, {Term: "推理模型", Kind: "topic"}}); err != nil {
		t.Fatalf("replace clustered: %v", err)
	}

	ns, err := ts.Neighbors(ctx, "OpenAI", nil, nil, 10)
	if err != nil {
		t.Fatalf("neighbors: %v", err)
	}
	if len(ns) != 2 {
		t.Fatalf("neighbors: %+v", ns)
	}
	// 加权后 推理模型(2) 排在 开源(1) 前面
	if ns[0].Term != "推理模型" || ns[0].Weight != 2 || ns[1].Term != "开源" || ns[1].Weight != 1 {
		t.Fatalf("weights: %+v", ns)
	}
}

func TestItemsForTermAndTermInfo(t *testing.T) {
	is, ts := newTestStores(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedItem(t, is, "newer", now.Add(-1*time.Hour))
	seedItem(t, is, "older", now.Add(-3*time.Hour))
	for _, id := range []string{"newer", "older"} {
		if err := ts.ReplaceForItem(ctx, id, []Term{{Term: "OpenAI", Kind: "entity"}}); err != nil {
			t.Fatalf("replace %s: %v", id, err)
		}
	}

	its, err := ts.ItemsForTerm(ctx, "OpenAI", nil, nil, 10)
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	if len(its) != 2 || its[0].ID != "newer" || its[1].ID != "older" {
		t.Fatalf("order: %+v", its)
	}
	if its[0].Title != "title-newer" || its[0].Permalink != "/items/newer" {
		t.Fatalf("fields: %+v", its[0])
	}

	kind, count, err := ts.TermInfo(ctx, "OpenAI", nil, nil)
	if err != nil {
		t.Fatalf("terminfo: %v", err)
	}
	if kind != "entity" || count != 2 {
		t.Fatalf("terminfo: kind=%q count=%d", kind, count)
	}
	// 不存在的词：count=0 不报错
	kind, count, err = ts.TermInfo(ctx, "没有的词", nil, nil)
	if err != nil || count != 0 || kind != "" {
		t.Fatalf("missing term: kind=%q count=%d err=%v", kind, count, err)
	}
}
