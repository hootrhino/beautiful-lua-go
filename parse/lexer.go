package parse

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/hootrhino/beautiful-lua-go/ast"
)

const EOF = -1
const whitespace1 = 1<<'\t' | 1<<' '
const whitespace2 = 1<<'\t' | 1<<'\n' | 1<<'\r' | 1<<' '

type Error struct {
	Pos     ast.Position
	Message string
	Token   string
}

func (e *Error) Error() string {
	pos := e.Pos
	if pos.Line == EOF {
		return fmt.Sprintf("%v at EOF:   %s\n", pos.Source, e.Message)
	} else {
		return fmt.Sprintf("%v line:%d(column:%d) near '%v':   %s\n", pos.Source, pos.Line, pos.Column, e.Token, e.Message)
	}
}

func writeChar(buf *bytes.Buffer, c int) { buf.WriteByte(byte(c)) }

func isDecimal(ch int) bool { return '0' <= ch && ch <= '9' }

func isOctal(ch int) bool { return '0' <= ch && ch <= '7' }

func isBinary(ch int) bool { return ch == '0' || ch == '1' }

func isDigit(ch int) bool {
	return '0' <= ch && ch <= '9' || 'a' <= ch && ch <= 'f' || 'A' <= ch && ch <= 'F'
}

func isIdent(ch int, pos int) bool {
	return ch == '_' || 'A' <= ch && ch <= 'Z' || 'a' <= ch && ch <= 'z' || isDecimal(ch) && pos > 0
}

type Scanner struct {
	Pos    ast.Position
	reader *bufio.Reader
}

func NewScanner(reader io.Reader, source string) *Scanner {
	return &Scanner{
		Pos: ast.Position{
			Source: source,
			Line:   1,
			Column: 0,
		},
		reader: bufio.NewReaderSize(reader, 4096),
	}
}

func (sc *Scanner) Error(tok string, msg string) *Error { return &Error{sc.Pos, msg, tok} }

func (sc *Scanner) TokenError(tok ast.Token, msg string) *Error { return &Error{tok.Pos, msg, tok.Str} }

func (sc *Scanner) readNext() int {
	ch, err := sc.reader.ReadByte()
	if err == io.EOF {
		return EOF
	}
	return int(ch)
}

func (sc *Scanner) Newline(ch int) {
	if ch < 0 {
		return
	}
	sc.Pos.Line += 1
	sc.Pos.Column = 0
	next := sc.Peek()
	if ch == '\n' && next == '\r' || ch == '\r' && next == '\n' {
		sc.reader.ReadByte()
	}
}

func (sc *Scanner) Next() int {
	ch := sc.readNext()
	switch ch {
	case '\n', '\r':
		sc.Newline(ch)
		ch = int('\n')
	case EOF:
		sc.Pos.Line = EOF
		sc.Pos.Column = 0
	default:
		sc.Pos.Column++
	}
	return ch
}

func (sc *Scanner) Peek() int {
	ch := sc.readNext()
	if ch != EOF {
		sc.reader.UnreadByte()
	}
	return ch
}

func (sc *Scanner) skipWhiteSpace(whitespace int64) int {
	ch := sc.Next()
	for ; whitespace&(1<<uint(ch)) != 0; ch = sc.Next() {
	}
	return ch
}

func (sc *Scanner) skipComments(ch int) error {
	// multiline comment
	if sc.Peek() == '[' {
		ch = sc.Next()
		if sc.Peek() == '[' || sc.Peek() == '=' {
			var buf bytes.Buffer
			if err := sc.scanMultilineString(sc.Next(), &buf); err != nil {
				return sc.Error(buf.String(), "invalid multiline comment")
			}
			return nil
		}
	}
	for {
		if ch == '\n' || ch == '\r' || ch < 0 {
			break
		}
		ch = sc.Next()
	}
	return nil
}

func (sc *Scanner) scanIdent(ch int, buf *bytes.Buffer) error {
	writeChar(buf, ch)
	for isIdent(sc.Peek(), 1) {
		writeChar(buf, sc.Next())
	}
	return nil
}

func (sc *Scanner) scanDecimal(ch int, buf *bytes.Buffer) error {
	writeChar(buf, ch)
	for isDecimal(sc.Peek()) || sc.Peek() == '_' {
		if sc.Peek() == '_' {
			sc.Next()
			continue
		}
		writeChar(buf, sc.Next())
	}
	return nil
}

func (sc *Scanner) scanNumber(ch int, buf *bytes.Buffer) (float64, error) {
	if ch == '0' {
		switch sc.Peek() {
		case 'x', 'X':
			n := sc.Next()
			if !isDigit(sc.Peek()) {
				writeChar(buf, ch)
				writeChar(buf, n)
				return 0, sc.Error(buf.String(), "hex number expected")
			}
			for isDigit(sc.Peek()) || sc.Peek() == '_' {
				if sc.Peek() == '_' {
					sc.Next()
					continue
				}
				writeChar(buf, sc.Next())
			}
			val, err := strconv.ParseInt(buf.String(), 16, 64)
			return float64(val), err
		case 'b', 'B':
			n := sc.Next()
			if !isBinary(sc.Peek()) {
				writeChar(buf, ch)
				writeChar(buf, n)
				return 0, sc.Error(buf.String(), "binary number expected")
			}
			for isBinary(sc.Peek()) || sc.Peek() == '_' {
				if sc.Peek() == '_' {
					sc.Next()
					continue
				}
				writeChar(buf, sc.Next())
			}
			val, err := strconv.ParseInt(buf.String(), 2, 64)
			return float64(val), err
		case 'o', 'O':
			n := sc.Next()
			if !isOctal(sc.Peek()) {
				writeChar(buf, ch)
				writeChar(buf, n)
				return 0, sc.Error(buf.String(), "octal number expected")
			}
			for isOctal(sc.Peek()) || sc.Peek() == '_' {
				if sc.Peek() == '_' {
					sc.Next()
					continue
				}
				writeChar(buf, sc.Next())
			}
			val, err := strconv.ParseInt(buf.String(), 8, 64)
			return float64(val), err
		default:
			if sc.Peek() != '.' && isDecimal(sc.Peek()) {
				ch = sc.Next()
			}
		}
	}
	sc.scanDecimal(ch, buf)
	if sc.Peek() == '.' {
		sc.scanDecimal(sc.Next(), buf)
	}
	if ch = sc.Peek(); ch == 'e' || ch == 'E' {
		writeChar(buf, sc.Next())
		if ch = sc.Peek(); ch == '-' || ch == '+' {
			writeChar(buf, sc.Next())
		}
		sc.scanDecimal(sc.Next(), buf)
	}
	return strconv.ParseFloat(buf.String(), 64)
}

func (sc *Scanner) scanString(quote int, buf *bytes.Buffer) error {
	ch := sc.Next()
	for ch != quote {
		if ch == '\n' || ch == '\r' || ch < 0 {
			return sc.Error(buf.String(), "unterminated string")
		}
		if ch == '\\' {
			ch = sc.Next()
			if ch == 'z' {
				ch = sc.skipWhiteSpace(whitespace2)
				continue
			}
			if err := sc.scanEscape(ch, buf); err != nil {
				return err
			}
		} else {
			writeChar(buf, ch)
		}
		ch = sc.Next()
	}
	return nil
}

func (sc *Scanner) scanEscape(ch int, buf *bytes.Buffer) error {
	switch ch {
	case 'a':
		buf.WriteByte('\a')
	case 'b':
		buf.WriteByte('\b')
	case 'f':
		buf.WriteByte('\f')
	case 'n':
		buf.WriteByte('\n')
	case 'r':
		buf.WriteByte('\r')
	case 't':
		buf.WriteByte('\t')
	case 'v':
		buf.WriteByte('\v')
	case 'x':
		var bytes []byte
		for i := 0; i < 2; i++ {
			ch = sc.Next()
			if !isDigit(ch) {
				return sc.Error(buf.String(), "hex digit expected")
			}
			bytes = append(bytes, byte(ch))
		}
		val, _ := strconv.ParseInt(string(bytes), 16, 32)
		buf.WriteRune(rune(val))
	case 'u':
		var index int
		var bytes []byte

		if sc.Next() != '{' {
			return sc.Error(buf.String(), "{ expected")
		}

		for {
			ch = sc.Next()
			if ch == '}' && index > 0 {
				break
			}
			if !isDigit(ch) {
				return sc.Error(buf.String(), "hex digit expected")
			}
			if index > 4 {
				return sc.Error(buf.String(), "UTF-8 value too large")
			}
			bytes = append(bytes, byte(ch))
			index++
		}
		val, _ := strconv.ParseInt(string(bytes), 16, 32)
		buf.WriteRune(rune(val))
	case '\\':
		buf.WriteByte('\\')
	case '"':
		buf.WriteByte('"')
	case '\'':
		buf.WriteByte('\'')
	case '\n':
		buf.WriteByte('\n')
	case '\r':
		buf.WriteByte('\n')
		sc.Newline('\r')
	default:
		if '0' <= ch && ch <= '9' {
			bytes := []byte{byte(ch)}
			for i := 0; i < 2 && isDecimal(sc.Peek()); i++ {
				bytes = append(bytes, byte(sc.Next()))
			}
			val, _ := strconv.ParseInt(string(bytes), 10, 32)
			writeChar(buf, int(val))
		} else {
			buf.WriteByte('\\')
			writeChar(buf, ch)
			return sc.Error(buf.String(), "Invalid escape sequence")
		}
	}
	return nil
}

func (sc *Scanner) countSep(ch int) (int, int) {
	count := 0
	for ; ch == '='; count = count + 1 {
		ch = sc.Next()
	}
	return count, ch
}

func (sc *Scanner) scanMultilineString(ch int, buf *bytes.Buffer) error {
	var count1, count2 int
	count1, ch = sc.countSep(ch)
	if ch != '[' {
		return sc.Error(string(ch), "invalid multiline string")
	}
	ch = sc.Next()
	if ch == '\n' || ch == '\r' {
		ch = sc.Next()
	}
	for {
		if ch < 0 {
			return sc.Error(buf.String(), "unterminated multiline string")
		} else if ch == ']' {
			count2, ch = sc.countSep(sc.Next())
			if count1 == count2 && ch == ']' {
				goto finally
			}
			buf.WriteByte(']')
			buf.WriteString(strings.Repeat("=", count2))
			continue
		}
		writeChar(buf, ch)
		ch = sc.Next()
	}

finally:
	return nil
}

var reservedWords = map[string]int{
	"and": TAnd, "break": TBreak, "continue": TContinue, "do": TDo, "else": TElse, "elseif": TElseIf,
	"end": TEnd, "false": TFalse, "for": TFor, "function": TFunction,
	"if": TIf, "in": TIn, "local": TLocal, "nil": TNil, "not": TNot, "or": TOr,
	"return": TReturn, "repeat": TRepeat, "then": TThen, "true": TTrue,
	"until": TUntil, "while": TWhile, "goto": TGoto}

func (sc *Scanner) Scan(lexer *Lexer) (ast.Token, error) {
redo:
	var err error
	tok := ast.Token{}
	newline := false

	ch := sc.skipWhiteSpace(whitespace1)
	if ch == '\n' || ch == '\r' {
		newline = true
		ch = sc.skipWhiteSpace(whitespace2)
	}

	if ch == '(' && lexer.PrevTokenType == ')' {
		lexer.PNewLine = newline
	} else {
		lexer.PNewLine = false
	}

	var _buf bytes.Buffer
	buf := &_buf
	tok.Pos = sc.Pos

	switch {
	case isIdent(ch, 0):
		tok.Type = TIdent
		err = sc.scanIdent(ch, buf)
		tok.Str = buf.String()
		if err != nil {
			goto finally
		}
		if typ, ok := reservedWords[tok.Str]; ok {
			tok.Type = typ
		}
	case isDecimal(ch):
		tok.Type = TNumber
		tok.Num, err = sc.scanNumber(ch, buf)
	default:
		switch ch {
		case EOF:
			tok.Type = EOF
		case '-':
			ch2 := sc.Peek()
			switch ch2 {
			case '-':
				err = sc.skipComments(sc.Next())
				if err != nil {
					goto finally
				}
				goto redo
			case '=':
				tok.Type = TCompound
				tok.Str = "-="
				sc.Next()
			default:
				tok.Type = ch
				tok.Str = string(ch)
			}
		case '"', '\'':
			tok.Type = TString
			err = sc.scanString(ch, buf)
			tok.Str = buf.String()
		case '[':
			if c := sc.Peek(); c == '[' || c == '=' {
				tok.Type = TString
				err = sc.scanMultilineString(sc.Next(), buf)
				tok.Str = buf.String()
			} else {
				tok.Type = ch
				tok.Str = string(ch)
			}
		case '=':
			if sc.Peek() == '=' {
				tok.Type = TEqeq
				tok.Str = "=="
				sc.Next()
			} else {
				tok.Type = ch
				tok.Str = string(ch)
			}
		case '~':
			if sc.Peek() == '=' {
				tok.Type = TNeq
				tok.Str = "~="
				sc.Next()
			} else {
				tok.Type = ch
				tok.Str = string(ch)
			}
		case '<':
			ch2 := sc.Peek()
			switch ch2 {
			case '=':
				tok.Type = TLte
				tok.Str = "<="
				sc.Next()
			case '<':
				tok.Type = TLshift
				tok.Str = "<<"
				sc.Next()
			default:
				tok.Type = ch
				tok.Str = string(ch)
			}
		case '>':
			ch2 := sc.Peek()
			switch ch2 {
			case '=':
				tok.Type = TGte
				tok.Str = ">="
				sc.Next()
			case '>':
				tok.Type = TRshift
				tok.Str = ">>"
				sc.Next()
			default:
				tok.Type = ch
				tok.Str = string(ch)
			}
		case '/':
			ch2 := sc.Peek()
			switch ch2 {
			case '/':
				tok.Type = TFloorDiv
				tok.Str = "//"
				sc.Next()
			case '=':
				tok.Type = TCompound
				tok.Str = "/="
				sc.Next()
			default:
				tok.Type = ch
				tok.Str = string(ch)
			}
		case ':':
			if sc.Peek() == ':' {
				tok.Type = T2Colon
				tok.Str = "::"
				sc.Next()
			} else {
				tok.Type = ch
				tok.Str = string(ch)
			}
		case '.':
			ch2 := sc.Peek()
			switch {
			case isDecimal(ch2):
				tok.Type = TNumber
				tok.Num, err = sc.scanNumber(ch, buf)
			case ch2 == '.':
				writeChar(buf, ch)
				writeChar(buf, sc.Next())
				switch sc.Peek() {
				case '.':
					writeChar(buf, sc.Next())
					tok.Type = T3Comma
				case '=':
					writeChar(buf, sc.Next())
					tok.Type = TCompound
				default:
					tok.Type = T2Comma
				}
			default:
				tok.Type = '.'
			}
			tok.Str = buf.String()
		case '+':
			if sc.Peek() == '=' {
				tok.Type = TCompound
				tok.Str = "+="
				sc.Next()
			} else {
				tok.Type = ch
				tok.Str = string(ch)
			}
		case '*':
			if sc.Peek() == '=' {
				tok.Type = TCompound
				tok.Str = "*="
				sc.Next()
			} else {
				tok.Type = ch
				tok.Str = string(ch)
			}
		case '%':
			if sc.Peek() == '=' {
				tok.Type = TCompound
				tok.Str = "%="
				sc.Next()
			} else {
				tok.Type = ch
				tok.Str = string(ch)
			}
		case '^':
			if sc.Peek() == '=' {
				tok.Type = TCompound
				tok.Str = "^="
				sc.Next()
			} else {
				tok.Type = ch
				tok.Str = string(ch)
			}
		case '#', '(', ')', '{', '}', ']', ';', ',', '&', '|':
			tok.Type = ch
			tok.Str = string(ch)
		default:
			writeChar(buf, ch)
			err = sc.Error(buf.String(), "Invalid token")
			goto finally
		}
	}

finally:
	tok.Name = TokenName(int(tok.Type))
	return tok, err
}

// yacc interface {{{

type Lexer struct {
	scanner       *Scanner
	Chunk         ast.Chunk
	PNewLine      bool
	Token         ast.Token
	PrevTokenType int
}

func (lx *Lexer) Lex(lval *yySymType) int {
	lx.PrevTokenType = lx.Token.Type
	tok, err := lx.scanner.Scan(lx)
	if err != nil {
		panic(err)
	}
	if tok.Type < 0 {
		return 0
	}
	lval.token = tok
	lx.Token = tok
	return int(tok.Type)
}

func (lx *Lexer) Error(message string) {
	panic(lx.scanner.Error(lx.Token.Str, message))
}

func (lx *Lexer) TokenError(tok ast.Token, message string) {
	panic(lx.scanner.TokenError(tok, message))
}

func Parse(reader io.Reader, name string) (chunk ast.Chunk, err error) {
	lexer := &Lexer{NewScanner(reader, name), nil, false, ast.Token{Str: ""}, TNil}
	chunk = nil
	defer func() {
		if e := recover(); e != nil {
			err, _ = e.(error)
		}
	}()
	yyParse(lexer)
	chunk = lexer.Chunk
	return
}

// }}}

// Dump {{{

func isInlineDumpNode(rv reflect.Value) bool {
	switch rv.Kind() {
	case reflect.Struct, reflect.Slice, reflect.Interface, reflect.Ptr:
		return false
	default:
		return true
	}
}

func dump(node interface{}, level int, s string) string {
	rt := reflect.TypeOf(node)
	if fmt.Sprint(rt) == "<nil>" {
		return strings.Repeat(s, level) + "<nil>"
	}

	rv := reflect.ValueOf(node)
	buf := []string{}
	switch rt.Kind() {
	case reflect.Slice:
		if rv.Len() == 0 {
			return strings.Repeat(s, level) + "<empty>"
		}
		for i := 0; i < rv.Len(); i++ {
			buf = append(buf, dump(rv.Index(i).Interface(), level, s))
		}
	case reflect.Ptr:
		vt := rv.Elem()
		tt := rt.Elem()
		indicies := []int{}
		for i := 0; i < tt.NumField(); i++ {
			if strings.Index(tt.Field(i).Name, "Base") > -1 {
				continue
			}
			indicies = append(indicies, i)
		}
		switch {
		case len(indicies) == 0:
			return strings.Repeat(s, level) + "<empty>"
		case len(indicies) == 1 && isInlineDumpNode(vt.Field(indicies[0])):
			for _, i := range indicies {
				buf = append(buf, strings.Repeat(s, level)+"- Node$"+tt.Name()+": "+dump(vt.Field(i).Interface(), 0, s))
			}
		default:
			buf = append(buf, strings.Repeat(s, level)+"- Node$"+tt.Name())
			for _, i := range indicies {
				if isInlineDumpNode(vt.Field(i)) {
					inf := dump(vt.Field(i).Interface(), 0, s)
					buf = append(buf, strings.Repeat(s, level+1)+tt.Field(i).Name+": "+inf)
				} else {
					buf = append(buf, strings.Repeat(s, level+1)+tt.Field(i).Name+": ")
					buf = append(buf, dump(vt.Field(i).Interface(), level+2, s))
				}
			}
		}
	default:
		buf = append(buf, strings.Repeat(s, level)+fmt.Sprint(node))
	}
	return strings.Join(buf, "\n")
}

func Dump(chunk []ast.Stmt) string {
	return dump(chunk, 0, "   ")
}

// }}
