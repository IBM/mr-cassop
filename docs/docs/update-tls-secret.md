---
title: Update TLS Secret
slug: /update-tls-secret
---

## Update TLS CA Secret without downtime

Whenever the TLS CA Secret is changed, the operator must regenerate the TLS Node Secret. To track TLS CA Secret changes, the operator manages a `cassandra-cluster-checksum` annotation on the secret.
The steps for rotating CA certificates without downtime are described below.

### Steps for TLS CA replacement in single region setup

1. Generate new CA keypair

```bash
./utils/tls.sh create-ca-keypair
Creating tmp directory...
/var/folders/yv/fd263_ps51z7jxstvq3l0j280000gn/T/tmp.F0D44cCf

Generating RSA private key, 4096 bit long modulus
.................................................++
...............................................++
e is 65537 (0x10001)
CA Alias: ca2

CA keypair has been generated in /var/folders/yv/fd263_ps51z7jxstvq3l0j280000gn/T/tmp.F0D44cCf
```

>**Note:** the aliases of TLS CA certificate in `truststore.jks` should be different, otherwise error `javax.net.ssl.SSLHandshakeException: Certificate signature validation failed` will occur.

Use command below to check the TLS CA certificate alias

```bash
openssl x509 -in /var/folders/yv/fd263_ps51z7jxstvq3l0j280000gn/T/tmp.F0D44cCf/ca_crt.pem -text -noout | grep Subject:
        Subject: O=ca2
```

2. Update TLS CA Secret by adding new CA certificate `ca2.crt`

>**Note:** you new certificate entry should have extension `*.crt`.

Add new CA certificate from `/var/folders/yv/fd263_ps51z7jxstvq3l0j280000gn/T/tmp.F0D44cCf/ca_crt.pem` as `ca2.crt`
```bash
apiVersion: v1
data:
  ca.crt: <ca1base64data>
  ca.key: <ca1base64data>
  ca2.crt: <ca2base64data>
kind: Secret
metadata:
  name: example-cluster-manual-tls-ca
  namespace: anton
type: Opaque
```

3. Wait until all C* pods in statefulset are restarted
4. Update TLS CA Secret by setting new CA keypair as main entries and set previous CA as `ca2.crt`
```bash
apiVersion: v1
data:
  ca.crt: <ca2base64data>
  ca.key: <ca2base64data>
  ca2.crt: <ca1base64data>
kind: Secret
metadata:
  name: example-cluster-manual-tls-ca
  namespace: anton
type: Opaque
```

5. Wait until all C* pods in statefulset are restarted
6. Remove old CA `ca2.crt` from TLS CA Secret
```bash
apiVersion: v1
data:
  ca.crt: <ca2base64data>
  ca.key: <ca2base64data>
kind: Secret
metadata:
  name: example-cluster-manual-tls-ca
  namespace: anton
type: Opaque
```

7. Wait until all C* pods in statefulset are restarted
Now cluster is using TLS keypair signed by new TLS CA.

### Steps for TLS CA replacement in multiregion setup

1. Generate new CA keypair
2. Update TLS CA Secret in region1 by adding new CA certificate `ca2.crt`
3. Wait until all C* pods in region1 are restarted
4. Update TLS CA Secret in region2 by adding new CA certificate `ca2.crt`
5. Wait until all C* pods in region2 are restarted
6. Update TLS CA Secret in region1 by setting new CA keypair as main entries and set previous CA as `ca2.crt`
7. Wait until all C* pods in region1 are restarted
8. Update TLS CA Secret in region2 by setting new CA keypair as main entries and set previous CA as `ca2.crt`
9. Wait until all C* pods in region2 are restarted
10. Remove old CA `ca2.crt` from TLS CA Secret in region1
11. Wait until all C* pods in region1 are restarted
12. Remove old CA `ca2.crt` from TLS CA Secret in region2
13. Wait until all C* pods in region2 are restarted
