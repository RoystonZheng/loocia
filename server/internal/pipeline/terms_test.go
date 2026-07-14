package pipeline

import (
	"context"
	"reflect"
	"testing"

	"aihot-server/internal/terms"
)

type fakeTermsLLM struct{ out string }

func (f fakeTermsLLM) Complete(ctx context.Context, system, user string) (string, error) {
	return f.out, nil
}

func TestExtractParsesPlainJSON(t *testing.T) {
	x := NewTermExtractor(fakeTermsLLM{out: `{"entities":["OpenAI","GPT-5.5"],"topics":["推理模型"]}`})
	got, err := x.Extract(context.Background(), "标题", "摘要")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := Terms{Entities: []string{"OpenAI", "GPT-5.5"}, Topics: []string{"推理模型"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestExtractStripsCodeFence(t *testing.T) {
	x := NewTermExtractor(fakeTermsLLM{out: "```json\n{\"entities\":[\"英伟达\"],\"topics\":[]}\n```"})
	got, err := x.Extract(context.Background(), "t", "s")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got.Entities) != 1 || got.Entities[0] != "英伟达" || len(got.Topics) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestParseTermsSanitizes(t *testing.T) {
	// 超限截断、空串/纯空白剔除、重复剔除、超长词剔除
	long := make([]rune, 41)
	for i := range long {
		long[i] = '长'
	}
	got, err := parseTerms(`{"entities":["A","B","C","D","E","F","","A"],"topics":["x","x","  ","` + string(long) + `","y","z","w"]}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !reflect.DeepEqual(got.Entities, []string{"A", "B", "C", "D", "E"}) {
		t.Fatalf("entities: %+v", got.Entities)
	}
	if !reflect.DeepEqual(got.Topics, []string{"x", "y", "z"}) {
		t.Fatalf("topics: %+v", got.Topics)
	}
}

func TestParseTermsRejectsGarbage(t *testing.T) {
	if _, err := parseTerms("抱歉，我无法处理"); err == nil {
		t.Fatal("expected error on non-JSON output")
	}
}

func TestTermRowsEntityWinsDedupe(t *testing.T) {
	rows := termRows(Terms{Entities: []string{"OpenAI"}, Topics: []string{"OpenAI", "开源"}})
	want := []terms.Term{{Term: "OpenAI", Kind: "entity"}, {Term: "开源", Kind: "topic"}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows: %+v", rows)
	}
}

func TestTermRowsEmpty(t *testing.T) {
	if rows := termRows(Terms{}); len(rows) != 0 {
		t.Fatalf("rows: %+v", rows)
	}
}
