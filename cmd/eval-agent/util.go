package main

import "strings"

func filterCases(cases []AgentCase, split string) []AgentCase {
	split = strings.ToLower(strings.TrimSpace(split))
	if split == "" || split == "all" {
		return cases
	}
	out := make([]AgentCase, 0, len(cases))
	for _, c := range cases {
		if strings.EqualFold(c.Split, split) {
			out = append(out, c)
		}
	}
	return out
}
