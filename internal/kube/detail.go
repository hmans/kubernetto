package kube

import (
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/yaml"
)

func (s *ResourceStore) Detail(kind ResourceKind, namespace, name string) ResourceDetail {
	def := resourceDef(kind)
	detail := ResourceDetail{Kind: def.Kind, Label: def.Label, Name: name, Namespace: namespace}
	if name == "" {
		return detail
	}
	if err := s.readinessError(); err != nil {
		detail.Error = err.Error()
		return detail
	}

	switch def.Kind {
	case KindPods:
		pod, err := s.pods.Pods(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(podDetail(*pod))
	case KindDeployments:
		deployment, err := s.deployments.Deployments(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(deploymentDetail(*deployment))
	case KindStatefulSet:
		statefulSet, err := s.statefulSets.StatefulSets(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(statefulSetDetail(*statefulSet))
	case KindDaemonSet:
		daemonSet, err := s.daemonSets.DaemonSets(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(daemonSetDetail(*daemonSet))
	case KindServices:
		service, err := s.services.Services(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(serviceDetail(*service))
	case KindIngresses:
		ingress, err := s.ingresses.Ingresses(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(ingressDetail(*ingress))
	case KindNodes:
		node, err := s.nodes.Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(nodeDetail(*node))
	case KindNamespaces:
		namespace, err := s.namespaces.Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(namespaceDetail(*namespace))
	default:
		detail.Error = "Unsupported resource kind."
		return detail
	}
}

func podDetail(pod corev1.Pod) ResourceDetail {
	yamlPod := pod
	yamlPod.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
	yamlPod.ManagedFields = nil

	ready, total, restarts := podReady(pod)
	status := string(pod.Status.Phase)
	if pod.DeletionTimestamp != nil {
		status = "Terminating"
	}
	detail := detailBase(KindPods, "Pods", pod.ObjectMeta, status, healthKey(status == "Running" && ready == total))
	detail.YAML = resourceYAML(yamlPod)
	detail.Fields = detailFields(
		"Problem", podProblem(pod),
		"Namespace", pod.Namespace,
		"Ready", fmt.Sprintf("%d/%d", ready, total),
		"Restarts", fmt.Sprint(restarts),
		"Node", pod.Spec.NodeName,
		"Pod IP", pod.Status.PodIP,
		"Host IP", pod.Status.HostIP,
		"Service Account", pod.Spec.ServiceAccountName,
		"QoS", string(pod.Status.QOSClass),
	)
	detail.Sections = append(detail.Sections,
		DetailSection{Title: "Containers", Fields: containerFields(pod)},
		DetailSection{Title: "Conditions", Fields: podConditionFields(pod.Status.Conditions)},
		ownerSection(pod.OwnerReferences),
	)
	return compactDetail(detail)
}

func deploymentDetail(deployment appsv1.Deployment) ResourceDetail {
	yamlDeployment := deployment
	yamlDeployment.TypeMeta = metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"}
	yamlDeployment.ManagedFields = nil

	desired := int32(0)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	healthy := deployment.Status.ReadyReplicas == desired && deployment.Status.UpdatedReplicas == desired
	detail := detailBase(KindDeployments, "Deployments", deployment.ObjectMeta, fmt.Sprintf("%d/%d ready", deployment.Status.ReadyReplicas, desired), healthKey(healthy))
	detail.YAML = resourceYAML(yamlDeployment)
	detail.Fields = detailFields(
		"Namespace", deployment.Namespace,
		"Ready", fmt.Sprintf("%d/%d", deployment.Status.ReadyReplicas, desired),
		"Up-to-date", fmt.Sprint(deployment.Status.UpdatedReplicas),
		"Available", fmt.Sprint(deployment.Status.AvailableReplicas),
		"Unavailable", fmt.Sprint(deployment.Status.UnavailableReplicas),
		"Strategy", string(deployment.Spec.Strategy.Type),
	)
	detail.Sections = append(detail.Sections,
		DetailSection{Title: "Selector", Fields: selectorFields(deployment.Spec.Selector)},
		DetailSection{Title: "Conditions", Fields: deploymentConditionFields(deployment.Status.Conditions)},
	)
	return compactDetail(detail)
}

func statefulSetDetail(statefulSet appsv1.StatefulSet) ResourceDetail {
	yamlStatefulSet := statefulSet
	yamlStatefulSet.TypeMeta = metav1.TypeMeta{APIVersion: "apps/v1", Kind: "StatefulSet"}
	yamlStatefulSet.ManagedFields = nil

	desired := int32(0)
	if statefulSet.Spec.Replicas != nil {
		desired = *statefulSet.Spec.Replicas
	}
	healthy := statefulSet.Status.ReadyReplicas == desired
	detail := detailBase(KindStatefulSet, "StatefulSets", statefulSet.ObjectMeta, fmt.Sprintf("%d/%d ready", statefulSet.Status.ReadyReplicas, desired), healthKey(healthy))
	detail.YAML = resourceYAML(yamlStatefulSet)
	detail.Fields = detailFields(
		"Namespace", statefulSet.Namespace,
		"Ready", fmt.Sprintf("%d/%d", statefulSet.Status.ReadyReplicas, desired),
		"Replicas", fmt.Sprint(statefulSet.Status.Replicas),
		"Current", fmt.Sprint(statefulSet.Status.CurrentReplicas),
		"Updated", fmt.Sprint(statefulSet.Status.UpdatedReplicas),
		"Service", statefulSet.Spec.ServiceName,
	)
	detail.Sections = append(detail.Sections,
		DetailSection{Title: "Selector", Fields: selectorFields(statefulSet.Spec.Selector)},
		ownerSection(statefulSet.OwnerReferences),
	)
	return compactDetail(detail)
}

func daemonSetDetail(daemonSet appsv1.DaemonSet) ResourceDetail {
	yamlDaemonSet := daemonSet
	yamlDaemonSet.TypeMeta = metav1.TypeMeta{APIVersion: "apps/v1", Kind: "DaemonSet"}
	yamlDaemonSet.ManagedFields = nil

	healthy := daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled
	detail := detailBase(KindDaemonSet, "DaemonSets", daemonSet.ObjectMeta, fmt.Sprintf("%d/%d ready", daemonSet.Status.NumberReady, daemonSet.Status.DesiredNumberScheduled), healthKey(healthy))
	detail.YAML = resourceYAML(yamlDaemonSet)
	detail.Fields = detailFields(
		"Namespace", daemonSet.Namespace,
		"Desired", fmt.Sprint(daemonSet.Status.DesiredNumberScheduled),
		"Ready", fmt.Sprint(daemonSet.Status.NumberReady),
		"Available", fmt.Sprint(daemonSet.Status.NumberAvailable),
		"Unavailable", fmt.Sprint(daemonSet.Status.NumberUnavailable),
		"Misscheduled", fmt.Sprint(daemonSet.Status.NumberMisscheduled),
	)
	detail.Sections = append(detail.Sections,
		DetailSection{Title: "Selector", Fields: selectorFields(daemonSet.Spec.Selector)},
		DetailSection{Title: "Conditions", Fields: daemonSetConditionFields(daemonSet.Status.Conditions)},
	)
	return compactDetail(detail)
}

func serviceDetail(service corev1.Service) ResourceDetail {
	yamlService := service
	yamlService.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Service"}
	yamlService.ManagedFields = nil

	detail := detailBase(KindServices, "Services", service.ObjectMeta, string(service.Spec.Type), "neutral")
	detail.YAML = resourceYAML(yamlService)
	detail.Fields = detailFields(
		"Namespace", service.Namespace,
		"Type", string(service.Spec.Type),
		"Cluster IP", service.Spec.ClusterIP,
		"External IPs", strings.Join(service.Spec.ExternalIPs, ", "),
		"Load Balancer IP", service.Spec.LoadBalancerIP,
		"Session Affinity", string(service.Spec.SessionAffinity),
	)
	detail.Sections = append(detail.Sections,
		DetailSection{Title: "Ports", Fields: servicePortFields(service.Spec.Ports)},
		DetailSection{Title: "Selector", Fields: mapFields(service.Spec.Selector)},
	)
	return compactDetail(detail)
}

func ingressDetail(ingress networkingv1.Ingress) ResourceDetail {
	yamlIngress := ingress
	yamlIngress.TypeMeta = metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "Ingress"}
	yamlIngress.ManagedFields = nil

	className := ""
	if ingress.Spec.IngressClassName != nil {
		className = *ingress.Spec.IngressClassName
	}
	detail := detailBase(KindIngresses, "Ingresses", ingress.ObjectMeta, "Ingress", "neutral")
	detail.YAML = resourceYAML(yamlIngress)
	detail.Fields = detailFields(
		"Namespace", ingress.Namespace,
		"Class", className,
		"Hosts", ingressHosts(ingress.Spec.Rules),
		"Address", ingressAddresses(ingress.Status.LoadBalancer.Ingress),
		"TLS Secrets", ingressTLSSecrets(ingress.Spec.TLS),
	)
	detail.Sections = append(detail.Sections, DetailSection{Title: "Rules", Fields: ingressRuleFields(ingress.Spec.Rules)})
	return compactDetail(detail)
}

func nodeDetail(node corev1.Node) ResourceDetail {
	yamlNode := node
	yamlNode.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Node"}
	yamlNode.ManagedFields = nil

	ready := false
	conditions := node.Status.Conditions
	for _, condition := range conditions {
		if condition.Type == corev1.NodeReady {
			ready = condition.Status == corev1.ConditionTrue
			break
		}
	}
	detail := detailBase(KindNodes, "Nodes", node.ObjectMeta, mapBool(ready, "Ready", "NotReady"), healthKey(ready))
	detail.YAML = resourceYAML(yamlNode)
	detail.Fields = detailFields(
		"Roles", nodeRoles(node.Labels),
		"Kubelet", node.Status.NodeInfo.KubeletVersion,
		"OS Image", node.Status.NodeInfo.OSImage,
		"Kernel", node.Status.NodeInfo.KernelVersion,
		"Container Runtime", node.Status.NodeInfo.ContainerRuntimeVersion,
		"Internal IP", nodeInternalIP(node.Status.Addresses),
		"Architecture", node.Status.NodeInfo.Architecture,
	)
	detail.Sections = append(detail.Sections,
		DetailSection{Title: "Capacity", Fields: resourceListFields(node.Status.Capacity)},
		DetailSection{Title: "Allocatable", Fields: resourceListFields(node.Status.Allocatable)},
		DetailSection{Title: "Conditions", Fields: nodeConditionFields(conditions)},
	)
	return compactDetail(detail)
}

func namespaceDetail(namespace corev1.Namespace) ResourceDetail {
	yamlNamespace := namespace
	yamlNamespace.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"}
	yamlNamespace.ManagedFields = nil

	healthy := namespace.Status.Phase == corev1.NamespaceActive
	detail := detailBase(KindNamespaces, "Namespaces", namespace.ObjectMeta, string(namespace.Status.Phase), healthKey(healthy))
	detail.YAML = resourceYAML(yamlNamespace)
	detail.Fields = detailFields("Phase", string(namespace.Status.Phase))
	detail.Sections = append(detail.Sections, ownerSection(namespace.OwnerReferences))
	return compactDetail(detail)
}

func (s *ResourceStore) withEvents(detail ResourceDetail) ResourceDetail {
	if s == nil || s.events == nil || detail.Name == "" {
		return detail
	}

	events := s.matchingEvents(detail)
	if len(events) == 0 {
		return detail
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].LastSeen.After(events[j].LastSeen)
	})
	if len(events) > 8 {
		events = events[:8]
	}
	detail.Events = events
	return detail
}

func (s *ResourceStore) matchingEvents(detail ResourceDetail) []ResourceEvent {
	items, _ := s.events.List(labels.Everything())
	events := make([]ResourceEvent, 0)
	for _, event := range items {
		ref := event.InvolvedObject
		if !eventMatchesDetail(ref, detail) {
			continue
		}
		lastSeen := eventTimestamp(*event)
		events = append(events, ResourceEvent{
			Type:     event.Type,
			Reason:   event.Reason,
			Message:  event.Message,
			Count:    event.Count,
			Age:      age(lastSeen),
			LastSeen: lastSeen,
		})
	}
	return events
}

func eventMatchesDetail(ref corev1.ObjectReference, detail ResourceDetail) bool {
	if detail.UID != "" && string(ref.UID) == detail.UID {
		return true
	}
	if ref.Kind != resourceObjectKind(detail.Kind) || ref.Name != detail.Name {
		return false
	}
	if detail.Namespace == "" {
		return ref.Namespace == ""
	}
	return ref.Namespace == detail.Namespace
}

func eventTimestamp(event corev1.Event) time.Time {
	switch {
	case !event.EventTime.IsZero():
		return event.EventTime.Time
	case !event.LastTimestamp.IsZero():
		return event.LastTimestamp.Time
	case !event.FirstTimestamp.IsZero():
		return event.FirstTimestamp.Time
	default:
		return event.CreationTimestamp.Time
	}
}

func resourceObjectKind(kind ResourceKind) string {
	switch kind {
	case KindPods:
		return "Pod"
	case KindDeployments:
		return "Deployment"
	case KindStatefulSet:
		return "StatefulSet"
	case KindDaemonSet:
		return "DaemonSet"
	case KindServices:
		return "Service"
	case KindIngresses:
		return "Ingress"
	case KindNodes:
		return "Node"
	case KindNamespaces:
		return "Namespace"
	default:
		return ""
	}
}

func resourceYAML(value any) string {
	out, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Sprintf("yaml error: %s", err)
	}
	return strings.TrimSpace(string(out))
}

func detailBase(kind ResourceKind, label string, meta metav1.ObjectMeta, status, statusKey string) ResourceDetail {
	return ResourceDetail{
		Kind:        kind,
		Label:       label,
		Name:        meta.Name,
		Namespace:   meta.Namespace,
		Status:      status,
		StatusKey:   statusKey,
		Age:         age(meta.CreationTimestamp.Time),
		CreatedAt:   meta.CreationTimestamp.Time,
		UID:         string(meta.UID),
		Labels:      mapFields(meta.Labels),
		Annotations: mapFields(meta.Annotations),
	}
}

func compactDetail(detail ResourceDetail) ResourceDetail {
	detail.Fields = compactFields(detail.Fields)
	sections := make([]DetailSection, 0, len(detail.Sections))
	for _, section := range detail.Sections {
		section.Fields = compactFields(section.Fields)
		if len(section.Fields) > 0 {
			sections = append(sections, section)
		}
	}
	detail.Sections = sections
	return detail
}

func compactFields(fields []DetailField) []DetailField {
	out := make([]DetailField, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.Value) != "" {
			out = append(out, field)
		}
	}
	return out
}

func detailFields(values ...string) []DetailField {
	fields := make([]DetailField, 0, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		fields = append(fields, DetailField{Name: values[i], Value: values[i+1]})
	}
	return fields
}

func mapFields(values map[string]string) []DetailField {
	fields := make([]DetailField, 0, len(values))
	for key, value := range values {
		fields = append(fields, DetailField{Name: key, Value: value})
	}
	sort.Slice(fields, func(i, j int) bool {
		return strings.ToLower(fields[i].Name) < strings.ToLower(fields[j].Name)
	})
	return fields
}

func podReady(pod corev1.Pod) (int, int, int32) {
	ready := 0
	restarts := int32(0)
	for _, status := range pod.Status.ContainerStatuses {
		if status.Ready {
			ready++
		}
		restarts += status.RestartCount
	}
	return ready, len(pod.Spec.Containers), restarts
}

func containerFields(pod corev1.Pod) []DetailField {
	statuses := map[string]corev1.ContainerStatus{}
	for _, status := range pod.Status.ContainerStatuses {
		statuses[status.Name] = status
	}
	fields := make([]DetailField, 0, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		status := statuses[container.Name]
		fields = append(fields, DetailField{
			Name:  container.Name,
			Value: containerStatusValue(container, status),
		})
	}
	return fields
}

func containerStatusValue(container corev1.Container, status corev1.ContainerStatus) string {
	parts := []string{containerStateText(status)}
	if container.Image != "" {
		parts = append(parts, container.Image)
	}
	parts = append(parts, fmt.Sprintf("restarts %d", status.RestartCount))
	return strings.Join(parts, " - ")
}

func containerStateText(status corev1.ContainerStatus) string {
	state := "waiting"
	switch {
	case status.State.Waiting != nil:
		state = "waiting"
		if text := reasonMessage(status.State.Waiting.Reason, status.State.Waiting.Message); text != "" {
			state += ": " + text
		}
	case status.State.Terminated != nil:
		state = "terminated"
		if text := reasonMessage(status.State.Terminated.Reason, status.State.Terminated.Message); text != "" {
			state += ": " + text
		}
	case status.Ready:
		state = "ready"
	case status.State.Running != nil:
		state = "running"
	}
	if status.LastTerminationState.Terminated != nil {
		last := status.LastTerminationState.Terminated
		if text := reasonMessage(last.Reason, last.Message); text != "" {
			state += "; last terminated: " + text
		}
	}
	return state
}

func podConditionFields(conditions []corev1.PodCondition) []DetailField {
	fields := make([]DetailField, 0, len(conditions))
	for _, condition := range conditions {
		value := string(condition.Status)
		if text := reasonMessage(condition.Reason, condition.Message); text != "" {
			value += " - " + text
		}
		fields = append(fields, DetailField{Name: string(condition.Type), Value: value})
	}
	return fields
}

func podProblem(pod corev1.Pod) string {
	if pod.DeletionTimestamp != nil {
		return "Pod is terminating"
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Waiting != nil {
			if text := reasonMessage(status.State.Waiting.Reason, status.State.Waiting.Message); text != "" {
				return status.Name + " waiting: " + text
			}
			return status.Name + " waiting"
		}
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Terminated != nil {
			if text := reasonMessage(status.State.Terminated.Reason, status.State.Terminated.Message); text != "" {
				return status.Name + " terminated: " + text
			}
			return status.Name + " terminated"
		}
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.LastTerminationState.Terminated != nil {
			last := status.LastTerminationState.Terminated
			if text := reasonMessage(last.Reason, last.Message); text != "" {
				return status.Name + " recently terminated: " + text
			}
			return status.Name + " recently terminated"
		}
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Status == corev1.ConditionFalse || condition.Status == corev1.ConditionUnknown {
			if text := reasonMessage(condition.Reason, condition.Message); text != "" {
				return string(condition.Type) + ": " + text
			}
			return string(condition.Type) + ": " + string(condition.Status)
		}
	}
	if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != "" {
		return "Pod phase is " + string(pod.Status.Phase)
	}
	return ""
}

func reasonMessage(reason, message string) string {
	switch {
	case reason != "" && message != "":
		return reason + " - " + message
	case reason != "":
		return reason
	default:
		return message
	}
}

func deploymentConditionFields(conditions []appsv1.DeploymentCondition) []DetailField {
	fields := make([]DetailField, 0, len(conditions))
	for _, condition := range conditions {
		value := string(condition.Status)
		if condition.Reason != "" {
			value += " - " + condition.Reason
		}
		fields = append(fields, DetailField{Name: string(condition.Type), Value: value})
	}
	return fields
}

func daemonSetConditionFields(conditions []appsv1.DaemonSetCondition) []DetailField {
	fields := make([]DetailField, 0, len(conditions))
	for _, condition := range conditions {
		value := string(condition.Status)
		if condition.Reason != "" {
			value += " - " + condition.Reason
		}
		fields = append(fields, DetailField{Name: string(condition.Type), Value: value})
	}
	return fields
}

func nodeConditionFields(conditions []corev1.NodeCondition) []DetailField {
	fields := make([]DetailField, 0, len(conditions))
	for _, condition := range conditions {
		value := string(condition.Status)
		if condition.Reason != "" {
			value += " - " + condition.Reason
		}
		fields = append(fields, DetailField{Name: string(condition.Type), Value: value})
	}
	return fields
}

func servicePortFields(ports []corev1.ServicePort) []DetailField {
	fields := make([]DetailField, 0, len(ports))
	for _, port := range ports {
		name := port.Name
		if name == "" {
			name = fmt.Sprintf("%d/%s", port.Port, port.Protocol)
		}
		value := fmt.Sprintf("%d -> %s/%s", port.Port, port.TargetPort.String(), port.Protocol)
		if port.NodePort != 0 {
			value += fmt.Sprintf(" - node %d", port.NodePort)
		}
		fields = append(fields, DetailField{Name: name, Value: value})
	}
	return fields
}

func selectorFields(selector *metav1.LabelSelector) []DetailField {
	if selector == nil {
		return nil
	}
	fields := mapFields(selector.MatchLabels)
	for _, expression := range selector.MatchExpressions {
		value := string(expression.Operator)
		if len(expression.Values) > 0 {
			value += " " + strings.Join(expression.Values, ", ")
		}
		fields = append(fields, DetailField{Name: expression.Key, Value: value})
	}
	return fields
}

func ingressRuleFields(rules []networkingv1.IngressRule) []DetailField {
	fields := make([]DetailField, 0, len(rules))
	for _, rule := range rules {
		host := rule.Host
		if host == "" {
			host = "*"
		}
		paths := []string{}
		if rule.HTTP != nil {
			for _, path := range rule.HTTP.Paths {
				backend := path.Backend.Service
				if backend == nil {
					continue
				}
				pathValue := path.Path
				if pathValue == "" {
					pathValue = "/"
				}
				paths = append(paths, fmt.Sprintf("%s -> %s:%s", pathValue, backend.Name, serviceBackendPort(backend.Port)))
			}
		}
		fields = append(fields, DetailField{Name: host, Value: strings.Join(paths, ", ")})
	}
	return fields
}

func ingressTLSSecrets(entries []networkingv1.IngressTLS) string {
	secrets := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.SecretName != "" {
			secrets = append(secrets, entry.SecretName)
		}
	}
	sort.Strings(secrets)
	return strings.Join(secrets, ", ")
}

func serviceBackendPort(port networkingv1.ServiceBackendPort) string {
	if port.Name != "" {
		return port.Name
	}
	if port.Number != 0 {
		return fmt.Sprint(port.Number)
	}
	return ""
}

func resourceListFields(values corev1.ResourceList) []DetailField {
	fields := make([]DetailField, 0, len(values))
	for name, quantity := range values {
		fields = append(fields, DetailField{Name: string(name), Value: quantity.String()})
	}
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Name < fields[j].Name
	})
	return fields
}

func ownerSection(owners []metav1.OwnerReference) DetailSection {
	fields := make([]DetailField, 0, len(owners))
	for _, owner := range owners {
		fields = append(fields, DetailField{Name: owner.Kind, Value: owner.Name})
	}
	return DetailSection{Title: "Owners", Fields: fields}
}
