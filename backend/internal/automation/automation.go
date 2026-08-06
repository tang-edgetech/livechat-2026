// Package automation evaluates the flat rule-list condition shape from
// overview.md §4/§9: `{ logic: and|or, rules: [{field, operator, value}] }`.
// Deliberately simpler than bot_flow's node-graph trigger — Automation
// only ever fires one message, no branching (§6.3).
package automation

import (
	"fmt"
	"strings"
)

type Rule struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

type ConditionSet struct {
	Logic string `json:"logic"`
	Rules []Rule `json:"rules"`
}

// Evaluate checks every rule against ctx (e.g. {"page_url": "...",
// "time_of_day": "14:05"}), combined with the set's and/or logic. An
// empty rule list matches unconditionally (a plain greeting with no
// targeting).
func Evaluate(cs ConditionSet, ctx map[string]string) bool {
	if len(cs.Rules) == 0 {
		return true
	}

	results := make([]bool, len(cs.Rules))
	for i, r := range cs.Rules {
		results[i] = evalRule(r, ctx[r.Field])
	}

	if strings.EqualFold(cs.Logic, "or") {
		for _, r := range results {
			if r {
				return true
			}
		}
		return false
	}

	for _, r := range results {
		if !r {
			return false
		}
	}
	return true
}

func evalRule(r Rule, actual string) bool {
	switch r.Operator {
	case "contains":
		return strings.Contains(strings.ToLower(actual), strings.ToLower(fmt.Sprint(r.Value)))
	case "equals":
		return actual == fmt.Sprint(r.Value)
	case "not_equals":
		return actual != fmt.Sprint(r.Value)
	case "between":
		bounds, ok := r.Value.([]any)
		if !ok || len(bounds) != 2 {
			return false
		}
		lo, hi := fmt.Sprint(bounds[0]), fmt.Sprint(bounds[1])
		return actual >= lo && actual <= hi
	default:
		return false
	}
}
