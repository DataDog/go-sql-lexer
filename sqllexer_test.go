package sqllexer

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLexer(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []Token
	}{
		{
			name:  "simple select with number",
			input: "SELECT * FROM users where id = 1",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
				{WS, " "},
				{IDENT, "where"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{NUMBER, "1"},
			},
		},
		{
			name:  "simple select with number",
			input: "SELECT * FROM users where id = '1'",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
				{WS, " "},
				{IDENT, "where"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{STRING, "'1'"},
			},
		},
		{
			name:  "simple select with negative number",
			input: "SELECT * FROM users where id = -1",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
				{WS, " "},
				{IDENT, "where"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{NUMBER, "-1"},
			},
		},
		{
			name:  "simple select with string",
			input: "SELECT * FROM users where id = '12'",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
				{WS, " "},
				{IDENT, "where"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{STRING, "'12'"},
			},
		},
		{
			name:  "simple select with double quoted identifier",
			input: "SELECT * FROM \"users table\" where id = 1",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "\"users table\""},
				{WS, " "},
				{IDENT, "where"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{NUMBER, "1"},
			},
		},
		{
			name:  "simple select with single line comment",
			input: "SELECT * FROM users where id = 1 -- comment here",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
				{WS, " "},
				{IDENT, "where"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{NUMBER, "1"},
				{WS, " "},
				{COMMENT, "-- comment here"},
			},
		},
		{
			name:  "simple select with multi line comment",
			input: "SELECT * /* comment here */ FROM users where id = 1",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{MULTILINE_COMMENT, "/* comment here */"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
				{WS, " "},
				{IDENT, "where"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{NUMBER, "1"},
			},
		},
		{
			name:  "simple malformed select",
			input: "SELECT * FROM users where id = 1 and name = 'j",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
				{WS, " "},
				{IDENT, "where"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{NUMBER, "1"},
				{WS, " "},
				{IDENT, "and"},
				{WS, " "},
				{IDENT, "name"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{INCOMPLETE_STRING, "'j"},
			},
		},
		{
			name:  "truncated sql",
			input: "SELECT * FROM users where id = ",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
				{WS, " "},
				{IDENT, "where"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
			},
		},
		{
			name:  "simple select with array of literals",
			input: "SELECT * FROM users where id in (1, '2')",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
				{WS, " "},
				{IDENT, "where"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{IDENT, "in"},
				{WS, " "},
				{PUNCTUATION, "("},
				{NUMBER, "1"},
				{PUNCTUATION, ","},
				{WS, " "},
				{STRING, "'2'"},
				{PUNCTUATION, ")"},
			},
		},
		{
			name:  "dollar quoted function",
			input: "SELECT $func$INSERT INTO table VALUES ('a', 1, 2)$func$ FROM users",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{DOLLAR_QUOTED_FUNCTION, "$func$INSERT INTO table VALUES ('a', 1, 2)$func$"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
			},
		},
		{
			name:  "dollar quoted string",
			input: "SELECT * FROM users where id = $tag$test$tag$",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
				{WS, " "},
				{IDENT, "where"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{DOLLAR_QUOTED_STRING, "$tag$test$tag$"},
			},
		},
		{
			name:  "dollar quoted string",
			input: "SELECT * FROM users where id = $$test$$",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
				{WS, " "},
				{IDENT, "where"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{DOLLAR_QUOTED_STRING, "$$test$$"},
			},
		},
		{
			name:  "numbered parameter",
			input: "SELECT * FROM users where id = $1",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
				{WS, " "},
				{IDENT, "where"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{NUMBERED_PARAMETER, "$1"},
			},
		},
		{
			name:  "identifier with underscore and period",
			input: "SELECT * FROM users where user_id = 2 and users.name = 'j'",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
				{WS, " "},
				{IDENT, "where"},
				{WS, " "},
				{IDENT, "user_id"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{NUMBER, "2"},
				{WS, " "},
				{IDENT, "and"},
				{WS, " "},
				{IDENT, "users.name"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{STRING, "'j'"},
			},
		},
		{
			name:  "select with hex and octal numbers",
			input: "SELECT * FROM users where id = 0x123 and id = 0X123 and id = 0123",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
				{WS, " "},
				{IDENT, "where"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{NUMBER, "0x123"},
				{WS, " "},
				{IDENT, "and"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{NUMBER, "0X123"},
				{WS, " "},
				{IDENT, "and"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{NUMBER, "0123"},
			},
		},
		{
			name:  "select with float numbers and scientific notation",
			input: "SELECT 1.2,1.2e3,1.2e-3,1.2E3,1.2E-3 FROM users",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{NUMBER, "1.2"},
				{PUNCTUATION, ","},
				{NUMBER, "1.2e3"},
				{PUNCTUATION, ","},
				{NUMBER, "1.2e-3"},
				{PUNCTUATION, ","},
				{NUMBER, "1.2E3"},
				{PUNCTUATION, ","},
				{NUMBER, "1.2E-3"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
			},
		},
		{
			name:  "select with double quoted identifier",
			input: `SELECT * FROM "users table"`,
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, `"users table"`},
			},
		},
		{
			name:  "select with double quoted identifier",
			input: `SELECT * FROM "public"."users table"`,
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, `"public"."users table"`},
			},
		},
		{
			name:  "select with escaped string",
			input: "SELECT * FROM users where id = 'j\\'s'",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
				{WS, " "},
				{IDENT, "where"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{OPERATOR, "="},
				{WS, " "},
				{STRING, "'j\\'s'"},
			},
		},
		{
			name:  "select with escaped string",
			input: "SELECT * FROM users where id =?",
			expected: []Token{
				{IDENT, "SELECT"},
				{WS, " "},
				{WILDCARD, "*"},
				{WS, " "},
				{IDENT, "FROM"},
				{WS, " "},
				{IDENT, "users"},
				{WS, " "},
				{IDENT, "where"},
				{WS, " "},
				{IDENT, "id"},
				{WS, " "},
				{OPERATOR, "="},
				{OPERATOR, "?"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := New(tt.input)
			tokens := lexer.ScanAll()
			assert.Equal(t, tt.expected, tokens)
		})
	}
}

func ExampleLexer() {
	query := "SELECT * FROM users WHERE id = 1"
	lexer := New(query)
	tokens := lexer.ScanAll()
	fmt.Println(tokens)
	// Output: [{6 SELECT} {2  } {8 *} {2  } {6 FROM} {2  } {6 users} {2  } {6 WHERE} {2  } {6 id} {2  } {7 =} {2  } {5 1}]
}
