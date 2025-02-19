package sqllexer

import (
	"strings"
	"sync"
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
	// Internal maps for deduping during collection - exclude from JSON
	tablesSet     map[string]struct{} `json:"-"`
	commentsSet   map[string]struct{} `json:"-"`
	commandsSet   map[string]struct{} `json:"-"`
	proceduresSet map[string]struct{} `json:"-"`
}

// addMetadata adds a value to a metadata slice if it doesn't exist in the set
func (sm *StatementMetadata) addMetadata(value string, set map[string]struct{}, slice *[]string) {
	if _, exists := set[value]; !exists {
		set[value] = struct{}{}
		*slice = append(*slice, value)
		sm.Size += len(value)
	}
}

type groupablePlaceholder struct {
	groupable bool
}

type headState struct {
	readFirstNonSpaceNonComment         bool
	inLeadingParenthesesExpression      bool
	foundLeadingExpressionInParentheses bool
	standaloneExpressionInParentheses   bool
	expressionInParentheses             strings.Builder
}

type Normalizer struct {
	config *normalizerConfig
}

// Add a pool for StatementMetadata reuse
var statementMetadataPool = sync.Pool{
	New: func() interface{} {
		return &StatementMetadata{
			Tables:        make([]string, 0, 4),
			Comments:      make([]string, 0, 2),
			Commands:      make([]string, 0, 4),
			Procedures:    make([]string, 0),
			tablesSet:     make(map[string]struct{}, 4),
			commentsSet:   make(map[string]struct{}, 2),
			commandsSet:   make(map[string]struct{}, 4),
			proceduresSet: make(map[string]struct{}),
		}
	},
}

// Reset StatementMetadata for reuse
func (sm *StatementMetadata) reset() {
	sm.Size = 0
	sm.Tables = sm.Tables[:0]
	sm.Comments = sm.Comments[:0]
	sm.Commands = sm.Commands[:0]
	sm.Procedures = sm.Procedures[:0]

	// Just create new maps instead of clearing old ones
	sm.tablesSet = make(map[string]struct{}, 4)
	sm.commentsSet = make(map[string]struct{}, 2)
	sm.commandsSet = make(map[string]struct{}, 4)
	sm.proceduresSet = make(map[string]struct{})
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

	statementMetadata = statementMetadataPool.Get().(*StatementMetadata)
	statementMetadata.reset()
	defer statementMetadataPool.Put(statementMetadata)

	var groupablePlaceholder groupablePlaceholder
	var headState headState
	var ctes map[string]bool

	// Only allocate CTEs map if collecting tables
	if n.config.CollectTables {
		ctes = make(map[string]bool, 2)
	}

	var lastValueToken *LastValueToken

	for {
		token := lexer.Scan()
		if n.shouldCollectMetadata() {
			n.collectMetadata(token, lastValueToken, statementMetadata, ctes)
		}
		n.normalizeSQL(token, lastValueToken, normalizedSQLBuilder, &groupablePlaceholder, &headState, lexerOpts...)
		if token.Type == EOF {
			break
		}
		if isValueToken(token) {
			lastValueToken = token.getLastValueToken()
		}
	}

	normalizedSQL = normalizedSQLBuilder.String()
	return n.trimNormalizedSQL(normalizedSQL), statementMetadata, nil
}

func (n *Normalizer) shouldCollectMetadata() bool {
	return n.config.CollectTables || n.config.CollectCommands || n.config.CollectComments || n.config.CollectProcedure
}

func (n *Normalizer) collectMetadata(token *Token, lastValueToken *LastValueToken, statementMetadata *StatementMetadata, ctes map[string]bool) {
	if n.config.CollectComments && (token.Type == COMMENT || token.Type == MULTILINE_COMMENT) {
		comment := token.Value
		statementMetadata.addMetadata(comment, statementMetadata.commentsSet, &statementMetadata.Comments)
	} else if token.Type == COMMAND {
		if n.config.CollectCommands {
			command := strings.ToUpper(token.Value)
			statementMetadata.addMetadata(command, statementMetadata.commandsSet, &statementMetadata.Commands)
		}
	} else if token.Type == IDENT || token.Type == QUOTED_IDENT || token.Type == FUNCTION {
		tokenVal := token.Value
		if token.Type == QUOTED_IDENT {
			tokenVal = trimQuotes(token)
			if !n.config.KeepIdentifierQuotation {
				// trim quotes and set the token type to IDENT
				token.Value = tokenVal
				token.Type = IDENT
			}
		}
		if lastValueToken != nil && lastValueToken.Type == CTE_INDICATOR {
			ctes[tokenVal] = true
		} else if n.config.CollectTables && lastValueToken != nil && lastValueToken.IsTableIndicator {
			if _, ok := ctes[tokenVal]; !ok {
				statementMetadata.addMetadata(tokenVal, statementMetadata.tablesSet, &statementMetadata.Tables)
			}
		} else if n.config.CollectProcedure && lastValueToken != nil && lastValueToken.Type == PROC_INDICATOR {
			statementMetadata.addMetadata(tokenVal, statementMetadata.proceduresSet, &statementMetadata.Procedures)
		}
	}
}

func (n *Normalizer) normalizeSQL(token *Token, lastValueToken *LastValueToken, normalizedSQLBuilder *strings.Builder, groupablePlaceholder *groupablePlaceholder, headState *headState, lexerOpts ...lexerOption) {
	if token.Type != SPACE && token.Type != COMMENT && token.Type != MULTILINE_COMMENT {
		if token.Type == QUOTED_IDENT && !n.config.KeepIdentifierQuotation {
			token.Value = trimQuotes(token)
		}

		// handle leading expression in parentheses
		if !headState.readFirstNonSpaceNonComment {
			headState.readFirstNonSpaceNonComment = true
			if token.Type == PUNCTUATION && token.Value == "(" {
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

		if token.Type == DOLLAR_QUOTED_FUNCTION && token.Value != StringPlaceholder {
			// if the token is a dollar quoted function and it is not obfuscated,
			// we need to recusively normalize the content of the dollar quoted function
			quotedFunc := token.Value[6 : len(token.Value)-6] // remove the $func$ prefix and suffix
			normalizedQuotedFunc, _, err := n.Normalize(quotedFunc, lexerOpts...)
			if err == nil {
				// replace the content of the dollar quoted function with the normalized content
				// if there is an error, we just keep the original content
				normalizedDollarQuotedFunc := new(strings.Builder)
				normalizedDollarQuotedFunc.Grow(len(normalizedQuotedFunc) + 12)
				normalizedDollarQuotedFunc.WriteString("$func$")
				normalizedDollarQuotedFunc.WriteString(normalizedQuotedFunc)
				normalizedDollarQuotedFunc.WriteString("$func$")
				token.Value = normalizedDollarQuotedFunc.String()
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
					n.appendWhitespace(token, lastValueToken, normalizedSQLBuilder)
					n.writeToken(lastValueToken.Type, lastValueToken.Value, normalizedSQLBuilder)
				}
			}
		}

		// group consecutive obfuscated values into single placeholder
		if n.isObfuscatedValueGroupable(token, lastValueToken, groupablePlaceholder, normalizedSQLBuilder) {
			// return the token but not write it to the normalizedSQLBuilder
			return
		}

		if headState.inLeadingParenthesesExpression {
			n.appendWhitespace(token, lastValueToken, &headState.expressionInParentheses)
			n.writeToken(token.Type, token.Value, &headState.expressionInParentheses)
			if token.Type == PUNCTUATION && token.Value == ")" {
				headState.inLeadingParenthesesExpression = false
				headState.foundLeadingExpressionInParentheses = true
			}
		} else {
			n.appendWhitespace(token, lastValueToken, normalizedSQLBuilder)
			n.writeToken(token.Type, token.Value, normalizedSQLBuilder)
		}
	}
}

func (n *Normalizer) writeToken(tokenType TokenType, tokenValue string, normalizedSQLBuilder *strings.Builder) {
	if n.config.UppercaseKeywords && (tokenType == COMMAND || tokenType == KEYWORD) {
		normalizedSQLBuilder.WriteString(strings.ToUpper(tokenValue))
	} else {
		normalizedSQLBuilder.WriteString(tokenValue)
	}
}

func (n *Normalizer) isObfuscatedValueGroupable(token *Token, lastValueToken *LastValueToken, groupablePlaceholder *groupablePlaceholder, normalizedSQLBuilder *strings.Builder) bool {
	if token.Value == NumberPlaceholder || token.Value == StringPlaceholder {
		if lastValueToken.Value == "(" || lastValueToken.Value == "[" {
			// if the last token is "(" or "[", and the current token is a placeholder,
			// we know it's the start of groupable placeholders
			// we don't return here because we still need to write the first placeholder
			groupablePlaceholder.groupable = true
		} else if lastValueToken.Value == "," && groupablePlaceholder.groupable {
			return true
		}
	}

	if lastValueToken != nil && (lastValueToken.Value == NumberPlaceholder || lastValueToken.Value == StringPlaceholder) && token.Value == "," && groupablePlaceholder.groupable {
		return true
	}

	if groupablePlaceholder.groupable && (token.Value == ")" || token.Value == "]") {
		// end of groupable placeholders
		groupablePlaceholder.groupable = false
		return false
	}

	if groupablePlaceholder.groupable && token.Value != NumberPlaceholder && token.Value != StringPlaceholder && lastValueToken.Value == "," {
		// This is a tricky edge case. If we are inside a groupbale block, and the current token is not a placeholder,
		// we not only want to write the current token to the normalizedSQLBuilder, but also write the last comma that we skipped.
		// For example, (?, ARRAY[?, ?, ?]) should be normalized as (?, ARRAY[?])
		normalizedSQLBuilder.WriteString(lastValueToken.Value)
		return false
	}

	return false
}

func (n *Normalizer) appendWhitespace(token *Token, lastValueToken *LastValueToken, normalizedSQLBuilder *strings.Builder) {
	// do not add a space between parentheses if RemoveSpaceBetweenParentheses is true
	if n.config.RemoveSpaceBetweenParentheses && lastValueToken != nil && (lastValueToken.Type == FUNCTION || lastValueToken.Value == "(" || lastValueToken.Value == "[") {
		return
	}

	if n.config.RemoveSpaceBetweenParentheses && (token.Value == ")" || token.Value == "]") {
		return
	}

	switch token.Value {
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
