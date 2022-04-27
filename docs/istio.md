# Introduction to Istio

## Preface

In the Envoy lab, we explored two scenarios:

1. A single Envoy "cluster" with two endpoints.

    In this scenario, we observed that a request to the proxy resulted in the load-balancing of requests across the two endpoints.

1. Two Envoy clusters, together with a routing configuration to route requests from the proxy to either cluster depending on the request's path prefix:

    - Requests having the path prefix of `/one` were routed to the first cluter's `/ip` endpoint, and
    - Requests with the path prefix of `/two` were routed to the second cluster's `/user-agent` endpoint.

In this lab, you will learn how to model both scenarios in the context of Istio.

Envoy is one of the building blocks of Istio.

In Istio, Envoy proxies are configured indirectly, using a combination of:

1. implicit information drawn from the Kubernetes environment in which services run, and
1. Istio-specific Kubernetes custom resource definitions (CRDs).

## Install Istio

Follow [these instructions](https://tetratelabs.github.io/istio-0to60/install/) to install Istio in your environment.

## Where are the Envoys?

In Istio, Envoy proxy instances are present in two distinct locations:

1. In the heart of the mesh: they are bundled as sidecar containers in the pods that run our workloads.
1. At the edge: as standalone gateways handling ingress and egress traffic in and out of the mesh.

An ingress gateway is deployed as part of the installation of Istio.  It resides in the `istio-system` namespace.  Verify this:

```shell
kubectl get deploy -n istio-system
```

To deploy Envoy as a sidecar, we will employ the convenient [automatic sidecar injection](https://istio.io/latest/docs/setup/additional-setup/sidecar-injection/#automatic-sidecar-injection), which works as follows:

1. Label the target namespace with the special label `istio-injection` with the value `enabled`:

    ```shell
    kubectl label ns default istio-injection=enabled
    ```

1. When using `kubectl` to apply a deployment, Istio employs a Kubernetes [admission controller](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/) to augment the pod specification to bundle Envoy into a sidecar container named `istio-proxy`.

    Verify this:  observe the presence of the istio sidecar injector in your Kubernetes cluster:

    ```shell
    kubectl get mutatingwebhookconfigurations
    ```

## Scenario 1: Load-balancing across multiple endpoints

- Deploy httpbin to the default namespace
- Deploy the sleep pod

Scale httpbin to two instances
Observe requests from sleep are load-balanced across the two endpoints

Observe the configuration of the Envoy in the sleep pod: listeners, clusters, and endpoints

## Scenarios 2: Two clusters with routing configuration

- Deploy a second httpbin service
- Define the routing configuration using a virtual service

Observe requests to `/one` go to the first cluster's `/ip` endpoint
Observe requests to `/two` go to the second cluter's `/user-agent` endpoint

Observe the configuration of the Envoy in the sleep pod: listeners, clusters, endpoints, and routes.


## Using an Ingress Gateway

- configure the gateway
- bind the virtual service to the gateway
- test the endpoints
- inspect the gateway configuration.

## Summary

In comparison to having to configure Envoy proxies manually, Istio provides a mechanism to configure Envoy proxies with much less effort.
It draws on information from the environment: awareness of running workloads, service discovery, provide the inputs necessary to derive Envoy's clusters and listeners configurations automatically.

Istio Custom Resource Definitions complement and complete the picture by providing mechanisms to configure routing rules, authorization policies and more.

Istio goes one step further:  it dynamically reconfigures the Envoy proxies any time that services are scaled, or added to and removed from the mesh.

Istio and Envoy together provide a foundation for running microservices at scale.

In the next lab, we turn our attention to Web Assembly, a mechanism for extending and customizing the behavior of the Envoy proxies running in the mesh.