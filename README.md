# Kubernetes Image Mapper

This consists of a MutatingAdmissionWebhook which rewrites kubernetes pods to use relocated image references.
The mapping from original to relocated image references is built by deploying `imagemap` custom resources.

Each `imagemap` is namespaced and applies only to pods in the same namespace.

When an `imagemap` is deployed, if it is inconsistent with other `imagemaps` in the namespace, it is rejected
and the status of the `imagemap` details the inconsistency (in a `Ready` condition with status `false`).
After the inconsistency has been corrected, the rejected `imagemap` is automatically redeployed after a
short delay (currently one minute).

If an `imagemap` is updated and this results in the `imagemap` being rejected, the original `imagemap` is undeployed.

*TODO: the webhook has not yet been implemented*

## Usage

The following was tested using a GKE cluster.

* Install Jetstack certificate manager:
```
kubectl create namespace cert-manager && \
kubectl label namespace cert-manager certmanager.k8s.io/disable-validation=true && \
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v0.10.1/cert-manager.yaml
```

* If you want the controller and webhook to provide detailed logs, enable debug logging in the currently active patch
file, such as`config/default/manager_auth_proxy_patch.yaml`, like this:
```
      ...
      - name: manager
        args:
        - "--metrics-addr=127.0.0.1:8080"
        - "--enable-leader-election"
        - "--debug"
      ...
```

* Build and deploy the webhook:
```
make docker-build docker-push IMG=<some-registry>/<project-name>:tag &&
make deploy IMG=<some-registry>/<project-name>:tag
```

* Deploy a sample imagemap custom resource, after editing it to replace `<repo prefix>` with a suitable repository
prefix (e.g. `gcr.io/my-sandbox`) to which you and the cluster have access:
```
# remember to edit config/samples/mapper_v1alpha1_imagemap.yaml
kubectl apply -f config/samples/mapper_v1alpha1_imagemap.yaml
```

* Create a pod, e.g.:
```
kubectl run kubernetes-bootcamp --image=gcr.io/google-samples/kubernetes-bootcamp:v1 --port=8080
```

* Observe the relocated image in the pod, e.g.:
```
kubectl get pod kubernetes-bootcamp-xxx -oyaml
```
Output:
```
...
spec:
  containers:
  - image: <repo prefix>/kubernetes-bootcamp:v1
...
```

Note: the `image` value under `containerStatuses` may not be the relocated value. This is a [known issue](https://github.com/kubernetes/kubernetes/issues/51017) when an image has multiple references referring to it. 

* View the logs from the webhook, e.g.:
```
kubectl logs ir-webhook-xxx -n image-relocation
```

* Now tidy up:
```
kubectl delete deployment kubernetes-bootcamp
kubectl delete -f config/samples/mapper_v1alpha1_imagemap.yaml
make deploy IMG=<some-registry>/<project-name>:tag
```
