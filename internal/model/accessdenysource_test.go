package model

import "testing"

func TestDenySourceRefs(t *testing.T) {
	src := []AccessListSource{{Name: "feed", URL: "https://feed.example.com/x.txt"}, {Name: "other", URL: "https://feed.example.com/y.txt"}}
	lists := []AccessList{
		// Two deny rules on one source count once; an allow rule never counts.
		{ObjectMeta: ObjectMeta{Name: "block"}, Sources: src, Rules: []IPRule{
			{Action: ActionDeny, Source: "feed"}, {Action: ActionDeny, Source: "feed"}, {Action: ActionAllow, Source: "other"},
		}},
		{ObjectMeta: ObjectMeta{Name: "off", Disabled: true}, Sources: src, Rules: []IPRule{{Action: ActionDeny, Source: "feed"}}},
		{ObjectMeta: ObjectMeta{Name: "lan"}, Rules: []IPRule{{Action: ActionDeny, CIDR: "192.0.2.0/24"}}},
		{ObjectMeta: ObjectMeta{Name: "two"}, Sources: src, Rules: []IPRule{{Action: ActionDeny, Source: "other"}}},
	}
	got := DenySourceRefs(lists)
	want := [][2]string{{"block", "feed"}, {"two", "other"}}
	if len(got) != len(want) {
		t.Fatalf("got %d refs %+v, want %v", len(got), got, want)
	}
	for i, w := range want {
		if got[i].List.Name != w[0] || got[i].Source != w[1] {
			t.Errorf("ref %d = %s/%s, want %s/%s", i, got[i].List.Name, got[i].Source, w[0], w[1])
		}
	}
}
