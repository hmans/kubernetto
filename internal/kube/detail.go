package kube

import (
	"fmt"
	"sort"
	"strings"
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	nodev1 "k8s.io/api/node/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	case KindReplicaSets:
		replicaSet, err := s.replicaSets.ReplicaSets(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(replicaSetDetail(*replicaSet))
	case KindJobs:
		job, err := s.jobs.Jobs(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(jobDetail(*job))
	case KindCronJobs:
		cronJob, err := s.cronJobs.CronJobs(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(cronJobDetail(*cronJob))
	case KindPersistentVolumeClaims:
		pvc, err := s.persistentVolumeClaims.PersistentVolumeClaims(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(persistentVolumeClaimDetail(*pvc))
	case KindPersistentVolumes:
		pv, err := s.persistentVolumes.Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(persistentVolumeDetail(*pv))
	case KindStorageClasses:
		storageClass, err := s.storageClasses.Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return storageClassDetail(*storageClass)
	case KindServices:
		service, err := s.services.Services(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(serviceDetail(*service))
	case KindEndpoints:
		endpoint, err := s.endpoints.Endpoints(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(endpointsDetail(*endpoint))
	case KindEndpointSlices:
		endpointSlice, err := s.endpointSlices.EndpointSlices(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(endpointSliceDetail(*endpointSlice))
	case KindIngresses:
		ingress, err := s.ingresses.Ingresses(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(ingressDetail(*ingress))
	case KindIngressClasses:
		ingressClass, err := s.ingressClasses.Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return ingressClassDetail(*ingressClass)
	case KindNetworkPolicies:
		policy, err := s.networkPolicies.NetworkPolicies(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(networkPolicyDetail(*policy))
	case KindServiceAccounts:
		account, err := s.serviceAccounts.ServiceAccounts(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(serviceAccountDetail(*account))
	case KindRoles:
		role, err := s.roles.Roles(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return roleDetail(*role)
	case KindRoleBindings:
		binding, err := s.roleBindings.RoleBindings(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return roleBindingDetail(*binding)
	case KindClusterRoles:
		role, err := s.clusterRoles.Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return clusterRoleDetail(*role)
	case KindClusterRoleBindings:
		binding, err := s.clusterRoleBindings.Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return clusterRoleBindingDetail(*binding)
	case KindConfigMaps:
		configMap, err := s.configMaps.ConfigMaps(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(configMapDetail(*configMap))
	case KindSecrets:
		secret, err := s.secrets.Secrets(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(secretDetail(*secret))
	case KindHorizontalPodAutoscalers:
		hpa, err := s.horizontalPodAutoscalers.HorizontalPodAutoscalers(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(horizontalPodAutoscalerDetail(*hpa))
	case KindPodDisruptionBudgets:
		pdb, err := s.podDisruptionBudgets.PodDisruptionBudgets(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(podDisruptionBudgetDetail(*pdb))
	case KindResourceQuotas:
		quota, err := s.resourceQuotas.ResourceQuotas(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(resourceQuotaDetail(*quota))
	case KindLimitRanges:
		limitRange, err := s.limitRanges.LimitRanges(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return s.withEvents(limitRangeDetail(*limitRange))
	case KindPriorityClasses:
		priorityClass, err := s.priorityClasses.Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return priorityClassDetail(*priorityClass)
	case KindRuntimeClasses:
		runtimeClass, err := s.runtimeClasses.Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return runtimeClassDetail(*runtimeClass)
	case KindLeases:
		lease, err := s.leases.Leases(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return leaseDetail(*lease)
	case KindMutatingWebhookConfigurations:
		config, err := s.mutatingWebhookConfigurations.Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return mutatingWebhookConfigurationDetail(*config)
	case KindValidatingWebhookConfigurations:
		config, err := s.validatingWebhookConfigurations.Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return validatingWebhookConfigurationDetail(*config)
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
	case KindEvents:
		event, err := s.events.Events(namespace).Get(name)
		if err != nil {
			detail.Error = err.Error()
			return detail
		}
		return eventDetail(*event)
	default:
		detail.Error = "Unsupported resource kind."
		return detail
	}
}

func usageGraphs(graphs ...UsageGraph) []UsageGraph {
	out := make([]UsageGraph, 0, len(graphs))
	for _, graph := range graphs {
		if graph.Value != "" && graph.Value != "-" {
			out = append(out, graph)
		}
	}
	return out
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
		ownerSection(pod.Namespace, pod.OwnerReferences),
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
		ownerSection(statefulSet.Namespace, statefulSet.OwnerReferences),
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
	detail.Sections = append(detail.Sections, ownerSection(namespace.Namespace, namespace.OwnerReferences))
	return compactDetail(detail)
}

func replicaSetDetail(replicaSet appsv1.ReplicaSet) ResourceDetail {
	yamlReplicaSet := replicaSet
	yamlReplicaSet.TypeMeta = metav1.TypeMeta{APIVersion: "apps/v1", Kind: "ReplicaSet"}
	yamlReplicaSet.ManagedFields = nil
	row := replicaSetRow(replicaSet)
	detail := detailBase(KindReplicaSets, "ReplicaSets", replicaSet.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlReplicaSet)
	detail.Fields = detailFields("Namespace", replicaSet.Namespace, "Desired", fmt.Sprint(valueOrZero(replicaSet.Spec.Replicas)), "Current", fmt.Sprint(replicaSet.Status.Replicas), "Ready", fmt.Sprint(replicaSet.Status.ReadyReplicas), "Available", fmt.Sprint(replicaSet.Status.AvailableReplicas))
	detail.Sections = append(detail.Sections, DetailSection{Title: "Selector", Fields: selectorFields(replicaSet.Spec.Selector)}, ownerSection(replicaSet.Namespace, replicaSet.OwnerReferences))
	return compactDetail(detail)
}

func jobDetail(job batchv1.Job) ResourceDetail {
	yamlJob := job
	yamlJob.TypeMeta = metav1.TypeMeta{APIVersion: "batch/v1", Kind: "Job"}
	yamlJob.ManagedFields = nil
	row := jobRow(job)
	detail := detailBase(KindJobs, "Jobs", job.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlJob)
	detail.Fields = detailFields("Namespace", job.Namespace, "Succeeded", fmt.Sprint(job.Status.Succeeded), "Failed", fmt.Sprint(job.Status.Failed), "Active", fmt.Sprint(job.Status.Active), "Parallelism", fmt.Sprint(valueOrZero(job.Spec.Parallelism)), "Completions", fmt.Sprint(valueOrZero(job.Spec.Completions)))
	detail.Sections = append(detail.Sections, DetailSection{Title: "Selector", Fields: selectorFields(job.Spec.Selector)}, ownerSection(job.Namespace, job.OwnerReferences))
	return compactDetail(detail)
}

func cronJobDetail(cronJob batchv1.CronJob) ResourceDetail {
	yamlCronJob := cronJob
	yamlCronJob.TypeMeta = metav1.TypeMeta{APIVersion: "batch/v1", Kind: "CronJob"}
	yamlCronJob.ManagedFields = nil
	row := cronJobRow(cronJob)
	lastSchedule := ""
	if cronJob.Status.LastScheduleTime != nil {
		lastSchedule = formatDetailTime(cronJob.Status.LastScheduleTime.Time)
	}
	detail := detailBase(KindCronJobs, "CronJobs", cronJob.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlCronJob)
	detail.Fields = detailFields("Namespace", cronJob.Namespace, "Schedule", cronJob.Spec.Schedule, "Suspend", fmt.Sprint(cronJob.Spec.Suspend != nil && *cronJob.Spec.Suspend), "Active Jobs", fmt.Sprint(len(cronJob.Status.Active)), "Last Schedule", lastSchedule)
	detail.Sections = append(detail.Sections, ownerSection(cronJob.Namespace, cronJob.OwnerReferences))
	return compactDetail(detail)
}

func persistentVolumeClaimDetail(pvc corev1.PersistentVolumeClaim) ResourceDetail {
	yamlPVC := pvc
	yamlPVC.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolumeClaim"}
	yamlPVC.ManagedFields = nil
	row := persistentVolumeClaimRow(pvc)
	className := ""
	if pvc.Spec.StorageClassName != nil {
		className = *pvc.Spec.StorageClassName
	}
	detail := detailBase(KindPersistentVolumeClaims, "PersistentVolumeClaims", pvc.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlPVC)
	detail.Fields = detailFields("Namespace", pvc.Namespace, "Phase", string(pvc.Status.Phase), "Volume", pvc.Spec.VolumeName, "StorageClass", className, "Access Modes", accessModes(pvc.Spec.AccessModes))
	detail.Sections = append(detail.Sections, DetailSection{Title: "Capacity", Fields: resourceListFields(pvc.Status.Capacity)}, DetailSection{Title: "Requested", Fields: resourceListFields(pvc.Spec.Resources.Requests)}, ownerSection(pvc.Namespace, pvc.OwnerReferences))
	return compactDetail(detail)
}

func persistentVolumeDetail(pv corev1.PersistentVolume) ResourceDetail {
	yamlPV := pv
	yamlPV.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolume"}
	yamlPV.ManagedFields = nil
	row := persistentVolumeRow(pv)
	claim := ""
	if pv.Spec.ClaimRef != nil {
		claim = namespacedName(pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
	}
	detail := detailBase(KindPersistentVolumes, "PersistentVolumes", pv.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlPV)
	detail.Fields = detailFields("Phase", string(pv.Status.Phase), "StorageClass", pv.Spec.StorageClassName, "Access Modes", accessModes(pv.Spec.AccessModes), "Reclaim Policy", string(pv.Spec.PersistentVolumeReclaimPolicy), "Claim", claim)
	detail.Sections = append(detail.Sections, DetailSection{Title: "Capacity", Fields: resourceListFields(pv.Spec.Capacity)}, ownerSection(pv.Namespace, pv.OwnerReferences))
	return compactDetail(detail)
}

func storageClassDetail(storageClass storagev1.StorageClass) ResourceDetail {
	yamlStorageClass := storageClass
	yamlStorageClass.TypeMeta = metav1.TypeMeta{APIVersion: "storage.k8s.io/v1", Kind: "StorageClass"}
	yamlStorageClass.ManagedFields = nil
	row := storageClassRow(storageClass)
	detail := detailBase(KindStorageClasses, "StorageClasses", storageClass.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlStorageClass)
	detail.Fields = detailFields("Provisioner", storageClass.Provisioner, "Default", fmt.Sprint(isDefaultStorageClass(storageClass.Annotations)), "Parameters", fmt.Sprint(len(storageClass.Parameters)))
	detail.Sections = append(detail.Sections, DetailSection{Title: "Parameters", Fields: mapFields(storageClass.Parameters)})
	return compactDetail(detail)
}

func endpointsDetail(endpoint corev1.Endpoints) ResourceDetail {
	yamlEndpoint := endpoint
	yamlEndpoint.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Endpoints"}
	yamlEndpoint.ManagedFields = nil
	row := endpointsRow(endpoint)
	detail := detailBase(KindEndpoints, "Endpoints", endpoint.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlEndpoint)
	detail.Fields = detailFields("Namespace", endpoint.Namespace, "Addresses", row.Cells[2].Value, "Not Ready", row.Cells[3].Value, "Ports", row.Cells[4].Value)
	detail.Sections = append(detail.Sections, ownerSection(endpoint.Namespace, endpoint.OwnerReferences))
	return compactDetail(detail)
}

func endpointSliceDetail(endpointSlice discoveryv1.EndpointSlice) ResourceDetail {
	yamlEndpointSlice := endpointSlice
	yamlEndpointSlice.TypeMeta = metav1.TypeMeta{APIVersion: "discovery.k8s.io/v1", Kind: "EndpointSlice"}
	yamlEndpointSlice.ManagedFields = nil
	row := endpointSliceRow(endpointSlice)
	detail := detailBase(KindEndpointSlices, "EndpointSlices", endpointSlice.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlEndpointSlice)
	detail.Fields = detailFields("Namespace", endpointSlice.Namespace, "Address Type", string(endpointSlice.AddressType), "Endpoints", fmt.Sprint(len(endpointSlice.Endpoints)), "Ports", row.Cells[4].Value)
	detail.Sections = append(detail.Sections, ownerSection(endpointSlice.Namespace, endpointSlice.OwnerReferences))
	return compactDetail(detail)
}

func ingressClassDetail(ingressClass networkingv1.IngressClass) ResourceDetail {
	yamlIngressClass := ingressClass
	yamlIngressClass.TypeMeta = metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "IngressClass"}
	yamlIngressClass.ManagedFields = nil
	row := ingressClassRow(ingressClass)
	detail := detailBase(KindIngressClasses, "IngressClasses", ingressClass.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlIngressClass)
	detail.Fields = detailFields("Controller", ingressClass.Spec.Controller, "Default", row.Cells[2].Value)
	return compactDetail(detail)
}

func networkPolicyDetail(policy networkingv1.NetworkPolicy) ResourceDetail {
	yamlPolicy := policy
	yamlPolicy.TypeMeta = metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "NetworkPolicy"}
	yamlPolicy.ManagedFields = nil
	row := networkPolicyRow(policy)
	detail := detailBase(KindNetworkPolicies, "NetworkPolicies", policy.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlPolicy)
	detail.Fields = detailFields("Namespace", policy.Namespace, "Pod Selector", labelSelectorString(&policy.Spec.PodSelector), "Policy Types", networkPolicyTypes(policy.Spec.PolicyTypes), "Ingress Rules", fmt.Sprint(len(policy.Spec.Ingress)), "Egress Rules", fmt.Sprint(len(policy.Spec.Egress)))
	detail.Sections = append(detail.Sections, ownerSection(policy.Namespace, policy.OwnerReferences))
	return compactDetail(detail)
}

func serviceAccountDetail(account corev1.ServiceAccount) ResourceDetail {
	yamlAccount := account
	yamlAccount.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "ServiceAccount"}
	yamlAccount.ManagedFields = nil
	row := serviceAccountRow(account)
	detail := detailBase(KindServiceAccounts, "ServiceAccounts", account.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlAccount)
	detail.Fields = detailFields("Namespace", account.Namespace, "Secrets", fmt.Sprint(len(account.Secrets)), "Image Pull Secrets", fmt.Sprint(len(account.ImagePullSecrets)), "Automount Token", boolPtrValue(account.AutomountServiceAccountToken))
	detail.Sections = append(detail.Sections, ownerSection(account.Namespace, account.OwnerReferences))
	return compactDetail(detail)
}

func roleDetail(role rbacv1.Role) ResourceDetail {
	yamlRole := role
	yamlRole.TypeMeta = metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role"}
	yamlRole.ManagedFields = nil
	row := roleRow(role)
	detail := detailBase(KindRoles, "Roles", role.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlRole)
	detail.Fields = detailFields("Namespace", role.Namespace, "Rules", fmt.Sprint(len(role.Rules)))
	return compactDetail(detail)
}

func roleBindingDetail(binding rbacv1.RoleBinding) ResourceDetail {
	yamlBinding := binding
	yamlBinding.TypeMeta = metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding"}
	yamlBinding.ManagedFields = nil
	row := roleBindingRow(binding)
	detail := detailBase(KindRoleBindings, "RoleBindings", binding.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlBinding)
	detail.Fields = detailFields("Namespace", binding.Namespace, "Role", binding.RoleRef.Kind+"/"+binding.RoleRef.Name, "Subjects", subjectsValue(binding.Subjects))
	return compactDetail(detail)
}

func clusterRoleDetail(role rbacv1.ClusterRole) ResourceDetail {
	yamlRole := role
	yamlRole.TypeMeta = metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole"}
	yamlRole.ManagedFields = nil
	row := clusterRoleRow(role)
	detail := detailBase(KindClusterRoles, "ClusterRoles", role.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlRole)
	detail.Fields = detailFields("Rules", fmt.Sprint(len(role.Rules)))
	return compactDetail(detail)
}

func clusterRoleBindingDetail(binding rbacv1.ClusterRoleBinding) ResourceDetail {
	yamlBinding := binding
	yamlBinding.TypeMeta = metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRoleBinding"}
	yamlBinding.ManagedFields = nil
	row := clusterRoleBindingRow(binding)
	detail := detailBase(KindClusterRoleBindings, "ClusterRoleBindings", binding.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlBinding)
	detail.Fields = detailFields("Role", binding.RoleRef.Kind+"/"+binding.RoleRef.Name, "Subjects", subjectsValue(binding.Subjects))
	return compactDetail(detail)
}

func configMapDetail(configMap corev1.ConfigMap) ResourceDetail {
	yamlConfigMap := configMap
	yamlConfigMap.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"}
	yamlConfigMap.ManagedFields = nil
	row := configMapRow(configMap)
	detail := detailBase(KindConfigMaps, "ConfigMaps", configMap.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlConfigMap)
	detail.Fields = detailFields("Namespace", configMap.Namespace, "Data", fmt.Sprint(len(configMap.Data)), "Binary Data", fmt.Sprint(len(configMap.BinaryData)))
	detail.Sections = append(detail.Sections, ownerSection(configMap.Namespace, configMap.OwnerReferences))
	return compactDetail(detail)
}

func secretDetail(secret corev1.Secret) ResourceDetail {
	yamlSecret := secret
	yamlSecret.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}
	yamlSecret.ManagedFields = nil
	yamlSecret.Data = nil
	yamlSecret.StringData = nil
	row := secretRow(secret)
	detail := detailBase(KindSecrets, "Secrets", secret.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlSecret)
	detail.Fields = detailFields("Namespace", secret.Namespace, "Type", string(secret.Type), "Data Keys", fmt.Sprint(len(secret.Data)))
	detail.Sections = append(detail.Sections, ownerSection(secret.Namespace, secret.OwnerReferences))
	return compactDetail(detail)
}

func horizontalPodAutoscalerDetail(hpa autoscalingv2.HorizontalPodAutoscaler) ResourceDetail {
	yamlHPA := hpa
	yamlHPA.TypeMeta = metav1.TypeMeta{APIVersion: "autoscaling/v2", Kind: "HorizontalPodAutoscaler"}
	yamlHPA.ManagedFields = nil
	row := horizontalPodAutoscalerRow(hpa)
	detail := detailBase(KindHorizontalPodAutoscalers, "HPAs", hpa.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlHPA)
	detail.Fields = detailFields("Namespace", hpa.Namespace, "Reference", hpa.Spec.ScaleTargetRef.Kind+"/"+hpa.Spec.ScaleTargetRef.Name, "Min", row.Cells[3].Value, "Max", row.Cells[4].Value, "Current Replicas", fmt.Sprint(hpa.Status.CurrentReplicas))
	detail.Sections = append(detail.Sections, ownerSection(hpa.Namespace, hpa.OwnerReferences))
	return compactDetail(detail)
}

func podDisruptionBudgetDetail(pdb policyv1.PodDisruptionBudget) ResourceDetail {
	yamlPDB := pdb
	yamlPDB.TypeMeta = metav1.TypeMeta{APIVersion: "policy/v1", Kind: "PodDisruptionBudget"}
	yamlPDB.ManagedFields = nil
	row := podDisruptionBudgetRow(pdb)
	detail := detailBase(KindPodDisruptionBudgets, "PodDisruptionBudgets", pdb.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlPDB)
	detail.Fields = detailFields("Namespace", pdb.Namespace, "Min Available", intOrStringValue(pdb.Spec.MinAvailable), "Max Unavailable", intOrStringValue(pdb.Spec.MaxUnavailable), "Disruptions Allowed", fmt.Sprint(pdb.Status.DisruptionsAllowed), "Current Healthy", fmt.Sprint(pdb.Status.CurrentHealthy), "Desired Healthy", fmt.Sprint(pdb.Status.DesiredHealthy))
	detail.Sections = append(detail.Sections, DetailSection{Title: "Selector", Fields: selectorFields(pdb.Spec.Selector)}, ownerSection(pdb.Namespace, pdb.OwnerReferences))
	return compactDetail(detail)
}

func resourceQuotaDetail(quota corev1.ResourceQuota) ResourceDetail {
	yamlQuota := quota
	yamlQuota.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "ResourceQuota"}
	yamlQuota.ManagedFields = nil
	row := resourceQuotaRow(quota)
	detail := detailBase(KindResourceQuotas, "ResourceQuotas", quota.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlQuota)
	detail.Fields = detailFields("Namespace", quota.Namespace, "Hard", fmt.Sprint(len(quota.Status.Hard)), "Used", fmt.Sprint(len(quota.Status.Used)))
	detail.Sections = append(detail.Sections, DetailSection{Title: "Hard", Fields: resourceListFields(quota.Status.Hard)}, DetailSection{Title: "Used", Fields: resourceListFields(quota.Status.Used)}, ownerSection(quota.Namespace, quota.OwnerReferences))
	return compactDetail(detail)
}

func limitRangeDetail(limitRange corev1.LimitRange) ResourceDetail {
	yamlLimitRange := limitRange
	yamlLimitRange.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "LimitRange"}
	yamlLimitRange.ManagedFields = nil
	row := limitRangeRow(limitRange)
	detail := detailBase(KindLimitRanges, "LimitRanges", limitRange.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlLimitRange)
	detail.Fields = detailFields("Namespace", limitRange.Namespace, "Limits", fmt.Sprint(len(limitRange.Spec.Limits)))
	detail.Sections = append(detail.Sections, ownerSection(limitRange.Namespace, limitRange.OwnerReferences))
	return compactDetail(detail)
}

func priorityClassDetail(priorityClass schedulingv1.PriorityClass) ResourceDetail {
	yamlPriorityClass := priorityClass
	yamlPriorityClass.TypeMeta = metav1.TypeMeta{APIVersion: "scheduling.k8s.io/v1", Kind: "PriorityClass"}
	yamlPriorityClass.ManagedFields = nil
	row := priorityClassRow(priorityClass)
	detail := detailBase(KindPriorityClasses, "PriorityClasses", priorityClass.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlPriorityClass)
	detail.Fields = detailFields("Value", fmt.Sprint(priorityClass.Value), "Global Default", fmt.Sprint(priorityClass.GlobalDefault), "Preemption", preemptionPolicyValue(priorityClass.PreemptionPolicy), "Description", priorityClass.Description)
	return compactDetail(detail)
}

func runtimeClassDetail(runtimeClass nodev1.RuntimeClass) ResourceDetail {
	yamlRuntimeClass := runtimeClass
	yamlRuntimeClass.TypeMeta = metav1.TypeMeta{APIVersion: "node.k8s.io/v1", Kind: "RuntimeClass"}
	yamlRuntimeClass.ManagedFields = nil
	row := runtimeClassRow(runtimeClass)
	detail := detailBase(KindRuntimeClasses, "RuntimeClasses", runtimeClass.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlRuntimeClass)
	detail.Fields = detailFields("Handler", runtimeClass.Handler)
	return compactDetail(detail)
}

func leaseDetail(lease coordinationv1.Lease) ResourceDetail {
	yamlLease := lease
	yamlLease.TypeMeta = metav1.TypeMeta{APIVersion: "coordination.k8s.io/v1", Kind: "Lease"}
	yamlLease.ManagedFields = nil
	row := leaseRow(lease)
	detail := detailBase(KindLeases, "Leases", lease.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlLease)
	detail.Fields = detailFields("Namespace", lease.Namespace, "Holder", row.Cells[2].Value, "Renew Time", row.Cells[3].Value)
	return compactDetail(detail)
}

func mutatingWebhookConfigurationDetail(config admissionv1.MutatingWebhookConfiguration) ResourceDetail {
	yamlConfig := config
	yamlConfig.TypeMeta = metav1.TypeMeta{APIVersion: "admissionregistration.k8s.io/v1", Kind: "MutatingWebhookConfiguration"}
	yamlConfig.ManagedFields = nil
	row := mutatingWebhookConfigurationRow(config)
	detail := detailBase(KindMutatingWebhookConfigurations, "MutatingWebhookConfigurations", config.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlConfig)
	detail.Fields = detailFields("Webhooks", fmt.Sprint(len(config.Webhooks)))
	return compactDetail(detail)
}

func validatingWebhookConfigurationDetail(config admissionv1.ValidatingWebhookConfiguration) ResourceDetail {
	yamlConfig := config
	yamlConfig.TypeMeta = metav1.TypeMeta{APIVersion: "admissionregistration.k8s.io/v1", Kind: "ValidatingWebhookConfiguration"}
	yamlConfig.ManagedFields = nil
	row := validatingWebhookConfigurationRow(config)
	detail := detailBase(KindValidatingWebhookConfigurations, "ValidatingWebhookConfigurations", config.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlConfig)
	detail.Fields = detailFields("Webhooks", fmt.Sprint(len(config.Webhooks)))
	return compactDetail(detail)
}

func eventDetail(event corev1.Event) ResourceDetail {
	yamlEvent := event
	yamlEvent.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Event"}
	yamlEvent.ManagedFields = nil
	row := eventRow(event)
	detail := detailBase(KindEvents, "Events", event.ObjectMeta, row.Status, row.StatusKey)
	detail.YAML = resourceYAML(yamlEvent)
	detail.Fields = detailFields("Namespace", event.Namespace, "Type", event.Type, "Reason", event.Reason, "Object", row.Cells[4].Value, "Count", fmt.Sprint(event.Count), "Message", event.Message)
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
	events := make([]ResourceEvent, 0)
	for _, event := range s.listEvents() {
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
	timestamps := []time.Time{}
	if !event.EventTime.IsZero() {
		timestamps = append(timestamps, event.EventTime.Time)
	}
	if !event.FirstTimestamp.IsZero() {
		timestamps = append(timestamps, event.FirstTimestamp.Time)
	}
	if !event.LastTimestamp.IsZero() {
		timestamps = append(timestamps, event.LastTimestamp.Time)
	}
	if event.Series != nil && !event.Series.LastObservedTime.IsZero() {
		timestamps = append(timestamps, event.Series.LastObservedTime.Time)
	}
	if !event.CreationTimestamp.IsZero() {
		timestamps = append(timestamps, event.CreationTimestamp.Time)
	}
	latest := time.Time{}
	for _, timestamp := range timestamps {
		if timestamp.After(latest) {
			latest = timestamp
		}
	}
	return latest
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
	case KindReplicaSets:
		return "ReplicaSet"
	case KindJobs:
		return "Job"
	case KindCronJobs:
		return "CronJob"
	case KindPersistentVolumeClaims:
		return "PersistentVolumeClaim"
	case KindPersistentVolumes:
		return "PersistentVolume"
	case KindStorageClasses:
		return "StorageClass"
	case KindServices:
		return "Service"
	case KindEndpoints:
		return "Endpoints"
	case KindEndpointSlices:
		return "EndpointSlice"
	case KindIngresses:
		return "Ingress"
	case KindIngressClasses:
		return "IngressClass"
	case KindNetworkPolicies:
		return "NetworkPolicy"
	case KindServiceAccounts:
		return "ServiceAccount"
	case KindRoles:
		return "Role"
	case KindRoleBindings:
		return "RoleBinding"
	case KindClusterRoles:
		return "ClusterRole"
	case KindClusterRoleBindings:
		return "ClusterRoleBinding"
	case KindConfigMaps:
		return "ConfigMap"
	case KindSecrets:
		return "Secret"
	case KindHorizontalPodAutoscalers:
		return "HorizontalPodAutoscaler"
	case KindPodDisruptionBudgets:
		return "PodDisruptionBudget"
	case KindResourceQuotas:
		return "ResourceQuota"
	case KindLimitRanges:
		return "LimitRange"
	case KindPriorityClasses:
		return "PriorityClass"
	case KindRuntimeClasses:
		return "RuntimeClass"
	case KindLeases:
		return "Lease"
	case KindMutatingWebhookConfigurations:
		return "MutatingWebhookConfiguration"
	case KindValidatingWebhookConfigurations:
		return "ValidatingWebhookConfiguration"
	case KindNodes:
		return "Node"
	case KindNamespaces:
		return "Namespace"
	case KindEvents:
		return "Event"
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

func valueOrZero(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}

func boolPtrValue(value *bool) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(*value)
}

func formatDetailTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
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

func ownerSection(namespace string, owners []metav1.OwnerReference) DetailSection {
	fields := make([]DetailField, 0, len(owners))
	for _, owner := range owners {
		field := DetailField{Name: owner.Kind, Value: owner.Name}
		if kind, ok := ownerResourceKind(owner.Kind); ok {
			linkNamespace := namespace
			if resourceDef(kind).Scope == "cluster" {
				linkNamespace = ""
			}
			field.Link = &DetailLink{Resource: kind, Namespace: linkNamespace, Name: owner.Name}
		}
		fields = append(fields, field)
	}
	return DetailSection{Title: "Owners", Fields: fields}
}

func ownerResourceKind(kind string) (ResourceKind, bool) {
	switch kind {
	case "Pod":
		return KindPods, true
	case "Deployment":
		return KindDeployments, true
	case "StatefulSet":
		return KindStatefulSet, true
	case "DaemonSet":
		return KindDaemonSet, true
	case "ReplicaSet":
		return KindReplicaSets, true
	case "Job":
		return KindJobs, true
	case "CronJob":
		return KindCronJobs, true
	case "PersistentVolumeClaim":
		return KindPersistentVolumeClaims, true
	case "PersistentVolume":
		return KindPersistentVolumes, true
	case "StorageClass":
		return KindStorageClasses, true
	case "Service":
		return KindServices, true
	case "Endpoints":
		return KindEndpoints, true
	case "EndpointSlice":
		return KindEndpointSlices, true
	case "Ingress":
		return KindIngresses, true
	case "IngressClass":
		return KindIngressClasses, true
	case "NetworkPolicy":
		return KindNetworkPolicies, true
	case "ServiceAccount":
		return KindServiceAccounts, true
	case "Role":
		return KindRoles, true
	case "RoleBinding":
		return KindRoleBindings, true
	case "ClusterRole":
		return KindClusterRoles, true
	case "ClusterRoleBinding":
		return KindClusterRoleBindings, true
	case "ConfigMap":
		return KindConfigMaps, true
	case "Secret":
		return KindSecrets, true
	case "HorizontalPodAutoscaler":
		return KindHorizontalPodAutoscalers, true
	case "PodDisruptionBudget":
		return KindPodDisruptionBudgets, true
	case "ResourceQuota":
		return KindResourceQuotas, true
	case "LimitRange":
		return KindLimitRanges, true
	case "PriorityClass":
		return KindPriorityClasses, true
	case "RuntimeClass":
		return KindRuntimeClasses, true
	case "Lease":
		return KindLeases, true
	case "MutatingWebhookConfiguration":
		return KindMutatingWebhookConfigurations, true
	case "ValidatingWebhookConfiguration":
		return KindValidatingWebhookConfigurations, true
	case "Node":
		return KindNodes, true
	case "Namespace":
		return KindNamespaces, true
	case "Event":
		return KindEvents, true
	default:
		return "", false
	}
}
