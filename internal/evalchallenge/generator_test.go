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
	v5a := BuildAgentChallengeV5("sha256:source")
	v5b := BuildAgentChallengeV5("sha256:source")
	if hash(mustJSON(v5a)) != hash(mustJSON(v5b)) {
		t.Fatal("v5 agent suite is not deterministic")
	}
	v6a := BuildAgentChallengeV6("sha256:source")
	v6b := BuildAgentChallengeV6("sha256:source")
	if hash(mustJSON(v6a)) != hash(mustJSON(v6b)) {
		t.Fatal("v6 agent suite is not deterministic")
	}
	v61a := BuildAgentRegressionV61("sha256:source")
	v61b := BuildAgentRegressionV61("sha256:source")
	if hash(mustJSON(v61a)) != hash(mustJSON(v61b)) {
		t.Fatal("v6.1 agent regression suite is not deterministic")
	}
}

func TestAgentV61SeparatesTaskAndRuntimeContracts(t *testing.T) {
	suite := BuildAgentRegressionV61("sha256:source")
	if len(suite.Agent.Cases) != 48 || suite.Agent.Splits["challenge"].Sealed {
		t.Fatalf("v6.1 must be a 48-case inspectable regression suite: %+v", suite.Agent.Splits)
	}
	for _, c := range suite.Agent.Cases {
		if c.Expected.RuntimeVersion != "" || c.Expected.RequireAnswerVerified ||
			c.Expected.MaxSearchRounds != 0 || c.Expected.MaxReviewPagesPerShop != 0 {
			t.Fatalf("case %s leaked runtime conformance into task expectations: %+v", c.ID, c.Expected)
		}
		for _, tag := range c.Tags {
			if tag == "v2_runtime" {
				t.Fatalf("case %s retained v2_runtime task tag", c.ID)
			}
		}
	}
}

func TestAgentV6CarriesRuntimeEvidenceBudgets(t *testing.T) {
	suite := BuildAgentChallengeV6("sha256:source")
	if len(suite.Agent.Cases) != 48 {
		t.Fatalf("v6 cases=%d", len(suite.Agent.Cases))
	}
	for _, c := range suite.Agent.Cases {
		if c.Expected.RuntimeVersion != "v2_react" || c.Expected.MaxSteps != 4 ||
			c.Expected.MaxToolCalls != 10 || c.Expected.MaxSearchRounds != 2 ||
			c.Expected.MaxReviewPagesPerShop != 2 || !c.Expected.RequireAnswerVerified {
			t.Fatalf("case %s missing V2 contract: %+v", c.ID, c.Expected)
		}
	}
}

func TestExpandedAgentV5Coverage(t *testing.T) {
	suite := BuildAgentChallengeV5("sha256:source")
	if len(suite.Agent.Cases) != 48 || suite.Agent.Splits["dev"].Cases != 8 || suite.Agent.Splits["challenge"].Cases != 40 {
		t.Fatalf("invalid v5 shape: cases=%d splits=%+v", len(suite.Agent.Cases), suite.Agent.Splits)
	}
	wantFamilies := map[string]bool{
		"semantic_ood": false, "typo": false, "preference_correction": false,
		"coreference": false, "prompt_injection": false, "no_result": false,
		"claim_level_grounding": false, "session_isolation": false,
	}
	for _, c := range suite.Agent.Cases {
		for _, tag := range c.Tags {
			if _, tracked := wantFamilies[tag]; tracked {
				wantFamilies[tag] = true
			}
		}
	}
	for family, found := range wantFamilies {
		if !found {
			t.Fatalf("v5 missing capability family %s", family)
		}
	}
	if suite.Manifest.AgentCases != 48 || suite.Manifest.AgentChallengeTrials <= 40 {
		t.Fatalf("invalid v5 manifest counts: %+v", suite.Manifest)
	}
}
