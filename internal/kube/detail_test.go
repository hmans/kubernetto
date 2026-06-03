package kube

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodDetailSurfacesContainerProblems(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "api", Image: "example/api:v1"},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "api",
					RestartCount: 4,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "back-off restarting failed container",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:  "Error",
							Message: "exited with code 1",
						},
					},
				},
			},
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodReady,
					Status:  corev1.ConditionFalse,
					Reason:  "ContainersNotReady",
					Message: "containers with unready status: [api]",
				},
			},
		},
	}

	detail := podDetail(pod)

	if got := detailFieldValue(detail.Fields, "Problem"); got != "api waiting: CrashLoopBackOff - back-off restarting failed container" {
		t.Fatalf("problem = %q", got)
	}
	container := detailSectionFieldValue(detail.Sections, "Containers", "api")
	for _, want := range []string{
		"waiting: CrashLoopBackOff - back-off restarting failed container",
		"example/api:v1",
		"restarts 4",
		"last terminated: Error - exited with code 1",
	} {
		if !strings.Contains(container, want) {
			t.Fatalf("container detail %q does not contain %q", container, want)
		}
	}
	if got := detailSectionFieldValue(detail.Sections, "Conditions", string(corev1.PodReady)); got != "False - ContainersNotReady - containers with unready status: [api]" {
		t.Fatalf("ready condition = %q", got)
	}
}

func TestPodDetailUsesConditionAsFallbackProblem(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "api", Image: "example/api:v1"},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "api",
					Ready: false,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.ContainersReady,
					Status:  corev1.ConditionFalse,
					Reason:  "ContainersNotReady",
					Message: "readiness probe failed",
				},
			},
		},
	}

	detail := podDetail(pod)

	if got := detailFieldValue(detail.Fields, "Problem"); got != "ContainersReady: ContainersNotReady - readiness probe failed" {
		t.Fatalf("problem = %q", got)
	}
	if got := detailSectionFieldValue(detail.Sections, "Conditions", string(corev1.ContainersReady)); got != "False - ContainersNotReady - readiness probe failed" {
		t.Fatalf("containers ready condition = %q", got)
	}
}

func TestOwnerSectionLinksKnownOwners(t *testing.T) {
	detail := podDetail(corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-pod",
			Namespace: "prod",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "api"},
				{Kind: "Widget", Name: "custom-owner"},
			},
		},
	})

	owner := detailSectionField(detail.Sections, "Owners", "Deployment")
	if owner == nil {
		t.Fatalf("deployment owner field missing")
	}
	if owner.Link == nil {
		t.Fatalf("deployment owner link missing")
	}
	if owner.Link.Resource != KindDeployments || owner.Link.Namespace != "prod" || owner.Link.Name != "api" {
		t.Fatalf("deployment owner link = %#v", owner.Link)
	}

	customOwner := detailSectionField(detail.Sections, "Owners", "Widget")
	if customOwner == nil {
		t.Fatalf("custom owner field missing")
	}
	if customOwner.Link != nil {
		t.Fatalf("custom owner link = %#v, want nil for unknown owner kind", customOwner.Link)
	}
}

func detailFieldValue(fields []DetailField, name string) string {
	for _, field := range fields {
		if field.Name == name {
			return field.Value
		}
	}
	return ""
}

func detailSectionFieldValue(sections []DetailSection, title, name string) string {
	field := detailSectionField(sections, title, name)
	if field == nil {
		return ""
	}
	return field.Value
}

func detailSectionField(sections []DetailSection, title, name string) *DetailField {
	for _, section := range sections {
		if section.Title != title {
			continue
		}
		for i := range section.Fields {
			if section.Fields[i].Name == name {
				return &section.Fields[i]
			}
		}
	}
	return nil
}
