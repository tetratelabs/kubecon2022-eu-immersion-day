# Install Istio

In this lab you will install Istio.


## Download Istio

1. Run the following command from your home directory.

    ```{.shell .language-shell}
    curl -L https://istio.io/downloadIstio | ISTIO_VERSION={{istio.version}} sh -
    ```

1. Navigate into the directory created by the above command.

    ```{.shell .language-shell}
    cd istio-{{istio.version}}
    ```


## Add `istioctl` to your PATH

The `istioctl` CLI is located in the `bin/` subdirectory.

!!! note

    Cloud Shell only preserves files located inside your home directory across sessions.

    This means that if you install a binary to a `PATH` such as `/usr/local/bin`, after your session times out that file will no longer be there!

    As a workaround, you will add `${HOME}/bin` to your `PATH` and place the binary there.


1. Create a `bin` subdirectory in your home directory:

    ```{.shell .language-shell}
    mkdir ~/bin
    ```

1. Copy the CLI to that subdirectory:

    ```{.shell .language-shell}
    cp ./bin/istioctl ~/bin
    ```

1. Add your home `bin` subdirectory to your `PATH`

    ```shell
    cat << EOF >> ~/.bashrc

    export PATH="~/bin:\$PATH"

    EOF
    ```

    And then:

    ```shell
    source ~/.bashrc
    ```

Verify that `istioctl` is installed with:

```{.shell .language-shell}
istioctl version
```

With the CLI installed, proceed to install Istio to Kubernetes.

## Install Istio

1. Istio can be installed directly with the CLI:

    ```{.shell .language-shell}
    istioctl install
    ```

1. When prompted, enter `y` to proceed to install Istio.

Take a moment to learn more about [Istio installation profiles](https://istio.io/latest/docs/setup/additional-setup/config-profiles/){target=_blank}.

## Verify that Istio is installed

1. List Kubernetes namespaces and note the new namespace `istio-system`

    ```{.shell .language-shell}
    kubectl get ns
    ```

1. Verify that the `istiod` controller pod is running in that namespace

    ```{.shell .language-shell}
    kubectl get pod -n istio-system
    ```

1. Re-run `istioctl version`.  The output should include a _control plane_ version, indicating that Istio is indeed present in the cluster.
