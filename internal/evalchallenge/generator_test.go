package evalchallenge

import "testing"

func TestChallengeContractAndIsolation(t *testing.T) {
	d := Build("sha256:source")
	if len(d.Retrieval.Cases) != 144 {
		t.Fatalf("retrieval cases=%d want=144", len(d.Retrieval.Cases))
	}
	if len(d.Agent.Cases) != 36 {
		t.Fatalf("agent cases=%d want=36", len(d.Agent.Cases))
	}
	if d.Retrieval.Splits["dev"].Cases != 24 || d.Retrieval.Splits["challenge"].Cases != 120 {
		t.Fatalf("retrieval splits=%+v", d.Retrieval.Splits)
	}
	if d.Agent.Splits["dev"].Cases != 8 || d.Agent.Splits["challenge"].Cases != 28 {
		t.Fatalf("agent splits=%+v", d.Agent.Splits)
	}

	retrievalIDs := map[string]struct{}{}
	retrievalQuestions := map[string]string{}
	for _, c := range d.Retrieval.Cases {
		if _, exists := retrievalIDs[c.ID]; exists {
			t.Fatalf("duplicate retrieval id %s", c.ID)
		}
		retrievalIDs[c.ID] = struct{}{}
		if previousSplit, exists := retrievalQuestions[c.Question]; exists {
			t.Fatalf("duplicate retrieval question crosses/duplicates split %s/%s: %q", previousSplit, c.Split, c.Question)
		}
		retrievalQuestions[c.Question] = c.Split
		if c.ExpectNoResults && len(c.RelevantShopIDs) != 0 {
			t.Fatalf("%s no-result has relevant shops %v", c.ID, c.RelevantShopIDs)
		}
	}

	agentIDs := map[string]struct{}{}
	agentPrompts := map[string]string{}
	criticalCases, criticalTrials, challengeTrials := 0, 0, 0
	for _, c := range d.Agent.Cases {
		if _, exists := agentIDs[c.ID]; exists {
			t.Fatalf("duplicate agent id %s", c.ID)
		}
		agentIDs[c.ID] = struct{}{}
		prompt := ""
		for _, turn := range c.Turns {
			prompt += "\n" + turn.User
		}
		if previousSplit, exists := agentPrompts[prompt]; exists {
			t.Fatalf("duplicate agent prompt crosses/duplicates split %s/%s", previousSplit, c.Split)
		}
		agentPrompts[prompt] = c.Split
		if c.Split == "challenge" {
			challengeTrials += c.Trials
		}
		for _, tag := range c.Tags {
			if tag == "critical" {
				criticalCases++
				criticalTrials += c.Trials
				if c.Split != "challenge" || c.Trials != 3 {
					t.Fatalf("critical case must be 3-trial challenge: %+v", c)
				}
			}
		}
	}
	if criticalCases != 10 || criticalTrials != 30 || challengeTrials != 48 {
		t.Fatalf("critical cases/trials=%d/%d challenge trials=%d", criticalCases, criticalTrials, challengeTrials)
	}
	if d.Manifest.FormalEvaluationExecutedAtGeneration {
		t.Fatal("generated manifest must never claim formal execution")
	}
}

func TestChallengeBuildIsDeterministic(t *testing.T) {
	a := Build("sha256:source")
	b := Build("sha256:source")
	if hash(mustJSON(a)) != hash(mustJSON(b)) {
		t.Fatal("challenge build is not deterministic")
	}
}

func TestCorrectedAgentSuitesHaveValidPositiveGoldens(t *testing.T) {
	v31 := BuildAgentRegressionV31("sha256:source")
	v4 := BuildAgentChallengeV4("sha256:source")
	for name, suite := range map[string]AgentSuite{"v31": v31, "v4": v4} {
		t.Run(name, func(t *testing.T) {
			if len(suite.Agent.Cases) != 36 || suite.Agent.Splits["challenge"].Cases != 28 {
				t.Fatalf("invalid suite shape: cases=%d splits=%+v", len(suite.Agent.Cases), suite.Agent.Splits)
			}
			challengeTrials := 0
			hasExhaustiveBroadGolden := false
			for _, c := range suite.Agent.Cases {
				if c.Split == "challenge" {
					challengeTrials += c.Trials
				}
				if !c.Expected.ExpectNoResults && len(c.Expected.AllowedShopIDs) == 0 && len(c.Expected.RequiredCitedShopIDs) == 0 {
					t.Fatalf("%s positive case has empty allowed set", c.ID)
				}
				if !c.Expected.RequireRecommendationHeader {
					t.Fatalf("%s must use recommendation/citation separation", c.ID)
				}
				if c.Expected.AllowedOnly && len(c.Expected.AllowedShopIDs) > 10 {
					hasExhaustiveBroadGolden = true
				}
				for _, tag := range c.Tags {
					if tag == "typo" {
						t.Fatalf("%s typo robustness is outside the measured v3.1+ scope", c.ID)
					}
				}
			}
			if challengeTrials != 48 || suite.Manifest.AgentChallengeTrials != 48 {
				t.Fatalf("challenge trials=%d manifest=%d", challengeTrials, suite.Manifest.AgentChallengeTrials)
			}
			if !hasExhaustiveBroadGolden {
				t.Fatal("corrected AllowedOnly goldens must not be silently capped at top 10")
			}
		})
	}
	if hash(mustJSON(v31.Agent)) == hash(mustJSON(v4.Agent)) {
		t.Fatal("newly seeded v4 must differ from v3.1 regression")
	}
}

func TestAgentSuiteBuildIsDeterministic(t *testing.T) {
	a := BuildAgentChallengeV4("sha256:source")
	b := BuildAgentChallengeV4("sha256:source")
	if hash(mustJSON(a)) != hash(mustJSON(b)) {
		t.Fatal("v4 agent suite is not deterministic")
	}
}
