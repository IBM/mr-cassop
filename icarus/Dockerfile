ARG DOCKER_PROXY_REGISTRY=""
FROM ${DOCKER_PROXY_REGISTRY}openjdk:11-jre-bullseye
ARG ICARUS_VERSION=2.0.4

RUN wget https://github.com/instaclustr/icarus/releases/download/icarus-${ICARUS_VERSION}/icarus.jar

RUN addgroup --gid 999 cassandra && adduser --uid 999 --gid 999 cassandra

RUN apt-get update && apt-get -y upgrade && rm -rf /var/lib/apt/lists/*

USER cassandra

ENTRYPOINT ["java", "-jar", "icarus.jar", "icarus"]
