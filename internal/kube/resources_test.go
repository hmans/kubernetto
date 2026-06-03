package kube

import "testing"

func TestResourceDefsCoverGroupsAndDefaults(t *testing.T) {
	seenKinds := map[ResourceKind]bool{}
	for _, def := range ResourceDefs {
		if def.Kind == "" || def.Label == "" || def.Scope == "" || def.Group == "" {
			t.Fatalf("incomplete resource def: %#v", def)
		}
		if seenKinds[def.Kind] {
			t.Fatalf("duplicate resource kind: %s", def.Kind)
		}
		seenKinds[def.Kind] = true
		if NormalizeKind(string(def.Kind)) != def.Kind {
			t.Fatalf("NormalizeKind(%q) did not return %q", def.Kind, def.Kind)
		}
	}

	for _, group := range ResourceGroups {
		if group.ID == "" || group.Label == "" || group.DefaultKind == "" {
			t.Fatalf("incomplete resource group: %#v", group)
		}
		if !seenKinds[group.DefaultKind] {
			t.Fatalf("group %q default kind %q has no resource def", group.ID, group.DefaultKind)
		}
		for _, kind := range group.Kinds {
			if !seenKinds[kind] {
				t.Fatalf("group %q references missing kind %q", group.ID, kind)
			}
		}
	}
}

func TestNormalizeKindFallsBackToOverview(t *testing.T) {
	if got := NormalizeKind("definitely-not-a-resource"); got != KindOverview {
		t.Fatalf("NormalizeKind fallback = %q, want %q", got, KindOverview)
	}
}

func TestReplicaSummaryCellCompactsHealthyAndDivergentDetails(t *testing.T) {
	if got, want := replicaSummaryCell(3, 3, "good", replicaDetail(3, "upd"), replicaDetail(3, "avail")).Value, "3/3 ready"; got != want {
		t.Fatalf("healthy replica summary = %q, want %q", got, want)
	}

	if got, want := replicaSummaryCell(2, 3, "warn", replicaDetail(1, "upd"), replicaDetail(2, "avail")).Value, "2/3 ready · upd 1 · avail 2"; got != want {
		t.Fatalf("degraded replica summary = %q, want %q", got, want)
	}
}
