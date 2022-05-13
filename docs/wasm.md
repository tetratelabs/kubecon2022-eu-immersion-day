# Extending Envoy and Istio with Wasm


## Envoy Wasm filter

Let's look more closely at the Envoy configuration from the previous section:

```sh
HTTPBIN_POD=$(kubectl get pod -l app=httpbin -ojsonpath='{.items[0].metadata.name}')
```

We'll use the `/config_dump` endpoint on the Envoy proxy container in the pod to get a full Envoy configuration dump:

```shell
kubectl exec -it $HTTPBIN_POD -c istio-proxy -- curl localhost:15000/config_dump > envoy.json
```

Because the configuration is enormous, let's search for the `type.googleapis.com/envoy.extensions.filters.network.wasm.v3.Wasm`. Here's a snippet:

??? tldr "wasm-filter.json"
    ```json linenums="1"
    --8<-- "wasm/wasmfilter.json"
    ```

!!! Note
    There will be more than one instance of the `type.googleapis.com/envoy.extensions.filters.network.wasm.v3.Wasm` filter in the configuration. The `envoy.wasm.stats` extension gets executed on multiple paths for multiple listeners.

The `istio.stats` extension is a Wasm extension built into Envoy. How do we know that? Well, the built-in extensions use the `envoy.wasm.runtime.null` runtime. If we wanted to run our Wasm extension, we could bundle it with Envoy. However, there are easier ways to do this.

We can tell Envoy to load a Wasm extension from a specific `.wasm` file we provide in the configuration. We don't have to rebuild Envoy and maintain our Envoy binary.

### Using the Wasm filter

To configure a Wasm extension, we use a HTTP filter called `envoy.extensions.filters.network.wasm.v3.Wasm`. Since this is a HTTP filter, we know we have to configure it inside the `http_filters` section right before the router filter (`envoy.filters.http.router`).

Let's use the Envoy configuration we're already familiar with and see if we can figure out how to configure Envoy to load a Wasm extension.

```yaml linenums="1" hl_lines="9-20"
...
    filter_chains:
    - filters:
      - name: envoy.filters.network.http_connection_manager
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          stat_prefix: hello_world_service
          http_filters:
          - name: envoy.filters.http.wasm
            typed_config:
              "@type": type.googleapis.com/udpa.type.v1.TypedStruct
              type_url: type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm
              value:
                config:
                  vm_config:
                    vm_id: "my_vm"
                    runtime: "envoy.wasm.runtime.v8" # (1)
                    code:
                      local:
                        filename: "main.wasm" # (2)
          - name: envoy.filters.http.router
          route_config:
          ...
```

1. To tell Envoy our extension is not built-in, we use the `envoy.wasm.runtime.v8` runtime.
2. We provide the `main.wasm` file that contains our extension. Note that we could replace `local` with `remote` and point to an URL instead.

Make sure you run the two Docker containers from the first section:

```shell
docker run -d -p 8100:80 kennethreitz/httpbin
docker run -d -p 8200:80 kennethreitz/httpbin
```

Because we'll be building a Wasm extension, let's create a separate folder for it so that we can store all files in the same place:

```shell
mkdir wasm-extension && cd wasm-extension
```

We can now run func-e with the following configuration:

!!! tldr "envoy-config.yaml"
    ```yaml linenums="1" hl_lines="15-26"

    --8<-- "wasm/envoy-config.yaml"
    ```

```shell
func-e run -c envoy-config.yaml
```

```console
[2022-04-28 19:03:46.607][2672][critical][main] [source/server/server.cc:114] error initializing configuration 'envoy-config.yaml': Invalid path: main.wasm
[2022-04-28 19:03:46.607][2672][info][main] [source/server/server.cc:891] exiting
Invalid path: main.wasm
```

It should fail because there's no `main.wasm` file. Let's build one!

### Building a Wasm Extension

We'll build a simple Wasm extension that adds a custom response HTTP header to all requests.

From the `wasm-extension` folder, let's initialize the Go module:

```shell
go mod init wasm-extension
```

Next, let's create the `main.go` file where the code for our Wasm extension will live:

```go linenums="1" title="main.go" hl_lines="35-47"
--8<-- "wasm/main.go"
```

Save the above to `main.go`.

In the `main.go` file we defined a couple of functions that will be called by Envoy when the extension is loaded or when the requests are being processed. The part where we add the custom response header is in the `OnHttpResponseHeaders` function, as shown below:

```golang linenums="1"
func (ctx *httpContext) OnHttpResponseHeaders(numHeaders int, endOfStream bool) types.Action {
  proxywasm.LogInfo("OnHttpResponseHeaders") // (1)

  key := "x-custom-header"
  value := "custom-value"

  if err := proxywasm.AddHttpResponseHeader(key, value); err != nil { // (2)
    proxywasm.LogCriticalf("failed to add header: %v", err)
    return types.ActionPause // (3)
  }
  proxywasm.LogInfof("header set: %s=%s", key, value)
  return types.ActionContinue // (4)
}
```

1. ProxyWasm library has built-in functions for logging.
2. We can `AddHttpResponseHeader` to add a custom response header.
3. In case of an error, we return `types.ActionPause` to tell Envoy to stop executing subsequent filters.
4. If there are no errors, we continue with the execution.

!!! note "Proxy Wasm Go SDK API"

    The SDK API is in the `proxywasm` package included in the source code. The SDK provides a set of functions we can use to interact with the Envoy proxy and/or the requests and responses. It contains functions for adding and manipulating HTTP headers, body, logging functions, and other APIs for using shared queues, shared data, and more.

To build the extension, we'll use the [TinyGo compiler](https://tinygo.org) - follow [these instructions](https://tetratelabs.github.io/wasm-workshop/1_prerequisites/#installing-tinygo){target=_blank} to install TinyGo.

With TinyGo installed, we can download the dependencies and build the extension:

```shell
go mod tidy
tinygo build -o main.wasm -scheduler=none -target=wasi main.go
```

```console
go: finding module for package github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types
go: finding module for package github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm
go: found github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm in github.com/tetratelabs/proxy-wasm-go-sdk v0.17.0
go: found github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types in github.com/tetratelabs/proxy-wasm-go-sdk v0.17.0
```

The `build` command should run successfully and generate a `main.wasm` file.

We already have the Envoy config, so let's re-run `func-e`:

```shell
func-e run -c envoy-config.yaml &
```

This time we won't get any errors because the `main.wasm` file we referenced in the configuration exists.

Let's try sending a couple of requests to `localhost:10000/one` to see the custom header we added to the response and the log entries.

```shell
curl -v http://localhost:10000/one
```

```console linenums="1" hl_lines="20"
*   Trying 127.0.0.1:10000...
* Connected to localhost (127.0.0.1) port 10000 (#0)
> GET /one HTTP/1.1
> Host: localhost:10000
> User-Agent: curl/7.74.0
> Accept: */*
>
[2022-04-28 19:19:39.191][4295][info][wasm] [source/extensions/common/wasm/context.cc:1167] wasm log my_vm: NewHttpContext
[2022-04-28 19:19:39.194][4295][info][wasm] [source/extensions/common/wasm/context.cc:1167] wasm log my_vm: OnHttpResponseHeaders
[2022-04-28 19:19:39.194][4295][info][wasm] [source/extensions/common/wasm/context.cc:1167] wasm log my_vm: header set: x-custom-header=custom-value
* Mark bundle as not supporting multiuse
< HTTP/1.1 200 OK
< server: envoy
< date: Thu, 28 Apr 2022 19:19:39 GMT
< content-type: application/json
< content-length: 29
< access-control-allow-origin: *
< access-control-allow-credentials: true
< x-envoy-upstream-service-time: 1
< x-custom-header: custom-value
<
{
  "origin": "172.18.0.1"
}
* Connection #0 to host localhost left intact
```

Running the Wasm extension like this is helpful. However, we want to run it next to the Envoy proxies in the Istio service mesh.

## Istio WasmPlugin resource

The WasmPlugin allows us to select the workloads we want to apply the Wasm module to and point to the Wasm module.

The WasmPlugin resource includes a feature that enables the Istio proxy (or istio-agent) to download the Wasm file from an OCI-compliant registry. That means we can treat the Wasm files like we treat Docker images. We can push them to a registry, version them using tags, and reference them from the WasmPlugin resource.

There was no need to push or publish the `main.wasm` file anywhere in the previous labs, as it was accessible by the Envoy proxy because everything was running locally. However, now that we want to run the Wasm module in Envoy proxies that are part of the Istio service mesh, we need to make the `main.wasm` file available so all those proxies can load and run it.

### Building the Wasm image

Since we'll be building and pushing the Wasm file, we'll need a very minimal `Dockerfile` in the project:

```dockerfile
FROM scratch
COPY main.wasm ./plugin.wasm
```

This Docker file copies the `main.wasm` file to the container as `plugin.wasm`. Save the above contents to `Dockerfile`.

Next, we can build and push the Docker image:

```shell
export REPOSITORY=[REPOSITORY]
docker build -t ${REPOSITORY}/wasm:v1 .
docker push ${REPOSITORY}/wasm:v1
```

??? note "Setting up your registry"
    You can use any OCI-compliant registry to host your Wasm files. For example, you can use [Docker Hub](https://hub.docker.com), or if you're using GCP, you can set up the [Docker registry here](https://console.cloud.google.com/artifacts), by clicking the **Create Repository** button, selecting the Docker format and clicking **Create**. Then, follow the setup instructions to complete setting up the GCP registry, and don't forget to [configure access control](https://cloud.google.com/artifact-registry/docs/access-control), so you can push to it and Istio can pull from it.

You can also use the pre-built images that's available here: `europe-west8-docker.pkg.dev/peterjs-project/kubecon2022/wasm:v1`.

### Creating WasmPlugin resource

We can now create the WasmPlugin resource that tells Envoy where to download the extension and which workloads to apply it to (we'll use `httpbin` workload we deployed in the previous lab). 

???+ note "WasmPlugin resource"
    ```go linenums="1" title="plugin.yaml"
    --8<-- "wasm/plugin.yaml"
    ```

You should update the `REPOSITORY` value in the `url` field before saving the above YAML to `plugin.yaml` and deploying it using `kubectl apply -f plugin.yaml`.

Let's try out the deployed Wasm module!

Capture the gateway's external IP address:

```shell
GATEWAY_IP=$(kubectl get service istio-ingressgateway -n istio-system -ojsonpath='{.status.loadBalancer.ingress[0].ip}')
```

Because we applied the WasmPlugin to the first `httpbin` deployment (see the selector labels in the WasmPlugin resource), we can send the request to `$GATEWAY_IP/one`:

```shell
curl -v $GATEWAY_IP/one
```

```console linenums="1" hl_lines="15"
> GET /one HTTP/1.1
> Host: 34.82.240.26
> User-Agent: curl/7.74.0
> Accept: */*
>
* Mark bundle as not supporting multiuse
< HTTP/1.1 200 OK
< server: istio-envoy
< date: Thu, 28 Apr 2022 19:33:48 GMT
< content-type: application/json
< content-length: 32
< access-control-allow-origin: *
< access-control-allow-credentials: true
< x-envoy-upstream-service-time: 34
< x-custom-header: custom-value
<
{
  "origin": "10.138.15.210"
}
```

## Summary

In this lab, you learned how to create and configure a Wasm extension using Go and the proxy-wasm-go-sdk. You've learned how to run a single Envoy proxy that loads a Wasm extension and use the WasmPlugin resource to deploy the Wasm extension to Envoy proxies inside the Istio service mesh.