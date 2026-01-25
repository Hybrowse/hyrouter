package routing

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

func applyFilters(req Request, backends []Backend, filters []Filter) []Backend {
	if len(filters) == 0 {
		out := make([]Backend, len(backends))
		copy(out, backends)
		return out
	}
	out := make([]Backend, 0, len(backends))
	for _, b := range backends {
		ok := true
		for _, f := range filters {
			if !filterMatches(req, b, f) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, b)
		}
	}
	return out
}

func filterMatches(req Request, b Backend, f Filter) bool {
	t := normalizeStrategy(f.Type)
	switch t {
	case "compare":
		leftRaw, lok, ln := sortValue(b, f.Left, "number")
		_ = leftRaw
		rightRaw, rok, rn := sortValue(b, f.Right, "number")
		_ = rightRaw
		if !lok || !rok {
			return false
		}
		op := normalizeStrategy(f.Op)
		switch op {
		case "lt":
			return ln < rn
		case "lte":
			return ln <= rn
		case "gt":
			return ln > rn
		case "gte":
			return ln >= rn
		case "eq":
			return ln == rn
		case "neq":
			return ln != rn
		default:
			return false
		}
	case "whitelist":
		enabledKey := strings.TrimSpace(f.EnabledKey)
		if enabledKey == "" {
			return false
		}
		v := strings.ToLower(strings.TrimSpace(metaGet(b.Meta, enabledKey)))
		enabled := v == "true" || v == "1" || v == "yes"
		if !enabled {
			return true
		}
		listKey := strings.TrimSpace(f.ListKey)
		if listKey == "" {
			return false
		}
		raw := strings.TrimSpace(metaGet(b.Meta, listKey))
		if raw == "" {
			return false
		}
		subject := normalizeStrategy(f.Subject)
		if subject == "" {
			subject = "uuid"
		}
		want := ""
		switch subject {
		case "uuid":
			want = req.UUID
		case "username":
			want = req.Username
		default:
			return false
		}
		want = strings.TrimSpace(want)
		if want == "" {
			return false
		}
		return listContains(raw, want)
	case "game_start_not_past":
		key := strings.TrimSpace(f.Key)
		if key == "" {
			return false
		}
		raw := strings.TrimSpace(metaGet(b.Meta, key))
		if raw == "" {
			return true
		}
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return false
		}
		if n == -1 {
			return true
		}
		now := time.Now().UnixMilli()
		vms := n
		if n > 0 && n < 10_000_000_000 {
			vms = n * 1000
		}
		return vms > now
	default:
		return false
	}
}

func listContains(raw string, want string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(raw, "[") {
		var xs []string
		if err := json.Unmarshal([]byte(raw), &xs); err != nil {
			return false
		}
		for _, x := range xs {
			if x == want {
				return true
			}
		}
		return false
	}
	parts := strings.Split(raw, ",")
	for _, p := range parts {
		if strings.TrimSpace(p) == want {
			return true
		}
	}
	return false
}
