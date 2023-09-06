package sqllexer

import (
	"reflect"
	"testing"
)

func TestNormalizer(t *testing.T) {
	tests := []struct {
		input             string
		want              string
		statementMetadata StatementMetadata
	}{
		{
			input: "SELECT ?",
			want:  "SELECT ?",
			statementMetadata: StatementMetadata{
				Tables:   []string{},
				Comments: []string{},
				Commands: []string{"SELECT"},
			},
		},
		{
			input: `
			/*dddbs='orders-mysql',dde='dbm-agent-integration',ddps='orders-app',ddpv='7825a16',traceparent='00-000000000000000068e229d784ee697c-569d1b940c1fb3ac-00'*/
			/* date='12%2F31',key='val' */
			SELECT * FROM users WHERE id = ?`,
			want: "SELECT * FROM users WHERE id = ?",
			statementMetadata: StatementMetadata{
				Tables:   []string{"users"},
				Comments: []string{"/*dddbs='orders-mysql',dde='dbm-agent-integration',ddps='orders-app',ddpv='7825a16',traceparent='00-000000000000000068e229d784ee697c-569d1b940c1fb3ac-00'*/", "/* date='12%2F31',key='val' */"},
				Commands: []string{"SELECT"},
			},
		},
		{
			input: "SELECT * FROM users WHERE id IN (?, ?) and name IN ARRAY[?, ?]",
			want:  "SELECT * FROM users WHERE id IN ( ? ) AND name IN ARRAY [ ? ]",
			statementMetadata: StatementMetadata{
				Tables:   []string{"users"},
				Comments: []string{},
				Commands: []string{"SELECT"},
			},
		},
		{
			input: `
			SELECT h.id, h.org_id, h.name, ha.name as alias, h.created 
			FROM vs?.host h 
				JOIN vs?.host_alias ha on ha.host_id = h.id 
			WHERE ha.org_id = ? AND ha.name = ANY ( ?, ? )
			`,
			want: "SELECT h.id, h.org_id, h.name, ha.name, h.created FROM vs?.host h JOIN vs?.host_alias ha ON ha.host_id = h.id WHERE ha.org_id = ? AND ha.name = ANY ( ? )",
			statementMetadata: StatementMetadata{
				Tables:   []string{"vs?.host", "vs?.host_alias"},
				Comments: []string{},
				Commands: []string{"SELECT", "JOIN"},
			},
		},
		{
			input: "/* this is a comment */ SELECT * FROM users WHERE id = ?",
			want:  "SELECT * FROM users WHERE id = ?",
			statementMetadata: StatementMetadata{
				Tables:   []string{"users"},
				Comments: []string{"/* this is a comment */"},
				Commands: []string{"SELECT"},
			},
		},
		{
			input: `
			/* this is a 
multiline comment */
			SELECT * FROM users /* comment comment */ WHERE id = ?
			-- this is another comment
			`,
			want: "SELECT * FROM users WHERE id = ?",
			statementMetadata: StatementMetadata{
				Tables:   []string{"users"},
				Comments: []string{"/* this is a \nmultiline comment */", "/* comment comment */", "-- this is another comment"},
				Commands: []string{"SELECT"},
			},
		},
		{
			input: "SELECT u.id as ID, u.name as Name FROM users as u WHERE u.id = ?",
			want:  "SELECT u.id, u.name FROM users WHERE u.id = ?",
			statementMetadata: StatementMetadata{
				Tables:   []string{"users"},
				Comments: []string{},
				Commands: []string{"SELECT"},
			},
		},
		{
			input: "UPDATE users SET name = (SELECT name FROM test_users WHERE id = ?) WHERE id = ?",
			want:  "UPDATE users SET name = ( SELECT name FROM test_users WHERE id = ? ) WHERE id = ?",
			statementMetadata: StatementMetadata{
				Tables:   []string{"users", "test_users"},
				Comments: []string{},
				Commands: []string{"UPDATE", "SELECT"},
			},
		},
		{
			input: `
			INSERT INTO order_status_change ( dbm_order_id, message, price, state ) 
			VALUES ( ( 
				SELECT id as dbm_order_id 
				FROM dbm_order 
				WHERE id = ? 
			) ( 
				-- random comment
				SELECT ( t.price * t.quantity * d.discount_percent ) AS price 
				FROM dbm_order o 
					JOIN order_item t ON o.id = t.dbm_order_id 
					JOIN discount d ON d.dbm_item_id = t.id 
				WHERE o.id = ? 
				LIMIT ? 
			) )`,
			want: "INSERT INTO order_status_change ( dbm_order_id, message, price, state ) VALUES ( ( SELECT id FROM dbm_order WHERE id = ? ) ( SELECT ( t.price * t.quantity * d.discount_percent ) FROM dbm_order o JOIN order_item t ON o.id = t.dbm_order_id JOIN discount d ON d.dbm_item_id = t.id WHERE o.id = ? LIMIT ? ) )",
			statementMetadata: StatementMetadata{
				Tables:   []string{"order_status_change", "dbm_order", "order_item", "discount"},
				Comments: []string{"-- random comment"},
				Commands: []string{"INSERT", "SELECT", "JOIN"},
			},
		},
		{
			input: "DELETE FROM users WHERE id IN (?, ?)",
			want:  "DELETE FROM users WHERE id IN ( ? )",
			statementMetadata: StatementMetadata{
				Tables:   []string{"users"},
				Comments: []string{},
				Commands: []string{"DELETE"},
			},
		},
		{
			input: `
			CREATE PROCEDURE test_procedure()
			BEGIN
				SELECT * FROM users WHERE id = ?;
				Update test_users set name = ? WHERE id = ?;
				Delete FROM user? WHERE id = ?;
			END
			`,
			want: "CREATE PROCEDURE test_procedure ( ) BEGIN SELECT * FROM users WHERE id = ? ; UPDATE test_users SET name = ? WHERE id = ? ; DELETE FROM user? WHERE id = ? ; END",
			statementMetadata: StatementMetadata{
				Tables:   []string{"users", "test_users", "user?"},
				Comments: []string{},
				Commands: []string{"CREATE", "BEGIN", "SELECT", "UPDATE", "DELETE"},
			},
		},
		{
			input: `
			SELECT org_id, resource_type, meta_key, meta_value 
			FROM public.schema_meta 
			WHERE org_id IN ( ? ) AND resource_type IN ( ? ) AND meta_key IN ( ? )
			`,
			want: "SELECT org_id, resource_type, meta_key, meta_value FROM public.schema_meta WHERE org_id IN ( ? ) AND resource_type IN ( ? ) AND meta_key IN ( ? )",
			statementMetadata: StatementMetadata{
				Tables:   []string{"public.schema_meta"},
				Comments: []string{},
				Commands: []string{"SELECT"},
			},
		},
		{
			// double quoted table name
			input: `SELECT * FROM "users" WHERE id = ?`,
			want:  `SELECT * FROM "users" WHERE id = ?`,
			statementMetadata: StatementMetadata{
				Tables:   []string{`"users"`},
				Comments: []string{},
				Commands: []string{"SELECT"},
			},
		},
		{
			// double quoted table name
			input: `SELECT * FROM "public"."users" WHERE id = ?`,
			want:  `SELECT * FROM "public"."users" WHERE id = ?`,
			statementMetadata: StatementMetadata{
				Tables:   []string{`"public"."users"`},
				Comments: []string{},
				Commands: []string{"SELECT"},
			},
		},
		{
			input: `
			WITH cte AS (
				SELECT id, name, age
				FROM person
				WHERE age > ?
			  )
			UPDATE person
			SET age = ?
			WHERE id IN (SELECT id FROM cte);
			INSERT INTO person (name, age)
			SELECT name, ?
			FROM cte
			WHERE age <= ?;
			`,
			want: "WITH cte AS ( SELECT id, name, age FROM person WHERE age > ? ) UPDATE person SET age = ? WHERE id IN ( SELECT id FROM cte ) ; INSERT INTO person ( name, age ) SELECT name, ? FROM cte WHERE age <= ? ;",
			statementMetadata: StatementMetadata{
				Tables:   []string{"person", "cte"},
				Comments: []string{},
				Commands: []string{"SELECT", "UPDATE", "INSERT"},
			},
		},
		{
			input: "WITH updates AS ( UPDATE metrics_metadata SET metric_type = ? updated = ? :: timestamp, interval = ? unit_id = ? per_unit_id = ? description = ? orientation = ? integration = ? short_name = ? WHERE metric_key = ? AND org_id = ? RETURNING ? ) INSERT INTO metrics_metadata ( org_id, metric_key, metric_type, interval, unit_id, per_unit_id, description, orientation, integration, short_name ) SELECT ? WHERE NOT EXISTS ( SELECT ? FROM updates )",
			want:  "WITH updates AS ( UPDATE metrics_metadata SET metric_type = ? updated = ? :: timestamp, interval = ? unit_id = ? per_unit_id = ? description = ? orientation = ? integration = ? short_name = ? WHERE metric_key = ? AND org_id = ? RETURNING ? ) INSERT INTO metrics_metadata ( org_id, metric_key, metric_type, interval, unit_id, per_unit_id, description, orientation, integration, short_name ) SELECT ? WHERE NOT EXISTS ( SELECT ? FROM updates )",
			statementMetadata: StatementMetadata{
				Tables:   []string{"metrics_metadata", "updates"},
				Comments: []string{},
				Commands: []string{"UPDATE", "INSERT", "SELECT"},
			},
		},
		{
			input: `
			/* Multi-line comment */
			SELECT * FROM clients WHERE (clients.first_name = ?) LIMIT ? BEGIN INSERT INTO owners (created_at, first_name, locked, orders_count, updated_at) VALUES (?, ?, ?, ?, ?) COMMIT`,
			want: "SELECT * FROM clients WHERE ( clients.first_name = ? ) LIMIT ? BEGIN INSERT INTO owners ( created_at, first_name, locked, orders_count, updated_at ) VALUES ( ? ) COMMIT",
			statementMetadata: StatementMetadata{
				Tables:   []string{"clients", "owners"},
				Comments: []string{"/* Multi-line comment */"},
				Commands: []string{"SELECT", "BEGIN", "INSERT", "COMMIT"},
			},
		},
		{
			input: `-- Single line comment
			-- Another single line comment
			-- Another another single line comment
			GRANT USAGE, DELETE ON SCHEMA datadog TO datadog`,
			want: "GRANT USAGE, DELETE ON SCHEMA datadog TO datadog",
			statementMetadata: StatementMetadata{
				Tables:   []string{},
				Comments: []string{"-- Single line comment", "-- Another single line comment", "-- Another another single line comment"},
				Commands: []string{"GRANT", "DELETE"},
			},
		},
		{
			input: `-- Testing table value constructor SQL expression
			SELECT * FROM (VALUES (?, ?)) AS d (id, animal)`,
			want: "SELECT * FROM ( VALUES ( ? ) ) ( id, animal )",
			statementMetadata: StatementMetadata{
				Tables:   []string{},
				Comments: []string{"-- Testing table value constructor SQL expression"},
				Commands: []string{"SELECT"},
			},
		},
		{
			input: `ALTER TABLE tabletest DROP COLUMN columna`,
			want:  "ALTER TABLE tabletest DROP COLUMN columna",
			statementMetadata: StatementMetadata{
				Tables:   []string{"tabletest"},
				Comments: []string{},
				Commands: []string{"ALTER", "DROP"},
			},
		},
		{
			input: `REVOKE ALL ON SCHEMA datadog FROM datadog`,
			want:  "REVOKE ALL ON SCHEMA datadog FROM datadog",
			statementMetadata: StatementMetadata{
				Tables:   []string{"datadog"},
				Comments: []string{},
				Commands: []string{"REVOKE"},
			},
		},
		{
			input: "/* Testing explicit table SQL expression */ WITH T1 AS (SELECT PNO , PNAME , COLOR , WEIGHT , CITY FROM P WHERE CITY = ?), T2 AS (SELECT PNO, PNAME, COLOR, WEIGHT, CITY, ? * WEIGHT AS NEW_WEIGHT, ? AS NEW_CITY FROM T1), T3 AS ( SELECT PNO , PNAME, COLOR, NEW_WEIGHT AS WEIGHT, NEW_CITY AS CITY FROM T2), T4 AS ( TABLE P EXCEPT CORRESPONDING TABLE T1) TABLE T4 UNION CORRESPONDING TABLE T3",
			want:  "WITH T1 AS ( SELECT PNO, PNAME, COLOR, WEIGHT, CITY FROM P WHERE CITY = ? ), T2 AS ( SELECT PNO, PNAME, COLOR, WEIGHT, CITY, ? * WEIGHT, ? FROM T1 ), T3 AS ( SELECT PNO, PNAME, COLOR, NEW_WEIGHT, NEW_CITY FROM T2 ), T4 AS ( TABLE P EXCEPT CORRESPONDING TABLE T1 ) TABLE T4 UNION CORRESPONDING TABLE T3",
			statementMetadata: StatementMetadata{
				Tables:   []string{"P", "T1", "T2", "T4", "T3"},
				Comments: []string{"/* Testing explicit table SQL expression */"},
				Commands: []string{"SELECT"},
			},
		},
		{
			// truncated
			input: "SELECT * FROM users WHERE id =",
			want:  "SELECT * FROM users WHERE id =",
			statementMetadata: StatementMetadata{
				Tables:   []string{"users"},
				Comments: []string{},
				Commands: []string{"SELECT"},
			},
		},
	}

	normalizer := NewNormalizer(
		WithCollectComments(true),
		WithCollectCommands(true),
		WithCollectTables(true),
		WithKeepSQLAlias(false),
	)

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			got, statementMetadata, err := normalizer.Normalize(test.input)
			if err != nil {
				t.Errorf("error during normalization: %v", err)
			}
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
			if !reflect.DeepEqual(statementMetadata.Commands, test.statementMetadata.Commands) {
				t.Errorf("got %v, want %v", statementMetadata.Commands, test.statementMetadata.Commands)
			}
			if !reflect.DeepEqual(statementMetadata.Comments, test.statementMetadata.Comments) {
				t.Errorf("got %v, want %v", statementMetadata.Comments, test.statementMetadata.Comments)
				t.Errorf(statementMetadata.Comments[0])
				t.Errorf(test.statementMetadata.Comments[0])
			}
			if !reflect.DeepEqual(statementMetadata.Tables, test.statementMetadata.Tables) {
				t.Errorf("got %v, want %v", statementMetadata.Tables, test.statementMetadata.Tables)
			}
		})
	}
}

func TestNormalizerNotCollectMetadata(t *testing.T) {
	tests := []struct {
		input             string
		want              string
		statementMetadata StatementMetadata
	}{
		{
			input: "SELECT ?",
			want:  "SELECT ?",
			statementMetadata: StatementMetadata{
				Tables:   []string{},
				Comments: []string{},
				Commands: []string{},
			},
		},
		{
			input: "SELECT * FROM users WHERE id = ?",
			want:  "SELECT * FROM users WHERE id = ?",
			statementMetadata: StatementMetadata{
				Tables:   []string{},
				Comments: []string{},
				Commands: []string{},
			},
		},
		{
			input: "SELECT id as ID, name as Name FROM users WHERE id IN (?, ?)",
			want:  "SELECT id AS ID, name AS Name FROM users WHERE id IN ( ? )",
			statementMetadata: StatementMetadata{
				Tables:   []string{},
				Comments: []string{},
				Commands: []string{},
			},
		},
		{
			input: `TRUNCATE TABLE datadog`,
			want:  "TRUNCATE TABLE datadog",
			statementMetadata: StatementMetadata{
				Tables:   []string{},
				Comments: []string{},
				Commands: []string{},
			},
		},
	}

	normalizer := NewNormalizer(
		WithCollectComments(false),
		WithCollectCommands(false),
		WithCollectTables(false),
		WithKeepSQLAlias(true),
	)

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			got, statementMetadata, err := normalizer.Normalize(test.input)
			if err != nil {
				t.Errorf("error during normalization: %v", err)
			}
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
			if !reflect.DeepEqual(statementMetadata.Commands, test.statementMetadata.Commands) {
				t.Errorf("got %v, want %v", statementMetadata.Commands, test.statementMetadata.Commands)
			}
			if !reflect.DeepEqual(statementMetadata.Comments, test.statementMetadata.Comments) {
				t.Errorf("got %v, want %v", statementMetadata.Comments, test.statementMetadata.Comments)
			}
			if !reflect.DeepEqual(statementMetadata.Tables, test.statementMetadata.Tables) {
				t.Errorf("got %v, want %v", statementMetadata.Tables, test.statementMetadata.Tables)
			}
		})
	}
}

func TestNormalizerFormatting(t *testing.T) {
	tests := []struct {
		queries  []string
		expected string
	}{
		{
			queries: []string{
				"SELECT id,name, address FROM users where id = ?",
				"select id, name, address FROM users where id = ?",
				"select id as ID, name as Name, address FROM users where id = ?",
			},
			expected: "SELECT id, name, address FROM users WHERE id = ?",
		},
		{
			queries: []string{
				"SELECT id,name, address FROM users where id IN (?, ?,?, ?)",
				"select id, name, address FROM users where id IN ( ? )",
				"select id, name, address FROM users where id IN ( ? )",
				"select id, name, address FROM users where id IN (?,?,?)",
			},
			expected: "SELECT id, name, address FROM users WHERE id IN ( ? )",
		},
		{
			queries: []string{
				"SELECT * FROM discount where description LIKE ?",
				"select * from discount where description LIKE ?",
				"select * from discount where description like ?",
			},
			expected: "SELECT * FROM discount WHERE description LIKE ?",
		},
	}

	normalizer := NewNormalizer(
		WithCollectComments(false),
	)
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			for _, query := range test.queries {
				got, _, err := normalizer.Normalize(query)
				if err != nil {
					t.Errorf("error during normalization: %v", err)
				}
				if got != test.expected {
					t.Errorf("got %q, want %q", got, test.expected)
				}
			}
		})
	}
}

func TestGroupObfuscatedValues(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "( ? )",
			want:  "( ? )",
		},
		{
			input: "(?, ?)",
			want:  "( ? )",
		},
		{
			input: "( ?, ?, ? )",
			want:  "( ? )",
		},
		{
			input: "( ? )",
			want:  "( ? )",
		},
		{
			input: "( ?, ? )",
			want:  "( ? )",
		},
		{
			input: "( ?,?)",
			want:  "( ? )",
		},
		{
			input: "[ ? ]",
			want:  "[ ? ]",
		},
		{
			input: "[?, ?]",
			want:  "[ ? ]",
		},
		{
			input: "[ ?, ?, ? ]",
			want:  "[ ? ]",
		},
		{
			input: "[ ? ]",
			want:  "[ ? ]",
		},
		{
			input: "[ ?, ? ]",
			want:  "[ ? ]",
		},
		{
			input: "[ ?,?]",
			want:  "[ ? ]",
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			got := groupObfuscatedValues(test.input)
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}
