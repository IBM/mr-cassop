#!/bin/bash

NODETOOL_AUTH_FALLBACK="-u cassandra -pw cassandra"
nt_fallback() {
 ntout=$(command nodetool $NODETOOL_AUTH "$@")
 if [[ $ntout == *"username"* ]]; then
   nt_is_fallback=true
   ntout=$(command nodetool $NODETOOL_AUTH_FALLBACK "$@")
 else
   return 1
 fi
}

find_nodetool_user() {
  if [[ "${CASSANDRA_JMX_AUTH}" == 'true' ]]; then
    for fn in $(find "${ROLES_DIR}" -type f); do
      read -r pw nt <<< $(jq -cr '[.password, .nodetoolUser] | "\(.[0]) \(.[1])"' ${fn})
      NODETOOL_USER=$(basename "${fn}")
      NODETOOL_PASSWORD="${pw}"
      if [[ -n "$PROBE_USER" ]]; then
        if [[ "$PROBE_USER" == "$NODETOOL_USER" ]]; then return 0; fi
      elif [[ "${nt}" == 'true' ]]; then
        return 0
      fi
    done
    echo "unable to find a nodetool user"
    exit 2
  fi
}

find_cassandra_user() {
  for fn in $(find "${ROLES_DIR}" -type f); do
    read -r pw super <<< $(jq -cr '[.password, .super] | "\(.[0]) \(.[1])"' ${fn})
    CASSANDRA_USER=$(basename "${fn}")
    CASSANDRA_PASSWORD="${pw}"
    if [[ -n "$PROBE_USER" ]]; then
      if [[ "$PROBE_USER" == "$CASSANDRA_USER" ]]; then return 0; fi
    elif [[ "${super}" == 'true' ]]; then
      return 0
    fi
  done
  if [[ -n "$PROBE_USER" ]]; then
    echo "Unable to find username: '$PROBE_USER'. Modify the environment variable 'PROBE_USER' and try again."
    exit 3
  fi
  echo "WARN: unable to find a cassandra super user"
}

get_cert_passwords() {
  CASSANDRA_TRUSTSTORE_PASSWORD=$(cat ${KEYSTORE_PWDS_DIR}/truststore.password)
  CASSANDRA_KEYSTORE_PASSWORD=$(cat ${KEYSTORE_PWDS_DIR}/keystore.password)
}

gossip_purge() {
  echo "purging gossip state"
  [[ -d /var/lib/cassandra/data/system ]] && \
  find /var/lib/cassandra/data/system -mindepth 1 -maxdepth 1 -name 'peers*' -type d -exec rm -rf {} \;
}

mk_auth() {
  if [[ "${CASSANDRA_JMX_AUTH}" == 'true' ]]; then
    NODETOOL_AUTH="-u ${NODETOOL_USER} -pw ${NODETOOL_PASSWORD}"
  fi
  if [[ "${CASSANDRA_JMX_SSL}" == 'true' ]]; then
    NODETOOL_AUTH="$NODETOOL_AUTH --ssl"
    NODETOOL_AUTH_FALLBACK="$NODETOOL_AUTH_FALLBACK --ssl"
  fi

  if [[ "${CASSANDRA_INTERNAL_AUTH}" == 'true' ]]; then
     CQLSH_AUTH="-u ${CASSANDRA_USER} -p ${CASSANDRA_PASSWORD}"
  fi
  CQLSH_AUTH="$CQLSH_AUTH $CQLSH_SSL"
}

find_nodetool_user
find_cassandra_user
mk_auth

case $1 in
  drain)
    nt_fallback drain
    exit 0
    ;;
  u)
    echo "$NODETOOL_USER"
    ;;
  p)
    echo "$NODETOOL_PASSWORD"
    ;;
  auth)
    if nt_fallback status &>/dev/null; then
      if [[ "${nt_is_fallback}" == "true" ]]; then
        echo "$NODETOOL_AUTH_FALLBACK"
        exit
      fi
    fi
    echo "$NODETOOL_AUTH"
    ;;
  cqlu)
    echo "$CASSANDRA_USER"
    ;;
  cqlp)
    echo "$CASSANDRA_PASSWORD"
    ;;
  cqlauth)
    echo "$CQLSH_AUTH"
    ;;
  superuser)
    if [[ -z "$PROBE_USER" ]]; then
      echo "$1 requires the environment variable 'PROBE_USER' set to the username to check if superuser == true"
      exit 4;
    fi
    if [[ "${super}" == 'true' ]]; then echo True; else echo False; exit 5; fi
    ;;
  purge)
    gossip_purge
    ;;
  truststore_password)
    get_cert_passwords
    echo "$CASSANDRA_TRUSTSTORE_PASSWORD"
    ;;
  keystore_password)
    get_cert_passwords
    echo "$CASSANDRA_KEYSTORE_PASSWORD"
    ;;
  truststore_arg)
    get_cert_passwords
    [ ! -z $CASSANDRA_TRUSTSTORE_PASSWORD ] && echo "truststore-password=$CASSANDRA_TRUSTSTORE_PASSWORD"
    ;;
  keystore_arg)
    get_cert_passwords
    [ ! -z $CASSANDRA_KEYSTORE_PASSWORD ] && echo "keystore-password=$CASSANDRA_KEYSTORE_PASSWORD"
    ;;
  *)
    echo "Unknown command - $1"
    exit 1
    ;;
esac
