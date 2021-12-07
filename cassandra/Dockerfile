ARG DOCKER_PROXY_REGISTRY=""
ARG CASSANDRA_VERSION=3.11.9
FROM ${DOCKER_PROXY_REGISTRY}cassandra:${CASSANDRA_VERSION}
ARG JMX_EXPORTER_VERSION=0.14.0
ENV INSTACLUSTR_CASSANDRA_EXPORTER_VERSION=0.9.10
ENV DATASTAX_CASSANDRA_EXPORTER_VERSION=0.3.0

RUN set -eux; \
  # wget_jar org name version dest_dir
  wget_jar() { \
    curl -sSf https://repo1.maven.org/maven2/$1/$2/$3/$2-$3.jar --create-dirs -o $4/$2.jar; \
  }; \
  apt-get update && apt-get install -y --no-install-recommends \
    dnsutils \
    jq \
    net-tools \
    s3cmd \
    httpie \
    wget; \
  wget_jar mx4j mx4j-tools 3.0.1 /mx4j; \
  wget_jar io/prometheus/jmx jmx_prometheus_javaagent ${JMX_EXPORTER_VERSION} /prometheus; \
  wget https://github.com/instaclustr/cassandra-exporter/releases/download/v$INSTACLUSTR_CASSANDRA_EXPORTER_VERSION/cassandra-exporter-agent-$INSTACLUSTR_CASSANDRA_EXPORTER_VERSION.jar -O /prometheus/cassandra-exporter-agent.jar; \
  wget https://github.com/datastax/metric-collector-for-apache-cassandra/releases/download/v$DATASTAX_CASSANDRA_EXPORTER_VERSION/datastax-mcac-agent-$DATASTAX_CASSANDRA_EXPORTER_VERSION.tar.gz -O /tmp/datastax-mcac-agent.tgz; \
  tar zxf /tmp/datastax-mcac-agent.tgz -C /prometheus; \
  mv /prometheus/datastax-mcac-agent-* /prometheus/datastax-mcac-agent; \
  rm -rf /var/lib/apt/lists/* /tmp/datastax-mcac-agent.tgz /prometheus/datastax-mcac-agent/config

RUN mkdir /home/cassandra && \
    chown cassandra:cassandra /home/cassandra && \
    usermod -d /home/cassandra cassandra

WORKDIR /home/cassandra

COPY .bashrc openssl_fix.sh ./

RUN ./openssl_fix.sh && \
    rm openssl_fix.sh && \
    chown cassandra:cassandra /home/cassandra/.bashrc

RUN chown cassandra:cassandra /opt/cassandra /prometheus
USER cassandra
