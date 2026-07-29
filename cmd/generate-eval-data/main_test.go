package main

import (
	"math/rand"
	"strings"
	"testing"
)

func TestGeneratedDatasetContract(t *testing.T) {
	rng := rand.New(rand.NewSource(generatorSeed))
	shops := generateShops(rng)
	reviews := generateReviews(rng, shops)
	retrieval := generateRetrieval(shops)
	agents := generateAgents(shops)

	if got := 25 + len(shops); got != 200 {
		t.Fatalf("total shops=%d want=200", got)
	}
	if got := 45 + len(reviews); got != 1000 {
		t.Fatalf("total reviews=%d want=1000", got)
	}
	if got := len(retrieval.Cases); got < 60 || got > 80 {
		t.Fatalf("retrieval cases=%d want 60..80", got)
	}
	if got := len(agents.Cases); got < 24 || got > 30 {
		t.Fatalf("agent cases=%d want 24..30", got)
	}

	seenRetrieval := map[string]bool{}
	noResult, injection := 0, 0
	for _, c := range retrieval.Cases {
		if seenRetrieval[c.ID] {
			t.Fatalf("duplicate retrieval id %s", c.ID)
		}
		seenRetrieval[c.ID] = true
		if c.ExpectNoResults {
			noResult++
			if len(c.RelevantShopIDs) != 0 {
				t.Fatalf("%s no-result case has relevant ids", c.ID)
			}
		} else if len(c.RelevantShopIDs) == 0 {
			t.Fatalf("%s has no relevant ids", c.ID)
		}
		for _, tag := range c.Tags {
			if tag == "prompt_injection" {
				injection++
			}
		}
	}
	if noResult == 0 || injection == 0 {
		t.Fatalf("coverage no_result=%d prompt_injection=%d", noResult, injection)
	}

	critical := 0
	for _, c := range agents.Cases {
		for _, tag := range c.Tags {
			if tag == "critical" {
				critical++
				if c.Trials < 3 {
					t.Fatalf("%s critical trials=%d want>=3", c.ID, c.Trials)
				}
			}
		}
	}
	if critical < 6 {
		t.Fatalf("critical cases=%d want>=6", critical)
	}
}

func TestGenerationIsDeterministic(t *testing.T) {
	build := func() (string, string, string) {
		rng := rand.New(rand.NewSource(generatorSeed))
		shops := generateShops(rng)
		reviews := generateReviews(rng, shops)
		return hash([]byte(renderSQL(shops, reviews))),
			hash(mustJSON(generateRetrieval(shops))),
			hash(mustJSON(generateAgents(shops)))
	}
	sql1, retrieval1, agent1 := build()
	sql2, retrieval2, agent2 := build()
	if sql1 != sql2 || retrieval1 != retrieval2 || agent1 != agent2 {
		t.Fatalf("generation changed: %s/%s %s/%s %s/%s",
			sql1, sql2, retrieval1, retrieval2, agent1, agent2)
	}
	if !strings.HasPrefix(sql1, "sha256:") {
		t.Fatalf("unexpected hash %q", sql1)
	}
}
