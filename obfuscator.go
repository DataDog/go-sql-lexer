package sqllexer

import (
	"regexp"
	"strings"
)

type SQLObfuscatorConfig struct {
	ReplaceDigits    bool
	DollarQuotedFunc bool
}

type SQLObfuscator struct {
	config *SQLObfuscatorConfig
}

func NewSQLObfuscator(config *SQLObfuscatorConfig) *SQLObfuscator {
	return &SQLObfuscator{config: config}
}

const (
	StringPlaceholder = "?"
	NumberPlaceholder = "?"
)

// Obfuscate takes an input SQL string and returns an obfuscated SQL string.
// The obfuscator replaces all literal values with a single placeholder
func (o *SQLObfuscator) Obfuscate(input string) string {
	var obfuscatedSQL string

	lexer := NewSQLLexer(input)
	for token := range lexer.ScanAllTokens() {
		switch token.Type {
		case NUMBER:
			obfuscatedSQL += NumberPlaceholder
		case STRING:
			obfuscatedSQL += StringPlaceholder
		case INCOMPLETE_STRING:
			obfuscatedSQL += StringPlaceholder
		case IDENT:
			if o.config.ReplaceDigits {
				// regex to replace digits in identifier
				// we try to avoid using regex as much as possible,
				// as regex isn't the most performant,
				// but it's the easiest to implement and maintain
				digits_regex := regexp.MustCompile(`\d+`)
				obfuscatedSQL += digits_regex.ReplaceAllString(token.Value, "?")
			} else {
				obfuscatedSQL += token.Value
			}
		case COMMENT:
			obfuscatedSQL += token.Value
		case MULTILINE_COMMENT:
			// replace newlines and tabs in multiline comment with whitespace
			obfuscatedSQL += token.Value
		case DOLLAR_QUOTED_STRING:
			obfuscatedSQL += "?"
		case DOLLAR_QUOTED_FUNCTION:
			if o.config.DollarQuotedFunc {
				// obfuscate the content of dollar quoted function
				quotedFunc := strings.TrimPrefix(token.Value, "$func$")
				quotedFunc = strings.TrimSuffix(quotedFunc, "$func$")
				obfuscatedSQL += "$func$" + o.Obfuscate(quotedFunc) + "$func$"
			} else {
				// treat dollar quoted function as dollar quoted string
				obfuscatedSQL += "?"
			}
		case ERROR | UNKNOWN:
			// if we encounter an error or unknown token, we just append the value
			obfuscatedSQL += token.Value
		default:
			obfuscatedSQL += token.Value
		}
	}

	return strings.TrimSpace(obfuscatedSQL)
}
