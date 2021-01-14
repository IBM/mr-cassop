module github.com/ibm/cassandra-operator

go 1.13

require (
	github.com/caarlos0/env/v6 v6.2.1
	github.com/go-logr/zapr v0.1.0
	github.com/gocql/gocql v0.0.0-20201215165327-e49edf966d90
	github.com/gogo/protobuf v1.3.1
	github.com/golang/mock v1.4.4
	github.com/google/go-cmp v0.4.0
	github.com/google/go-querystring v1.0.0
	github.com/onsi/ginkgo v1.14.2
	github.com/onsi/gomega v1.10.4
	github.com/pkg/errors v0.8.1
	go.uber.org/zap v1.10.0
	k8s.io/api v0.18.2
	k8s.io/apimachinery v0.18.2
	k8s.io/client-go v0.18.2
	sigs.k8s.io/controller-runtime v0.6.0
	sigs.k8s.io/yaml v1.2.0
)
