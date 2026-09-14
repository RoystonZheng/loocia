package tools

import (
	"context"
	"testing"
)

func createDiscoveredTool(t *testing.T, s *Store, nodeID string) Tool {
	t.Helper()
	upsert, err := s.ManualAdd(context.Background(), sampleRepo(nodeID, "openai", nodeID), "alice")
	if err != nil {
		t.Fatalf("ManualAdd: %v", err)
	}
	return upsert.Tool
}

func TestStartEvaluationAllowsDeferredCooperURLAndRejectsDuplicateStart(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tool := createDiscoveredTool(t, s, "agents")

	eval, err := s.StartEvaluation(ctx, tool.ID, "张三", "", "李四")
	if err != nil {
		t.Fatalf("StartEvaluation: %v", err)
	}
	if eval.ToolID != tool.ID || eval.Evaluator != "张三" || eval.Operator != "李四" || eval.CooperURL != nil {
		t.Fatalf("evaluation mismatch: %+v", eval)
	}

	items, err := s.ListTools(ctx, ListToolsParams{Status: ToolEvaluating, Limit: 10})
	if err != nil {
		t.Fatalf("ListTools evaluating: %v", err)
	}
	if len(items) != 1 || items[0].ID != tool.ID {
		t.Fatalf("tool should move to evaluating: %+v", items)
	}

	_, err = s.StartEvaluation(ctx, tool.ID, "王五", "", "王五")
	if err == nil {
		t.Fatal("duplicate StartEvaluation should fail")
	}
	var derr *DiscoveryError
	if !AsDiscoveryError(err, &derr) || derr.Class != ErrorStatusConflict {
		t.Fatalf("duplicate start error mismatch: %#v", err)
	}

	var events int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM tool_status_events WHERE tool_id=$1 AND to_status='evaluating'", tool.ID).Scan(&events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if events != 1 {
		t.Fatalf("want one status event, got %d", events)
	}
}

func TestUpdateEvaluationCooperURLValidatesDomain(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tool := createDiscoveredTool(t, s, "browser-agent")
	if _, err := s.StartEvaluation(ctx, tool.ID, "alice", "", "alice"); err != nil {
		t.Fatalf("StartEvaluation: %v", err)
	}

	if err := s.UpdateEvaluationCooperURL(ctx, tool.ID, "https://example.com/didocs/1", "alice"); err == nil {
		t.Fatal("invalid cooper URL should fail")
	}
	if err := s.UpdateEvaluationCooperURL(ctx, tool.ID, "https://cooper.didichuxing.com/didocs/2209600482571", "alice"); err != nil {
		t.Fatalf("UpdateEvaluationCooperURL: %v", err)
	}
	eval, err := s.GetActiveEvaluation(ctx, tool.ID)
	if err != nil {
		t.Fatalf("GetActiveEvaluation: %v", err)
	}
	if eval.CooperURL == nil || *eval.CooperURL != "https://cooper.didichuxing.com/didocs/2209600482571" {
		t.Fatalf("cooper URL mismatch: %+v", eval.CooperURL)
	}
}

func TestExcludeDiscoveredMovesToolOutOfDiscoveredList(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tool := createDiscoveredTool(t, s, "noise-tool")

	if err := s.ExcludeDiscovered(ctx, tool.ID, "不是 AI 工具", "alice"); err != nil {
		t.Fatalf("ExcludeDiscovered: %v", err)
	}
	discovered, err := s.ListTools(ctx, ListToolsParams{Status: ToolDiscovered, Limit: 10})
	if err != nil {
		t.Fatalf("ListTools discovered: %v", err)
	}
	if len(discovered) != 0 {
		t.Fatalf("excluded tool should leave discovered list: %+v", discovered)
	}
	excluded, err := s.ListTools(ctx, ListToolsParams{Status: ToolExcluded, Limit: 10})
	if err != nil {
		t.Fatalf("ListTools excluded: %v", err)
	}
	if len(excluded) != 1 || excluded[0].ExcludedStage == nil || *excluded[0].ExcludedStage != string(ToolDiscovered) || excluded[0].ExcludedReason == nil {
		t.Fatalf("excluded metadata mismatch: %+v", excluded)
	}
}

func TestFinishEvaluationRequiresCooperURLAndMovesToIncludedOrExcluded(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tool := createDiscoveredTool(t, s, "include-tool")
	eval, err := s.StartEvaluation(ctx, tool.ID, "alice", "", "alice")
	if err != nil {
		t.Fatalf("StartEvaluation: %v", err)
	}

	err = s.FinishEvaluation(ctx, FinishEvaluationRequest{
		ToolID:       tool.ID,
		EvaluationID: eval.ID,
		Result:       EvaluationIncluded,
		Operator:     "alice",
		FinalSummary: "这是一个面向研发团队的 AI agent 工具，适合用于自动化代码理解、任务拆解和开发辅助。",
	})
	if err == nil {
		t.Fatal("finish without cooper URL should fail")
	}
	var derr *DiscoveryError
	if !AsDiscoveryError(err, &derr) || derr.Class != ErrorMissingCooperURL {
		t.Fatalf("missing cooper URL error mismatch: %#v", err)
	}

	if err := s.UpdateEvaluationCooperURL(ctx, tool.ID, "https://cooper.didichuxing.com/didocs/1", "alice"); err != nil {
		t.Fatalf("UpdateEvaluationCooperURL: %v", err)
	}
	if err := s.FinishEvaluation(ctx, FinishEvaluationRequest{
		ToolID:       tool.ID,
		EvaluationID: eval.ID,
		Result:       EvaluationIncluded,
		Operator:     "alice",
		FinalSummary: "这是一个面向研发团队的 AI agent 工具，适合用于自动化代码理解、任务拆解和开发辅助。",
	}); err != nil {
		t.Fatalf("FinishEvaluation included: %v", err)
	}
	included, err := s.ListTools(ctx, ListToolsParams{Status: ToolIncluded, Limit: 10})
	if err != nil {
		t.Fatalf("ListTools included: %v", err)
	}
	if len(included) != 1 || included[0].IncludedAt == nil || included[0].FinalSummary == nil {
		t.Fatalf("included tool mismatch: %+v", included)
	}

	tool2 := createDiscoveredTool(t, s, "exclude-after-eval")
	eval2, err := s.StartEvaluation(ctx, tool2.ID, "bob", "https://cooper.didichuxing.com/didocs/2", "bob")
	if err != nil {
		t.Fatalf("StartEvaluation 2: %v", err)
	}
	if err := s.FinishEvaluation(ctx, FinishEvaluationRequest{
		ToolID:            tool2.ID,
		EvaluationID:      eval2.ID,
		Result:            EvaluationExcluded,
		Operator:          "bob",
		NotIncludedReason: "安装复杂，当前团队没有稳定使用场景。",
	}); err != nil {
		t.Fatalf("FinishEvaluation excluded: %v", err)
	}
	excluded, err := s.ListTools(ctx, ListToolsParams{Status: ToolExcluded, Limit: 10})
	if err != nil {
		t.Fatalf("ListTools excluded: %v", err)
	}
	if len(excluded) != 1 || excluded[0].ExcludedStage == nil || *excluded[0].ExcludedStage != string(ToolEvaluating) {
		t.Fatalf("evaluating exclusion mismatch: %+v", excluded)
	}
}

func TestFinishEvaluationRejectsNonEvaluatingTool(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tool := createDiscoveredTool(t, s, "not-evaluating")
	err := s.FinishEvaluation(ctx, FinishEvaluationRequest{
		ToolID:            tool.ID,
		EvaluationID:      "missing",
		Result:            EvaluationExcluded,
		Operator:          "alice",
		NotIncludedReason: "不适合当前阶段。",
	})
	if err == nil {
		t.Fatal("finishing non-evaluating tool should fail")
	}
	var derr *DiscoveryError
	if !AsDiscoveryError(err, &derr) || derr.Class != ErrorStatusConflict {
		t.Fatalf("status conflict mismatch: %#v", err)
	}
}

func TestTeamToolImportUpdateAndSoftDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	imported, err := s.ImportTeamTool(ctx, sampleRepo("node-team", "openai", "team-tool"), "alice", "团队用它处理浏览器任务。", "")
	if err != nil {
		t.Fatalf("ImportTeamTool: %v", err)
	}
	if !imported.Created || imported.Tool.Status != ToolIncluded || imported.Tool.IncludedAt == nil {
		t.Fatalf("imported team tool mismatch: %+v", imported)
	}

	included, err := s.ListTools(ctx, ListToolsParams{Status: ToolIncluded, Limit: 10})
	if err != nil {
		t.Fatalf("ListTools included: %v", err)
	}
	if len(included) != 1 || included[0].Evaluation == nil || included[0].Evaluation.CooperURL != nil {
		t.Fatalf("included projection mismatch: %+v", included)
	}

	if err := s.UpdateTeamTool(ctx, imported.Tool.ID, "bob", "团队改用它沉淀自动化操作流程。", "https://cooper.didichuxing.com/didocs/1"); err != nil {
		t.Fatalf("UpdateTeamTool: %v", err)
	}
	included, err = s.ListTools(ctx, ListToolsParams{Status: ToolIncluded, Q: nullableString("bob"), Limit: 10})
	if err != nil {
		t.Fatalf("ListTools included after update: %v", err)
	}
	if len(included) != 1 || included[0].FinalSummary == nil || *included[0].FinalSummary != "团队改用它沉淀自动化操作流程。" || included[0].Evaluation == nil || included[0].Evaluation.CooperURL == nil {
		t.Fatalf("updated team tool mismatch: %+v", included)
	}

	if err := s.DeleteTeamTool(ctx, imported.Tool.ID, "团队不再使用", "carol"); err != nil {
		t.Fatalf("DeleteTeamTool: %v", err)
	}
	included, err = s.ListTools(ctx, ListToolsParams{Status: ToolIncluded, Limit: 10})
	if err != nil {
		t.Fatalf("ListTools included after delete: %v", err)
	}
	if len(included) != 0 {
		t.Fatalf("deleted team tool should leave included list: %+v", included)
	}
	excluded, err := s.ListTools(ctx, ListToolsParams{Status: ToolExcluded, Limit: 10})
	if err != nil {
		t.Fatalf("ListTools excluded: %v", err)
	}
	if len(excluded) != 1 || excluded[0].ExcludedReason == nil || *excluded[0].ExcludedReason != "团队不再使用" {
		t.Fatalf("deleted team tool should be retained with reason: %+v", excluded)
	}
	var deleteEvents int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM tool_status_events
		WHERE tool_id=$1 AND from_status=$2 AND to_status=$3 AND actor=$4 AND note=$5`,
		imported.Tool.ID, ToolIncluded, ToolExcluded, "carol", "团队不再使用").Scan(&deleteEvents); err != nil {
		t.Fatalf("delete events: %v", err)
	}
	if deleteEvents != 1 {
		t.Fatalf("want one delete status event, got %d", deleteEvents)
	}
}
