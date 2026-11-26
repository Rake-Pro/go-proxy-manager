package api

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// hashPlaceholder is what every API read returns in place of a stored
// basic-auth passwordHash: the auth middleware's `mode: basic` users, the
// inline auth block on a host, a location or a discovery template, and the
// deprecated AccessList.basicAuth. It is the same marker a literal Secret
// reads as. A write that sends it back keeps the stored hash of that user (see
// keepStoredHashes), so a GET-edit-PUT round trip never needs the hash itself.
//
// The redaction is applied to the value an API reply encodes (redactForReply)
// rather than in a MarshalJSON on model.BasicAuthUser: the data plane also
// JSON-encodes auth specs, to key a host's open tunnels to the chain they were
// admitted under (specKey), and that key must change when a password does.
const hashPlaceholder = "***"

var (
	basicAuthUserType = reflect.TypeOf(model.BasicAuthUser{})
	locationType      = reflect.TypeOf(model.Location{})
)

// scalarKind reports a kind that can hold no basic-auth user, so a walk does
// not visit every element of a []byte or []string.
func scalarKind(k reflect.Kind) bool {
	return k <= reflect.Complex128 || k == reflect.String
}

// maxRedactDepth bounds the reply walk. Config objects nest a few levels
// deep; a value deeper than this is a pointer cycle, which encoding/json
// would refuse too, and the reply fails instead of exhausting the stack.
const maxRedactDepth = 200

var holdsUsersCache sync.Map // reflect.Type -> bool

// typeHoldsUsers reports whether a value of type t can reach a
// BasicAuthUser, through the fields encoding/json encodes: exported ones and
// embedded ones (an embedded unexported struct's exported fields are
// promoted). An interface can hold anything.
func typeHoldsUsers(t reflect.Type) bool {
	if v, ok := holdsUsersCache.Load(t); ok {
		return v.(bool)
	}
	// Only the outermost answer is cached: an inner one found while a type
	// further up was still being worked out can be a false negative.
	r := holdsUsers(t, map[reflect.Type]bool{})
	holdsUsersCache.Store(t, r)
	return r
}

func holdsUsers(t reflect.Type, seen map[reflect.Type]bool) bool {
	if t == basicAuthUserType {
		return true
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	switch t.Kind() {
	case reflect.Interface:
		return true
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return holdsUsers(t.Elem(), seen)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if f := t.Field(i); (f.IsExported() || f.Anonymous) && holdsUsers(f.Type, seen) {
				return true
			}
		}
	}
	return false
}

// redactForReply returns v with every non-empty BasicAuthUser.PasswordHash
// replaced by the placeholder. Only the containers on the way to a user are
// copied; nothing reachable from v is modified, so a stored, cached or shared
// object is never altered by a reply. It fails rather than let a hash through
// when a user sits where it cannot be copied (inside an embedded unexported
// struct) or the value is a pointer cycle.
func redactForReply(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	out, changed, err := redactValue(reflect.ValueOf(v), 0)
	if err != nil || !changed {
		return v, err
	}
	return out.Interface(), nil
}

func redactValue(v reflect.Value, depth int) (reflect.Value, bool, error) {
	if !typeHoldsUsers(v.Type()) {
		return v, false, nil
	}
	if depth > maxRedactDepth {
		return v, false, fmt.Errorf("reply value of type %s nests deeper than %d levels (a pointer cycle?)", v.Type(), maxRedactDepth)
	}
	depth++
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return v, false, nil
		}
		e, ok, err := redactValue(v.Elem(), depth)
		if !ok || err != nil {
			return v, false, err
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(e)
		return out, true, nil
	case reflect.Pointer:
		if v.IsNil() {
			return v, false, nil
		}
		e, ok, err := redactValue(v.Elem(), depth)
		if !ok || err != nil {
			return v, false, err
		}
		out := reflect.New(v.Type().Elem())
		out.Elem().Set(e)
		return out, true, nil
	case reflect.Struct:
		if v.Type() == basicAuthUserType {
			u := v.Interface().(model.BasicAuthUser)
			if u.PasswordHash == "" {
				return v, false, nil
			}
			u.PasswordHash = hashPlaceholder
			return reflect.ValueOf(u), true, nil
		}
		var out reflect.Value
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				if f.Anonymous && typeHoldsUsers(f.Type) {
					return v, false, fmt.Errorf("reply type %s embeds the unexported %s, which can hold basic-auth users the redaction cannot copy", t, f.Type)
				}
				continue
			}
			nf, ok, err := redactValue(v.Field(i), depth)
			if err != nil {
				return v, false, err
			}
			if !ok {
				continue
			}
			if !out.IsValid() {
				out = reflect.New(t).Elem()
				out.Set(v)
			}
			out.Field(i).Set(nf)
		}
		if !out.IsValid() {
			return v, false, nil
		}
		return out, true, nil
	case reflect.Slice, reflect.Array:
		var out reflect.Value
		for i := 0; i < v.Len(); i++ {
			e, ok, err := redactValue(v.Index(i), depth)
			if err != nil {
				return v, false, err
			}
			if !ok {
				continue
			}
			if !out.IsValid() {
				if v.Kind() == reflect.Slice {
					out = reflect.MakeSlice(v.Type(), v.Len(), v.Len())
					reflect.Copy(out, v)
				} else {
					out = reflect.New(v.Type()).Elem()
					out.Set(v)
				}
			}
			out.Index(i).Set(e)
		}
		if !out.IsValid() {
			return v, false, nil
		}
		return out, true, nil
	case reflect.Map:
		var out reflect.Value
		it := v.MapRange()
		for it.Next() {
			e, ok, err := redactValue(it.Value(), depth)
			if err != nil {
				return v, false, err
			}
			if !ok {
				continue
			}
			if !out.IsValid() {
				out = reflect.MakeMapWithSize(v.Type(), v.Len())
				all := v.MapRange()
				for all.Next() {
					out.SetMapIndex(all.Key(), all.Value())
				}
			}
			out.SetMapIndex(it.Key(), e)
		}
		if !out.IsValid() {
			return v, false, nil
		}
		return out, true, nil
	}
	return v, false, nil
}

// locationKeys names each location of a []model.Location by its path and its
// occurrence among locations with the same path, so a list keeps its name when
// other locations are added, removed or reordered, and two locations that
// share a path (which validation allows) stay apart.
func locationKeys(v reflect.Value) (keys, paths []string) {
	seen := map[string]int{}
	out := make([]string, v.Len())
	paths = make([]string, v.Len())
	for i := range out {
		p := v.Index(i).Interface().(model.Location).Path
		paths[i] = p
		n := seen[p]
		seen[p] = n + 1
		if n == 0 {
			out[i] = fmt.Sprintf("[%q]", p)
		} else {
			out[i] = fmt.Sprintf("[%q#%d]", p, n+1)
		}
	}
	return out, paths
}

func fieldPath(path string, f reflect.StructField) (string, bool) {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	if name == "-" {
		return "", false
	}
	if name == "" {
		name = f.Name
	}
	if path != "" {
		name = path + "." + name
	}
	return name, true
}

// eachBasicAuthUser calls fn for every basic-auth user reachable from v (a
// pointer to a config object or to Settings), with the JSON path of the user
// list that holds it.
func eachBasicAuthUser(v reflect.Value, path string, fn func(list string, u *model.BasicAuthUser)) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			eachBasicAuthUser(v.Elem(), path, fn)
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			if !t.Field(i).IsExported() {
				continue
			}
			if p, ok := fieldPath(path, t.Field(i)); ok {
				eachBasicAuthUser(v.Field(i), p, fn)
			}
		}
	case reflect.Slice, reflect.Array:
		et := v.Type().Elem()
		switch {
		case et == basicAuthUserType:
			for i := 0; i < v.Len(); i++ {
				if e := v.Index(i); e.CanAddr() {
					fn(path, e.Addr().Interface().(*model.BasicAuthUser))
				}
			}
		case et == locationType:
			keys, _ := locationKeys(v)
			for i, k := range keys {
				eachBasicAuthUser(v.Index(i), path+k, fn)
			}
		case !scalarKind(et.Kind()):
			for i := 0; i < v.Len(); i++ {
				eachBasicAuthUser(v.Index(i), fmt.Sprintf("%s[%d]", path, i), fn)
			}
		}
	case reflect.Map:
		if scalarKind(v.Type().Elem().Kind()) {
			return
		}
		for _, k := range v.MapKeys() {
			eachBasicAuthUser(v.MapIndex(k), fmt.Sprintf("%s[%v]", path, k.Interface()), fn)
		}
	}
}

// hasHashPlaceholder reports whether obj (a pointer) carries a user whose
// passwordHash is the read placeholder.
func hasHashPlaceholder(obj any) bool {
	found := false
	eachBasicAuthUser(reflect.ValueOf(obj), "", func(_ string, u *model.BasicAuthUser) {
		if u.PasswordHash == hashPlaceholder {
			found = true
		}
	})
	return found
}

// counterpart says how a user list of a write relates to the stored object.
type counterpart int

const (
	// noCounterpart: the stored object has no list at this place.
	noCounterpart counterpart = iota
	// movedCounterpart: the list sits under the one renamed profile or
	// location of its level, paired with the one stored entry that no longer
	// exists there (see pairLeftovers).
	movedCounterpart
	// sameCounterpart: the stored object has a list at exactly this place.
	sameCounterpart
)

// userList is one basic-auth user list of a write and its stored counterpart.
type userList struct {
	path   string
	users  reflect.Value // the written []BasicAuthUser
	stored reflect.Value // the stored []BasicAuthUser; invalid when there is none
	match  counterpart
}

// pairUserLists walks the written value in and the stored value st side by
// side and calls fn for every user list in in. Locations and map entries are
// paired by pairLeftovers; anything unpaired has no counterpart.
func pairUserLists(in, st reflect.Value, path string, match counterpart, fn func(userList)) {
	if !st.IsValid() || st.Type() != in.Type() {
		st, match = reflect.Value{}, noCounterpart
	}
	switch in.Kind() {
	case reflect.Pointer, reflect.Interface:
		if in.IsNil() {
			return
		}
		if st.IsValid() {
			if st.IsNil() {
				st = reflect.Value{}
			} else {
				st = st.Elem()
			}
		}
		pairUserLists(in.Elem(), st, path, match, fn)
	case reflect.Struct:
		t := in.Type()
		for i := 0; i < in.NumField(); i++ {
			if !t.Field(i).IsExported() {
				continue
			}
			p, ok := fieldPath(path, t.Field(i))
			if !ok {
				continue
			}
			var sf reflect.Value
			if st.IsValid() {
				sf = st.Field(i)
			}
			pairUserLists(in.Field(i), sf, p, match, fn)
		}
	case reflect.Slice, reflect.Array:
		et := in.Type().Elem()
		switch {
		case et == basicAuthUserType:
			if in.Len() > 0 {
				fn(userList{path: path, users: in, stored: st, match: match})
			}
		case et == locationType:
			inKeys, inPaths := locationKeys(in)
			var stKeys, stPaths []string
			if st.IsValid() {
				stKeys, stPaths = locationKeys(st)
			}
			pairLeftovers(inKeys, inPaths, stKeys, stPaths, func(i, j int, m counterpart) {
				var se reflect.Value
				if j >= 0 {
					se = st.Index(j)
				}
				pairUserLists(in.Index(i), se, path+inKeys[i], min(match, m), fn)
			})
		case !scalarKind(et.Kind()):
			for i := 0; i < in.Len(); i++ {
				var se reflect.Value
				if st.IsValid() && i < st.Len() {
					se = st.Index(i)
				}
				pairUserLists(in.Index(i), se, fmt.Sprintf("%s[%d]", path, i), match, fn)
			}
		}
	case reflect.Map:
		if scalarKind(in.Type().Elem().Kind()) {
			return
		}
		inKeys, inVals := sortedMap(in)
		var stKeys []string
		var stVals []reflect.Value
		if st.IsValid() {
			stKeys, stVals = sortedMap(st)
		}
		pairLeftovers(inKeys, inKeys, stKeys, stKeys, func(i, j int, m counterpart) {
			var se reflect.Value
			if j >= 0 {
				se = stVals[j]
			}
			pairUserLists(inVals[i], se, fmt.Sprintf("%s[%s]", path, inKeys[i]), min(match, m), fn)
		})
	}
}

func sortedMap(v reflect.Value) ([]string, []reflect.Value) {
	keys := v.MapKeys()
	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = fmt.Sprint(k.Interface())
	}
	idx := make([]int, len(keys))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return names[idx[a]] < names[idx[b]] })
	outNames := make([]string, len(keys))
	outVals := make([]reflect.Value, len(keys))
	for i, j := range idx {
		outNames[i], outVals[i] = names[j], v.MapIndex(keys[j])
	}
	return outNames, outVals
}

// pairLeftovers pairs written entries with stored ones. Each entry has a key
// (unique on its side) and a group: a location's key is its path and
// occurrence and its group the path; a map entry's key and group are both the
// map key. An entry is paired by key (sameCounterpart) when its group has as
// many entries on both sides. Every entry of a group whose counts differ
// (absent from the other side, or present a different number of times) is
// left over; one written and one stored leftover are paired as a rename
// (movedCounterpart) only when each side has exactly one, since with more the
// order says nothing about which became which. Anything else gets j = -1.
func pairLeftovers(inKeys, inGroups, stKeys, stGroups []string, fn func(i, j int, m counterpart)) {
	inCount, stCount := map[string]int{}, map[string]int{}
	for _, g := range inGroups {
		inCount[g]++
	}
	for _, g := range stGroups {
		stCount[g]++
	}
	stIdx := map[string]int{}
	for j, k := range stKeys {
		stIdx[k] = j
	}
	var leftIn, leftSt []int
	for i, g := range inGroups {
		if inCount[g] != stCount[g] {
			leftIn = append(leftIn, i)
		}
	}
	for j, g := range stGroups {
		if inCount[g] != stCount[g] {
			leftSt = append(leftSt, j)
		}
	}
	for i, k := range inKeys {
		g := inGroups[i]
		j, ok := stIdx[k]
		switch {
		case ok && inCount[g] == stCount[g]:
			fn(i, j, sameCounterpart)
		case inCount[g] != stCount[g] && len(leftIn) == 1 && len(leftSt) == 1:
			fn(i, leftSt[0], movedCounterpart)
		default:
			fn(i, -1, noCounterpart)
		}
	}
}

// keepStoredHashes replaces each placeholder passwordHash in obj (a pointer)
// with a stored hash from stored (a pointer to the stored version of the same
// object, or nil when there is none). The rules, in order:
//
//   - The user's own list in the stored object (same place, or the stored list
//     a renamed profile or location is paired with) has a user of that name:
//     its hash is kept. A username listed several times (the deprecated
//     access-list basicAuth allows it) is matched by occurrence when both
//     lists hold it equally often.
//   - Otherwise, for a list that exists at the same place in the stored
//     object, the write is refused: a placeholder never brings another list's
//     hash into an existing list.
//   - For a list with no counterpart, or a renamed one, the one hash that
//     username has anywhere in the stored object is used. Several different
//     hashes, or none, refuse the write.
//
// It runs after plaintext passwords are hashed, so a user that sends a new
// password together with the placeholder gets the new password.
func keepStoredHashes(obj, stored any) error {
	byUser := map[string]map[string][]string{} // username -> hash -> lists
	var st reflect.Value
	if stored != nil && !reflect.ValueOf(stored).IsNil() {
		st = reflect.ValueOf(stored)
		eachBasicAuthUser(st, "", func(list string, u *model.BasicAuthUser) {
			if byUser[u.Username] == nil {
				byUser[u.Username] = map[string][]string{}
			}
			byUser[u.Username][u.PasswordHash] = append(byUser[u.Username][u.PasswordHash], list)
		})
	}
	var err error
	pairUserLists(reflect.ValueOf(obj), st, "", sameCounterpart, func(l userList) {
		count, seen := map[string]int{}, map[string]int{}
		for i := 0; i < l.users.Len(); i++ {
			count[l.users.Index(i).Interface().(model.BasicAuthUser).Username]++
		}
		for i := 0; i < l.users.Len() && err == nil; i++ {
			e := l.users.Index(i)
			name := e.Interface().(model.BasicAuthUser).Username
			occ := seen[name]
			seen[name]++
			if e.Interface().(model.BasicAuthUser).PasswordHash != hashPlaceholder {
				continue
			}
			if !e.CanAddr() {
				err = fmt.Errorf("%s: user %q: passwordHash is the redacted placeholder %q in a place it cannot be resolved; send the password", l.path, e.Interface().(model.BasicAuthUser).Username, hashPlaceholder)
				return
			}
			u := e.Addr().Interface().(*model.BasicAuthUser)
			err = resolvePlaceholder(l, u, occ, count[name], byUser)
		}
	})
	return err
}

// resolvePlaceholder resolves the placeholder of u, the occ-th of n users
// with its name in the written list l.
func resolvePlaceholder(l userList, u *model.BasicAuthUser, occ, n int, byUser map[string]map[string][]string) error {
	prefix := fmt.Sprintf("%s: user %q: passwordHash is the redacted placeholder %q", l.path, u.Username, hashPlaceholder)
	if l.stored.IsValid() {
		own := map[string]bool{}
		var inOrder []string
		for j := 0; j < l.stored.Len(); j++ {
			if s := l.stored.Index(j).Interface().(model.BasicAuthUser); s.Username == u.Username {
				own[s.PasswordHash] = true
				inOrder = append(inOrder, s.PasswordHash)
			}
		}
		switch {
		case len(own) == 1:
			u.PasswordHash = inOrder[0]
			return nil
		case len(own) > 1 && len(inOrder) == n:
			u.PasswordHash = inOrder[occ]
			return nil
		case len(own) > 1:
			return fmt.Errorf("%s, but this list stores that username more than once with different hashes, and the write lists it a different number of times; send the password for this user", prefix)
		}
	}
	if l.match == sameCounterpart {
		return fmt.Errorf("%s, but this user list has no stored user of that name, and a placeholder only keeps a hash already in the same list; a new or renamed user needs a password", prefix)
	}
	hashes := byUser[u.Username]
	switch len(hashes) {
	case 0:
		return fmt.Errorf("%s, but no stored user of that name has a hash to keep; a new or renamed user needs a password", prefix)
	case 1:
		for h := range hashes {
			u.PasswordHash = h
		}
		return nil
	}
	var lists []string
	for _, ls := range hashes {
		lists = append(lists, ls...)
	}
	sort.Strings(lists)
	return fmt.Errorf("%s, but that username is stored with different hashes in %s, so which one to keep is ambiguous; send the password for this user", prefix, strings.Join(lists, ", "))
}

// yamlHashKeyRe finds a passwordHash mapping key, quoted or not, followed by
// its colon.
var yamlHashKeyRe = regexp.MustCompile(`(?:^|[^\w"'])(["']?)passwordHash["']?[ \t]*:`)

// bcryptRe matches a bcrypt hash anywhere, whatever key (or anchor) it sits
// under, with each "$" written plainly or as a YAML escape.
var bcryptRe = regexp.MustCompile(`(?:\$|\\x24|\\u0024|\\U00000024)2[abxy]?(?:\$|\\x24|\\u0024|\\U00000024)\d\d(?:\$|\\x24|\\u0024|\\U00000024)[./A-Za-z0-9]{53}`)

// yamlEntryRe matches a line (indentation stripped) that starts a new mapping
// key or sequence item, or a document marker: the only lines that can end a
// block value.
var yamlEntryRe = regexp.MustCompile(`^(?:-(?:[ \t]|$)|(?:"(?:[^"\\]|\\.)*"|'(?:[^']|'')*'|[^\s#'"{}\[\],&*!|>%@` + "`" + `][^#]*?)[ \t]*:(?:[ \t]|$)|---|\.\.\.)`)

// yamlRedactor carries, from one line of YAML to the next, a passwordHash
// value that has not ended yet.
type yamlRedactor struct {
	// indent >= 0: a block-style value. It runs until a non-blank line,
	// indented at most this far, that starts a new key or sequence item; every
	// other line on the way (a continuation at any column, a block scalar
	// line) is part of it.
	indent int
	// pend != 0: a value still inside a scalar that has not ended: '"' or '\''
	// inside a quoted scalar, 'f' inside a plain flow scalar (it ends at ',',
	// '}' or ']').
	pend byte
	// hashOnly: the value region started at a hunk boundary, not at a key, so
	// only lines that could continue a hash are blanked (see hashTail).
	hashOnly bool
}

func newYAMLRedactor() *yamlRedactor { return &yamlRedactor{indent: -1} }

// hunkStart is the state at the first line of a diff hunk of a file that can
// hold basic-auth users: the hunk may start inside a hash value whose key line
// is outside it, so up to the first line that starts a new key or sequence
// item, a line that could continue a hash is blanked.
func hunkStart() *yamlRedactor { return &yamlRedactor{indent: 1 << 30, hashOnly: true} }

// maxEscapedHashLen is a bcrypt hash with each of its three "$" written as the
// longest YAML escape (\U00000024).
const maxEscapedHashLen = 60 + 3*9

// hashTail reports whether body (a line without its indentation) could be
// part of a hash value continued from an earlier line: up to its closing
// quote, and without a trailing line-continuation backslash, it holds only
// characters a bcrypt hash or its escaped "$" can contain, and is at most a
// hash long. What follows the closing quote (a flow close, a comma, a
// comment) does not count, unless it is the ":" of a quoted key.
func hashTail(body string) bool {
	body = strings.TrimLeft(strings.TrimRight(body, "\r"), `"'`)
	if i := strings.IndexAny(body, `"'`); i >= 0 {
		if strings.HasPrefix(strings.TrimLeft(body[i+1:], " \t"), ":") {
			return false
		}
		body = body[:i]
	}
	body = strings.TrimSuffix(strings.TrimRight(body, " \t"), `\`)
	if body == "" || len(body) > maxEscapedHashLen {
		return false
	}
	for i := 0; i < len(body); i++ {
		c := body[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '/' || c == '$' || c == '\\') {
			return false
		}
	}
	return true
}

// holdsUsersFile reports whether a config file (its path in the config repo)
// is of a kind that can hold basic-auth users.
func holdsUsersFile(path string) bool {
	for _, dir := range []string{"proxy-hosts/", "middlewares/", "access-lists/"} {
		if strings.HasPrefix(path, dir) {
			return true
		}
	}
	return path == "settings.yaml"
}

func leadingSpace(s string) string { return s[:len(s)-len(strings.TrimLeft(s, " \t"))] }

// line redacts one line of YAML: every passwordHash value it holds or
// continues, and any bcrypt-shaped string.
func (y *yamlRedactor) line(s string) string {
	return bcryptRe.ReplaceAllString(y.keys(y.continued(s)), hashPlaceholder)
}

// continued redacts the part of s that continues a value from an earlier line.
func (y *yamlRedactor) continued(s string) string {
	lead := leadingSpace(s)
	if y.pend != 0 {
		end := y.flowEnd(s, 0)
		if end < 0 {
			return lead + hashPlaceholder
		}
		y.pend = 0
		if y.indent >= 0 {
			return lead + hashPlaceholder
		}
		return lead + hashPlaceholder + s[end:]
	}
	if y.indent >= 0 {
		body := strings.TrimRight(s[len(lead):], "\r")
		if body == "" {
			return s
		}
		// At a hunk start the tail check comes first: the closing line of a
		// split hash can carry a flow close that reads like a new entry, and a
		// real entry line never passes it (":" and blanks are not hash
		// characters).
		if y.hashOnly && hashTail(body) {
			return lead + hashPlaceholder
		}
		if len(lead) <= y.indent && yamlEntryRe.MatchString(body) {
			y.indent, y.hashOnly = -1, false
			return s
		}
		if y.hashOnly {
			return s
		}
		return lead + hashPlaceholder
	}
	return s
}

// flowEnd returns the index just past the end of the open scalar in s,
// scanning from i, or -1 when it runs past the line (y.pend is then kept, or
// set from the opening quote).
func (y *yamlRedactor) flowEnd(s string, i int) int {
	if y.pend == 'f' {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i < len(s) && (s[i] == '"' || s[i] == '\'') {
			y.pend = s[i]
			i++
		} else {
			if j := strings.IndexAny(s[i:], ",}]"); j >= 0 {
				return i + j
			}
			return -1
		}
	}
	for ; i < len(s); i++ {
		switch {
		case y.pend == '"' && s[i] == '\\':
			i++
		case y.pend == '"' && s[i] == '"':
			return i + 1
		case y.pend == '\'' && s[i] == '\'':
			if i+1 < len(s) && s[i+1] == '\'' {
				i++
				continue
			}
			return i + 1
		}
	}
	return -1
}

// flowScan tracks, character by character, how many flow collections are
// open and whether a quoted scalar is, so a line is scanned once however many
// keys it holds.
type flowScan struct {
	depth int
	q     byte
	esc   bool
}

func (f *flowScan) advance(s string, from, to int) {
	for i := from; i < to; i++ {
		c := s[i]
		switch {
		case f.esc:
			f.esc = false
		case f.q == '"' && c == '\\':
			f.esc = true
		case f.q != 0:
			if c == f.q {
				f.q = 0
			}
		case c == '"' || c == '\'':
			f.q = c
		case c == '{' || c == '[':
			f.depth++
		case c == '}' || c == ']':
			f.depth--
		}
	}
}

// skipProps returns the index of the first character of the value starting at
// i, past blanks, anchors and tags.
func skipProps(s string, i int) int {
	for {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i < len(s) && (s[i] == '&' || s[i] == '!') {
			for i < len(s) && s[i] != ' ' && s[i] != '\t' {
				i++
			}
			continue
		}
		return i
	}
}

// keys redacts every passwordHash value that starts on s. It runs in one pass
// over the line: the flow state is carried forward, and a replaced value
// counts as the balanced placeholder that replaces it.
func (y *yamlRedactor) keys(s string) string {
	ms := yamlHashKeyRe.FindAllStringSubmatchIndex(s, -1)
	if ms == nil {
		return s
	}
	var b strings.Builder
	var fs flowScan
	pos, scanned := 0, 0 // s[:pos] is emitted or replaced; fs has seen s[:scanned]
	for _, m := range ms {
		keyAt, valAt := m[2], m[1]
		if keyAt < pos {
			continue // inside a value already replaced
		}
		fs.advance(s, scanned, keyAt)
		depth := fs.depth
		fs.advance(s, keyAt, valAt)
		i := skipProps(s, valAt)
		b.WriteString(s[pos:valAt])
		b.WriteString(` "` + hashPlaceholder + `"`)
		if depth <= 0 {
			// Block style: the value is the rest of the line and the lines
			// after it up to the next entry; a quoted scalar left open keeps
			// every line redacted until it closes.
			y.indent = keyAt
			if i < len(s) && (s[i] == '"' || s[i] == '\'') {
				y.pend = s[i]
				if y.flowEnd(s, i+1) >= 0 {
					y.pend = 0
				}
			}
			return b.String()
		}
		y.pend = 'f'
		end := y.flowEnd(s, i)
		if end < 0 {
			return b.String()
		}
		y.pend = 0
		pos, scanned = end, end
	}
	b.WriteString(s[pos:])
	return b.String()
}

// redactHashesYAML redacts every passwordHash value in a YAML document (the
// dry-run preview of the file a write would commit).
func redactHashesYAML(s string) string {
	y := newYAMLRedactor()
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = y.line(l)
	}
	return strings.Join(lines, "\n")
}

// redactHashesPatch does the same for a unified diff of YAML files (GET
// /history/diff). The old and the new side of a hunk are tracked apart, so a
// value continued on the next line of its own side is redacted whatever lines
// of the other side come between. In a file that can hold users, a hunk may
// start inside a hash value (see hunkStart).
func redactHashesPatch(s string) string {
	oldSide, newSide := newYAMLRedactor(), newYAMLRedactor()
	inHunk, usersFile := false, false
	// cols is the number of marker columns of the current hunk: one, or one
	// per parent in the combined diff of a merge commit.
	cols := 1
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "diff --"):
			inHunk = false
			// A header that cannot be read (a quoted path) counts as a file
			// that can hold users.
			usersFile = true
			if rest, ok := strings.CutPrefix(l, "diff --git "); ok {
				if _, b, ok := strings.Cut(rest, " b/"); ok {
					usersFile = holdsUsersFile(b)
				}
			} else if path, ok := strings.CutPrefix(l, "diff --cc "); ok && !strings.HasPrefix(path, `"`) {
				usersFile = holdsUsersFile(path)
			} else if path, ok := strings.CutPrefix(l, "diff --combined "); ok && !strings.HasPrefix(path, `"`) {
				usersFile = holdsUsersFile(path)
			}
		case strings.HasPrefix(l, "@@"):
			inHunk = true
			cols = len(l) - len(strings.TrimLeft(l, "@")) - 1
			if usersFile {
				oldSide, newSide = hunkStart(), hunkStart()
			} else {
				oldSide, newSide = newYAMLRedactor(), newYAMLRedactor()
			}
			// The header may repeat a line of the file as context.
			lines[i] = newYAMLRedactor().line(l)
			continue
		}
		if !inHunk || l == "" {
			lines[i] = bcryptRe.ReplaceAllString(l, hashPlaceholder)
			continue
		}
		if cols > 1 {
			// A combined diff interleaves more than two sides, so a split value
			// cannot be followed per side: one state for all, and in a file
			// that can hold users every line that could be a hash tail.
			if len(l) < cols {
				continue
			}
			body := l[cols:]
			red := newSide.line(body)
			if usersFile && hashTail(strings.TrimLeft(body, " \t")) {
				red = leadingSpace(body) + hashPlaceholder
			}
			lines[i] = l[:cols] + red
			continue
		}
		body := l[1:]
		switch l[0] {
		case '-':
			lines[i] = "-" + oldSide.line(body)
		case '+':
			lines[i] = "+" + newSide.line(body)
		case ' ':
			a, b := oldSide.line(body), newSide.line(body)
			switch {
			case a == b:
				lines[i] = " " + a
			case a == body:
				lines[i] = " " + b
			case b == body:
				lines[i] = " " + a
			default:
				lines[i] = " " + leadingSpace(body) + hashPlaceholder
			}
		default:
			lines[i] = bcryptRe.ReplaceAllString(l, hashPlaceholder)
		}
	}
	return strings.Join(lines, "\n")
}
