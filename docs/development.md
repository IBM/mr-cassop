# Development

###Requirements:

* Kubernetes 1.16 or newer. You can use [minikube](https://kubernetes.io/docs/setup/minikube/) or [kind](https://github.com/kubernetes-sigs/kind) for local development.
* Go 1.12+ with enabled go modules
* [OperatorSDK](https://github.com/operator-framework/operator-sdk) 1.0.0+
* [kustomize](https://github.com/kubernetes-sigs/kustomize) 3.1.0+
* [helm](https://helm.sh/) v2.14.3+
* [docker](https://docs.docker.com/install/)
* [goimports](https://godoc.org/golang.org/x/tools/cmd/goimports)
* [GolangCI-Lint](https://github.com/golangci/golangci-lint) 1.19.1+

## Run the operator locally

The operator can be run only using the whole Helm chart with all necessary components. The operator interacts with Cassandra clusters so it has to live in the cluster. It is not possible to run the operator having the binary locally (the `make run` way).

To run your code with changes, you need to build the docker image and deploy it to the cluster. It can be done by manually building and pushing the container image, or using [skaffold](https://skaffold.dev/).

After you have your image with the code changes in the cluster, override the default container image through Helm values override. If you're pulling the image from a private container registry, you also need to specify the image pull secret. An another option would be to get the image to the cluster and set the `imagePullPolicy` to `Never`.

Your values override could look the following:

```
container:
  image: path.to.your.image:local
  imagePullSecret: icm-coreeng-pull-secret #if private registry is used
  imagePullPolicy: Always #Set to `Never` if the image exists only locally on the cluster
logLevel: debug
logFormat: json
```

Once the cluster is up and running, use the sample in `config/samples/cassandracluster.yaml` to run your cluster. The sample can be run without additional configuration, except you need to set your image pull secret name in the spec.

`kubectl apply -f config/samples/cassandracluster.yaml`

If all set correctly, you should see the components getting created.