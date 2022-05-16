module.exports = {
  docsSidebar: [
    'home',
    'quickstart',
    'operator-configuration',
    'operator-upgrade',
    'cassandracluster-configuration',
    'admin-auth',
    'roles-management',
    'keyspace-management',
    {
      type: 'category',
      label: 'Reaper',
      collapsed: true,
      items: [
        'reaper',
        'reaper-repairs-configuration',
      ],
    },
    {
      type: 'category',
      label: 'TLS Encryption',
      collapsed: true,
      items: [
        'server-tls-encryption-configuration',
        'client-tls-encryption-configuration',
        'create-tls-secret',
        'update-tls-secret',
      ],
    },
    'exposing-clusters',
    'multi-region-cluster-configuration',
    'sysctl',
    'maintenance-mode',
    {
      type: 'category',
      label: 'Architecture',
      collapsed: true,
      items: [
        'architecture-overview',
        'cassandracluster-lifecycle',
        'prober',
      ],
    },
    'development',
    'cql-configmaps',
    'upgrading',
  ],
};
