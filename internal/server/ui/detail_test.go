package ui

import (
	"strings"
	"testing"

	"kubernetto/internal/kube"
)

func TestDetailSectionRendersOwnerLink(t *testing.T) {
	body := RenderFragment(DetailSectionView(kube.DetailSection{
		Title: "Owners",
		Fields: []kube.DetailField{
			{
				Name:  "Deployment",
				Value: "api",
				Link: &kube.DetailLink{
					Resource:  kube.KindDeployments,
					Namespace: "prod",
					Name:      "api",
				},
			},
		},
	}, Signals{Context: "dev", Clusters: "dev,prod"}))

	for _, want := range []string{
		"<a ",
		`resource=deployments`,
		`namespace=prod`,
		`selectedName=api`,
		`selectedNamespace=prod`,
		`context=dev`,
		`clusters=dev%2Cprod`,
		">api</a>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered owner link missing %q in %s", want, body)
		}
	}
	if strings.Contains(body, `data-on:`) {
		t.Fatalf("rendered owner link should be a plain browser link: %s", body)
	}
}
