---
title: Client TLS Encryption configuration
slug: /client-tls-encryption-configuration
---

## Cassandra Client TLS Encryption configuration

By default, the Client TLS Encryption is disabled.

## Client TLS Encryption Field Specification Reference

| Field                                                | Description                                                                                                                                                                                     | Is Required | Default                          |
|------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------|----------------------------------|
| `encryption.client                                 ` | Client TLS Encryption configuration.                                                                                                                                                            | `N`         |                                  |
| `encryption.client.enabled                         ` | Enables Client Encryption.                                                                                                                                                                      | `N`         | `false`                          |
| `encryption.client.optional                        ` | If enabled both encrypted and unencrypted connections are allowed.                                                                                                                              | `N`         | `false`                          |
| `encryption.client.tlsSecret                       ` | TLS Secret fields configuration.                                                                                                                                                                | `N`         | ``                               |
| `encryption.client.tlsSecret.name                  ` | Secret name which stores TLS data. Required when `encryption.client.enabled` is set to true.                                                                                            | `Y`         | ``                               |
| `encryption.client.tlsSecret.KeystoreFileKey       ` | TLS Secret field which holds Keystore file. Keystore should contain keypair chains.                                                                                                             | `N`         | `keystore.jks`                   |
| `encryption.client.tlsSecret.KeystorePasswordKey   ` | TLS Secret field which holds password file for Keystore. The password must match that one is used when generating the Keystore.                                                                 | `N`         | `cassandra`                      |
| `encryption.client.tlsSecret.TruststoreFileKey     ` | TLS Secret field which holds Truststore file. Truststore should contain chain of trusted CA certificates.                                                                                       | `N`         | `truststore.jks`                 |
| `encryption.client.tlsSecret.TruststorePasswordKey ` | TLS Secret field which holds password file for Truststore. The password must match that one is used when generating the Truststore.                                                             | `N`         | `cassandra`                      |
| `encryption.client.protocol                        ` | Cryptographic protocol.                                                                                                                                                                         | `N`         | `TLS`                            |
| `encryption.client.algorithm                       ` | Key exchange or key agreement method.                                                                                                                                                           | `N`         | `SunX509`                        |
| `encryption.client.storeType                       ` | Archive format of Keystore.                                                                                                                                                                     | `N`         | `JKS`                            |
| `encryption.client.cipherSuites                    ` | The list of cipher suites for the server to support, in order of preference.                                                                                                                    | `N`         | `[TLS_RSA_WITH_AES_128_CBC_SHA,TLS_RSA_WITH_AES_256_CBC_SHA]` |
| `encryption.client.requireClientAuth               ` | Enables or disables Two-Way TLS authentication. If enabled the server verifies the certificate used by the client. The client certificate should be signed by a CA that the server trusts.      | `N`         | `true`                           |

## Grant access to external clients

To grant access to C* nodes via CQL and/or JMX protocols the TLS Secret for client should be generated (signed) using the same CA.
