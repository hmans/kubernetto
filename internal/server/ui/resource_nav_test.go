package ui

import (
	"strings"
	"testing"

	"kubernetto/internal/kube"
)

func TestResourceNavGroupsCustomResourcesByAPIGroup(t *testing.T) {
	stableWidget := kube.ResourceDef{
		Kind:        kube.CustomResourceID("stable.example.com", "v1", "widgets"),
		Label:       "Widget",
		Scope:       "namespaced",
		Group:       "custom",
		APIGroup:    "stable.example.com",
		APIVersion:  "v1",
		APIResource: "widgets",
		Custom:      true,
	}
	monitoringRule := kube.ResourceDef{
		Kind:        kube.CustomResourceID("monitoring.coreos.com", "v1", "prometheusrules"),
		Label:       "PrometheusRule",
		Scope:       "namespaced",
		Group:       "custom",
		APIGroup:    "monitoring.coreos.com",
		APIVersion:  "v1",
		APIResource: "prometheusrules",
		Custom:      true,
	}

	body := RenderFragment(ResourceNavView(PageState{
		Resources: []kube.ResourceDef{stableWidget, monitoringRule},
		Signals:   Signals{Resource: string(stableWidget.Kind)},
	}))

	for _, want := range []string{
		`data-resource-group="custom:monitoring.coreos.com"`,
		`data-resource-group="custom:stable.example.com"`,
		"monitoring.coreos.com",
		"stable.example.com",
		"PrometheusRule",
		"Widget",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("resource nav missing %q in %s", want, body)
		}
	}
	if strings.Contains(body, "Custom Resources") {
		t.Fatalf("custom resources should be grouped by API group domain: %s", body)
	}
}
