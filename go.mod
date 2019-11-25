module github.com/pivotal/kubernetes-image-mapper

go 1.13

require (
	github.com/go-logr/logr v0.1.0
	github.com/onsi/ginkgo v1.10.3
	github.com/onsi/gomega v1.7.0
	gomodules.xyz/jsonpatch/v2 v2.0.1
	k8s.io/api v0.17.1
	k8s.io/apiextensions-apiserver v0.17.1 // indirect
	k8s.io/apimachinery v0.17.1
	k8s.io/client-go v0.17.1
	k8s.io/code-generator v0.17.1
	sigs.k8s.io/controller-runtime v0.4.0
	sigs.k8s.io/controller-tools v0.2.4 // indirect
)
