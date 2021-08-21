---
title: Development
slug: /development
---

### Requirements:

* Kubernetes 1.16 or newer. You can use [minikube](https://kubernetes.io/docs/setup/minikube/) or [kind](https://github.com/kubernetes-sigs/kind) for local development.
* Go 1.12+ with enabled go modules
* [OperatorSDK](https://github.com/operator-framework/operator-sdk) 1.0.0+
* [kustomize](https://github.com/kubernetes-sigs/kustomize) 3.1.0+
* [helm](https://helm.sh/) v2.14.3+
* [docker](https://docs.docker.com/install/)
* [goimports](https://godoc.org/golang.org/x/tools/cmd/goimports)
* [GolangCI-Lint](https://github.com/golangci/golangci-lint) 1.19.1+
* [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder) to setup test environment
* [gomock](https://github.com/golang/mock)

## Run Operator Locally

The operator can be run only using the whole Helm chart with all necessary components. The operator interacts with Cassandra clusters so it has to live in the cluster. It is not possible to run the operator having the binary locally (the `make run` way).

To run your code with changes, you need to build the docker image and deploy it to the cluster. It can be done by manually building and pushing the container image, or using [skaffold](https://skaffold.dev/). See example below:

```bash
export YOUR_USERNAME=anton
skaffold run
skaffold apply ./config/samples/cassandracluster.yaml
```

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

## Tests

### Integration and unit tests

To run the tests, simply run `make tests` from the command line.

It will run the usual Go unit and integration tests, which utilize [testenv](https://book.kubebuilder.io/reference/envtest.html) to execute the test against on a semi-real k8s cluster. `testenv` requires assets that are installed with `kubebuilder`, so it must installed first.

Integration tests use the Ginkgo framework and are located in `./test/integration`. Ginkgo has its own [options]([Ginkgo flags](https://onsi.github.io/ginkgo/#the-ginkgo-cli)) that can be arguments to `go test`. For example, to run a specific test (`--focus` option):

`go test ./test/integration -ginkgo.focus="regex matcher"` 

For debugging you might want to see operator logs as the tests run. Pass the `-enableOperatorLogs=true` option to your  `go test ...` command to enable it:

`go test ./tests/integration -v -enableOperatorLogs=true`

or

`ginkgo -v -r ./tests/integration -- -enableOperatorLogs=true`

if you use Ginkgo CLI.

### E2E tests

E2E tests run on a k8s cluster. These tests deploy the C* Custom Resource Definition (CRD) in the k8s cluster.

>Note: before running, make sure the Cassandra operator is deployed in your k8s namespace. 

To run e2e tests:

```
make e2e-tests
```

## Docs

To download and run docs locally, clone the repo and then go to the docs directory:

```console
cd ./cassandra-operator/docs
```

If `npm` is not already installed it can be installed with:

```console
brew install npm
```

Then install the dependencies:

```console
npm install
```

To build and serve the documentation locally, run the command:

```console
npm start
```
