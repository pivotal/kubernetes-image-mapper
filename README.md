# Kubernetes Image Mapper Prototype

The goal of this repository is to allow a kubernetes application to be deployed with images which
have been moved to a private registry, but _without editing_ the application configuration. A webhook rewrites
the image references in the application's pods according to a mapping which is configured using custom resources.

To do:
- [ ] Additional features - see [issues](https://github.com/pivotal/kubernetes-image-mapper/issues)

For more context, please see the image relocation repository's [README](https://github.com/pivotal/image-relocation).

## Details
This repository consists of a MutatingAdmissionWebhook which rewrites kubernetes pods to use relocated image
references.
The mapping from original to relocated image references is built by deploying `imagemap` custom resources
which are processed by a controller also provided by this repository.

Each `imagemap` is namespaced and applies only to pods in the same namespace.

When an `imagemap` is deployed, if it is inconsistent with other `imagemaps` in the namespace, it is rejected
and the status of the `imagemap` details the inconsistency (in a `Ready` condition with status `false`).
After the inconsistency has been corrected, the rejected `imagemap` is automatically redeployed after a
short delay (currently one minute).

If an `imagemap` is updated and this results in the `imagemap` being rejected, the original `imagemap` is undeployed.

## Usage

The following was tested using a GKE cluster.

* Install Jetstack certificate manager:
```
kubectl create namespace cert-manager && \
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v0.12.0/cert-manager.yaml
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

* Observe the image map resources:
```
kubectl get imagemaps
```
Output:
```
NAME              AGE
bootcamp-sample   1m
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
kubectl logs image-mapper-controller-manager-xxx -n image-mapper-system
```

* Now tidy up:
```
kubectl delete deployment kubernetes-bootcamp
kubectl delete -f config/samples/mapper_v1alpha1_imagemap.yaml
make undeploy IMG=<some-registry>/<project-name>:tag
kubectl delete -f https://github.com/jetstack/cert-manager/releases/download/v0.12.0/cert-manager.yaml
```
