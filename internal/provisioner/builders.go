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

package provisioner

import (
	"strconv"

	bedrockv1alpha1 "github.com/DataKnifeAI/bedrock-operator/api/v1alpha1"
	"github.com/DataKnifeAI/bedrock-operator/internal/gamecfg"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

const dataMountPath = "/data"

// BuildPVC creates (or describes) the /data volume for worlds + BDS package.
// When ExistingClaim is set, returns nil — caller adopts the named claim.
func BuildPVC(server *bedrockv1alpha1.BedrockServer) *corev1.PersistentVolumeClaim {
	if server.Spec.Persistence.ExistingClaim != "" {
		return nil
	}
	size := gamecfg.PersistenceSize(server.Spec)
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PVCName(server),
			Namespace: server.Namespace,
			Labels:    LabelsFor(server),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(size),
				},
			},
		},
	}
	if server.Spec.Persistence.StorageClassName != "" {
		sc := server.Spec.Persistence.StorageClassName
		pvc.Spec.StorageClassName = &sc
	}
	return pvc
}

// BuildService exposes NetherNet TCP signaling + gameplay UDP range (or legacy RakNet UDP).
func BuildService(server *bedrockv1alpha1.BedrockServer) *corev1.Service {
	port := gamecfg.ServerPort(server.Spec)
	svcType := corev1.ServiceTypeClusterIP
	if !gamecfg.GatewayEnabled(server.Spec) && server.Spec.Gateway.ServiceType != "" {
		svcType = server.Spec.Gateway.ServiceType
	}

	transport := gamecfg.Transport(server.Spec)
	ports := []corev1.ServicePort{
		{
			Name:       "nethernet-tcp",
			Protocol:   corev1.ProtocolTCP,
			Port:       port,
			TargetPort: intstr.FromInt32(port),
		},
	}
	if transport == bedrockv1alpha1.TransportRakNet {
		ports = append(ports, corev1.ServicePort{
			Name:       "game-udp",
			Protocol:   corev1.ProtocolUDP,
			Port:       port,
			TargetPort: intstr.FromInt32(port),
		})
		if server.Spec.Settings.IPv6Enabled {
			v6 := gamecfg.ServerPortV6(server.Spec)
			ports = append(ports, corev1.ServicePort{
				Name:       "game-udp-v6",
				Protocol:   corev1.ProtocolUDP,
				Port:       v6,
				TargetPort: intstr.FromInt32(v6),
			})
		}
	} else {
		// NetherNet: one UDP gameplay port per concurrent player (itzg SERVER_UDP_PORTS).
		for p := gamecfg.UdpGameplayPortStart(server.Spec); p <= gamecfg.UdpGameplayPortEnd(server.Spec); p++ {
			ports = append(ports, corev1.ServicePort{
				Name:       "nether-udp-" + strconv.FormatInt(int64(p), 10),
				Protocol:   corev1.ProtocolUDP,
				Port:       p,
				TargetPort: intstr.FromInt32(p),
			})
		}
	}

	spec := corev1.ServiceSpec{
		Type:     svcType,
		Selector: LabelsFor(server),
		Ports:    ports,
	}
	if !gamecfg.GatewayEnabled(server.Spec) && svcType == corev1.ServiceTypeLoadBalancer {
		if addr := server.Spec.Gateway.Address; addr != "" {
			spec.LoadBalancerIP = addr
		}
		spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyCluster
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName(server),
			Namespace: server.Namespace,
			Labels:    LabelsFor(server),
		},
		Spec: spec,
	}
}

// BuildSecret creates an owned Secret shell when generateSecrets is true.
func BuildSecret(server *bedrockv1alpha1.BedrockServer) *corev1.Secret {
	if !server.Spec.Secrets.GenerateSecrets {
		return nil
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretName(server),
			Namespace: server.Namespace,
			Labels:    LabelsFor(server),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			// Placeholders — operators populate operators.json / allowlist.json.
			"operators.json": []byte("[]"),
			"allowlist.json": []byte("[]"),
		},
	}
}

// BuildStatefulSet creates the single-replica StatefulSet for BDS.
func BuildStatefulSet(server *bedrockv1alpha1.BedrockServer, env []corev1.EnvVar) *appsv1.StatefulSet {
	replicas := int32(1)
	grace := gamecfg.GraceSeconds(server.Spec)
	labels := LabelsFor(server)
	image := gamecfg.ImageRef(server.Spec)
	pullPolicy := server.Spec.ServerImage.PullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullIfNotPresent
	}
	port := gamecfg.ServerPort(server.Spec)
	resources := defaultResources(server)

	transport := gamecfg.Transport(server.Spec)
	containerPorts := []corev1.ContainerPort{
		{Name: "nethernet", ContainerPort: port, Protocol: corev1.ProtocolTCP},
	}
	if transport == bedrockv1alpha1.TransportRakNet {
		containerPorts = append(containerPorts, corev1.ContainerPort{
			Name: "game-udp", ContainerPort: port, Protocol: corev1.ProtocolUDP,
		})
		if server.Spec.Settings.IPv6Enabled {
			v6 := gamecfg.ServerPortV6(server.Spec)
			containerPorts = append(containerPorts, corev1.ContainerPort{
				Name: "game-udp-v6", ContainerPort: v6, Protocol: corev1.ProtocolUDP,
			})
		}
	} else {
		for p := gamecfg.UdpGameplayPortStart(server.Spec); p <= gamecfg.UdpGameplayPortEnd(server.Spec); p++ {
			containerPorts = append(containerPorts, corev1.ContainerPort{
				Name: "nudp-" + strconv.FormatInt(int64(p), 10), ContainerPort: p, Protocol: corev1.ProtocolUDP,
			})
		}
	}

	portStr := strconv.FormatInt(int64(port), 10)
	readiness := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				// mc-monitor ships in the itzg image; force IPv4 to avoid dual-stack probe weirdness.
				Command: []string{"mc-monitor", "status-bedrock", "--host", "127.0.0.1", "--port", portStr},
			},
		},
		InitialDelaySeconds: 60,
		PeriodSeconds:       30,
		TimeoutSeconds:      10,
		FailureThreshold:    6,
	}
	liveness := readiness.DeepCopy()

	podSpec := corev1.PodSpec{
		TerminationGracePeriodSeconds: &grace,
		ImagePullSecrets:              server.Spec.ImagePullSecrets,
		NodeSelector:                  server.Spec.NodeSelector,
		HostNetwork:                   server.Spec.HostNetworkDiscovery,
		DNSPolicy:                     corev1.DNSClusterFirst,
		SecurityContext: &corev1.PodSecurityContext{
			FSGroup: ptr.To(int64(1000)),
		},
		Containers: []corev1.Container{{
			Name:            bedrockv1alpha1.ContainerName,
			Image:           image,
			ImagePullPolicy: pullPolicy,
			Env:             env,
			TTY:             true,
			Stdin:           true,
			Ports:           containerPorts,
			Resources:       resources,
			VolumeMounts: []corev1.VolumeMount{
				{Name: "data", MountPath: dataMountPath},
			},
			ReadinessProbe: readiness,
			LivenessProbe:  liveness,
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: ptr.To(false),
			},
		}},
		Volumes: []corev1.Volume{{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: PVCName(server),
				},
			},
		}},
	}
	if server.Spec.HostNetworkDiscovery {
		podSpec.DNSPolicy = corev1.DNSClusterFirstWithHostNet
	}

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      StatefulSetName(server),
			Namespace: server.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: ServiceName(server),
			Selector:    &metav1.LabelSelector{MatchLabels: labels},
			// Recreate semantics: BDS binds UDP ports; RollingUpdate cannot run two pods.
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					MaxUnavailable: ptr.To(intstr.FromInt32(1)),
					Partition:      ptr.To(int32(0)),
				},
			},
			// PodManagementPolicy OrderedReady + replicas=1 ≈ Recreate for single-instance.
			PodManagementPolicy: appsv1.OrderedReadyPodManagement,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       podSpec,
			},
		},
	}
}

func defaultResources(server *bedrockv1alpha1.BedrockServer) corev1.ResourceRequirements {
	if server.Spec.Resources != nil {
		return *server.Spec.Resources
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}
