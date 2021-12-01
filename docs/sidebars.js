module.exports = {
  docsSidebar: [
    'home',
    'quickstart',
    'operator-configuration',
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
      ],
    },
    'exposing-clusters',
    'multi-region-cluster-configuration',
    'maintenance-mode',
    'development',
  ],
};
