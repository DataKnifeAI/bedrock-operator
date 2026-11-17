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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bedrockv1alpha1 "github.com/DataKnifeAI/bedrock-operator/api/v1alpha1"
)

var _ = Describe("BedrockServer Controller", func() {
	Context("When reconciling a BedrockServer", func() {
		const (
			resourceName      = "survival-1"
			resourceNamespace = "default"
		)

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: resourceNamespace}
		gatewayDisabled := false

		makeServer := func(eula bool) *bedrockv1alpha1.BedrockServer {
			return &bedrockv1alpha1.BedrockServer{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: resourceNamespace},
				Spec: bedrockv1alpha1.BedrockServerSpec{
					EULA: bedrockv1alpha1.EULAConfig{Accepted: eula},
					Settings: bedrockv1alpha1.ServerSettings{
						LevelName:  "survival-1",
						GameMode:   "survival",
						Difficulty: "normal",
						MaxPlayers: 10,
					},
					Gateway: bedrockv1alpha1.GatewayConfig{
						Enabled:     &gatewayDisabled,
						ServiceType: corev1.ServiceTypeClusterIP,
					},
					Persistence: bedrockv1alpha1.PersistenceConfig{Size: "8Gi"},
				},
			}
		}

		deleteServer := func() {
			resource := &bedrockv1alpha1.BedrockServer{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if apierrors.IsNotFound(err) {
				return
			}
			Expect(err).NotTo(HaveOccurred())
			controllerutil.RemoveFinalizer(resource, finalizerName)
			_ = k8sClient.Update(ctx, resource)
			_ = k8sClient.Delete(ctx, resource)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, &bedrockv1alpha1.BedrockServer{})
				return apierrors.IsNotFound(err)
			}, time.Second*5, time.Millisecond*100).Should(BeTrue())
		}

		BeforeEach(func() {
			deleteServer()
		})

		AfterEach(func() {
			deleteServer()
			_ = k8sClient.DeleteAllOf(ctx, &appsv1.StatefulSet{}, client.InNamespace(resourceNamespace))
			_ = k8sClient.DeleteAllOf(ctx, &corev1.Service{}, client.InNamespace(resourceNamespace))
			_ = k8sClient.DeleteAllOf(ctx, &corev1.PersistentVolumeClaim{}, client.InNamespace(resourceNamespace))
			_ = k8sClient.DeleteAllOf(ctx, &corev1.Secret{}, client.InNamespace(resourceNamespace))
		})

		It("should reject when EULA is not accepted and create no children", func() {
			Expect(k8sClient.Create(ctx, makeServer(false))).To(Succeed())

			r := &BedrockServerReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			got := &bedrockv1alpha1.BedrockServer{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, got)).To(Succeed())
			Expect(got.Status.Phase).To(Equal(bedrockv1alpha1.PhaseFailed))

			sts := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, typeNamespacedName, sts)
			Expect(err).To(HaveOccurred())
		})

		It("should create PVC, Service, StatefulSet and be deterministic on second reconcile", func() {
			Expect(k8sClient.Create(ctx, makeServer(true))).To(Succeed())

			r := &BedrockServerReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}

			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			pvc := &corev1.PersistentVolumeClaim{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName + "-data", Namespace: resourceNamespace}, pvc)).To(Succeed())

			svc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, svc)).To(Succeed())

			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, sts)).To(Succeed())
			Expect(*sts.Spec.Replicas).To(Equal(int32(1)))

			var eula, version, level string
			for _, e := range sts.Spec.Template.Spec.Containers[0].Env {
				switch e.Name {
				case "EULA":
					eula = e.Value
				case "VERSION":
					version = e.Value
				case "LEVEL_NAME":
					level = e.Value
				}
			}
			Expect(eula).To(Equal("TRUE"))
			Expect(version).To(Equal("LATEST"))
			Expect(level).To(Equal("survival-1"))
			Expect(sts.Spec.Template.Spec.Containers[0].TTY).To(BeTrue())
			Expect(sts.Spec.Template.Spec.Containers[0].Stdin).To(BeTrue())

			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			sts2 := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, sts2)).To(Succeed())
			Expect(*sts2.Spec.Replicas).To(Equal(int32(1)))
		})

		It("should adopt existingClaim when present", func() {
			claim := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "restored-world", Namespace: resourceNamespace},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("8Gi"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, claim)).To(Succeed())

			srv := makeServer(true)
			srv.Spec.Persistence.ExistingClaim = "restored-world"
			Expect(k8sClient.Create(ctx, srv)).To(Succeed())

			r := &BedrockServerReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}
			_, _ = r.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, sts)).To(Succeed())
			Expect(sts.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal("restored-world"))
		})
	})
})
