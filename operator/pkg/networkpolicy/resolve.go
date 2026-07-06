/*
Copyright 2023 Red Hat, Inc.

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

package networkpolicy

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	kubernetesServiceName      = "kubernetes"
	kubernetesDefaultNamespace = "default"
	clusterNetworkName         = "cluster"
)

// ResolveAPIServerCIDRs discovers Kubernetes API server endpoint addresses for NetworkPolicy ipBlock rules.
func ResolveAPIServerCIDRs(ctx context.Context, c client.Client) ([]string, error) {
	cidrs, err := resolveServiceCIDRs(ctx, c, kubernetesDefaultNamespace, kubernetesServiceName)
	if err != nil {
		return nil, fmt.Errorf("no kubernetes API server addresses found: %w", err)
	}
	return cidrs, nil
}

// ResolvePostgresCIDRs resolves a BYO Postgres database URI host to ipBlock CIDR strings.
func ResolvePostgresCIDRs(ctx context.Context, c client.Client, namespace, databaseURI string) ([]string, error) {
	databaseURI = strings.TrimSpace(databaseURI)
	if databaseURI == "" {
		return nil, fmt.Errorf("empty postgres database URI")
	}

	objURI, err := url.Parse(databaseURI)
	if err != nil {
		return nil, fmt.Errorf("invalid postgres database URI host")
	}
	host := strings.TrimSpace(objURI.Hostname())
	if host == "" {
		return nil, fmt.Errorf("empty postgres host in database URI")
	}

	cidrs := map[string]struct{}{}
	if err := resolveHostToCIDRs(ctx, c, host, namespace, cidrs); err != nil {
		return nil, fmt.Errorf("resolve postgres host %q: %w", host, err)
	}
	if len(cidrs) == 0 {
		return nil, fmt.Errorf("no postgres addresses resolved for %q", host)
	}
	return sortedCIDRs(cidrs), nil
}

// ResolveBootstrapServerCIDRs resolves Kafka bootstrap broker hostnames/IPs to ipBlock CIDR strings.
// clusterNamespace is used to resolve unqualified in-cluster service names.
func ResolveBootstrapServerCIDRs(
	ctx context.Context,
	c client.Client,
	bootstrapServer string,
	clusterNamespace string,
) ([]string, error) {
	bootstrapServer = strings.TrimSpace(bootstrapServer)
	if bootstrapServer == "" {
		return nil, fmt.Errorf("empty bootstrap server")
	}

	cidrs := map[string]struct{}{}
	var resolveErrs []string
	for _, broker := range strings.Split(bootstrapServer, ",") {
		broker = strings.TrimSpace(broker)
		if broker == "" {
			continue
		}
		host := brokerHost(broker)
		if host == "" {
			continue
		}
		if err := resolveHostToCIDRs(ctx, c, host, clusterNamespace, cidrs); err != nil {
			resolveErrs = append(resolveErrs, fmt.Sprintf("%s: %v", host, err))
		}
	}

	if len(cidrs) == 0 {
		if len(resolveErrs) > 0 {
			return nil, fmt.Errorf("no bootstrap broker addresses resolved: %s",
				strings.Join(resolveErrs, "; "))
		}
		return nil, fmt.Errorf("no bootstrap broker addresses resolved")
	}
	return sortedCIDRs(cidrs), nil
}

// ResolveServiceNetworkCIDRs returns cluster service network CIDRs when available (OpenShift Network CR).
func ResolveServiceNetworkCIDRs(ctx context.Context, c client.Client) ([]string, error) {
	network := &configv1.Network{}
	err := c.Get(ctx, types.NamespacedName{Name: clusterNetworkName}, network)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get cluster Network config: %w", err)
	}
	if len(network.Spec.ServiceNetwork) == 0 {
		return nil, nil
	}
	cidrs := make([]string, 0, len(network.Spec.ServiceNetwork))
	for _, serviceNetwork := range network.Spec.ServiceNetwork {
		if _, _, err := net.ParseCIDR(serviceNetwork); err != nil {
			return nil, fmt.Errorf("invalid service network CIDR %q: %w", serviceNetwork, err)
		}
		cidrs = append(cidrs, serviceNetwork)
	}
	return cidrs, nil
}

func resolveHostToCIDRs(
	ctx context.Context,
	c client.Client,
	host string,
	defaultNamespace string,
	cidrs map[string]struct{},
) error {
	host = strings.Trim(host, "[]")
	if ip := net.ParseIP(host); ip != nil {
		addHostCIDR(cidrs, ip)
		return nil
	}

	if !strings.Contains(host, ".") {
		if svcCIDRs, err := resolveServiceCIDRs(ctx, c, defaultNamespace, host); err == nil && len(svcCIDRs) > 0 {
			for _, cidr := range svcCIDRs {
				cidrs[cidr] = struct{}{}
			}
			return nil
		}
	}

	svcNamespace, svcName := parseServiceHost(host, defaultNamespace)
	if svcName != "" {
		svcCIDRs, err := resolveServiceCIDRs(ctx, c, svcNamespace, svcName)
		if err == nil && len(svcCIDRs) > 0 {
			for _, cidr := range svcCIDRs {
				cidrs[cidr] = struct{}{}
			}
			return nil
		}
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return fmt.Errorf("no addresses returned for %q", host)
	}
	for _, ipAddr := range ips {
		addHostCIDR(cidrs, ipAddr.IP)
	}
	return nil
}

func resolveServiceCIDRs(ctx context.Context, c client.Client, namespace, name string) ([]string, error) {
	cidrs := map[string]struct{}{}

	svc := &corev1.Service{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, svc); err != nil {
		return nil, err
	}
	if ip := net.ParseIP(svc.Spec.ClusterIP); ip != nil && svc.Spec.ClusterIP != "" && svc.Spec.ClusterIP != "None" {
		addHostCIDR(cidrs, ip)
	}

	sliceList := &discoveryv1.EndpointSliceList{}
	if err := c.List(
		ctx, sliceList,
		client.InNamespace(namespace),
		client.MatchingLabels{discoveryv1.LabelServiceName: name},
	); err != nil {
		log.Warnw("failed to list EndpointSlices for service",
			"namespace", namespace, "name", name, "error", err)
	} else {
		addEndpointSliceCIDRs(sliceList, cidrs)
	}

	if len(cidrs) == 0 {
		return nil, fmt.Errorf("no addresses found for service %s/%s", namespace, name)
	}
	return sortedCIDRs(cidrs), nil
}

func addEndpointSliceCIDRs(sliceList *discoveryv1.EndpointSliceList, cidrs map[string]struct{}) {
	for i := range sliceList.Items {
		for _, endpoint := range sliceList.Items[i].Endpoints {
			for _, addr := range endpoint.Addresses {
				if ip := net.ParseIP(addr); ip != nil {
					addHostCIDR(cidrs, ip)
				}
			}
		}
	}
}

func brokerHost(broker string) string {
	host, _, err := net.SplitHostPort(broker)
	if err != nil {
		return strings.TrimSpace(broker)
	}
	return strings.TrimSpace(host)
}

func parseServiceHost(host, defaultNamespace string) (namespace, name string) {
	host = strings.TrimSuffix(host, ".")
	switch {
	case strings.HasSuffix(host, ".svc.cluster.local"):
		host = strings.TrimSuffix(host, ".svc.cluster.local")
	case strings.HasSuffix(host, ".svc"):
		host = strings.TrimSuffix(host, ".svc")
	default:
		return "", ""
	}

	parts := strings.Split(host, ".")
	switch len(parts) {
	case 1:
		return defaultNamespace, parts[0]
	case 2:
		return parts[1], parts[0]
	default:
		return "", ""
	}
}

func addHostCIDR(cidrs map[string]struct{}, ip net.IP) {
	if ip == nil {
		return
	}
	if v4 := ip.To4(); v4 != nil {
		cidrs[v4.String()+"/32"] = struct{}{}
		return
	}
	cidrs[ip.String()+"/128"] = struct{}{}
}

func sortedCIDRs(cidrs map[string]struct{}) []string {
	out := make([]string, 0, len(cidrs))
	for cidr := range cidrs {
		out = append(out, cidr)
	}
	sort.Strings(out)
	return out
}
