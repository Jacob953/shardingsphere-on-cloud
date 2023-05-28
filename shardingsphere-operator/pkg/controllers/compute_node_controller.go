/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/api/v1alpha1"
	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/pkg/kubernetes/configmap"
	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/pkg/kubernetes/deployment"
	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/pkg/kubernetes/service"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	computeNodeControllerName = "compute-node-controller"
	defaultRequeueTime        = 10 * time.Second
)

// ComputeNodeReconciler is a controller for the compute node
type ComputeNodeReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger

	Deployment deployment.Deployment
	Service    service.Service
	ConfigMap  configmap.ConfigMap
}

// SetupWithManager sets up the controller with the Manager
func (r *ComputeNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ComputeNode{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}

// Reconcile handles main function of this controller
func (r *ComputeNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues(computeNodeControllerName, req.NamespacedName)

	cn := &v1alpha1.ComputeNode{}
	if err := r.Get(ctx, req.NamespacedName, cn); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{RequeueAfter: defaultRequeueTime}, nil
		}

		logger.Error(err, "Failed to get the compute node")
		return ctrl.Result{Requeue: true}, err
	}

	if err := r.reconcileStatus(ctx, cn); err != nil {
		logger.Error(err, "Failed to reconcile status")
	}

	errors := []error{}
	if err := r.reconcileDeployment(ctx, cn); err != nil {
		logger.Error(err, "Failed to reconcile deployement")
		errors = append(errors, err)
	}
	if err := r.reconcileService(ctx, cn); err != nil {
		logger.Error(err, "Failed to reconcile service")
		errors = append(errors, err)
	}
	if err := r.reconcileConfigMap(ctx, cn); err != nil {
		logger.Error(err, "Failed to reconcile configmap")
		errors = append(errors, err)
	}

	if len(errors) != 0 {
		return ctrl.Result{Requeue: true}, errors[0]
	}

	return ctrl.Result{RequeueAfter: defaultRequeueTime}, nil
}

func (r *ComputeNodeReconciler) reconcileDeployment(ctx context.Context, cn *v1alpha1.ComputeNode) error {
	deploy, err := r.getDeploymentByNamespacedName(ctx, types.NamespacedName{Namespace: cn.Namespace, Name: cn.Name})
	if err != nil {
		return err
	}
	if deploy != nil {
		return r.updateDeployment(ctx, cn, deploy)
	}
	return r.createDeployment(ctx, cn)
}

func (r *ComputeNodeReconciler) createDeployment(ctx context.Context, cn *v1alpha1.ComputeNode) error {
	deploy := r.Deployment.Build(ctx, cn)
	err := r.Deployment.Create(ctx, deploy)
	if err != nil && apierrors.IsAlreadyExists(err) || err == nil {
		return nil
	}
	return err
}

func (r *ComputeNodeReconciler) updateDeployment(ctx context.Context, cn *v1alpha1.ComputeNode, deploy *appsv1.Deployment) error {
	exp := r.Deployment.Build(ctx, cn)
	exp.ObjectMeta = deploy.ObjectMeta
	exp.ObjectMeta.ResourceVersion = ""
	exp.Labels = deploy.Labels
	exp.Annotations = deploy.Annotations
	return r.Deployment.Update(ctx, exp)
}

func (r *ComputeNodeReconciler) getDeploymentByNamespacedName(ctx context.Context, namespacedName types.NamespacedName) (*appsv1.Deployment, error) {
	dp, err := r.Deployment.GetByNamespacedName(ctx, namespacedName)
	if err != nil {
		return nil, err
	}
	return dp, nil
}

func (r *ComputeNodeReconciler) reconcileService(ctx context.Context, cn *v1alpha1.ComputeNode) error {
	svc, err := r.getServiceByNamespacedName(ctx, types.NamespacedName{Namespace: cn.Namespace, Name: cn.Name})
	if err != nil {
		return err
	}
	if svc != nil {
		return r.updateService(ctx, cn, svc)
	}
	return r.createService(ctx, cn)
}

func (r *ComputeNodeReconciler) createService(ctx context.Context, cn *v1alpha1.ComputeNode) error {
	svc := r.Service.Build(ctx, cn)
	err := r.Service.Create(ctx, svc)
	if err != nil && apierrors.IsAlreadyExists(err) || err == nil {
		return nil
	}
	return err
}

func (r *ComputeNodeReconciler) updateService(ctx context.Context, cn *v1alpha1.ComputeNode, cur *corev1.Service) error {
	switch cn.Spec.ServiceType {
	case corev1.ServiceTypeClusterIP:
		updateServiceClusterIP(cn.Spec.PortBindings)
		if err := r.Update(ctx, cn); err != nil {
			return err
		}
	case corev1.ServiceTypeExternalName:
		fallthrough
	case corev1.ServiceTypeLoadBalancer:
		fallthrough
	case corev1.ServiceTypeNodePort:
		updateServiceNodePort(cn.Spec.PortBindings, cur.Spec.Ports)
		if err := r.Update(ctx, cn); err != nil {
			return err
		}
	}

	exp := r.Service.Build(ctx, cn)
	exp.ObjectMeta = cur.ObjectMeta
	exp.Spec.ClusterIP = cur.Spec.ClusterIP
	exp.Spec.ClusterIPs = cur.Spec.ClusterIPs
	if cn.Spec.ServiceType == corev1.ServiceTypeNodePort {
		exp.Spec.Ports = updateNodePorts(cn.Spec.PortBindings, cur.Spec.Ports)
	}
	return r.Update(ctx, exp)
}

func updateServiceNodePort(portBindings []v1alpha1.PortBinding, svcports []corev1.ServicePort) {
	for idx := range svcports {
		for i := range portBindings {
			if svcports[idx].Name == portBindings[i].Name {
				if portBindings[i].NodePort == 0 {
					portBindings[i].NodePort = svcports[idx].NodePort
				}
				break
			}
		}
	}
}

func updateServiceClusterIP(portBindings []v1alpha1.PortBinding) {
	for idx := range portBindings {
		if portBindings[idx].NodePort != 0 {
			portBindings[idx].NodePort = 0
			break
		}
	}
}

func updateNodePorts(portbindings []v1alpha1.PortBinding, svcports []corev1.ServicePort) []corev1.ServicePort {
	ports := []corev1.ServicePort{}
	for pb := range portbindings {
		for sp := range svcports {
			if portbindings[pb].Name == svcports[sp].Name {
				port := corev1.ServicePort{
					Name:       portbindings[pb].Name,
					TargetPort: intstr.FromInt(int(portbindings[pb].ContainerPort)),
					Port:       portbindings[pb].ServicePort,
					Protocol:   portbindings[pb].Protocol,
				}
				if svcports[sp].NodePort != 0 {
					port.NodePort = svcports[sp].NodePort
				}
				ports = append(ports, port)
			}
		}
	}
	return ports
}

func (r *ComputeNodeReconciler) getServiceByNamespacedName(ctx context.Context, namespacedName types.NamespacedName) (*corev1.Service, error) {
	svc, err := r.Service.GetByNamespacedName(ctx, namespacedName)
	if err != nil {
		return nil, err
	}
	return svc, nil
}

func (r *ComputeNodeReconciler) createConfigMap(ctx context.Context, cn *v1alpha1.ComputeNode) error {
	cm := r.ConfigMap.Build(ctx, cn)
	err := r.ConfigMap.Create(ctx, cm)
	if err != nil && apierrors.IsAlreadyExists(err) || err == nil {
		return nil
	}
	return err
}

func (r *ComputeNodeReconciler) updateConfigMap(ctx context.Context, cn *v1alpha1.ComputeNode, cm *corev1.ConfigMap) error {
	exp := r.ConfigMap.Build(ctx, cn)
	exp.ObjectMeta = cm.ObjectMeta
	exp.ObjectMeta.ResourceVersion = ""
	exp.Labels = cm.Labels
	exp.Annotations = cm.Annotations
	return r.ConfigMap.Update(ctx, exp)
}

func (r *ComputeNodeReconciler) getConfigMapByNamespacedName(ctx context.Context, namespacedName types.NamespacedName) (*corev1.ConfigMap, error) {
	cm, err := r.ConfigMap.GetByNamespacedName(ctx, namespacedName)
	if err != nil {
		return nil, err
	}
	return cm, nil
}

func (r *ComputeNodeReconciler) reconcileConfigMap(ctx context.Context, cn *v1alpha1.ComputeNode) error {
	cm, err := r.getConfigMapByNamespacedName(ctx, types.NamespacedName{Namespace: cn.Namespace, Name: cn.Name})
	if err != nil {
		return err
	}
	if cm != nil {
		return r.updateConfigMap(ctx, cn, cm)
	}
	return r.createConfigMap(ctx, cn)
}

func (r *ComputeNodeReconciler) reconcileStatus(ctx context.Context, cn *v1alpha1.ComputeNode) error {
	podlist := &corev1.PodList{}
	if err := r.List(ctx, podlist, client.InNamespace(cn.Namespace), client.MatchingLabels(cn.Spec.Selector.MatchLabels)); err != nil {
		return err
	}

	service := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: cn.Namespace,
		Name:      cn.Name,
	}, service); err != nil {
		return err
	}

	rt, err := r.getRuntimeComputeNode(ctx, types.NamespacedName{
		Namespace: cn.Namespace,
		Name:      cn.Name,
	})
	if err != nil {
		return err
	}

	/*
		status := reconcileComputeNodeStatus(podlist, service)
		rt.Status = *status
	*/
	reconcileComputeNodeStatus(podlist, service, rt)
	fmt.Printf("status conditions: %#v\n", rt.Status.Conditions)

	// TODO: Compare Status with or without modification
	return r.Status().Update(ctx, rt)
}

func getReadyProxyInstances(podlist *corev1.PodList) int32 {
	var cnt int32

	findRunningPod := func(pod *corev1.Pod) {
		if pod.Status.Phase != corev1.PodRunning {
			return
		}

		if isTrueReadyPod(pod) {
			for j := range pod.Status.ContainerStatuses {
				if pod.Status.ContainerStatuses[j].Name == "shardingsphere-proxy" && pod.Status.ContainerStatuses[j].Ready {
					cnt++
				}
			}
		}
	}

	for idx := range podlist.Items {
		findRunningPod(&podlist.Items[idx])
	}
	return cnt
}

func isTrueReadyPod(pod *corev1.Pod) bool {
	for i := range pod.Status.Conditions {
		if pod.Status.Conditions[i].Type == corev1.PodReady && pod.Status.Conditions[i].Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func newConditions(conditions []v1alpha1.ComputeNodeCondition, cond *v1alpha1.ComputeNodeCondition) []v1alpha1.ComputeNodeCondition {
	if conditions == nil {
		conditions = []v1alpha1.ComputeNodeCondition{}
	}
	if cond.Type == "" {
		return conditions
	}

	found := false
	for idx := range conditions {
		if conditions[idx].Type != cond.Type {
			continue
		}
		conditions[idx].LastUpdateTime = cond.LastUpdateTime
		conditions[idx].Status = cond.Status
		found = true
		break
	}

	if !found {
		conditions = append(conditions, *cond)
	}

	return conditions
}

func updateReadyConditions(conditions []v1alpha1.ComputeNodeCondition, cond *v1alpha1.ComputeNodeCondition) []v1alpha1.ComputeNodeCondition {
	return newConditions(conditions, cond)
}

func updateNotReadyConditions(conditions []v1alpha1.ComputeNodeCondition, cond *v1alpha1.ComputeNodeCondition) []v1alpha1.ComputeNodeCondition {
	cur := newConditions(conditions, cond)

	for idx := range cur {
		if cur[idx].Type == v1alpha1.ComputeNodeConditionReady {
			cur[idx].LastUpdateTime = metav1.Now()
			cur[idx].Status = v1alpha1.ConditionStatusFalse
		}
	}

	return cur
}

func newConditionUnknown(reason, message string) v1alpha1.ComputeNodeCondition {
	return newCondition(v1alpha1.ComputeNodeConditionUnknown, reason, message)
}

func newConditionPending(reason, message string) v1alpha1.ComputeNodeCondition {
	return newCondition(v1alpha1.ComputeNodeConditionPending, reason, message)
}

func newConditionDeployed(reason, message string) v1alpha1.ComputeNodeCondition {
	return newCondition(v1alpha1.ComputeNodeConditionDeployed, reason, message)
}

func newConditionStarted(reason, message string) v1alpha1.ComputeNodeCondition {
	return newCondition(v1alpha1.ComputeNodeConditionStarted, reason, message)
}

func newConditionInitialized(reason, message string) v1alpha1.ComputeNodeCondition {
	return newCondition(v1alpha1.ComputeNodeConditionInitialized, reason, message)
}

func newConditionReady(reason, message string) v1alpha1.ComputeNodeCondition {
	return newCondition(v1alpha1.ComputeNodeConditionReady, reason, message)
}

func newConditionFailed(reason, message string) v1alpha1.ComputeNodeCondition {
	return newCondition(v1alpha1.ComputeNodeConditionFailed, reason, message)
}

func newCondition(t v1alpha1.ComputeNodeConditionType, reason, message string) v1alpha1.ComputeNodeCondition {
	return v1alpha1.ComputeNodeCondition{
		Type:               t,
		Status:             v1alpha1.ConditionStatusTrue,
		LastUpdateTime:     metav1.NewTime(time.Now()),
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             reason,
		Message:            message,
	}
}

func setConditionUnknown(conditions []v1alpha1.ComputeNodeCondition, reason, message string) {
	setCondition(conditions, v1alpha1.ComputeNodeConditionUnknown, reason, message, true)
}

func setConditionPending(conditions []v1alpha1.ComputeNodeCondition, reason, message string) {
	setCondition(conditions, v1alpha1.ComputeNodeConditionPending, reason, message, false)
}

func setConditionDeployed(conditions []v1alpha1.ComputeNodeCondition, reason, message string) {
	setCondition(conditions, v1alpha1.ComputeNodeConditionDeployed, reason, message, false)
}

func setConditionInitialized(conditions []v1alpha1.ComputeNodeCondition, reason, message string) {
	setCondition(conditions, v1alpha1.ComputeNodeConditionInitialized, reason, message, false)
}

func setConditionStarted(conditions []v1alpha1.ComputeNodeCondition, reason, message string) {
	setCondition(conditions, v1alpha1.ComputeNodeConditionStarted, reason, message, false)
}

func setConditionReady(conditions []v1alpha1.ComputeNodeCondition, reason, message string) {
	setCondition(conditions, v1alpha1.ComputeNodeConditionReady, reason, message, false)
}

func setConditionFailed(conditions []v1alpha1.ComputeNodeCondition, reason, message string) {
	setCondition(conditions, v1alpha1.ComputeNodeConditionFailed, reason, message, true)
}

func setCondition(conditions []v1alpha1.ComputeNodeCondition, t v1alpha1.ComputeNodeConditionType, reason, message string, exlusive bool) {
	cond := v1alpha1.ComputeNodeCondition{
		Type:               t,
		Status:             v1alpha1.ConditionStatusTrue,
		LastUpdateTime:     metav1.NewTime(time.Now()),
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             reason,
		Message:            message,
	}

	var found bool
	for i := range conditions {
		if conditions[i].Type == cond.Type {
			found = true
			conditions[i] = cond
		} else {
			if cond.Type != v1alpha1.ComputeNodeConditionUnknown {

			}
			if exlusive {
				conditions[i].LastUpdateTime = cond.LastUpdateTime
				conditions[i].Status = v1alpha1.ConditionStatusFalse
			}
		}
	}

	// check current conditions
	if len(conditions) == 0 || !found {
		conditions = append(conditions, cond)
	}
}

func getConditionFromPods(podlist *corev1.PodList) v1alpha1.ComputeNodeCondition {
	if len(podlist.Items) == 0 {
		return newConditionUnknown("PodNotFound", "No pod was found")
	}
	var cond v1alpha1.ComputeNodeCondition
	result := map[v1alpha1.ComputeNodeConditionType]int{}
	for _, p := range podlist.Items {
		pc := getPreferedConditionFromPod(p)
		result[pc.Type]++
	}

	if result[v1alpha1.ComputeNodeConditionUnknown] == len(podlist.Items) {
		return newConditionUnknown("PodUnknown", "All pods are unknown")
	}

	if result[v1alpha1.ComputeNodeConditionReady] > 0 {
		return newConditionReady("PodReady", "Some pods are ready")
	}

	if result[v1alpha1.ComputeNodeConditionStarted] > 0 {
		return newConditionStarted("PodStarted", "Some pods are started")
	}

	if result[v1alpha1.ComputeNodeConditionInitialized] > 0 {
		return newConditionInitialized("PodInitialized", "Some pods are initialized")
	}

	if result[v1alpha1.ComputeNodeConditionDeployed] > 0 {
		return newConditionDeployed("PodDeployed", "Some pods are deployed")
	}

	if result[v1alpha1.ComputeNodeConditionPending] > 0 {
		return newConditionPending("PodPending", "Some pods are pending")
	}

	if result[v1alpha1.ComputeNodeConditionFailed] > 0 {
		return newConditionFailed("PodFailed", "Some pods are failed")
	}

	return cond
}

func getPreferedConditionFromPod(pod corev1.Pod) v1alpha1.ComputeNodeCondition {
	if pod.Status.Phase == corev1.PodUnknown {
		return v1alpha1.ComputeNodeCondition{
			Type: v1alpha1.ComputeNodeConditionUnknown,
		}
	}

	if pod.Status.Phase == corev1.PodPending {
		return v1alpha1.ComputeNodeCondition{
			Type: v1alpha1.ComputeNodeConditionPending,
		}
	}

	if pod.Status.Phase == corev1.PodFailed {
		return v1alpha1.ComputeNodeCondition{
			Type: v1alpha1.ComputeNodeConditionFailed,
		}
	}

	return getPreferedConditionFromPodConditions(pod.Status.Conditions)
}

func getPreferedConditionFromPodConditions(conditions []corev1.PodCondition) v1alpha1.ComputeNodeCondition {
	var (
		sched       bool
		initialized bool
		conReady    bool
		ready       bool
	)

	for _, c := range conditions {
		if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionTrue {
			sched = true
		}
		if c.Type == corev1.PodInitialized && c.Status == corev1.ConditionTrue {
			initialized = true
		}
		if c.Type == corev1.ContainersReady && c.Status == corev1.ConditionTrue {
			conReady = true
		}
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			ready = true
		}
	}

	if ready {
		return v1alpha1.ComputeNodeCondition{
			Type: v1alpha1.ComputeNodeConditionReady,
		}
	}

	if conReady {
		return v1alpha1.ComputeNodeCondition{
			Type: v1alpha1.ComputeNodeConditionStarted,
		}
	}

	if initialized {
		return v1alpha1.ComputeNodeCondition{
			Type: v1alpha1.ComputeNodeConditionInitialized,
		}
	}

	if sched {
		return v1alpha1.ComputeNodeCondition{
			Type: v1alpha1.ComputeNodeConditionDeployed,
		}
	}

	return v1alpha1.ComputeNodeCondition{}
}

func updateComputeNodeStatusCondition(conditions []v1alpha1.ComputeNodeCondition, cond v1alpha1.ComputeNodeCondition) []v1alpha1.ComputeNodeCondition {
	var found bool
	for i := range conditions {
		if conditions[i].Type == cond.Type {
			found = true
			conditions[i] = cond
		} else {
			if cond.Type == v1alpha1.ComputeNodeConditionUnknown {
				conditions[i].LastUpdateTime = cond.LastUpdateTime
				conditions[i].Status = v1alpha1.ConditionStatusFalse
			} else {
				if conditions[i].Type == v1alpha1.ComputeNodeConditionUnknown {
					conditions[i].Status = v1alpha1.ConditionStatusFalse
				}
				conditions[i].LastUpdateTime = cond.LastUpdateTime
			}
		}
	}

	// check current conditions
	if len(conditions) == 0 || !found {
		conditions = append(conditions, cond)
	}

	return conditions
}

func reconcileComputeNodeStatus(podlist *corev1.PodList, svc *corev1.Service, cn *v1alpha1.ComputeNode) {
	cond := getConditionFromPods(podlist)
	cn.Status.Conditions = updateComputeNodeStatusCondition(cn.Status.Conditions, cond)

	ready := getReadyProxyInstances(podlist)
	cn.Status.Ready = fmt.Sprintf("%d/%d", ready, cn.Spec.Replicas)
	//TODO: consider removing this readyInstances
	cn.Status.ReadyInstances = ready

	if ready > 0 {
		cn.Status.Phase = v1alpha1.ComputeNodeStatusReady
	} else {
		cn.Status.Phase = v1alpha1.ComputeNodeStatusNotReady
	}

	cn.Status.LoadBalancer.ClusterIP = svc.Spec.ClusterIP
	cn.Status.LoadBalancer.Ingress = svc.Status.LoadBalancer.Ingress
}

func (r *ComputeNodeReconciler) getRuntimeComputeNode(ctx context.Context, namespacedName types.NamespacedName) (*v1alpha1.ComputeNode, error) {
	rt := &v1alpha1.ComputeNode{}
	err := r.Get(ctx, namespacedName, rt)
	return rt, err
}
