module github.com/ibm/cassandra-operator

go 1.16

require (
	github.com/caarlos0/env/v6 v6.2.1
	github.com/go-logr/zapr v0.4.0
	github.com/gocql/gocql v0.0.0-20201215165327-e49edf966d90
	github.com/gogo/protobuf v1.3.2
	github.com/golang/mock v1.5.0
	github.com/google/go-cmp v0.5.5
	github.com/google/go-querystring v1.0.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/pkg/errors v0.9.1
	go.uber.org/zap v1.17.0
	golang.org/x/net v0.0.0-20210428140749-89ef3d95e781
	k8s.io/api v0.21.2
	k8s.io/apiextensions-apiserver v0.21.2
	k8s.io/apimachinery v0.21.2
	k8s.io/client-go v0.21.2
	sigs.k8s.io/controller-runtime v0.9.2
	sigs.k8s.io/yaml v1.2.0
)
