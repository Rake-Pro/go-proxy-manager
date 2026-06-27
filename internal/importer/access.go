package importer

import (
	"fmt"
	"strings"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"golang.org/x/crypto/bcrypt"
)

// passAuthDefault is NPM's default for access_list.pass_auth (pass credentials
// upstream). We warn only when a list diverges from it.
const passAuthDefault = 1

func (s *importState) importAccessLists() error {
	want := []string{"id", "name", "satisfy_any", "pass_auth", "is_deleted"}
	cols, _, ok, err := s.selectAvailable("access_list", want)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	rows, err := s.queryRows("access_list", cols)
	if err != nil {
		return err
	}

	authByList, err := s.collectAuth()
	if err != nil {
		return err
	}
	clientsByList, err := s.collectClients()
	if err != nil {
		return err
	}

	for _, r := range rows {
		id := asInt(r["id"])
		nm := asString(r["name"])
		label := fmt.Sprintf("access_list #%d (%s)", id, firstNonEmpty(nm, "unnamed"))

		name := s.uniqueName("AccessList", nm, "accesslist", id)

		al := model.AccessList{
			ObjectMeta: model.ObjectMeta{Name: name, DisplayName: nm},
			SatisfyAny: asBool(r["satisfy_any"]),
			BasicAuth:  authByList[id],
			Rules:      clientsByList[id],
		}

		if _, has := r["pass_auth"]; has && asInt(r["pass_auth"]) != passAuthDefault {
			s.warn(label, "pass_auth",
				"pass_auth differs from default; verify credential pass-through after import")
		}

		if !s.add(label, "", al) {
			continue
		}
		s.alNames[id] = name
	}
	return nil
}

func (s *importState) collectAuth() (map[int64][]model.BasicAuthUser, error) {
	out := map[int64][]model.BasicAuthUser{}
	want := []string{"id", "access_list_id", "username", "password", "is_deleted"}
	cols, _, ok, err := s.selectAvailable("access_list_auth", want)
	if err != nil {
		return nil, err
	}
	if !ok {
		return out, nil
	}
	rows, err := s.queryRows("access_list_auth", cols)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		listID := asInt(r["access_list_id"])
		username := strings.TrimSpace(asString(r["username"]))
		password := asString(r["password"])
		label := fmt.Sprintf("access_list_auth #%d (list %d)", asInt(r["id"]), listID)

		if username == "" || password == "" {
			s.warn(label, "username/password", "missing username or password; auth user skipped")
			continue
		}

		hash := password
		if !looksBcrypt(password) {
			h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				s.warn(label, "password", fmt.Sprintf("could not hash password: %v; auth user skipped", err))
				continue
			}
			hash = string(h)
		}
		out[listID] = append(out[listID], model.BasicAuthUser{Username: username, PasswordHash: hash})
	}
	return out, nil
}

func (s *importState) collectClients() (map[int64][]model.IPRule, error) {
	out := map[int64][]model.IPRule{}
	want := []string{"id", "access_list_id", "address", "directive", "is_deleted"}
	cols, _, ok, err := s.selectAvailable("access_list_client", want)
	if err != nil {
		return nil, err
	}
	if !ok {
		return out, nil
	}
	rows, err := s.queryRows("access_list_client", cols)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		listID := asInt(r["access_list_id"])
		addr := strings.TrimSpace(asString(r["address"]))
		dir := strings.ToLower(strings.TrimSpace(asString(r["directive"])))
		label := fmt.Sprintf("access_list_client #%d (list %d)", asInt(r["id"]), listID)

		if addr == "" {
			s.warn(label, "address", "empty address; rule skipped")
			continue
		}
		if dir != model.ActionAllow && dir != model.ActionDeny {
			s.warn(label, "directive", fmt.Sprintf("unrecognized directive %q; rule skipped", dir))
			continue
		}
		out[listID] = append(out[listID], model.IPRule{Action: dir, CIDR: addr})
	}
	return out, nil
}

func looksBcrypt(p string) bool {
	return strings.HasPrefix(p, "$2a$") || strings.HasPrefix(p, "$2b$") || strings.HasPrefix(p, "$2y$")
}
