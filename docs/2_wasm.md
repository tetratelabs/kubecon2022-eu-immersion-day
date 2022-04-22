# 3. Extending Envoy and Istio with Wasm

We'll create a minimal Wasm extension in this lab and run it locally using Envoy.

We'll start by creating a new folder for our extension, initializing the Go module, and downloading the SDK dependency:

```shell
mkdir wasm-extension && cd wasm-extension
go mod init wasm-extension
```

Next, let's create the `main.go` file where the code for our Wasm extension will live. We'll start with the minimal code:

```go linenums="1" title="main.go"
--8<-- "minimal_main.go"
```

Save the above to `main.go`.

Let's download the dependencies and then we can build the extension to check everything is good:

```shell
# Download the dependencies
go mod tidy

# Build the wasm file
tinygo build -o main.wasm -scheduler=none -target=wasi main.go
```

The build command should run successfully and generate a file called `main.wasm`.

We'll use `func-e` to run a local Envoy instance to test our built extension.

First, we need an Envoy config that will configure the extension:

```yaml linenums="1" title="envoy.yaml"
--8<-- "envoy.yaml"
```

Save the above to `envoy.yaml`.

The Envoy configuration sets up a single listener on port 18000 that returns a direct response (HTTP 200) with body `hello world`. Inside the `http_filters` section, we're configuring the `envoy.filters.http.wasm` filter and referencing the local WASM file (`main.wasm`) we've built earlier.

Let's run the Envoy with this configuration in the background:

```shell
func-e run -c envoy.yaml &
```

Envoy instance should start without any issues. Once it's started, we can send a request to the port Envoy is listening on (`18000`):

```shell
curl localhost:18000
```

```console
[2022-01-25 22:21:53.493][5217][info][wasm] [source/extensions/common/wasm/context.cc:1167] wasm log: NewHttpContext
hello world
```

The output shows the single log entry coming from the Envoy proxy. This is the `LogInfo` function we called in the `NewHttpContext` callback. The `NewHttpContext` is called for each new HTTP stream. Similarly, a `NewTcpContext` method gets called for each new TCP connection.

# Configuration and headers

We'll learn how to add additional headers to HTTP responses in this lab. We'll use the `main.go` file created in the previous lab.

## Adding HTTP response header

We'll create the `OnHttpResponseHeaders` function, and within the function, we'll add a new response header using the `AddHttpResponseHeader` from the SDK.

Create the `OnHttpResponseHeaders` function that looks like this:

```go
func (ctx *httpContext) OnHttpResponseHeaders(numHeaders int, endOfStream bool) types.Action {
  proxywasm.LogInfo("OnHttpResponseHeaders")
  err := proxywasm.AddHttpResponseHeader("my-new-header", "some-value-here")
  if err != nil {
    proxywasm.LogCriticalf("failed to add response header: %v", err)
  }
  return types.ActionContinue
}
```

Let's rebuild the extension:

```shell
tinygo build -o main.wasm -scheduler=none -target=wasi main.go
```

And we can now re-run the Envoy proxy with the updated extension:

```shell
func-e run -c envoy.yaml &
```

Now, if we send a request again (make sure to add the `-v` flag), we'll see the header that got added to the response:

```shell
curl -v localhost:18000
```

```console
...
< HTTP/1.1 200 OK
< content-length: 13
< content-type: text/plain
< my-new-header: some-value-here
< date: Mon, 22 Jun 2021 17:02:31 GMT
< server: envoy
<
hello world
```

You can stop running Envoy by typing `fg` and pressing ++ctrl+c++.

## Reading values from configuration

Hard-coded values in code are never a good idea. Let's see how we could read the additional headers from the configuration we provided in the Envoy configuration file.

Let's start by adding the plumbing that will allow us to read the headers from the configuration.

1. Add the `additionalHeaders` and `contextID` to the `pluginContext` struct:

    ```go hl_lines="5-6"
    type pluginContext struct {
      // Embed the default plugin context here,
      // so that we don't need to reimplement all the methods.
      types.DefaultPluginContext
      additionalHeaders map[string]string
      contextID         uint32
    }
    ```

!!! note
    The `additionalHeaders` variables is a map of strings that will store header keys and values we'll read from the configuration.

1. Update the `NewPluginContext` function to return the plugin context with initialized variables:

    ```go
    func (*vmContext) NewPluginContext(contextID uint32) types.PluginContext {
      return &pluginContext{contextID: contextID, additionalHeaders: map[string]string{}}
    }
    ```

As for reading the configuration - we have two options - we can read the configuration set at the plugin level using the `GetPluginConfiguration` function or at the VM level using the `GetVMConfiguration` function.

Typically, you'd read the configuration when the plugin starts (`OnPluginStart`) or when the VM starts (`OnVMStart`).

### Parsing JSON from config file

Let's add the `OnPluginStart` function where we read in values from the Envoy configuration and store the key/value pairs in the `additionalHeaders` map. We'll use the [fastjson library](https://github.com/valyala/fastjson) (`github.com/valyala/fastjson`) to parse the JSON string:

```go
func (ctx *pluginContext) OnPluginStart(pluginConfigurationSize int) types.OnPluginStartStatus {
  data, err := proxywasm.GetPluginConfiguration() // (1)
  if err != nil {
    proxywasm.LogCriticalf("error reading plugin configuration: %v", err)
  }

  var p fastjson.Parser
  v, err := p.ParseBytes(data)
  if err != nil {
    proxywasm.LogCriticalf("error parsing configuration: %v", err)
  }

  obj, err := v.Object()
  if err != nil {
    proxywasm.LogCriticalf("error getting object from json value: %v", err)
  }

  obj.Visit(func(k []byte, v *fastjson.Value) {
    ctx.additionalHeaders[string(k)] = string(v.GetStringBytes()) // (2)
  })

  return types.OnPluginStartStatusOK
}
```

1. We use the `GetPluginConfiguration` function to read the configuration section from the Envoy config file.
2. We iterate through all key/value pairs from the configuration and store them in the `additionalHeaders` map.

!!! note 
    Make sure to add the `github.com/valyala/fastjson` to the import statements at the top of the file and run `go mod tidy` to download the dependency.

To access the configuration values we've set, we need to add the map to the HTTP context when we initialize it. To do that, we need to update the `httpContext` struct first and add the `additionalHeaders` map:

```go hl_lines="6"
type httpContext struct {
  // Embed the default http context here,
  // so that we don't need to reimplement all the methods.
  types.DefaultHttpContext
  contextID         uint32
  additionalHeaders map[string]string
}
```

Then, in the `NewHttpContext` function we can instantiate the `httpContext` with the `additionalHeaders` map coming from the `pluginContext`:

```go
func (ctx *pluginContext) NewHttpContext(contextID uint32) types.HttpContext {
  return &httpContext{contextID: contextID, additionalHeaders: ctx.additionalHeaders}
}
```

### Calling `AddHttpResponseHeader`

Finally, in order to set the headers we modify the `OnHttpResponseHeaders` function, iterate through the `additionalHeaders` map and call the `AddHttpResponseHeader` for each item:

```go
func (ctx *httpContext) OnHttpResponseHeaders(numHeaders int, endOfStream bool) types.Action {
  proxywasm.LogInfo("OnHttpResponseHeaders")

  for key, value := range ctx.additionalHeaders { // (1)
    if err := proxywasm.AddHttpResponseHeader(key, value); err != nil { // (2)
        proxywasm.LogCriticalf("failed to add header: %v", err)
        return types.ActionPause
    }
    proxywasm.LogInfof("header set: %s=%s", key, value) // (3)
  }

  return types.ActionContinue
}
```

1. Iterate through the `additionalHeaders` map created in the `NewPluginContext`. The map contains the data from the config file we parsed in the `OnPluginStart` function

2. We call the `AddHttpResponseHeader` to set the response headers
3. We also log out the header key and value

??? note "Complete main.go file"
    ```go linenums="1" title="main.go"
    --8<-- "config_main.go"
    ```

Let's rebuild the extension again:

```shell
tinygo build -o main.wasm -scheduler=none -target=wasi main.go
```

We also have to update the config file to include additional headers in the filter configuration (the `configuration` field):

```yaml hl_lines="12-18"
- name: envoy.filters.http.wasm
  typed_config:
    '@type': type.googleapis.com/udpa.type.v1.TypedStruct
    type_url: type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm
    value:
      config:
        vm_config:
          runtime: 'envoy.wasm.runtime.v8'
          code:
            local:
              filename: 'main.wasm'
        configuration:
          '@type': type.googleapis.com/google.protobuf.StringValue
          value: |
            { 
              "header_1": "somevalue", 
              "header_2": "secondvalue"
            }
```

??? note "Complete envoy.yaml file"
    ```go linenums="1" title="envoy.yaml"
    --8<-- "envoy-config.yaml"
    ```

We can re-run the Envoy proxy with the updated configuration using `func-e run -c envoy.yaml &`.

This time, when we send a request, we'll notice the headers we set in the extension configuration is added to the response:

```shell
curl -v localhost:18000
```

```console hl_lines="5-6"
...
< HTTP/1.1 200 OK
< content-length: 13
< content-type: text/plain
< header_1: somevalue
< header_2: secondvalue
< date: Mon, 22 Jun 2021 17:54:53 GMT
< server: envoy
...
```

You can stop running Envoy by bringing it to the foreground using `fg` and pressing ++ctrl+c++.

# Istio WasmPlugin

We'll create a WasmPlugin resource in this lab and deploy it to the Kubernetes cluster.

The WasmPlugin allows us to select the workloads we want to apply the Wasm module to and point to the Wasm module.

??? note "What about EnvoyFilter?" 
    In previous Istio versions, we'd have to use the EnvoyFilter resource to configure the Wasm plugins. We could either point to a local Wasm file (i.e., file accessible by the Istio proxy) or a remote location. Using the remote location (e.g. `http://some-storage-account/main.wasm`), the Istio proxy would download the Wasm file and cache it in the volume accessible to the proxy.

The WasmPlugin resource includes a feature that enables Istio proxy (or istio-agent to be precise) to download the Wasm file from an OCI-compliant registry. That means we can treat the Wasm files just like we treat Docker images. We can push them to a registry, version them using tags, and reference them from the WasmPlugin resource.

There was no need to push or publish the `main.wasm` file anywhere in the previous labs, as it was accessible by the Envoy proxy because everything was running locally. However, now that we want to run the Wasm module in Envoy proxies that are part of the Istio service mesh, we need to make the `main.wasm` file available to all those proxies they can load and run it.

## Building the Wasm image

Since we'll be building and pushing the Wasm file, we'll need a very minimal Dockerfile in the project:

```dockerfile
FROM scratch
COPY main.wasm ./plugin.wasm
```

This Docker file copies the `main.wasm` file to the container as `plugin.wasm`. Save the above contents to `Dockerfile`.

Next, we can build and push the Docker image:

```shell
docker build -t [REPOSITORY]/wasm:v1 .
docker push [REPOSITORY]/wasm:v1
```

## Creating WasmPlugin resource

We can now create the WasmPlugin resource that tells Envoy where to download the extension from and which workloads to apply it to (we'll use `httpbin` workload we'll deploy next). 

???+ note "WasmPlugin resource"
    ```go linenums="1" title="plugin.yaml"
    --8<-- "plugin.yaml"
    ```

You should update the `REPOSITORY` value in the `url` field before saving the above YAML to `plugin.yaml` and deploying it using `kubectl apply -f plugin.yaml`.

We'll deploy a sample workload to try out the Wasm extension. We'll use `httpbin`. Make sure the `default` namespace is labeled for Istio sidecar injection (`kubectl label ns default istio-injection=enabled`) and then deploy the `httpbin`:

??? note "httpbin.yaml"
    ```yaml
    apiVersion: v1
    kind: ServiceAccount
    metadata:
      name: httpbin
    ---
    apiVersion: v1
    kind: Service
    metadata:
      name: httpbin
      labels:
        app: httpbin
        service: httpbin
    spec:
      ports:
      - name: http
        port: 8000
        targetPort: 80
      selector:
        app: httpbin
    ---
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: httpbin
    spec:
      replicas: 1
      selector:
        matchLabels:
          app: httpbin
          version: v1
      template:
        metadata:
          labels:
            app: httpbin
            version: v1
        spec:
          serviceAccountName: httpbin
          containers:
          - image: docker.io/kennethreitz/httpbin
            imagePullPolicy: IfNotPresent
            name: httpbin
            ports:
            - containerPort: 80
    ```

Save the above YAML to `httpbin.yaml` and deploy it using `kubectl apply -f httpbin.yaml`.

Before continuing, check that the `httpbin` Pod is up and running:

```shell
$ kubectl get po
```

```console
NAME                       READY   STATUS        RESTARTS   AGE
httpbin-66cdbdb6c5-4pv44   2/2     Running       1          11m
```

To see if something went wrong with downloading the Wasm module, you can look at the proxy logs.

Let's try out the deployed Wasm module!

We will create a single Pod inside the cluster, and from there, we will send a request to `http://httpbin:8000/get` and include the `hello` header.

```shell
kubectl run curl --image=curlimages/curl -it --rm -- /bin/sh
```

```console
Defaulted container "curl" out of: curl, istio-proxy, istio-init (init)
If you don't see a command prompt, try pressing enter.
/ $
```

Once you get the prompt to the curl container, send a request to the `httpbin` service:

```shell
curl -v -H "hello: something" http://httpbin:8000/headers
```

```console
> GET /headers HTTP/1.1
> User-Agent: curl/7.35.0
> Host: httpbin:8000
> Accept: */*
>
< HTTP/1.1 200 OK
< server: envoy
< date: Mon, 22 Jun 2021 18:52:17 GMT
< content-type: application/json
< content-length: 525
< access-control-allow-origin: *
< access-control-allow-credentials: true
< x-envoy-upstream-service-time: 3
...
```

If we exit the pod and look at the stats, we'll notice that the `hello_header_counter` has increased:

```shell
kubectl exec -it [httpbin-pod-name] -c istio-proxy -- curl localhost:15000/stats/prometheus | grep hello
```

```console
# TYPE hello_header_counter counter
hello_header_counter{} 1
```
## Cleanup

To delete all resources created during this lab, run the following:

```shell
kubectl delete wasmplugin wasm-example
kubectl delete deployment httpbin
kubectl delete svc httpbin
kubectl delete sa httpbin
```