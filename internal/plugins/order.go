package plugins

import (
	"fmt"
	"strings"

	"github.com/hybrowse/hyrouter/internal/config"
)

func OrderPluginConfigs(cfgs []config.PluginConfig) ([]config.PluginConfig, error) {
	// Plugins are executed in a deterministic order:
	// 1) First by stage (deny -> route -> mutate).
	// 2) Then within a stage by a topological sort over before/after constraints.
	idxsByStage := map[string][]int{}
	for i, p := range cfgs {
		stage := strings.ToLower(p.Stage)
		if stage == "" {
			stage = "route"
		}
		idxsByStage[stage] = append(idxsByStage[stage], i)
	}

	orderedIdxs := make([]int, 0, len(cfgs))
	stages := []string{"deny", "route", "mutate"}
	for _, stage := range stages {
		ids := idxsByStage[stage]
		if len(ids) == 0 {
			continue
		}
		o, err := topoSortByConstraints(cfgs, ids)
		if err != nil {
			return nil, fmt.Errorf("stage %q: %w", stage, err)
		}
		orderedIdxs = append(orderedIdxs, o...)
		delete(idxsByStage, stage)
	}

	for stage, ids := range idxsByStage {
		o, err := topoSortByConstraints(cfgs, ids)
		if err != nil {
			return nil, fmt.Errorf("stage %q: %w", stage, err)
		}
		orderedIdxs = append(orderedIdxs, o...)
	}

	out := make([]config.PluginConfig, 0, len(cfgs))
	for _, i := range orderedIdxs {
		out = append(out, cfgs[i])
	}
	return out, nil
}

func topoSortByConstraints(cfgs []config.PluginConfig, ids []int) ([]int, error) {
	// Constraints:
	// - after: [X]  => X must run before this plugin
	// - before: [X] => X must run after this plugin
	nameToIdx := map[string]int{}
	for _, idx := range ids {
		nameToIdx[cfgs[idx].Name] = idx
	}

	indeg := map[int]int{}
	edges := map[int][]int{}

	for _, idx := range ids {
		p := cfgs[idx]
		for _, dep := range p.After {
			if j, ok := nameToIdx[dep]; ok {
				edges[j] = append(edges[j], idx)
				indeg[idx]++
			}
		}
		for _, dep := range p.Before {
			if j, ok := nameToIdx[dep]; ok {
				edges[idx] = append(edges[idx], j)
				indeg[j]++
			}
		}
	}

	processed := map[int]bool{}
	out := make([]int, 0, len(ids))

	for len(out) < len(ids) {
		found := -1
		for _, idx := range ids {
			if processed[idx] {
				continue
			}
			if indeg[idx] == 0 {
				found = idx
				break
			}
		}
		if found == -1 {
			return nil, fmt.Errorf("cycle in before/after constraints")
		}

		processed[found] = true
		out = append(out, found)
		for _, to := range edges[found] {
			indeg[to]--
		}
	}

	return out, nil
}
