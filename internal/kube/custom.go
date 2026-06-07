package kube

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

const customResourceGroupID = "custom"

var customResourceDefinitionGVR = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}

func (s *ResourceStore) CustomResourceDefs() []ResourceDef {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]ResourceDef(nil), s.customResources...)
}

func (s *ResourceStore) resourceDef(kind ResourceKind) ResourceDef {
	for _, def := range ResourceDefs {
		if def.Kind == kind {
			return def
		}
	}
	for _, def := range s.CustomResourceDefs() {
		if def.Kind == kind {
			return def
		}
	}
	return ResourceDefs[0]
}

func (s *ResourceStore) loadCustomResourceDefs(ctx context.Context) {
	if s == nil || s.cluster == nil || s.cluster.DynamicClient == nil {
		return
	}
	list, err := s.cluster.DynamicClient.Resource(customResourceDefinitionGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Debug("custom resource definition discovery failed", "error", err)
		}
		return
	}
	defs := customResourceDefsFromCRDs(list)
	s.mu.Lock()
	s.customResources = defs
	s.mu.Unlock()
}

func customResourceDefsFromCRDs(list *unstructured.UnstructuredList) []ResourceDef {
	if list == nil {
		return nil
	}
	defs := make([]ResourceDef, 0, len(list.Items))
	seen := map[ResourceKind]bool{}
	for index := range list.Items {
		crd := &list.Items[index]
		group, _, _ := unstructured.NestedString(crd.Object, "spec", "group")
		plural, _, _ := unstructured.NestedString(crd.Object, "spec", "names", "plural")
		objectKind, _, _ := unstructured.NestedString(crd.Object, "spec", "names", "kind")
		version, ok := customResourceServedVersion(crd)
		if group == "" || plural == "" || objectKind == "" || !ok {
			continue
		}
		kind := CustomResourceID(group, version, plural)
		if kind == "" || seen[kind] {
			continue
		}
		seen[kind] = true
		scope := "cluster"
		if crdScope, _, _ := unstructured.NestedString(crd.Object, "spec", "scope"); crdScope == "Namespaced" {
			scope = "namespaced"
		}
		defs = append(defs, ResourceDef{
			Kind:        kind,
			Label:       objectKind,
			Scope:       scope,
			Group:       customResourceGroupID,
			APIGroup:    group,
			APIVersion:  version,
			APIResource: plural,
			ObjectKind:  objectKind,
			Custom:      true,
		})
	}
	sort.Slice(defs, func(i, j int) bool {
		left := strings.ToLower(defs[i].Label + "/" + defs[i].APIGroup + "/" + defs[i].APIResource)
		right := strings.ToLower(defs[j].Label + "/" + defs[j].APIGroup + "/" + defs[j].APIResource)
		return left < right
	})
	return defs
}

func customResourceServedVersion(crd *unstructured.Unstructured) (string, bool) {
	versions, ok, _ := unstructured.NestedSlice(crd.Object, "spec", "versions")
	if !ok {
		return "", false
	}
	firstServed := ""
	for _, raw := range versions {
		version, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := version["name"].(string)
		served, _ := version["served"].(bool)
		storage, _ := version["storage"].(bool)
		if name == "" || !served {
			continue
		}
		if firstServed == "" {
			firstServed = name
		}
		if storage {
			return name, true
		}
	}
	if firstServed != "" {
		return firstServed, true
	}
	return "", false
}

func (s *ResourceStore) customResourceTable(def ResourceDef, namespace, query, sortColumn, sortOrder string) Table {
	table := Table{
		Kind:       def.Kind,
		Label:      def.Label,
		Namespace:  namespace,
		Query:      query,
		UpdatedAt:  time.Now(),
		Namespaced: def.Scope == "namespaced",
	}
	if def.Scope == "namespaced" {
		table.Columns = []string{"Name", "Namespace", "Status", "Age"}
	} else {
		table.Columns = []string{"Name", "Status", "Age"}
	}
	client, err := s.customResourceClient(def, namespace)
	if err != nil {
		table.Error = err.Error()
		return table
	}
	list, err := client.List(context.Background(), metav1.ListOptions{})
	if err != nil {
		table.Error = err.Error()
		return table
	}
	for index := range list.Items {
		row := customResourceRow(&list.Items[index], def.Scope == "namespaced")
		if matches(row, query) {
			table.Rows = append(table.Rows, row)
		}
	}
	sortTableRows(&table, sortColumn, sortOrder)
	return table
}

func (s *ResourceStore) customResourceDetail(def ResourceDef, namespace, name string) ResourceDetail {
	detail := ResourceDetail{Kind: def.Kind, Label: def.Label, Name: name, Namespace: namespace}
	client, err := s.customResourceClient(def, namespace)
	if err != nil {
		detail.Error = err.Error()
		return detail
	}
	if def.Scope == "namespaced" && namespace == "" {
		detail.Error = "Namespace is required for this custom resource."
		return detail
	}
	obj, err := client.Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		detail.Error = err.Error()
		return detail
	}
	return customResourceDetail(def, obj)
}

func (s *ResourceStore) customResourceClient(def ResourceDef, namespace string) (dynamic.ResourceInterface, error) {
	if s == nil || s.cluster == nil || s.cluster.DynamicClient == nil {
		return nil, fmt.Errorf("No Kubernetes dynamic client is configured.")
	}
	gvr, ok := def.GroupVersionResource()
	if !ok {
		return nil, fmt.Errorf("Custom resource definition is incomplete.")
	}
	resource := s.cluster.DynamicClient.Resource(gvr)
	if def.Scope == "namespaced" {
		return resource.Namespace(namespace), nil
	}
	return resource, nil
}

func customResourceRow(obj *unstructured.Unstructured, namespaced bool) Row {
	status, statusKey := customResourceStatus(obj)
	cells := []Cell{{Value: obj.GetName(), Class: "primary"}}
	if namespaced {
		cells = append(cells, Cell{Value: obj.GetNamespace()})
	}
	cells = append(cells, Cell{Value: status, Class: "status " + statusKey}, ageCell(obj.GetCreationTimestamp().Time))
	return Row{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Status:    status,
		StatusKey: statusKey,
		Cells:     cells,
	}
}

func customResourceDetail(def ResourceDef, obj *unstructured.Unstructured) ResourceDetail {
	status, statusKey := customResourceStatus(obj)
	meta := metav1.ObjectMeta{
		Name:              obj.GetName(),
		Namespace:         obj.GetNamespace(),
		UID:               types.UID(obj.GetUID()),
		CreationTimestamp: obj.GetCreationTimestamp(),
		Labels:            obj.GetLabels(),
		Annotations:       obj.GetAnnotations(),
		OwnerReferences:   obj.GetOwnerReferences(),
	}
	detail := detailBase(def.Kind, def.Label, meta, status, statusKey)
	detail.Fields = detailFields(
		"API Version", obj.GetAPIVersion(),
		"Kind", obj.GetKind(),
		"Resource", def.APIResource,
		"Generation", fmt.Sprint(obj.GetGeneration()),
	)
	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		detail.Sections = append(detail.Sections, DetailSection{Title: "Spec", Fields: customObjectFields(spec)})
	}
	if statusMap, ok, _ := unstructured.NestedMap(obj.Object, "status"); ok {
		detail.Sections = append(detail.Sections, DetailSection{Title: "Status", Fields: customObjectFields(statusMap)})
	}
	detail.Sections = append(detail.Sections, ownerSection(obj.GetNamespace(), obj.GetOwnerReferences()))
	detail.YAML = resourceYAML(obj.Object)
	return compactDetail(detail)
}

func customResourceStatus(obj *unstructured.Unstructured) (string, string) {
	for _, path := range [][]string{
		{"status", "phase"},
		{"status", "status"},
		{"status", "state"},
	} {
		if value, ok, _ := unstructured.NestedString(obj.Object, path...); ok && strings.TrimSpace(value) != "" {
			return value, genericStatusKey(value)
		}
	}
	if conditions, ok, _ := unstructured.NestedSlice(obj.Object, "status", "conditions"); ok {
		for _, raw := range conditions {
			condition, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			conditionType, _ := condition["type"].(string)
			conditionStatus, _ := condition["status"].(string)
			if strings.EqualFold(conditionType, "Ready") || strings.EqualFold(conditionType, "Synced") {
				return conditionType + "=" + conditionStatus, genericStatusKey(conditionStatus)
			}
		}
	}
	return "Unknown", "neutral"
}

func genericStatusKey(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "ready", "running", "succeeded", "available", "active", "healthy", "synced":
		return "good"
	case "false", "failed", "error", "degraded", "unavailable", "terminating":
		return "bad"
	default:
		return "neutral"
	}
}

func customObjectFields(values map[string]any) []DetailField {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]DetailField, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, DetailField{Name: key, Value: customFieldValue(values[key])})
	}
	return fields
}

func customFieldValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return fmt.Sprint(typed)
	case int64, int32, int, float64, float32:
		return fmt.Sprint(typed)
	case map[string]any:
		return fmt.Sprintf("%d fields", len(typed))
	case []any:
		return fmt.Sprintf("%d items", len(typed))
	default:
		return fmt.Sprint(typed)
	}
}
