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

// Package gateway owns Envoy Gateway + UDPRoute/TCPRoute wiring for BedrockServer.
package gateway

import (
	"fmt"
	"strings"

	bedrockv1alpha1 "github.com/DataKnifeAI/bedrock-operator/api/v1alpha1"
	"github.com/DataKnifeAI/bedrock-operator/internal/gamecfg"
	"github.com/DataKnifeAI/bedrock-operator/internal/provisioner"
	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	ListenerGameUDP   = "game-udp"
	ListenerGameUDPv6 = "game-udp-v6"
	ListenerNetherTCP = "nethernet-tcp"
	defaultClassName  = "envoy"
)

// Names holds derived Gateway resource names.
type Names struct {
	Gateway     string
	EnvoyProxy  string
	EnvoySvc    string
	GameRoute   string
	GameV6Route string
	NetherRoute string
}

// DeriveNames returns resource names for a server.
func DeriveNames(server *bedrockv1alpha1.BedrockServer) Names {
	base := strings.TrimSuffix(server.Name, "-server")
	gwName := base + "-gateway"
	if server.Spec.Gateway.GatewayName != "" {
		gwName = server.Spec.Gateway.GatewayName
	}
	epName := "game-" + base + "-kubevip"
	if server.Spec.Gateway.EnvoyProxyName != "" {
		epName = server.Spec.Gateway.EnvoyProxyName
	}
	return Names{
		Gateway:     gwName,
		EnvoyProxy:  epName,
		EnvoySvc:    server.Name + "-envoy",
		GameRoute:   base + "-game-udp",
		GameV6Route: base + "-game-udp-v6",
		NetherRoute: base + "-nethernet-tcp",
	}
}

func className(spec bedrockv1alpha1.BedrockServerSpec) string {
	if spec.Gateway.ClassName != "" {
		return spec.Gateway.ClassName
	}
	return defaultClassName
}

func gatewayLabels(server *bedrockv1alpha1.BedrockServer) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       provisioner.AppName,
		"app.kubernetes.io/instance":   server.Name,
		"app.kubernetes.io/managed-by": provisioner.ManagedBy,
		"app.kubernetes.io/component":  "envoy-gateway",
	}
}

// BuildEnvoyBackendService is the ClusterIP Service UDP/TCP routes target.
func BuildEnvoyBackendService(server *bedrockv1alpha1.BedrockServer) *corev1.Service {
	names := DeriveNames(server)
	port := gamecfg.ServerPort(server.Spec)
	ports := []corev1.ServicePort{
		{Name: "game-udp", Protocol: corev1.ProtocolUDP, Port: port, TargetPort: intstr.FromInt32(port)},
		{Name: "nethernet-tcp", Protocol: corev1.ProtocolTCP, Port: port, TargetPort: intstr.FromInt32(port)},
	}
	if server.Spec.Settings.IPv6Enabled {
		v6 := gamecfg.ServerPortV6(server.Spec)
		ports = append(ports, corev1.ServicePort{
			Name: "game-udp-v6", Protocol: corev1.ProtocolUDP, Port: v6, TargetPort: intstr.FromInt32(v6),
		})
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.EnvoySvc,
			Namespace: server.Namespace,
			Labels:    gatewayLabels(server),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: provisioner.LabelsFor(server),
			Ports:    ports,
		},
	}
}

// BuildEnvoyProxy pins the LoadBalancer VIP via Envoy Gateway infrastructure.
func BuildEnvoyProxy(server *bedrockv1alpha1.BedrockServer) *egv1a1.EnvoyProxy {
	names := DeriveNames(server)
	return &egv1a1.EnvoyProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.EnvoyProxy,
			Namespace: server.Namespace,
			Labels:    gatewayLabels(server),
		},
		Spec: egv1a1.EnvoyProxySpec{
			Logging: egv1a1.ProxyLogging{
				Level: map[egv1a1.ProxyLogComponent]egv1a1.LogLevel{
					egv1a1.LogComponentDefault: egv1a1.LogLevelWarn,
				},
			},
			Provider: &egv1a1.EnvoyProxyProvider{
				Type: egv1a1.ProviderTypeKubernetes,
				Kubernetes: &egv1a1.EnvoyProxyKubernetesProvider{
					EnvoyService: &egv1a1.KubernetesServiceSpec{
						Type:                  ptr.To(egv1a1.ServiceTypeLoadBalancer),
						LoadBalancerIP:        ptr.To(server.Spec.Gateway.Address),
						ExternalTrafficPolicy: ptr.To(egv1a1.ServiceExternalTrafficPolicyCluster),
					},
				},
			},
		},
	}
}

// BuildGateway creates the Gateway with UDP + NetherNet TCP listeners.
func BuildGateway(server *bedrockv1alpha1.BedrockServer) *gatewayv1.Gateway {
	names := DeriveNames(server)
	sameNS := &gatewayv1.AllowedRoutes{
		Namespaces: &gatewayv1.RouteNamespaces{From: ptr.To(gatewayv1.NamespacesFromSame)},
	}
	port := gamecfg.ServerPort(server.Spec)
	listeners := []gatewayv1.Listener{
		{
			Name:          gatewayv1.SectionName(ListenerGameUDP),
			Port:          gatewayv1.PortNumber(port),
			Protocol:      gatewayv1.UDPProtocolType,
			AllowedRoutes: sameNS,
		},
		{
			Name:          gatewayv1.SectionName(ListenerNetherTCP),
			Port:          gatewayv1.PortNumber(port),
			Protocol:      gatewayv1.TCPProtocolType,
			AllowedRoutes: sameNS,
		},
	}
	if server.Spec.Settings.IPv6Enabled {
		v6 := gamecfg.ServerPortV6(server.Spec)
		listeners = append(listeners, gatewayv1.Listener{
			Name:          gatewayv1.SectionName(ListenerGameUDPv6),
			Port:          gatewayv1.PortNumber(v6),
			Protocol:      gatewayv1.UDPProtocolType,
			AllowedRoutes: sameNS,
		})
	}
	return &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.Gateway,
			Namespace: server.Namespace,
			Labels:    gatewayLabels(server),
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName(className(server.Spec)),
			Addresses: []gatewayv1.GatewayAddress{
				{Type: ptr.To(gatewayv1.IPAddressType), Value: server.Spec.Gateway.Address},
			},
			Infrastructure: &gatewayv1.GatewayInfrastructure{
				ParametersRef: &gatewayv1.LocalParametersReference{
					Group: gatewayv1.Group(egv1a1.GroupVersion.Group),
					Kind:  gatewayv1.Kind("EnvoyProxy"),
					Name:  names.EnvoyProxy,
				},
			},
			Listeners: listeners,
		},
	}
}

// BuildUDPRoute routes one UDP listener to the envoy backend Service.
func BuildUDPRoute(server *bedrockv1alpha1.BedrockServer, routeName, listenerName string, port int32) *gatewayv1alpha2.UDPRoute {
	names := DeriveNames(server)
	return &gatewayv1alpha2.UDPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: server.Namespace,
			Labels:    gatewayLabels(server),
		},
		Spec: gatewayv1alpha2.UDPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{
					Name:        gatewayv1.ObjectName(names.Gateway),
					Namespace:   ptr.To(gatewayv1.Namespace(server.Namespace)),
					SectionName: ptr.To(gatewayv1.SectionName(listenerName)),
				}},
			},
			Rules: []gatewayv1alpha2.UDPRouteRule{{
				BackendRefs: []gatewayv1alpha2.BackendRef{{
					BackendObjectReference: gatewayv1.BackendObjectReference{
						Name: gatewayv1.ObjectName(names.EnvoySvc),
						Port: ptr.To(gatewayv1.PortNumber(port)),
					},
				}},
			}},
		},
	}
}

// BuildTCPRoute routes NetherNet TCP signaling to the envoy backend Service.
func BuildTCPRoute(server *bedrockv1alpha1.BedrockServer) *gatewayv1alpha2.TCPRoute {
	names := DeriveNames(server)
	port := gamecfg.ServerPort(server.Spec)
	return &gatewayv1alpha2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.NetherRoute,
			Namespace: server.Namespace,
			Labels:    gatewayLabels(server),
		},
		Spec: gatewayv1alpha2.TCPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{
					Name:        gatewayv1.ObjectName(names.Gateway),
					Namespace:   ptr.To(gatewayv1.Namespace(server.Namespace)),
					SectionName: ptr.To(gatewayv1.SectionName(ListenerNetherTCP)),
				}},
			},
			Rules: []gatewayv1alpha2.TCPRouteRule{{
				BackendRefs: []gatewayv1alpha2.BackendRef{{
					BackendObjectReference: gatewayv1.BackendObjectReference{
						Name: gatewayv1.ObjectName(names.EnvoySvc),
						Port: ptr.To(gatewayv1.PortNumber(port)),
					},
				}},
			}},
		},
	}
}

// ConnectionAddress returns the preferred client-facing IP.
func ConnectionAddress(server *bedrockv1alpha1.BedrockServer, gw *gatewayv1.Gateway) string {
	if gw != nil && len(gw.Status.Addresses) > 0 {
		return gw.Status.Addresses[0].Value
	}
	return server.Spec.Gateway.Address
}

// MissingGatewayClassMessage explains a missing GatewayClass.
func MissingGatewayClassMessage(class string) string {
	return fmt.Sprintf("GatewayClass %q not found or not Accepted — install Envoy Gateway (or set gateway.enabled=false and use Service LoadBalancer)", class)
}
