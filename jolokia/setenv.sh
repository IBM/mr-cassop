#!/bin/bash

# https://github.com/bodsch/docker-jolokia/blob/8f3ef375b230bebf302c7f95511118460f09fef3/rootfs/init/run.sh#L57-L86
# set pid file
CATALINA_PID="${CATALINA_HOME}/temp/catalina.pid"

# https://github.com/rhuss/jolokia/issues/222#issuecomment-170830887
# set rmi response timeout
CATALINA_OPTS="-Dsun.rmi.transport.tcp.responseTimeout=${JOLOKIA_RESPONSE_TIMEOUT} \
                -Dserver.name=${HOSTNAME} \
                ${CATALINA_OPTS}"
