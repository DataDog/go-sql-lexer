package sqllexer

import (
	"strings"
)

type normalizerConfig struct {
	// CollectTables specifies whether the normalizer should also extract the table names that a query addresses
	CollectTables bool `json:"collect_tables"`

	// CollectCommands specifies whether the normalizer should extract and return commands as SQL metadata
	CollectCommands bool `json:"collect_commands"`

	// CollectComments specifies whether the normalizer should extract and return comments as SQL metadata
	CollectComments bool `json:"collect_comments"`

	// CollectProcedure specifies whether the normalizer should extract and return procedure name as SQL metadata
	CollectProcedure bool `json:"collect_procedure"`

	// KeepSQLAlias specifies whether SQL aliases ("AS") should be truncated.
	KeepSQLAlias bool `json:"keep_sql_alias"`

	// UppercaseKeywords specifies whether SQL keywords should be uppercased.
	UppercaseKeywords bool `json:"uppercase_keywords"`

	// RemoveSpaceBetweenParentheses specifies whether spaces should be kept between parentheses.
	// Spaces are inserted between parentheses by default. but this can be disabled by setting this to true.
	RemoveSpaceBetweenParentheses bool `json:"remove_space_between_parentheses"`

	// KeepTrailingSemicolon specifies whether the normalizer should keep the trailing semicolon.
	// The trailing semicolon is removed by default, but this can be disabled by setting this to true.
	// PL/SQL requires a trailing semicolon, so this should be set to true when normalizing PL/SQL.
	KeepTrailingSemicolon bool `json:"keep_trailing_semicolon"`

	// KeepIdentifierQuotation specifies whether the normalizer should keep the quotation of identifiers.
	KeepIdentifierQuotation bool `json:"keep_identifier_quotation"`
}

type normalizerOption func(*normalizerConfig)

func WithCollectTables(collectTables bool) normalizerOption {
	return func(c *normalizerConfig) {
		c.CollectTables = collectTables
	}
}

func WithCollectCommands(collectCommands bool) normalizerOption {
	return func(c *normalizerConfig) {
		c.CollectCommands = collectCommands
	}
}

func WithCollectComments(collectComments bool) normalizerOption {
	return func(c *normalizerConfig) {
		c.CollectComments = collectComments
	}
}

func WithKeepSQLAlias(keepSQLAlias bool) normalizerOption {
	return func(c *normalizerConfig) {
		c.KeepSQLAlias = keepSQLAlias
	}
}

func WithUppercaseKeywords(uppercaseKeywords bool) normalizerOption {
	return func(c *normalizerConfig) {
		c.UppercaseKeywords = uppercaseKeywords
	}
}

func WithCollectProcedures(collectProcedure bool) normalizerOption {
	return func(c *normalizerConfig) {
		c.CollectProcedure = collectProcedure
	}
}

func WithRemoveSpaceBetweenParentheses(removeSpaceBetweenParentheses bool) normalizerOption {
	return func(c *normalizerConfig) {
		c.RemoveSpaceBetweenParentheses = removeSpaceBetweenParentheses
	}
}

func WithKeepTrailingSemicolon(keepTrailingSemicolon bool) normalizerOption {
	return func(c *normalizerConfig) {
		c.KeepTrailingSemicolon = keepTrailingSemicolon
	}
}

func WithKeepIdentifierQuotation(keepIdentifierQuotation bool) normalizerOption {
	return func(c *normalizerConfig) {
		c.KeepIdentifierQuotation = keepIdentifierQuotation
	}
}

type StatementMetadata struct {
	Size       int      `json:"size"`
	Tables     []string `json:"tables"`
	Comments   []string `json:"comments"`
	Commands   []string `json:"commands"`
	Procedures []string `json:"procedures"`
}

type groupablePlaceholder struct {
	groupable bool
}

type headState struct {
	readFirstNonWSNonComment            bool
	inLeadingParenthesesExpression      bool
	foundLeadingExpressionInParentheses bool
	standaloneExpressionInParentheses   bool
	expressionInParentheses             strings.Builder
}

type Normalizer struct {
	config *normalizerConfig
}

func NewNormalizer(opts ...normalizerOption) *Normalizer {
	normalizer := Normalizer{
		config: &normalizerConfig{},
	}

	for _, opt := range opts {
		opt(normalizer.config)
	}

	return &normalizer
}

// Normalize takes an input SQL string and returns a normalized SQL string, a StatementMetadata struct, and an error.
// The normalizer collapses input SQL into compact format, groups obfuscated values into single placeholder,
// and collects metadata such as table names, comments, and commands.
func (n *Normalizer) Normalize(input string, lexerOpts ...lexerOption) (normalizedSQL string, statementMetadata *StatementMetadata, err error) {
	lexer := New(
		input,
		lexerOpts...,
	)

	normalizedSQLBuilder := new(strings.Builder)
	normalizedSQLBuilder.Grow(len(input))

	statementMetadata = &StatementMetadata{
		Tables:     []string{},
		Comments:   []string{},
		Commands:   []string{},
		Procedures: []string{},
	}

	var groupablePlaceholder groupablePlaceholder
	var headState headState

	ctes := make(map[string]bool) // Holds the CTEs that are currently being processed

	var lastValueToken *LastValueToken

	for {
		token := lexer.Scan()
		n.collectMetadata(&input, token, lastValueToken, statementMetadata, ctes)
		n.normalizeSQL(&input, token, lastValueToken, normalizedSQLBuilder, &groupablePlaceholder, &headState, lexerOpts...)
		if token.Type == EOF {
			break
		}
		if isValueToken(token) {
			lastValueToken = token.GetLastValueToken(&input)
		}
	}

	normalizedSQL = normalizedSQLBuilder.String()

	// Dedupe collected metadata
	dedupeStatementMetadata(statementMetadata)

	return n.trimNormalizedSQL(normalizedSQL), statementMetadata, nil
}

func (n *Normalizer) collectMetadata(source *string, token *Token, lastValueToken *LastValueToken, statementMetadata *StatementMetadata, ctes map[string]bool) {
	if n.config.CollectComments && (token.Type == COMMENT || token.Type == MULTILINE_COMMENT) {
		// Collect comments
		statementMetadata.Comments = append(statementMetadata.Comments, token.String(source))
	} else if token.Type == COMMAND {
		if n.config.CollectCommands && token.Type == COMMAND {
			// Collect commands
			statementMetadata.Commands = append(statementMetadata.Commands, strings.ToUpper(token.String(source)))
		}
	} else if token.Type == IDENT || token.Type == QUOTED_IDENT || token.Type == FUNCTION {
		tokenVal := token.String(source)
		if token.Type == QUOTED_IDENT {
			tokenVal = trimQuotes(source, token)
			if !n.config.KeepIdentifierQuotation {
				token.SetOutputValue(tokenVal)
			}
		}
		if lastValueToken != nil && lastValueToken.Type == CTE_INDICATOR {
			// Collect CTEs so we can skip them later in table collection
			ctes[tokenVal] = true
		} else if n.config.CollectTables && lastValueToken != nil && lastValueToken.IsTableIndicator {
			// Collect table names the token is not a CTE
			if _, ok := ctes[tokenVal]; !ok {
				statementMetadata.Tables = append(statementMetadata.Tables, tokenVal)
			}
		} else if n.config.CollectProcedure && lastValueToken != nil && lastValueToken.Type == PROC_INDICATOR {
			// Collect procedure names
			statementMetadata.Procedures = append(statementMetadata.Procedures, tokenVal)
		}
	}
}

func (n *Normalizer) normalizeSQL(source *string, token *Token, lastValueToken *LastValueToken, normalizedSQLBuilder *strings.Builder, groupablePlaceholder *groupablePlaceholder, headState *headState, lexerOpts ...lexerOption) {
	if token.Type != WS && token.Type != COMMENT && token.Type != MULTILINE_COMMENT {

		// handle leading expression in parentheses
		if !headState.readFirstNonWSNonComment {
			headState.readFirstNonWSNonComment = true
			if token.Type == PUNCTUATION && token.String(source) == "(" {
				headState.inLeadingParenthesesExpression = true
				headState.standaloneExpressionInParentheses = true
			}
		}
		if token.Type == EOF {
			if headState.standaloneExpressionInParentheses {
				normalizedSQLBuilder.WriteString(headState.expressionInParentheses.String())
			}
			return
		} else if headState.foundLeadingExpressionInParentheses {
			headState.standaloneExpressionInParentheses = false
		}

		if token.Type == DOLLAR_QUOTED_FUNCTION && token.String(source) != StringPlaceholder {
			// if the token is a dollar quoted function and it is not obfuscated,
			// we need to recusively normalize the content of the dollar quoted function
			quotedFunc := token.String(source)[6 : len(token.String(source))-6] // remove the $func$ prefix and suffix
			normalizedQuotedFunc, _, err := n.Normalize(quotedFunc, lexerOpts...)
			if err == nil {
				// replace the content of the dollar quoted function with the normalized content
				// if there is an error, we just keep the original content
				var normalizedDollarQuotedFunc strings.Builder
				normalizedDollarQuotedFunc.WriteString("$func$")
				normalizedDollarQuotedFunc.WriteString(normalizedQuotedFunc)
				normalizedDollarQuotedFunc.WriteString("$func$")
				token.SetOutputValue(normalizedDollarQuotedFunc.String())
			}
		}

		if !n.config.KeepSQLAlias {
			// discard SQL alias
			if token.Type == ALIAS_INDICATOR {
				return
			}

			if lastValueToken != nil && lastValueToken.Type == ALIAS_INDICATOR {
				if token.Type == IDENT {
					return
				} else {
					// if the last token is AS and the current token is not IDENT,
					// this could be a CTE like WITH ... AS (...),
					// so we do not discard the current token
					n.appendWhitespace(source, token, lastValueToken, normalizedSQLBuilder)
					n.writeToken(source, lastValueToken.Type, lastValueToken.Value, normalizedSQLBuilder)
				}
			}
		}

		// group consecutive obfuscated values into single placeholder
		if n.isObfuscatedValueGroupable(source, token, lastValueToken, groupablePlaceholder, normalizedSQLBuilder) {
			// return the token but not write it to the normalizedSQLBuilder
			return
		}

		if headState.inLeadingParenthesesExpression {
			n.appendWhitespace(source, token, lastValueToken, &headState.expressionInParentheses)
			n.writeToken(source, token.Type, token.String(source), &headState.expressionInParentheses)
			if token.Type == PUNCTUATION && token.String(source) == ")" {
				headState.inLeadingParenthesesExpression = false
				headState.foundLeadingExpressionInParentheses = true
			}
		} else {
			n.appendWhitespace(source, token, lastValueToken, normalizedSQLBuilder)
			n.writeToken(source, token.Type, token.String(source), normalizedSQLBuilder)
		}
	}
}

func (n *Normalizer) writeToken(source *string, tokenType TokenType, tokenValue string, normalizedSQLBuilder *strings.Builder) {
	if n.config.UppercaseKeywords && (tokenType == COMMAND || tokenType == KEYWORD) {
		normalizedSQLBuilder.WriteString(strings.ToUpper(tokenValue))
	} else {
		normalizedSQLBuilder.WriteString(tokenValue)
	}
}

func (n *Normalizer) isObfuscatedValueGroupable(source *string, token *Token, lastValueToken *LastValueToken, groupablePlaceholder *groupablePlaceholder, normalizedSQLBuilder *strings.Builder) bool {
	if token.String(source) == NumberPlaceholder || token.String(source) == StringPlaceholder {
		if lastValueToken.Value == "(" || lastValueToken.Value == "[" {
			// if the last token is "(" or "[", and the current token is a placeholder,
			// we know it's the start of groupable placeholders
			// we don't return here because we still need to write the first placeholder
			groupablePlaceholder.groupable = true
		} else if lastValueToken.Value == "," && groupablePlaceholder.groupable {
			return true
		}
	}

	if lastValueToken != nil && (lastValueToken.Value == NumberPlaceholder || lastValueToken.Value == StringPlaceholder) && token.String(source) == "," && groupablePlaceholder.groupable {
		return true
	}

	if groupablePlaceholder.groupable && (token.String(source) == ")" || token.String(source) == "]") {
		// end of groupable placeholders
		groupablePlaceholder.groupable = false
		return false
	}

	if groupablePlaceholder.groupable && token.String(source) != NumberPlaceholder && token.String(source) != StringPlaceholder && lastValueToken.Value == "," {
		// This is a tricky edge case. If we are inside a groupbale block, and the current token is not a placeholder,
		// we not only want to write the current token to the normalizedSQLBuilder, but also write the last comma that we skipped.
		// For example, (?, ARRAY[?, ?, ?]) should be normalized as (?, ARRAY[?])
		normalizedSQLBuilder.WriteString(lastValueToken.Value)
		return false
	}

	return false
}

func (n *Normalizer) appendWhitespace(source *string, token *Token, lastValueToken *LastValueToken, normalizedSQLBuilder *strings.Builder) {
	// do not add a space between parentheses if RemoveSpaceBetweenParentheses is true
	if n.config.RemoveSpaceBetweenParentheses && lastValueToken != nil && (lastValueToken.Type == FUNCTION || lastValueToken.Value == "(" || lastValueToken.Value == "[") {
		return
	}

	if n.config.RemoveSpaceBetweenParentheses && (token.String(source) == ")" || token.String(source) == "]") {
		return
	}

	switch token.String(source) {
	case ",":
	case ";":
	case "=":
		if lastValueToken != nil && lastValueToken.Value == ":" {
			// do not add a space before an equals if a colon was
			// present before it.
			break
		}
		fallthrough
	default:
		normalizedSQLBuilder.WriteString(" ")
	}
}

func (n *Normalizer) trimNormalizedSQL(normalizedSQL string) string {
	if !n.config.KeepTrailingSemicolon {
		// Remove trailing semicolon
		normalizedSQL = strings.TrimSuffix(normalizedSQL, ";")
	}
	return strings.TrimSpace(normalizedSQL)
}

func dedupeCollectedMetadata(metadata []string) (dedupedMetadata []string, size int) {
	// Dedupe collected metadata
	// e.g. [SELECT, JOIN, SELECT, JOIN] -> [SELECT, JOIN]
	dedupedMetadata = []string{}
	var metadataSeen = make(map[string]struct{})
	for _, m := range metadata {
		if _, seen := metadataSeen[m]; !seen {
			metadataSeen[m] = struct{}{}
			dedupedMetadata = append(dedupedMetadata, m)
			size += len(m)
		}
	}
	return dedupedMetadata, size
}

func dedupeStatementMetadata(info *StatementMetadata) {
	var tablesSize, commentsSize, commandsSize, procedureSize int
	info.Tables, tablesSize = dedupeCollectedMetadata(info.Tables)
	info.Comments, commentsSize = dedupeCollectedMetadata(info.Comments)
	info.Commands, commandsSize = dedupeCollectedMetadata(info.Commands)
	info.Procedures, procedureSize = dedupeCollectedMetadata(info.Procedures)
	info.Size += tablesSize + commentsSize + commandsSize + procedureSize
}
