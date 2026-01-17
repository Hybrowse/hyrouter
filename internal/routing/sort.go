package routing

import (
	"sort"
	"strconv"
	"strings"
)

func applySort(backends []Backend, keys []SortKey) {
	if len(keys) == 0 {
		return
	}
	sort.SliceStable(backends, func(i, j int) bool {
		a := backends[i]
		b := backends[j]
		for _, k := range keys {
			order := normalizeStrategy(k.Order)
			if order == "" {
				order = "asc"
			}
			typeHint := normalizeStrategy(k.Type)
			av, aok, an := sortValue(a, k.Key, typeHint)
			bv, bok, bn := sortValue(b, k.Key, typeHint)
			if aok != bok {
				return aok && !bok
			}
			if typeHint == "number" {
				if an != bn {
					if order == "desc" {
						return an > bn
					}
					return an < bn
				}
				continue
			}
			if av != bv {
				if order == "desc" {
					return av > bv
				}
				return av < bv
			}
		}
		return false
	})
}

func sortValue(b Backend, key string, typeHint string) (string, bool, float64) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false, 0
	}

	if normalizeStrategy(typeHint) == "number" {
		if key == "port" {
			return "", true, float64(b.Port)
		}
		if key == "weight" {
			return "", true, float64(b.Weight)
		}
	}

	var raw string
	switch {
	case key == "host":
		raw = b.Host
	case key == "port":
		raw = strconv.Itoa(b.Port)
	case key == "weight":
		raw = strconv.Itoa(b.Weight)
	case strings.HasPrefix(key, "label:"):
		raw = metaGet(b.Meta, "label."+strings.TrimSpace(strings.TrimPrefix(key, "label:")))
	case strings.HasPrefix(key, "annotation:"):
		raw = metaGet(b.Meta, "annotation."+strings.TrimSpace(strings.TrimPrefix(key, "annotation:")))
	case strings.HasPrefix(key, "counter:"):
		raw = metaGet(b.Meta, "counter."+strings.TrimSpace(strings.TrimPrefix(key, "counter:")))
	default:
		raw = metaGet(b.Meta, key)
	}
	if raw == "" {
		return "", false, 0
	}
	if normalizeStrategy(typeHint) == "number" {
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return "", false, 0
		}
		return "", true, n
	}
	return raw, true, 0
}

func metaGet(m map[string]string, k string) string {
	if m == nil {
		return ""
	}
	return m[k]
}
