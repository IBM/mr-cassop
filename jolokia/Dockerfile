ARG DOCKER_PROXY_REGISTRY=""
FROM ${DOCKER_PROXY_REGISTRY}alpine AS build
ARG JOLOKIA_VERSION="1.7.1"

RUN wget -O jolokia.tar.gz https://github.com/rhuss/jolokia/releases/download/v${JOLOKIA_VERSION}/jolokia-${JOLOKIA_VERSION}-bin.tar.gz;
RUN tar -C /tmp/ -xzf jolokia.tar.gz;
RUN cp /tmp/jolokia-${JOLOKIA_VERSION}/agents/jolokia-unsecured.war /tmp

ARG DOCKER_PROXY_REGISTRY=""
FROM ${DOCKER_PROXY_REGISTRY}tomcat:9-jdk8-openjdk-slim-buster
ENV JOLOKIA_JSR160_PROXY_ENABLED=true
ENV JOLOKIA_RESPONSE_TIMEOUT=30000

RUN apt-get update && apt-get upgrade -y && apt-get clean

RUN addgroup --gid 901 jolokia && adduser --uid 901 --gid 901 jolokia

COPY setenv.sh bin/
COPY --from=build /tmp/jolokia-unsecured.war $CATALINA_HOME/webapps/jolokia.war
RUN rm -rf /usr/local/tomcat/webapps.dist/manager \
    /usr/local/tomcat/webapps.dist/docs \
    /usr/local/tomcat/webapps.dist/examples \
    /usr/local/tomcat/webapps.dist/host-manager \
    /usr/local/tomcat/webapps.dist/ROOT && \
    chown -R jolokia:jolokia $CATALINA_HOME/webapps/

USER jolokia
