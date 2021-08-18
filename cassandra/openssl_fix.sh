#!/bin/bash

set -eux

openssl_cnf=/etc/ssl/openssl.cnf
echo -e "openssl_conf = default_conf\n$(cat $openssl_cnf)" > $openssl_cnf
echo '
[default_conf]
ssl_conf = ssl_sect

[ssl_sect]
system_default = system_default_sect

[system_default_sect]
CipherString = DEFAULT@SECLEVEL=1
' >> $openssl_cnf
