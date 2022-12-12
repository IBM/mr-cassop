package controllers

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/onsi/gomega"
)

func TestParseCQLScript(t *testing.T) {
	asserts := gomega.NewWithT(t)
	testCases := []struct {
		name            string
		script          string
		expectedQueries []string
	}{
		{
			name: "simple multi line",
			script: `
CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3};
CREATE KEYSPACE IF NOT EXISTS cities WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3};
`,
			expectedQueries: []string{
				"CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}",
				"CREATE KEYSPACE IF NOT EXISTS cities WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}",
			},
		},
		{
			name: "multi query without semicolon in the end",
			script: `
CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3};
CREATE KEYSPACE IF NOT EXISTS cities WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}
`,
			expectedQueries: []string{
				"CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}",
				"CREATE KEYSPACE IF NOT EXISTS cities WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}",
			},
		},
		{
			name: "multi query with two queries on one line",
			script: `
CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}; CREATE KEYSPACE IF NOT EXISTS cities 
WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}
`,
			expectedQueries: []string{
				"CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}",
				`CREATE KEYSPACE IF NOT EXISTS cities 
WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}`,
			},
		},
		{
			name: "single query script",
			script: `
CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}
`,
			expectedQueries: []string{
				"CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}",
			},
		},
		{
			name: "single query script with semicolon in the end",
			script: `
CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3};
`,
			expectedQueries: []string{
				"CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}",
			},
		},
		{
			name: "multi query script with and one being an empty query",
			script: `
CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}; ; USE schools;
`,
			expectedQueries: []string{
				"CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}",
				"USE schools",
			},
		},
		{
			name: "only a semicolon on one of the lines",
			script: `
CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3};
 ; 
USE schools;
`,
			expectedQueries: []string{
				"CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}",
				"USE schools",
			},
		},
		{
			name: "more than two queries on one line",
			script: `
CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}; USE schools; LIST ROLES; DROP ROLE test-role
`,
			expectedQueries: []string{
				"CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}",
				"USE schools",
				"LIST ROLES",
				"DROP ROLE test-role",
			},
		},
		{
			name: "the end of a multi line query ended on a multi query line",
			script: `
LIST ROLES; DROP ROLE test-role; CREATE KEYSPACE IF NOT EXISTS schools 
WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}; USE schools;
`,
			expectedQueries: []string{
				"LIST ROLES",
				"DROP ROLE test-role",
				"CREATE KEYSPACE IF NOT EXISTS schools \nWITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}",
				"USE schools",
			},
		},
		{
			name: "last line is empty",
			script: `
LIST ROLES; DROP ROLE test-role; CREATE KEYSPACE IF NOT EXISTS schools 
WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}; USE schools;
      
`,
			expectedQueries: []string{
				"LIST ROLES",
				"DROP ROLE test-role",
				"CREATE KEYSPACE IF NOT EXISTS schools \nWITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}",
				"USE schools",
			},
		},
		{
			name: "multi line script with comments",
			script: `CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3};
-- general comment
-- school management keyspace
--
-- classes
CREATE TABLE IF NOT EXISTS schools.classes (
  name text,
  level text,
  room_number text,
  PRIMARY KEY ((name, room_number), level)
) WITH
  compaction={'class': 'LeveledCompactionStrategy'} AND
  compression={'sstable_compression': 'LZ4Compressor'};
-- teachers
CREATE TABLE IF NOT EXISTS schools.teachers (
  name text,
  subject text,
  birtday text,
  PRIMARY KEY (name)
) WITH
  compaction={'class': 'LeveledCompactionStrategy'} AND
  compression={'sstable_compression': 'LZ4Compressor'};
-- subjects
CREATE TABLE IF NOT EXISTS schools.subjects (
  name text,
  grade text,
  PRIMARY KEY (name)
) WITH
  compaction={'class': 'LeveledCompactionStrategy'} AND
  compression={'sstable_compression': 'LZ4Compressor'};
-- Additional comment
-- Grant access for all permissions to admin user on schools keyspace
GRANT ALL PERMISSIONS ON KEYSPACE schools TO 'admin';
`,
			expectedQueries: []string{
				"CREATE KEYSPACE IF NOT EXISTS schools WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3}",
				`CREATE TABLE IF NOT EXISTS schools.classes (
  name text,
  level text,
  room_number text,
  PRIMARY KEY ((name, room_number), level)
) WITH
  compaction={'class': 'LeveledCompactionStrategy'} AND
  compression={'sstable_compression': 'LZ4Compressor'}`,
				`CREATE TABLE IF NOT EXISTS schools.teachers (
  name text,
  subject text,
  birtday text,
  PRIMARY KEY (name)
) WITH
  compaction={'class': 'LeveledCompactionStrategy'} AND
  compression={'sstable_compression': 'LZ4Compressor'}`,
				`CREATE TABLE IF NOT EXISTS schools.subjects (
  name text,
  grade text,
  PRIMARY KEY (name)
) WITH
  compaction={'class': 'LeveledCompactionStrategy'} AND
  compression={'sstable_compression': 'LZ4Compressor'}`,
				"GRANT ALL PERMISSIONS ON KEYSPACE schools TO 'admin'",
			},
		},
	}

	for _, tc := range testCases {
		t.Log(tc.name)
		actualQueries := parseCQLQueries(tc.script)
		asserts.Expect(actualQueries).To(gomega.Equal(tc.expectedQueries), cmp.Diff(actualQueries, tc.expectedQueries))
	}
}
