CONTROLLER_GEN_VERSION = v0.8.0
KUSTOMIZE_VERSION = v3.5.4

CLOUD_PROVIDER ?= ibm
E2E_TIMEOUT ?= 1h30m

IMAGE_PULL_SECRET ?= icm-coreeng-pull-secret
STORAGE_CLASS_NAME ?= ibmc-block-gold

# Obtain k8s cluster name
K8S_CLUSTER ?= $(shell kubectl config current-context | cut -f1 -d"/")
# Obtain k8s namespace
K8S_NAMESPACE ?= $(shell kubectl config view --minify -o json | jq '.contexts[].context.namespace')

ifeq ($(CLOUD_PROVIDER),ibm)
# Obtain IMB cloud ingress domain
INGRESS_SECRET = $(shell ibmcloud ks cluster get --cluster=$(K8S_CLUSTER) --json | jq '.ingressSecretName')
# Obtain IMB cloud ingress domain
INGRESS_DOMAIN = $(shell ibmcloud ks cluster get --cluster=$(K8S_CLUSTER) --json | jq '.ingressHostname')
endif
