---
title: Server TLS Encryption configuration
slug: /server-tls-encryption-configuration
---

## Cassandra Server TLS Encryption configuration

By default, the Server TLS Encryption is disabled.

## Server TLS Encryption Field Specification Reference

| Field                                                | Description                                                                                                                                                                                     | Is Required | Default                          |
|------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------|----------------------------------|
| `encryption.server                                 ` | Server TLS Encryption configuration.                                                                                                                                                            | `N`         |                                  |
| `encryption.server.internodeEncryption             ` | Server encryption type. Allowed values: `all`, `dc`, `rack`, `none`. To encrypt all inter-node communications use `all`. To encrypt the traffic between the datacenters use `dc`.               | `N`         | `none`                           |
| `encryption.server.tlsSecret                       ` | TLS Secret fields configuration.                                                                                                                                                                | `N`         | ``                               |
| `encryption.server.tlsSecret.name                  ` | Secret name which stores TLS data. Required when `encryption.server.internodeEncryption` is enabled.                                                                                            | `Y`         | ``                               |
| `encryption.server.tlsSecret.KeystoreFileKey       ` | TLS Secret field which holds Keystore file. Keystore should contain keypair chains.                                                                                                             | `N`         | `keystore.jks`                   |
| `encryption.server.tlsSecret.KeystorePasswordKey   ` | TLS Secret field which holds password file for Keystore. The password must match that one is used when generating the Keystore.                                                                 | `N`         | `cassandra`                      |
| `encryption.server.tlsSecret.TruststoreFileKey     ` | TLS Secret field which holds Truststore file. Truststore should contain chain of trusted CA certificates.                                                                                       | `N`         | `truststore.jks`                 |
| `encryption.server.tlsSecret.TruststorePasswordKey ` | TLS Secret field which holds password file for Truststore. The password must match that one is used when generating the Truststore.                                                             | `N`         | `cassandra`                      |
| `encryption.server.protocol                        ` | Cryptographic protocol.                                                                                                                                                                         | `N`         | `TLS`                            |
| `encryption.server.algorithm                       ` | Key exchange or key agreement method.                                                                                                                                                           | `N`         | `SunX509`                        |
| `encryption.server.storeType                       ` | Archive format of Keystore.                                                                                                                                                                     | `N`         | `JKS`                            |
| `encryption.server.cipherSuites                    ` | The list of cipher suites for the server to support, in order of preference.                                                                                                                    | `N`         | `[TLS_RSA_WITH_AES_128_CBC_SHA,TLS_RSA_WITH_AES_256_CBC_SHA]` |
| `encryption.server.requireClientAuth               ` | Enables or disables Two-Way TLS authentication. If enabled the C* node verifies the certificate used by the other C* nodes which connects to it. The certificate used by C* node should be signed by a CA that the other C* node trusts. | `N`         | `true`                           |
| `encryption.server.requireEndpointVerification     ` | Enables or disables host name verification.                                                                                                                                                     | `N`         | `false`                          |

## Multi-cluster setup

In multi-cluster setup TLS Secret should be generated (signed) using the same CA.
