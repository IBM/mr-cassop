#!/bin/bash


KEYSTORE_PASSWORD=cassandra
TRUSTSTORE_PASSWORD=cassandra
DEFAULT_CA_TLS_SECRET_NAME="cluster-tls-ca"
DEFAULT_NODE_TLS_SECRET_NAME="cluster-tls-node"
CA_TLS_CRT_VALIDITY=3650
NODE_TLS_CRT_VALIDITY=1825
KEY_SIZE=4096
SERVER_CERT_CN=localhost
SERVER_ALT_NAME=localhost


function prepare() {
  echo "Creating tmp directory..."
  tmp_dir=$(mktemp -d) || { echo "Failed to create temp directory"; exit 1; }
  echo -e "${tmp_dir}\n"
}

function cleanup() {
  if [ "$tmp_dir" != "" ]; then
    echo -e "\nRemoving ${tmp_dir}..."
    rm -rf $tmp_dir
  fi
}

function ca_keypair() {
  # Create CA key
  openssl genrsa -out ${tmp_dir}/ca_key.pem $KEY_SIZE
  # Create CA certificate
  openssl req -x509 -new -nodes -key ${tmp_dir}/ca_key.pem -sha256 -days $CA_TLS_CRT_VALIDITY -out ${tmp_dir}/ca_crt.pem \
    -subj "/C=US/O=cassandra_root/OU=cassandra_root AG/CN=cassandra_ca"
}

function create_ca_tls_secret() {
  prepare

  if [ "$ca_cert" != "" ] && [ "$ca_key" != "" ]; then
    ca_crt=$(cat $ca_cert | base64)
    ca_key=$(cat $ca_key | base64)
  else
    ca_keypair
    ca_crt=$(cat ${tmp_dir}/ca_crt.pem | base64)
    ca_key=$(cat ${tmp_dir}/ca_key.pem | base64)
  fi

  if [ "$secret_name" == "" ]; then
    secret_name=$DEFAULT_CA_TLS_SECRET_NAME
  fi

  cat > ${tmp_dir}/ca_tls_secret.yaml <<EOF
---
apiVersion: v1
kind: Secret
metadata:
  name: $secret_name
type: Opaque
data:
  ca.crt: $ca_crt
  ca.key: $ca_key
EOF

  if [[ $dry_run -eq 1 ]]; then
    echo -e "\nGenerated Secret:\n"
    cat ${tmp_dir}/ca_tls_secret.yaml
  else
    kubectl apply -f ${tmp_dir}/ca_tls_secret.yaml
  fi

  cleanup
}

function node_keypair() {
  # Create Node key
  openssl genrsa -out ${tmp_dir}/key.pem $KEY_SIZE
  # Create Node certificate
  openssl req -new -key ${tmp_dir}/key.pem -out ${tmp_dir}/${SERVER_CERT_CN}.csr \
    -subj "/CN=$SERVER_CERT_CN"

  # Configuration file for generating certificates
  cat > ${tmp_dir}/${SERVER_CERT_CN}.cnf <<EOF
authorityKeyIdentifier=keyid,issuer
keyUsage=digitalSignature
extendedKeyUsage=serverAuth,clientAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = $SERVER_ALT_NAME
EOF

  # Create the server certificate signing request
  openssl x509 -req -in ${tmp_dir}/${SERVER_CERT_CN}.csr -CA ${tmp_dir}/ca_crt.pem -CAkey ${tmp_dir}/ca_key.pem -CAcreateserial \
    -out ${tmp_dir}/crt.pem -days $NODE_TLS_CRT_VALIDITY -sha256 -extfile ${tmp_dir}/${SERVER_CERT_CN}.cnf
}

function createPKCS12() {
  # Create the pkcs12 store containing the server cert and the ca trust
  openssl pkcs12 -in ${tmp_dir}/crt.pem -inkey ${tmp_dir}/key.pem -certfile ${tmp_dir}/ca_crt.pem \
    -export -out ${tmp_dir}/keystore.pkcs12 -passout pass:$KEYSTORE_PASSWORD -name $SERVER_ALT_NAME
  # Show the content of keystore
  keytool -list -storetype PKCS12 -keystore ${tmp_dir}/keystore.pkcs12 -storepass $KEYSTORE_PASSWORD

  # Openssl cannot create a pkcs12 store from cert without key. This is why we create the truststore with the keytool.
  # Create a pkcs12 truststore containing the ca cert
  keytool -importcert -storetype PKCS12 -keystore ${tmp_dir}/truststore.pkcs12 -storepass $TRUSTSTORE_PASSWORD \
    -alias ca -file ${tmp_dir}/ca_crt.pem -noprompt
  # Show the content of the truststore
  keytool -list -storetype PKCS12 -keystore ${tmp_dir}/truststore.pkcs12 -storepass $TRUSTSTORE_PASSWORD
}

function create_node_tls_secret() {
  prepare
  ca_keypair
  node_keypair
  createPKCS12

  if [ "$secret_name" == "" ]; then
    secret_name=$DEFAULT_NODE_TLS_SECRET_NAME
  fi

  cat > ${tmp_dir}/node_tls_secret.yaml <<EOF
---
apiVersion: v1
kind: Secret
metadata:
  name: $secret_name
type: Opaque
data:
  ca.crt: $(cat ${tmp_dir}/ca_crt.pem | base64)
  tls.crt: $(openssl pkcs12 -in ${tmp_dir}/keystore.pkcs12 -passin "pass:$KEYSTORE_PASSWORD" -clcerts -nokeys -nomacver | sed -ne '/-BEGIN CERTIFICATE-/,/-END CERTIFICATE-/p' | base64)
  tls.key: $(openssl pkcs12 -in ${tmp_dir}/keystore.pkcs12 -passin "pass:$KEYSTORE_PASSWORD" -nocerts -nodes -nomacver | sed -ne '/-BEGIN PRIVATE KEY-/,/-END PRIVATE KEY-/p' | base64)
  keystore.p12: $(cat ${tmp_dir}/keystore.pkcs12 | base64)
  truststore.p12: $(cat ${tmp_dir}/truststore.pkcs12 | base64)
  keystore.password: $(echo -n $KEYSTORE_PASSWORD | base64)
  truststore.password: $(echo -n $TRUSTSTORE_PASSWORD | base64)
EOF

  if [[ $dry_run -eq 1 ]]; then
    echo -e "\nGenerated Secret:\n"
    cat ${tmp_dir}/node_tls_secret.yaml
  else
    echo -e "\nApplying TLS Secret"
    kubectl apply -f ${tmp_dir}/node_tls_secret.yaml
  fi

  cleanup
}

function create_ca_keypair() {
  prepare
  ca_keypair
  echo -e "\nCA keypair has been generated in $tmp_dir"
}

function create_node_keypair() {
  prepare
  ca_keypair
  node_keypair
  echo -e "\nNode keypair has been generated in $tmp_dir"
}

function display_usage() {
  cat << EOF

NAME:
  tls.sh - This bash script generates CA and Node TLS Secrets

USAGE:
  tls.sh command [options]

COMMANDS:
  create-ca-tls-secret    Generate and deploy CA TLS Secret
  create-node-tls-secret  Generate and deploy Node TLS Secret
  create-ca-keypair       Generate CA keypair into tmp directory
  create-node-keypair     Generate signed Node keypair into tmp directory

OPTIONS:
  --secret-name           Name of the TLS Secret to generate
  --ca-cert               CA certificate file to use when generating CA TLS Secret
  --ca-key                CA key file to use when generating CA TLS Secret
  --dry-run               Produces generated by this script commands to output and don't run it in k8s.
  -h, --help              Display this help.

EOF
}

if (( "$#" == 0 )); then display_usage; fi

while (( "$#" > 0 )); do
  key=$1

  case $key in
    create-ca-tls-secret)
      command='create-ca-tls-secret'
      ;;
    create-node-tls-secret)
      command='create-node-tls-secret'
      ;;
    create-ca-keypair)
      command='create-ca-keypair'
      ;;
    create-node-keypair)
      command='create-node-keypair'
      ;;
    --secret-name)
      secret_name=$2
      shift
      ;;
    --ca-cert)
      ca_cert=$2
      shift
      ;;
    --ca-key)
      ca_key=$2
      shift
      ;;
    --dry-run)
      dry_run=1
      ;;
    -h|--help)
      display_usage
      exit 0
      ;;
    *)
      echo -e "ERROR. Invalid command or argument \"$key\". See \"$(basename $0) --help\"."
      exit 1
      ;;
  esac
  shift
done

case $command in
  create-ca-tls-secret)
    create_ca_tls_secret
    ;;
  create-node-tls-secret)
    create_node_tls_secret
    ;;
  create-ca-keypair)
    create_ca_keypair
    ;;
  create-node-keypair)
    create_node_keypair
    ;;
  *)
    echo -e "ERROR. Invalid command \"$command\". See \"$(basename $0) --help\"."
    exit 1
    ;;
esac
