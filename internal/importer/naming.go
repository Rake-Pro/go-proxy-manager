package importer

import (
	"encoding/json"
	"fmt"
	"strings"
)

// parseDomains decodes NPM's domain_names, which is normally a JSON array of
// strings. It tolerates a bare/empty value and returns ok=false when nothing
// usable is found so callers can warn and skip.
func parseDomains(raw string) (domains []string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		for _, d := range arr {
			d = strings.TrimSpace(d)
			if d != "" {
				domains = append(domains, d)
			}
		}
		if len(domains) > 0 {
			return domains, true
		}
		return nil, false
	}
	// Fallback: treat as a single domain or comma-separated list.
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			domains = append(domains, p)
		}
	}
	return domains, len(domains) > 0
}

// slugify turns an arbitrary string into a model.ValidateName-safe candidate:
// lowercase; every disallowed rune becomes '-'; runs of '-' collapse; leading
// and trailing -_. are trimmed. Returns "" if nothing valid remains.
func slugify(in string) string {
	in = strings.ToLower(strings.TrimSpace(in))
	var b strings.Builder
	prevDash := false
	for _, r := range in {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		case r == '_' || r == '.':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-_.")
	if len(out) > 253 {
		out = strings.Trim(out[:253], "-_.")
	}
	return out
}

// uniqueName derives a globally-unique (per kind) object name from a preferred
// slug, falling back to "<fallbackKind>-<id>", and resolves collisions by
// suffixing "-<id>" then "-2", "-3", ...
func (s *importState) uniqueName(kind, preferred, fallbackKind string, id int64) string {
	set := s.usedNames[kind]
	if set == nil {
		set = map[string]bool{}
		s.usedNames[kind] = set
	}

	base := slugify(preferred)
	if base == "" {
		base = fmt.Sprintf("%s-%d", fallbackKind, id)
	}
	if !set[base] {
		set[base] = true
		return base
	}
	// First collision attempt: suffix the source id.
	withID := slugify(fmt.Sprintf("%s-%d", base, id))
	if withID != "" && !set[withID] {
		set[withID] = true
		return withID
	}
	for n := 2; ; n++ {
		cand := slugify(fmt.Sprintf("%s-%d", base, n))
		if cand != "" && !set[cand] {
			set[cand] = true
			return cand
		}
	}
}
