package minimax

import "testing"

func TestParseVisionVerdict(t *testing.T) {
	v, err := ParseVisionVerdict("```json\n{\"pass\":true,\"app_name\":\"轻羽云笔记\",\"rating\":5,\"review_text\":\"界面很好用强烈推荐给朋友们用用吧\",\"negative\":false,\"reason\":\"\"}\n```")
	if err != nil {
		t.Fatal(err)
	}
	ok, reason := ApplyHardRules(v)
	if !ok {
		t.Fatalf("want pass, got %s", reason)
	}
}

func TestApplyHardRulesRejects(t *testing.T) {
	cases := []VisionVerdict{
		{Pass: true, AppName: "别的笔记", Rating: 5, ReviewText: "界面很好用强烈推荐给朋友们用用吧"},
		{Pass: true, AppName: "轻羽云笔记", Rating: 4, ReviewText: "界面很好用强烈推荐给朋友们用用吧"},
		{Pass: true, AppName: "轻羽云笔记", Rating: 5, ReviewText: "太短了"},
		{Pass: true, AppName: "轻羽云笔记", Rating: 5, ReviewText: "界面很好用强烈推荐给朋友们用用吧", Negative: true},
	}
	for i, c := range cases {
		ok, _ := ApplyHardRules(c)
		if ok {
			t.Fatalf("case %d should fail", i)
		}
	}
}
