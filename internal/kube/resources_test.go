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
