package sqllexer

import (
	"unicode/utf8"
)

type TokenType int

const (
	ERROR TokenType = iota
	EOF
	WS                     // whitespace
	STRING                 // string literal
	INCOMPLETE_STRING      // incomplete string literal so that we can obfuscate it, e.g. 'abc
	NUMBER                 // number literal
	IDENT                  // identifier
	QUOTED_IDENT           // quoted identifier
	OPERATOR               // operator
	WILDCARD               // wildcard *
	COMMENT                // comment
	MULTILINE_COMMENT      // multiline comment
	PUNCTUATION            // punctuation
	DOLLAR_QUOTED_FUNCTION // dollar quoted function
	DOLLAR_QUOTED_STRING   // dollar quoted string
	POSITIONAL_PARAMETER   // numbered parameter
	BIND_PARAMETER         // bind parameter
	FUNCTION               // function
	SYSTEM_VARIABLE        // system variable
	UNKNOWN                // unknown token
	COMMAND                // SQL commands like SELECT, INSERT
	KEYWORD                // Other SQL keywords
	JSON_OP                // JSON operators
	BOOLEAN                // boolean literal
	NULL                   // null literal
	PROC_INDICATOR         // procedure indicator
	CTE_INDICATOR          // CTE indicator
	ALIAS_INDICATOR        // alias indicator
)

// Token represents a SQL token with its type and value.
type Token struct {
	Type             TokenType
	IsTableIndicator bool
	Start            int
	End              int
	ExtraInfo        *tokenExtraInfo
}

type LastValueToken struct {
	Type             TokenType
	Value            string
	IsTableIndicator bool
}

type tokenExtraInfo struct {
	Digits      []int
	Quotes      []int
	OutputValue string
}

func (t *Token) SetOutputValue(outputValue string) {
	if t.ExtraInfo == nil {
		t.ExtraInfo = &tokenExtraInfo{}
	}
	t.ExtraInfo.OutputValue = outputValue
}

// Add method to get value when needed
func (t *Token) Value(source *string) string {
	return (*source)[t.Start:t.End]
}

func (t *Token) String(source *string) string {
	if t.ExtraInfo != nil && t.ExtraInfo.OutputValue != "" {
		return t.ExtraInfo.OutputValue
	}
	return t.Value(source)
}

func (t *Token) GetLastValueToken(source *string) *LastValueToken {
	return &LastValueToken{
		Type:             t.Type,
		Value:            t.String(source),
		IsTableIndicator: t.IsTableIndicator,
	}
}

type LexerConfig struct {
	DBMS DBMSType `json:"dbms,omitempty"`
}

type lexerOption func(*LexerConfig)

func WithDBMS(dbms DBMSType) lexerOption {
	dbms = getDBMSFromAlias(dbms)
	return func(c *LexerConfig) {
		c.DBMS = dbms
	}
}

type trieNode struct {
	children         map[rune]*trieNode
	isEnd            bool
	tokenType        TokenType
	isTableIndicator bool
}

// SQL Lexer inspired from Rob Pike's talk on Lexical Scanning in Go
type Lexer struct {
	src              string // the input src string
	cursor           int    // the current position of the cursor
	start            int    // the start position of the current token
	config           *LexerConfig
	token            *Token
	digits           []int // Indexes of digits in the token
	quotes           []int // Indexes of quotes in the token
	isTableIndicator bool  // true if the token is a table indicator
}

func New(input string, opts ...lexerOption) *Lexer {
	lexer := &Lexer{
		src:    input,
		config: &LexerConfig{},
		token: &Token{
			ExtraInfo: &tokenExtraInfo{},
		},
	}
	for _, opt := range opts {
		opt(lexer.config)
	}
	return lexer
}

// Scan scans the next token and returns it.
func (s *Lexer) Scan() *Token {
	ch := s.peek()
	switch {
	case isWhitespace(ch):
		return s.scanWhitespace()
	case isLetter(ch):
		return s.scanIdentifier(ch)
	case isDoubleQuote(ch):
		return s.scanDoubleQuotedIdentifier('"')
	case isSingleQuote(ch):
		return s.scanString()
	case isSingleLineComment(ch, s.lookAhead(1)):
		return s.scanSingleLineComment()
	case isMultiLineComment(ch, s.lookAhead(1)):
		return s.scanMultiLineComment()
	case isLeadingSign(ch):
		// if the leading sign is followed by a digit, then it's a number
		// although this is not strictly true, it's good enough for our purposes
		nextCh := s.lookAhead(1)
		if isDigit(nextCh) || nextCh == '.' {
			return s.scanNumberWithLeadingSign()
		}
		return s.scanOperator(ch)
	case isDigit(ch):
		return s.scanNumber(ch)
	case isWildcard(ch):
		return s.scanWildcard()
	case ch == '$':
		if isDigit(s.lookAhead(1)) {
			// if the dollar sign is followed by a digit, then it's a numbered parameter
			return s.scanPositionalParameter()
		}
		if s.config.DBMS == DBMSSQLServer && isLetter(s.lookAhead(1)) {
			return s.scanIdentifier(ch)
		}
		return s.scanDollarQuotedString()
	case ch == ':':
		if s.config.DBMS == DBMSOracle && isAlphaNumeric(s.lookAhead(1)) {
			return s.scanBindParameter()
		}
		return s.scanOperator(ch)
	case ch == '`':
		if s.config.DBMS == DBMSMySQL {
			return s.scanDoubleQuotedIdentifier('`')
		}
		fallthrough
	case ch == '#':
		if s.config.DBMS == DBMSSQLServer {
			return s.scanIdentifier(ch)
		} else if s.config.DBMS == DBMSMySQL {
			// MySQL treats # as a comment
			return s.scanSingleLineComment()
		}
		return s.scanOperator(ch)
	case ch == '@':
		if s.lookAhead(1) == '@' {
			if isAlphaNumeric(s.lookAhead(2)) {
				return s.scanSystemVariable()
			}
			s.start = s.cursor
			s.nextBy(2) // consume @@
			return s.emit(JSON_OP)
		}
		if isAlphaNumeric(s.lookAhead(1)) {
			if s.config.DBMS == DBMSSnowflake {
				return s.scanIdentifier(ch)
			}
			return s.scanBindParameter()
		}
		if s.lookAhead(1) == '?' || s.lookAhead(1) == '>' {
			s.start = s.cursor
			s.nextBy(2) // consume @? or @>
			return s.emit(JSON_OP)
		}
		fallthrough
	case isOperator(ch):
		return s.scanOperator(ch)
	case isPunctuation(ch):
		if ch == '[' && s.config.DBMS == DBMSSQLServer {
			return s.scanDoubleQuotedIdentifier('[')
		}
		return s.scanPunctuation()
	case isEOF(ch):
		return s.emit(EOF)
	default:
		return s.emit(UNKNOWN)
	}
}

// lookAhead returns the rune n positions ahead of the cursor.
func (s *Lexer) lookAhead(n int) rune {
	if s.cursor+n >= len(s.src) || s.cursor+n < 0 {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(s.src[s.cursor+n:])
	return r
}

// peek returns the rune at the cursor position.
func (s *Lexer) peek() rune {
	return s.lookAhead(0)
}

// nextBy advances the cursor by n positions and returns the rune at the cursor position.
func (s *Lexer) nextBy(n int) rune {
	// advance the cursor by n and return the rune at the cursor position
	if s.cursor+n > len(s.src) {
		return 0
	}
	s.cursor += n
	if s.cursor >= len(s.src) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(s.src[s.cursor:])
	return r
}

// next advances the cursor by 1 position and returns the rune at the cursor position.
func (s *Lexer) next() rune {
	return s.nextBy(1)
}

func (s *Lexer) matchAt(match []rune) bool {
	if s.cursor+len(match) > len(s.src) {
		return false
	}
	for i, ch := range match {
		if s.src[s.cursor+i] != byte(ch) {
			return false
		}
	}
	return true
}

func (s *Lexer) scanNumberWithLeadingSign() *Token {
	s.start = s.cursor
	s.next() // consume the leading sign
	return s.scanDecimalNumber()
}

func (s *Lexer) scanNumber(ch rune) *Token {
	s.start = s.cursor
	return s.scanNumberic(ch)
}

func (s *Lexer) scanNumberic(ch rune) *Token {
	s.start = s.cursor
	if ch == '0' {
		nextCh := s.lookAhead(1)
		if nextCh == 'x' || nextCh == 'X' {
			return s.scanHexNumber()
		} else if nextCh >= '0' && nextCh <= '7' {
			return s.scanOctalNumber()
		}
	}

	s.next() // consume first digit
	return s.scanDecimalNumber()
}

func (s *Lexer) scanDecimalNumber() *Token {
	ch := s.peek()

	// scan digits
	for isDigit(ch) || ch == '.' || isExpontent(ch) {
		if isExpontent(ch) {
			ch = s.next()
			if isLeadingSign(ch) {
				s.next()
			}
		} else {
			s.next()
		}
		ch = s.peek()
	}
	return s.emit(NUMBER)
}

func (s *Lexer) scanHexNumber() *Token {
	ch := s.nextBy(2) // consume 0x or 0X

	for isDigit(ch) || ('a' <= ch && ch <= 'f') || ('A' <= ch && ch <= 'F') {
		ch = s.next()
	}
	return s.emit(NUMBER)
}

func (s *Lexer) scanOctalNumber() *Token {
	ch := s.nextBy(2) // consume the leading 0 and number

	for '0' <= ch && ch <= '7' {
		ch = s.next()
	}
	return s.emit(NUMBER)
}

func (s *Lexer) scanString() *Token {
	s.start = s.cursor
	escaped := false

	for ch := s.next(); !isEOF(ch); ch = s.next() {
		if escaped {
			// encountered an escape character
			// reset the escaped flag and continue
			escaped = false
			continue
		}

		if ch == '\\' {
			escaped = true
			continue
		}

		if ch == '\'' {
			s.next() // consume the closing quote
			return s.emit(STRING)
		}
	}
	// If we get here, we hit EOF before finding closing quote
	return s.emit(INCOMPLETE_STRING)
}

func (s *Lexer) scanIdentifier(ch rune) *Token {
	s.start = s.cursor
	node := keywordRoot
	pos := s.cursor

	// If first character is Unicode, skip trie lookup
	if ch > 127 {
		for isIdentifier(ch) {
			if isDigit(ch) {
				s.digits = append(s.digits, s.cursor)
			}
			ch = s.nextBy(utf8.RuneLen(ch))
		}
		return s.emit(IDENT)
	}

	// ASCII characters - try keyword matching
	for isAsciiLetter(ch) || ch == '_' {
		// Convert to uppercase for case-insensitive matching
		upperCh := ch
		if ch >= 'a' && ch <= 'z' {
			upperCh -= 32
		}

		// Try to follow trie path
		if next, exists := node.children[upperCh]; exists {
			node = next
			pos = s.cursor
			ch = s.next()
		} else {
			// No more matches possible in trie
			// Reset node for next potential keyword
			// and continue scanning identifier
			node = keywordRoot
			ch = s.next()
			break
		}
	}

	// If we found a complete keyword and next char is whitespace
	if node.isEnd && (isPunctuation(s.peek()) || isWhitespace(s.peek()) || isEOF(s.peek())) {
		s.cursor = pos + 1 // Include the last matched character
		s.isTableIndicator = node.isTableIndicator
		return s.emit(node.tokenType)
	}

	// Continue scanning identifier if no keyword match
	for isIdentifier(ch) {
		if isDigit(ch) {
			s.digits = append(s.digits, s.cursor)
		}
		ch = s.nextBy(utf8.RuneLen(ch))
	}

	if ch == '(' {
		return s.emit(FUNCTION)
	}
	return s.emit(IDENT)
}

func (s *Lexer) scanDoubleQuotedIdentifier(delimiter rune) *Token {
	closingDelimiter := delimiter
	if delimiter == '[' {
		closingDelimiter = ']'
	}

	s.start = s.cursor
	s.quotes = append(s.quotes, s.cursor) // store the opening quote position
	ch := s.next()                        // consume the opening quote
	for {
		// encountered the closing quote
		// BUT if it's followed by .", then we should keep going
		// e.g. postgre "foo"."bar"
		// e.g. sqlserver [foo].[bar]
		if ch == closingDelimiter {
			s.quotes = append(s.quotes, s.cursor)
			specialCase := []rune{closingDelimiter, '.', delimiter}
			if s.matchAt([]rune(specialCase)) {
				s.quotes = append(s.quotes, s.cursor+2)
				ch = s.nextBy(3) // consume the "."
				continue
			}
			break
		}
		if isEOF(ch) {
			return s.emit(ERROR)
		}
		if isDigit(ch) {
			s.digits = append(s.digits, s.cursor)
		}
		ch = s.next()
	}
	s.next() // consume the closing quote
	return s.emit(QUOTED_IDENT)
}

func (s *Lexer) scanWhitespace() *Token {
	// scan whitespace, tab, newline, carriage return
	s.start = s.cursor
	ch := s.next()
	for isWhitespace(ch) {
		ch = s.next()
	}
	return s.emit(WS)
}

func (s *Lexer) scanOperator(lastCh rune) *Token {
	s.start = s.cursor
	ch := s.next() // consume the first character

	// Check for json operators
	switch lastCh {
	case '-':
		if ch == '>' {
			ch = s.next()
			if ch == '>' {
				s.next()
				return s.emit(JSON_OP) // ->>
			}
			return s.emit(JSON_OP) // ->
		}
	case '#':
		if ch == '>' {
			ch = s.next()
			if ch == '>' {
				s.next()
				return s.emit(JSON_OP) // #>>
			}
			return s.emit(JSON_OP) // #>
		} else if ch == '-' {
			s.next()
			return s.emit(JSON_OP) // #-
		}
	case '?':
		if ch == '|' {
			s.next()
			return s.emit(JSON_OP) // ?|
		} else if ch == '&' {
			s.next()
			return s.emit(JSON_OP) // ?&
		}
	case '<':
		if ch == '@' {
			s.next()
			return s.emit(JSON_OP) // <@
		}
	}

	for isOperator(ch) && !(lastCh == '=' && (ch == '?' || ch == '@')) {
		// hack: we don't want to treat "=?" as an single operator
		lastCh = ch
		ch = s.next()
	}

	return s.emit(OPERATOR)
}

func (s *Lexer) scanWildcard() *Token {
	s.start = s.cursor
	s.next()
	return s.emit(WILDCARD)
}

func (s *Lexer) scanSingleLineComment() *Token {
	s.start = s.cursor
	ch := s.nextBy(2) // consume the opening dashes
	for ch != '\n' && !isEOF(ch) {
		ch = s.next()
	}
	return s.emit(COMMENT)
}

func (s *Lexer) scanMultiLineComment() *Token {
	s.start = s.cursor
	ch := s.nextBy(2) // consume the opening slash and asterisk
	for {
		if ch == '*' && s.lookAhead(1) == '/' {
			s.nextBy(2) // consume the closing asterisk and slash
			break
		}
		if isEOF(ch) {
			// encountered EOF before closing comment
			// this usually happens when the comment is truncated
			return s.emit(ERROR)
		}
		ch = s.next()
	}
	return s.emit(MULTILINE_COMMENT)
}

func (s *Lexer) scanPunctuation() *Token {
	s.start = s.cursor
	s.next()
	return s.emit(PUNCTUATION)
}

func (s *Lexer) scanDollarQuotedString() *Token {
	s.start = s.cursor
	ch := s.next() // consume the dollar sign
	tagStart := s.cursor

	for s.cursor < len(s.src) && ch != '$' {
		ch = s.next()
	}
	s.next()                            // consume the closing dollar sign of the tag
	tag := s.src[tagStart-1 : s.cursor] // include the opening and closing dollar sign e.g. $tag$

	for s.cursor < len(s.src) {
		if s.matchAt([]rune(tag)) {
			s.nextBy(len(tag)) // consume the closing tag
			if tag == "$func$" {
				return s.emit(DOLLAR_QUOTED_FUNCTION)
			}
			return s.emit(DOLLAR_QUOTED_STRING)
		}
		s.next()
	}
	return s.emit(ERROR)
}

func (s *Lexer) scanPositionalParameter() *Token {
	s.start = s.cursor
	ch := s.nextBy(2) // consume the dollar sign and the number
	for {
		if !isDigit(ch) {
			break
		}
		ch = s.next()
	}
	return s.emit(POSITIONAL_PARAMETER)
}

func (s *Lexer) scanBindParameter() *Token {
	s.start = s.cursor
	ch := s.nextBy(2) // consume the (colon|at sign) and the char
	for {
		if !isAlphaNumeric(ch) {
			break
		}
		ch = s.next()
	}
	return s.emit(BIND_PARAMETER)
}

func (s *Lexer) scanSystemVariable() *Token {
	s.start = s.cursor
	ch := s.nextBy(2) // consume @@
	// Must be followed by at least one alphanumeric character
	if !isAlphaNumeric(ch) {
		return s.emit(ERROR)
	}
	for isAlphaNumeric(ch) {
		ch = s.next()
	}
	return s.emit(SYSTEM_VARIABLE)
}

// Modify emit function to use positions and maintain links
func (s *Lexer) emit(t TokenType) *Token {
	tok := s.token

	extraInfo := tok.ExtraInfo

	// Zero other fields
	*tok = Token{
		Type:             t,
		Start:            s.start,
		End:              s.cursor,
		IsTableIndicator: s.isTableIndicator,
		ExtraInfo:        extraInfo,
	}

	if len(s.digits) > 0 || len(s.quotes) > 0 {
		tok.ExtraInfo.Digits = s.digits
		tok.ExtraInfo.Quotes = s.quotes
	} else {
		tok.ExtraInfo.Digits = nil
		tok.ExtraInfo.Quotes = nil
	}
	tok.ExtraInfo.OutputValue = "" // Reset this

	// Reset lexer state
	s.start = s.cursor
	s.digits = nil
	s.quotes = nil
	s.isTableIndicator = false

	return tok
}
