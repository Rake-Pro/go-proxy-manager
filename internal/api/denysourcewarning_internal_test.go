package api

import (
	"context"
	"errors"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// GET /health runs on every Overview poll, so the ledger is read only when an
// enabled list has a deny rule that names a source.
func TestDenySourceWarningsLoadsLedgerOnlyWhenNeeded(t *testing.T) {
	src := []model.AccessListSource{{Name: "feed", URL: "https://feed.example.com/x.txt"}}
	tests := []struct {
		name     string
		lists    []model.AccessList
		wantLoad bool
	}{
		{"no lists", nil, false},
		{"allow source only", []model.AccessList{{ObjectMeta: model.ObjectMeta{Name: "a"}, Sources: src, Rules: []model.IPRule{{Action: model.ActionAllow, Source: "feed"}}}}, false},
		{"deny source on a disabled list", []model.AccessList{{ObjectMeta: model.ObjectMeta{Name: "a", Disabled: true}, Sources: src, Rules: []model.IPRule{{Action: model.ActionDeny, Source: "feed"}}}}, false},
		{"deny source", []model.AccessList{{ObjectMeta: model.ObjectMeta{Name: "a"}, Sources: src, Rules: []model.IPRule{{Action: model.ActionDeny, Source: "feed"}}}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			loaded := false
			load := func(context.Context) (model.AccessListSourceLedger, string, error) {
				loaded = true
				return model.AccessListSourceLedger{}, "", errors.New("unreadable")
			}
			denySourceWarnings(context.Background(), tc.lists, load)
			if loaded != tc.wantLoad {
				t.Fatalf("ledger loaded = %v, want %v", loaded, tc.wantLoad)
			}
		})
	}
}
