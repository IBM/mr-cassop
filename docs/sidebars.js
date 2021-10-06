module.exports = {
  docsSidebar: [
    'home',
    'quickstart',
    'operator-configuration',
    'cassandracluster-configuration',
    'roles-management',
    'keyspace-management',
    {
      type: 'category',
      label: 'Reaper',
      collapsed: false,
      items: [
        'reaper',
        'reaper-repairs-configuration',
      ],
    },
    {
      type: 'category',
      label: 'TLS Encryption',
      collapsed: false,
      items: [
        'server-tls-encryption-configuration',
      ],
    },
    'multi-cluster-configurations',
    'maintenance-mode',
  ],
};
