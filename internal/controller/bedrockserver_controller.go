/*
Copyright 2026 DataKnifeAI.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	bedrockv1alpha1 "github.com/DataKnifeAI/bedrock-operator/api/v1alpha1"
	"github.com/DataKnifeAI/bedrock-operator/internal/gamecfg"
	"github.com/DataKnifeAI/bedrock-operator/internal/provisioner"
	"github.com/DataKnifeAI/bedrock-operator/internal/version"
	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const finalizerName = "bedrock.dataknife.ai/finalizer"

// BedrockServerReconciler reconciles a BedrockServer object.
type BedrockServerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Versions *version.Client
}

// +kubebuilder:rbac:groups=bedrock.dataknife.ai,resources=bedrockservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=bedrock.dataknife.ai,resources=bedrockservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=bedrock.dataknife.ai,resources=bedrockservers/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=udproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.envoyproxy.io,resources=envoyproxies,verbs=get;list;watch;create;update;patch;delete

func (r *BedrockServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	server := &bedrockv1alpha1.BedrockServer{}
	if err := r.Get(ctx, req.NamespacedName, server); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !controllerutil.ContainsFinalizer(server, finalizerName) {
		controllerutil.AddFinalizer(server, finalizerName)
		if err := r.Update(ctx, server); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if !server.DeletionTimestamp.IsZero() {
		controllerutil.RemoveFinalizer(server, finalizerName)
		if err := r.Update(ctx, server); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// 1. EULA hard gate — mirror the itzg image behavior.
	if err := gamecfg.ValidateSpec(server.Spec); err != nil {
		if r.Recorder != nil {
			r.Recorder.Event(server, corev1.EventTypeWarning, "ValidationFailed", err.Error())
		}
		_ = r.patchStatus(ctx, server, func(s *bedrockv1alpha1.BedrockServer) {
			s.Status.Phase = bedrockv1alpha1.PhaseFailed
			s.Status.Ready = false
			s.Status.Message = err.Error()
			s.Status.ObservedGeneration = s.Generation
			meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
				Type:               bedrockv1alpha1.ConditionEULAAccepted,
				Status:             eulaCond(server.Spec.EULA.Accepted),
				Reason:             "ValidationFailed",
				Message:            err.Error(),
				ObservedGeneration: s.Generation,
			})
			meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
				Type:               bedrockv1alpha1.ConditionReady,
				Status:             metav1.ConditionFalse,
				Reason:             "ValidationFailed",
				Message:            err.Error(),
				ObservedGeneration: s.Generation,
			})
		})
		log.Info("validation failed — not creating children", "error", err)
		return ctrl.Result{}, nil
	}

	env, err := gamecfg.EnvFromSpec(server.Spec, server.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 2. Secrets (optional shell for operators/allowlist).
	if secret := provisioner.BuildSecret(server); secret != nil {
		if err := controllerutil.SetControllerReference(server, secret, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.applyObject(ctx, secret); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 3. PVC (respect existingClaim).
	if pvc := provisioner.BuildPVC(server); pvc != nil {
		if err := controllerutil.SetControllerReference(server, pvc, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.applyObject(ctx, pvc); err != nil {
			return ctrl.Result{}, err
		}
	} else if claim := server.Spec.Persistence.ExistingClaim; claim != "" {
		existing := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: server.Namespace, Name: claim}, existing); err != nil {
			_ = r.patchStatus(ctx, server, func(s *bedrockv1alpha1.BedrockServer) {
				s.Status.Phase = bedrockv1alpha1.PhaseFailed
				s.Status.Ready = false
				s.Status.Message = fmt.Sprintf("existingClaim %q not found: %v", claim, err)
			})
			return ctrl.Result{}, nil
		}
	}

	// 4. Service.
	svc := provisioner.BuildService(server)
	if err := controllerutil.SetControllerReference(server, svc, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.applyObject(ctx, svc); err != nil {
		return ctrl.Result{}, err
	}

	// 5. Gateway / UDPRoute / TCPRoute.
	gw, err := r.reconcileGatewayStack(ctx, server)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 6. StatefulSet (replicas hard-capped at 1).
	sts := provisioner.BuildStatefulSet(server, env)
	if err := controllerutil.SetControllerReference(server, sts, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.applyObject(ctx, sts); err != nil {
		return ctrl.Result{}, err
	}

	ready := false
	phase := bedrockv1alpha1.PhaseDeploying
	message := "reconciling child resources"
	current := &appsv1.StatefulSet{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: server.Namespace, Name: sts.Name}, current); err == nil {
		if current.Status.ReadyReplicas >= 1 {
			ready = true
			phase = bedrockv1alpha1.PhaseRunning
			message = "bedrock StatefulSet is ready"
		}
		// Cap drift: force replicas=1 if somehow changed.
		if current.Spec.Replicas != nil && *current.Spec.Replicas != 1 {
			one := int32(1)
			current.Spec.Replicas = &one
			_ = r.Update(ctx, current)
		}
	}
	if gw.Message != "" {
		message = message + "; " + gw.Message
	}

	connAddr := gw.Address
	if !gamecfg.GatewayEnabled(server.Spec) {
		connAddr = server.Spec.Gateway.Address
	}

	// 7. Update watcher (Mojang feed).
	updateRes, requeueAfter := r.checkUpdates(ctx, server)

	_ = r.patchStatus(ctx, server, func(s *bedrockv1alpha1.BedrockServer) {
		s.Status.Phase = phase
		s.Status.Ready = ready
		s.Status.Message = message
		s.Status.ConnectionAddress = connAddr
		s.Status.ConnectionPort = gamecfg.ServerPort(server.Spec)
		s.Status.ObservedGeneration = server.Generation
		if updateRes.AvailableVersion != "" {
			s.Status.AvailableVersion = updateRes.AvailableVersion
			s.Status.UpdateAvailable = updateRes.UpdateAvailable
			now := metav1.Now()
			s.Status.LastUpdateCheck = &now
		}
		if updateRes.InstalledVersion != "" {
			s.Status.InstalledVersion = updateRes.InstalledVersion
		}
		meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
			Type:               bedrockv1alpha1.ConditionEULAAccepted,
			Status:             metav1.ConditionTrue,
			Reason:             "EULAAccepted",
			Message:            "spec.eula.accepted is true",
			ObservedGeneration: server.Generation,
		})
		meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
			Type:               bedrockv1alpha1.ConditionReady,
			Status:             boolCond(ready),
			Reason:             readyReason(ready),
			Message:            message,
			ObservedGeneration: server.Generation,
		})
		gwStatus := metav1.ConditionFalse
		gwReason := "GatewayPending"
		if gw.Configured {
			gwStatus = metav1.ConditionTrue
			gwReason = "GatewayConfigured"
		}
		meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
			Type:               bedrockv1alpha1.ConditionGatewayConfigured,
			Status:             gwStatus,
			Reason:             gwReason,
			Message:            gw.Message,
			ObservedGeneration: server.Generation,
		})
		updStatus := metav1.ConditionFalse
		updReason := "UpToDate"
		updMsg := "no update available"
		if updateRes.UpdateAvailable {
			updStatus = metav1.ConditionTrue
			updReason = "UpdateAvailable"
			updMsg = fmt.Sprintf("available %s (installed %s)", updateRes.AvailableVersion, updateRes.InstalledVersion)
		}
		meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
			Type:               bedrockv1alpha1.ConditionUpdateAvailable,
			Status:             updStatus,
			Reason:             updReason,
			Message:            updMsg,
			ObservedGeneration: server.Generation,
		})
	})

	if requeueAfter > 0 {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	return ctrl.Result{}, nil
}

func (r *BedrockServerReconciler) applyObject(ctx context.Context, obj client.Object) error {
	key := client.ObjectKeyFromObject(obj)
	existing := obj.DeepCopyObject().(client.Object)
	err := r.Get(ctx, key, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, obj)
	}
	if err != nil {
		return err
	}

	switch desired := obj.(type) {
	case *corev1.Service:
		cur := existing.(*corev1.Service)
		cur.Spec.Type = desired.Spec.Type
		cur.Spec.Ports = desired.Spec.Ports
		cur.Spec.Selector = desired.Spec.Selector
		cur.Spec.LoadBalancerIP = desired.Spec.LoadBalancerIP
		cur.Spec.ExternalTrafficPolicy = desired.Spec.ExternalTrafficPolicy
		if desired.Spec.ExternalTrafficPolicy != corev1.ServiceExternalTrafficPolicyLocal {
			cur.Spec.HealthCheckNodePort = 0
		}
		cur.Labels = desired.Labels
		return r.Update(ctx, cur)
	case *appsv1.StatefulSet:
		cur := existing.(*appsv1.StatefulSet)
		// Preserve volumeClaimTemplates immutability; update pod template + replicas.
		cur.Spec.Replicas = desired.Spec.Replicas
		cur.Spec.Template = desired.Spec.Template
		cur.Spec.UpdateStrategy = desired.Spec.UpdateStrategy
		cur.Labels = desired.Labels
		return r.Update(ctx, cur)
	case *corev1.PersistentVolumeClaim:
		cur := existing.(*corev1.PersistentVolumeClaim)
		cur.Labels = desired.Labels
		return r.Update(ctx, cur)
	case *corev1.Secret:
		cur := existing.(*corev1.Secret)
		// Never overwrite non-empty user-managed keys.
		if cur.Data == nil {
			cur.Data = map[string][]byte{}
		}
		for k, v := range desired.Data {
			if len(cur.Data[k]) == 0 {
				cur.Data[k] = v
			}
		}
		cur.Labels = desired.Labels
		return r.Update(ctx, cur)
	case *egv1a1.EnvoyProxy:
		cur := existing.(*egv1a1.EnvoyProxy)
		cur.Labels = desired.Labels
		cur.OwnerReferences = desired.OwnerReferences
		cur.Spec = desired.Spec
		return r.Update(ctx, cur)
	case *gatewayv1.Gateway:
		cur := existing.(*gatewayv1.Gateway)
		cur.Labels = desired.Labels
		cur.OwnerReferences = desired.OwnerReferences
		cur.Spec = desired.Spec
		return r.Update(ctx, cur)
	case *gatewayv1alpha2.UDPRoute:
		cur := existing.(*gatewayv1alpha2.UDPRoute)
		cur.Labels = desired.Labels
		cur.OwnerReferences = desired.OwnerReferences
		cur.Spec = desired.Spec
		return r.Update(ctx, cur)
	case *gatewayv1alpha2.TCPRoute:
		cur := existing.(*gatewayv1alpha2.TCPRoute)
		cur.Labels = desired.Labels
		cur.OwnerReferences = desired.OwnerReferences
		cur.Spec = desired.Spec
		return r.Update(ctx, cur)
	default:
		return fmt.Errorf("unsupported apply type %T", obj)
	}
}

func (r *BedrockServerReconciler) patchStatus(ctx context.Context, server *bedrockv1alpha1.BedrockServer, mutate func(*bedrockv1alpha1.BedrockServer)) error {
	latest := &bedrockv1alpha1.BedrockServer{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(server), latest); err != nil {
		return err
	}
	mutate(latest)
	return r.Status().Update(ctx, latest)
}

func boolCond(ready bool) metav1.ConditionStatus {
	if ready {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func eulaCond(accepted bool) metav1.ConditionStatus {
	if accepted {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func readyReason(ready bool) string {
	if ready {
		return "StatefulSetReady"
	}
	return "StatefulSetNotReady"
}

// SetupWithManager sets up the controller with the Manager.
func (r *BedrockServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Versions == nil {
		r.Versions = version.DefaultClient()
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&bedrockv1alpha1.BedrockServer{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&gatewayv1.Gateway{}).
		Owns(&gatewayv1alpha2.UDPRoute{}).
		Owns(&gatewayv1alpha2.TCPRoute{}).
		Owns(&egv1a1.EnvoyProxy{}).
		Named("bedrockserver").
		Complete(r)
}

// CheckInterval parses update.checkInterval (default 6h).
func CheckInterval(spec bedrockv1alpha1.BedrockServerSpec) time.Duration {
	s := spec.Update.CheckInterval
	if s == "" {
		s = bedrockv1alpha1.DefaultUpdateInterval
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		d = 6 * time.Hour
	}
	return d
}
