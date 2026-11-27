package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// settings.pathBlock names access lists in allowFrom. A settings write naming
// a missing list is refused, and so is deleting a named list: either would
// leave an exemption that exempts nobody.
func TestPathBlockAllowFromRefsStayResolvable(t *testing.T) {
	ctx := context.Background()
	author := Author{Name: "admin", Email: "admin@example.com"}
	list := func(name string) model.AccessList {
		return model.AccessList{ObjectMeta: model.ObjectMeta{Name: name}, DefaultAction: model.ActionDeny,
			Rules: []model.IPRule{{CIDR: "192.0.2.0/24", Action: model.ActionAllow}}}
	}
	st := newTestStore(t)
	for _, name := range []string{"office", "spare"} {
		if _, err := st.Save(ctx, list(name), author); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	_, s, err := st.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}

	s.PathBlock = &model.PathBlockMiddleware{Extensions: []string{".php"}, AllowFrom: []string{"@office", "@missing"}}
	if _, err := st.SaveSettings(ctx, s, author); err == nil || !strings.Contains(err.Error(), `pathBlock.allowFrom: references unknown accessList "missing"`) {
		t.Fatalf("settings naming a missing list: err = %v", err)
	}
	s.PathBlock.AllowFrom = []string{"192.0.2.0/24", "@office"}
	if _, err := st.SaveSettings(ctx, s, author); err != nil {
		t.Fatalf("settings naming a stored list: %v", err)
	}

	if _, err := st.Delete(ctx, "AccessList", "office", author); !errors.Is(err, ErrDanglingRef) || !strings.Contains(err.Error(), "pathBlock.allowFrom") {
		t.Fatalf("deleting the exempted list: err = %v, want ErrDanglingRef naming pathBlock.allowFrom", err)
	}
	if _, err := st.Delete(ctx, "AccessList", "spare", author); err != nil {
		t.Fatalf("deleting an unrelated list: %v", err)
	}

	// Once the settings stop naming it the list can go.
	s.PathBlock.AllowFrom = nil
	if _, err := st.SaveSettings(ctx, s, author); err != nil {
		t.Fatalf("dropping the exemption: %v", err)
	}
	if _, err := st.Delete(ctx, "AccessList", "office", author); err != nil {
		t.Fatalf("deleting the list after the exemption is gone: %v", err)
	}

	// The list round-trips through the store.
	_, got, err := st.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.PathBlock == nil || strings.Join(got.PathBlock.Extensions, ",") != ".php" {
		t.Fatalf("stored pathBlock = %+v", got.PathBlock)
	}
}

// A path-block middleware's own exemption is held by Config.Validate, like any
// other reference between objects.
func TestPathBlockMiddlewareRefBlocksListDelete(t *testing.T) {
	ctx := context.Background()
	author := Author{Name: "admin", Email: "admin@example.com"}
	st := newTestStore(t)
	office := model.AccessList{ObjectMeta: model.ObjectMeta{Name: "office"}, DefaultAction: model.ActionDeny,
		Rules: []model.IPRule{{CIDR: "192.0.2.0/24", Action: model.ActionAllow}}}
	mw := model.Middleware{ObjectMeta: model.ObjectMeta{Name: "probes"}, Type: model.MWTypePathBlock,
		PathBlock: &model.PathBlockMiddleware{Names: []string{".git"}, AllowFrom: []string{"@office"}}}
	if _, err := st.Save(ctx, mw, author); err == nil || !strings.Contains(err.Error(), `unknown accessList "office"`) {
		t.Fatalf("middleware naming a missing list: err = %v", err)
	}
	for _, o := range []model.Object{office, mw} {
		if _, err := st.Save(ctx, o, author); err != nil {
			t.Fatalf("save %s: %v", o.GetMeta().Name, err)
		}
	}
	if _, err := st.Delete(ctx, "AccessList", "office", author); !errors.Is(err, ErrDanglingRef) {
		t.Fatalf("deleting the exempted list: err = %v, want ErrDanglingRef", err)
	}
}
