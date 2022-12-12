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
      label: 'Security',
      items: [
        'security/network-policies',
        {
          type: 'category',
          label: 'TLS Encryption',
          items: [
            'security/server-tls-encryption-configuration',
            'security/client-tls-encryption-configuration',
            'security/create-tls-secret',
            'security/update-tls-secret',
          ],
        },
      ],
    },
    {
      type: 'category',
      label: 'Reaper',
      collapsed: true,
      items: [
        'reaper',
        'reaper-repairs-configuration',
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
  ],
};
