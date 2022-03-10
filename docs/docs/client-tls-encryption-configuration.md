---
title: Client TLS Encryption configuration
slug: /client-tls-encryption-configuration
---

## Cassandra Client TLS Encryption configuration

By default, the Client TLS Encryption is disabled.

## Client TLS Encryption Field Specification Reference

| Field                                                      | Description                                                                                                                                                                                | Is Required | Default          |
|------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------|------------------|
| `encryption.client                                 `       | Client TLS Encryption configuration.                                                                                                                                                       | `N`         |                  |
| `encryption.client.enabled                         `       | Enables Client Encryption.                                                                                                                                                                 | `N`         | `false`          |
| `encryption.client.optional                        `       | If enabled both encrypted and unencrypted connections are allowed.                                                                                                                         | `N`         | `false`          |
| `encryption.client.requireClientAuth               `       | Enables or disables Two-Way TLS authentication. If enabled the server verifies the certificate used by the client. The client certificate should be signed by a CA that the server trusts. | `N`         | `true`           |
| `encryption.client.caTLSSecret`                            | CA TLS Secret fields configuration.                                                                                                                                                        | `N`         | ``               |
| `encryption.client.caTLSSecret.name`                       | CA TLS Secret name which stores TLS CA data.                                                                                                                                               | `N`         | ``               |
| `encryption.client.caTLSSecret.fileKey`                    | CA TLS Secret fields which holds TLS CA key file.                                                                                                                                          | `N`         | `ca.key`         |
| `encryption.client.caTLSSecret.crtFileKey`                 | CA TLS Secret fields which holds TLS CA certificate file.                                                                                                                                  | `N`         | `ca.crt`         |
| `encryption.client.nodeTLSSecret`                          | Node TLS Secret name which stores TLS Node data.                                                                                                                                           | `N`         | ``               |
| `encryption.client.nodeTLSSecret.name`                     | Name of Node TLS Secret.                                                                                                                                                                   | `N`         | ``               |
| `encryption.client.nodeTLSSecret.keystoreFileKey`          | Node TLS Secret field which holds Keystore file. Keystore should contain keypair chains.                                                                                                   | `N`         | `keystore.jks`   |
| `encryption.client.nodeTLSSecret.keystorePasswordKey`      | Node TLS Secret field which holds password for Keystore. The password must match that one is used when generating the Keystore.                                                            | `N`         | `cassandra`      |
| `encryption.client.nodeTLSSecret.truststoreFileKey`        | Node TLS Secret field which holds Truststore file. Truststore should contain chain of trusted CA certificates.                                                                             | `N`         | `truststore.jks` |
| `encryption.client.nodeTLSSecret.truststorePasswordKey`    | Node TLS Secret field which holds password for Truststore. The password must match that one is used when generating the Truststore.                                                        | `N`         | `cassandra`      |
| `encryption.client.nodeTLSSecret.generateKeystorePassword` | Node TLS Secret field which holds password to encrypt keystore and truststore during generating. It's used only in case encryption.client.caTLSSecret.name is provided.                    | `N`         | `cassandra`      |
| `encryption.client.nodeTLSSecret.caFileKey`                | Node TLS Secret field which holds ca certificate file.                                                                                                                                     | `N`         | `ca.crt`         |
| `encryption.client.nodeTLSSecret.tlsFileKey`               | Node TLS Secret field which holds TLS certificate file.                                                                                                                                    | `N`         | `tls.crt`        |
| `encryption.client.nodeTLSSecret.tlsCrtFileKey`            | Node TLS Secret field which holds TLS key file.                                                                                                                                            | `N`         | `tls.key`        |
| `encryption.client.protocol`                               | Cryptographic protocol.                                                                                                                                                                    | `N`         | `TLS`            |
| `encryption.client.algorithm`                              | Key exchange or key agreement method.                                                                                                                                                      | `N`         | `SunX509`        |
| `encryption.client.storeType`                              | Archive format of Keystore.                                                                                                                                                                | `N`         | `JKS`            |
| `encryption.client.cipherSuites`                           | The list of cipher suites for the server to support, in order of preference.                                                                                                               | `N`         | ``               |

## Grant access to external clients

To grant access to C* nodes via CQL and/or JMX protocols the TLS Secret for client should be generated (signed) using the same CA.
