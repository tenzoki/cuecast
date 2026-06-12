package engine

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/tenzoki/cuecast/pkg/model"
)

// This file implements the gateway-condition expression language (decision
// 260612-1557[a], Option 2): a small hand-written lexer/parser/evaluator over an
// infix string. The grammar is deliberately restricted to v1 routing needs:
//
//	expr      := or
//	or        := and ( "||" and )*
//	and       := unary ( "&&" unary )*
//	unary     := "!" unary | comparison
//	comparison:= primary ( ( "==" | "!=" | ">" | ">=" | "<" | "<=" ) primary )?
//	primary   := number | string | bool | identifier | "(" expr ")"
//
// Operands are literals (number, quoted string, bool) or Context-key identifiers.
// Comparisons are non-associative (one operator per comparison). Evaluation is
// deterministic and does no I/O. Validate (C1) surfaces parse errors against the flow
// id via validateCondition; AccNext (C4) evaluates via the evalCondition seam.

// expr is a parsed condition AST node. Eval returns the boolean (or, for nested
// operand positions, the comparable value) of the node against a Context.
type expr interface {
	// evalBool resolves the node to a boolean against ctx.
	evalBool(ctx Context) (bool, error)
}

// compileCondition lexes and parses a model.Condition into an evaluable AST, or
// returns a parse error. It is the single compile seam used by both validateCondition
// (Validate) and evalCondition (AccNext), so validation and evaluation share one
// parser — no drift between "is it well-formed" and "what does it mean".
func compileCondition(c model.Condition) (expr, error) {
	toks, err := lex(c.Expr)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	node, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if !p.atEnd() {
		return nil, fmt.Errorf("unexpected token %q", p.peek().lit)
	}
	return node, nil
}

// validateCondition reports a parse fault for a flow condition, or nil if the
// expression is structurally well-formed. It replaces the Step-4 no-op stub.
func validateCondition(c model.Condition) error {
	_, err := compileCondition(c)
	return err
}

// evalCondition is the single evaluation seam AccNext uses to test a flow condition
// against context (per the plan: the rest of AccNext is unchanged across condition
// options). It compiles then evaluates; a well-validated model never produces a
// compile error here, but the error is surfaced rather than swallowed.
func evalCondition(c model.Condition, ctx Context) (bool, error) {
	node, err := compileCondition(c)
	if err != nil {
		return false, err
	}
	return node.evalBool(ctx)
}

// --- Lexer ---

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokNumber
	tokString
	tokBool
	tokIdent
	tokOp     // == != > >= < <= && || !
	tokLParen // (
	tokRParen // )
)

type token struct {
	kind tokenKind
	lit  string
}

// lex tokenises an infix condition expression. It recognises numbers, double-quoted
// strings (with \" and \\ escapes), the bool literals true/false, identifiers
// (Context keys), the comparison/boolean operators, and parentheses. Any other
// character is a lexical error surfaced with its position.
func lex(s string) ([]token, error) {
	var toks []token
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		r := runes[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '(':
			toks = append(toks, token{tokLParen, "("})
			i++
		case r == ')':
			toks = append(toks, token{tokRParen, ")"})
			i++
		case r == '"':
			lit, n, err := lexString(runes[i:])
			if err != nil {
				return nil, err
			}
			toks = append(toks, token{tokString, lit})
			i += n
		case r == '&' || r == '|':
			if i+1 < len(runes) && runes[i+1] == r {
				toks = append(toks, token{tokOp, string([]rune{r, r})})
				i += 2
			} else {
				return nil, fmt.Errorf("unexpected character %q (did you mean %q?)", string(r), string([]rune{r, r}))
			}
		case r == '=':
			if i+1 < len(runes) && runes[i+1] == '=' {
				toks = append(toks, token{tokOp, "=="})
				i += 2
			} else {
				return nil, fmt.Errorf("unexpected '='; use '==' for equality")
			}
		case r == '!':
			if i+1 < len(runes) && runes[i+1] == '=' {
				toks = append(toks, token{tokOp, "!="})
				i += 2
			} else {
				toks = append(toks, token{tokOp, "!"})
				i++
			}
		case r == '>' || r == '<':
			if i+1 < len(runes) && runes[i+1] == '=' {
				toks = append(toks, token{tokOp, string(r) + "="})
				i += 2
			} else {
				toks = append(toks, token{tokOp, string(r)})
				i++
			}
		case r == '-' || r == '+' || unicode.IsDigit(r):
			lit, n := lexNumber(runes[i:])
			if lit == "" {
				return nil, fmt.Errorf("invalid number near %q", string(runes[i:min(i+8, len(runes))]))
			}
			toks = append(toks, token{tokNumber, lit})
			i += n
		case isIdentStart(r):
			lit, n := lexIdent(runes[i:])
			if lit == "true" || lit == "false" {
				toks = append(toks, token{tokBool, lit})
			} else {
				toks = append(toks, token{tokIdent, lit})
			}
			i += n
		default:
			return nil, fmt.Errorf("unexpected character %q", string(r))
		}
	}
	toks = append(toks, token{tokEOF, ""})
	return toks, nil
}

func lexString(runes []rune) (string, int, error) {
	// runes[0] == '"'
	var b strings.Builder
	i := 1
	for i < len(runes) {
		c := runes[i]
		switch c {
		case '\\':
			if i+1 >= len(runes) {
				return "", 0, fmt.Errorf("unterminated escape in string literal")
			}
			next := runes[i+1]
			if next != '"' && next != '\\' {
				return "", 0, fmt.Errorf("invalid escape %q in string literal", "\\"+string(next))
			}
			b.WriteRune(next)
			i += 2
		case '"':
			return b.String(), i + 1, nil
		default:
			b.WriteRune(c)
			i++
		}
	}
	return "", 0, fmt.Errorf("unterminated string literal")
}

func lexNumber(runes []rune) (string, int) {
	i := 0
	if i < len(runes) && (runes[i] == '-' || runes[i] == '+') {
		i++
	}
	start := i
	seenDot := false
	for i < len(runes) {
		c := runes[i]
		if unicode.IsDigit(c) {
			i++
			continue
		}
		if c == '.' && !seenDot {
			seenDot = true
			i++
			continue
		}
		break
	}
	if i == start { // no digits consumed
		return "", 0
	}
	lit := string(runes[:i])
	if _, err := strconv.ParseFloat(lit, 64); err != nil {
		return "", 0
	}
	return lit, i
}

func lexIdent(runes []rune) (string, int) {
	i := 0
	for i < len(runes) && isIdentPart(runes[i]) {
		i++
	}
	return string(runes[:i]), i
}

func isIdentStart(r rune) bool { return unicode.IsLetter(r) || r == '_' }
func isIdentPart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.'
}

// --- Parser ---

type parser struct {
	toks []token
	pos  int
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) atEnd() bool { return p.peek().kind == tokEOF }
func (p *parser) advance() token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *parser) parseExpr() (expr, error) { return p.parseOr() }

func (p *parser) parseOr() (expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokOp && p.peek().lit == "||" {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &boolExpr{op: "||", left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokOp && p.peek().lit == "&&" {
		p.advance()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &boolExpr{op: "&&", left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseUnary() (expr, error) {
	if p.peek().kind == tokOp && p.peek().lit == "!" {
		p.advance()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &notExpr{operand: operand}, nil
	}
	return p.parseComparison()
}

func (p *parser) parseComparison() (expr, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	if p.peek().kind == tokOp && isComparisonOp(p.peek().lit) {
		op := p.advance().lit
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &cmpExpr{op: op, left: left, right: right}, nil
	}
	// No comparison operator: the primary must itself be boolean-valued (a bool
	// literal or identifier resolving to a bool). evalBool enforces that.
	return left, nil
}

func (p *parser) parsePrimary() (operand, error) {
	t := p.peek()
	switch t.kind {
	case tokLParen:
		p.advance()
		inner, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tokRParen {
			return nil, fmt.Errorf("expected ')' but found %q", p.peek().lit)
		}
		p.advance()
		return &groupExpr{inner: inner}, nil
	case tokNumber:
		p.advance()
		n, _ := strconv.ParseFloat(t.lit, 64)
		return &literal{kind: litNumber, num: n}, nil
	case tokString:
		p.advance()
		return &literal{kind: litString, str: t.lit}, nil
	case tokBool:
		p.advance()
		return &literal{kind: litBool, b: t.lit == "true"}, nil
	case tokIdent:
		p.advance()
		return &ident{key: t.lit}, nil
	case tokEOF:
		return nil, fmt.Errorf("unexpected end of expression")
	default:
		return nil, fmt.Errorf("unexpected token %q", t.lit)
	}
}

func isComparisonOp(op string) bool {
	switch op {
	case "==", "!=", ">", ">=", "<", "<=":
		return true
	}
	return false
}

// --- AST nodes ---

// operand is an expr that can also be resolved to a comparable value (for the two
// sides of a comparison). Literals, identifiers, and parenthesised groups are operands.
type operand interface {
	expr
	// resolve returns the operand's value against ctx for comparison.
	resolve(ctx Context) (any, error)
}

type litKind int

const (
	litNumber litKind = iota
	litString
	litBool
)

type literal struct {
	kind litKind
	num  float64
	str  string
	b    bool
}

func (l *literal) value() any {
	switch l.kind {
	case litNumber:
		return l.num
	case litString:
		return l.str
	default:
		return l.b
	}
}

func (l *literal) resolve(Context) (any, error) { return l.value(), nil }

func (l *literal) evalBool(Context) (bool, error) {
	if l.kind != litBool {
		return false, fmt.Errorf("expected a boolean expression, found a %s literal used on its own", l.kindName())
	}
	return l.b, nil
}

func (l *literal) kindName() string {
	switch l.kind {
	case litNumber:
		return "number"
	case litString:
		return "string"
	default:
		return "bool"
	}
}

// ident is a Context-key reference. A missing key resolves to nil for comparison.
// Equality against a missing key is well-defined (!= matches, == does not match);
// ordering (> >= < <=) against a missing or non-numeric key is a non-match (false),
// so a gateway falls through to its default rather than erroring (see compare).
type ident struct{ key string }

func (id *ident) resolve(ctx Context) (any, error) {
	v, _ := ctx.Get(id.key)
	return v, nil
}

func (id *ident) evalBool(ctx Context) (bool, error) {
	v, ok := ctx.Get(id.key)
	if !ok {
		return false, fmt.Errorf("context key %q used as a boolean but is absent", id.key)
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("context key %q used as a boolean but holds %T", id.key, v)
	}
	return b, nil
}

type groupExpr struct{ inner expr }

func (g *groupExpr) evalBool(ctx Context) (bool, error) { return g.inner.evalBool(ctx) }
func (g *groupExpr) resolve(ctx Context) (any, error) {
	// A parenthesised group in operand position resolves to its boolean value.
	b, err := g.inner.evalBool(ctx)
	if err != nil {
		return nil, err
	}
	return b, nil
}

type notExpr struct{ operand expr }

func (n *notExpr) evalBool(ctx Context) (bool, error) {
	b, err := n.operand.evalBool(ctx)
	if err != nil {
		return false, err
	}
	return !b, nil
}

type boolExpr struct {
	op          string // "&&" | "||"
	left, right expr
}

func (be *boolExpr) evalBool(ctx Context) (bool, error) {
	l, err := be.left.evalBool(ctx)
	if err != nil {
		return false, err
	}
	// Short-circuit, deterministically.
	if be.op == "&&" && !l {
		return false, nil
	}
	if be.op == "||" && l {
		return true, nil
	}
	return be.right.evalBool(ctx)
}

type cmpExpr struct {
	op          string
	left, right operand
}

func (ce *cmpExpr) evalBool(ctx Context) (bool, error) {
	lv, err := ce.left.resolve(ctx)
	if err != nil {
		return false, err
	}
	rv, err := ce.right.resolve(ctx)
	if err != nil {
		return false, err
	}
	return compare(ce.op, lv, rv)
}

// compare evaluates a comparison between two resolved values. Equality (== / !=)
// works across numbers, strings, bools, and nil (a missing context key). Ordering
// (> >= < <=) requires both sides to be numeric; when either side is absent (a
// missing context key) or non-numeric, the ordering comparison is a canonical
// non-match — it evaluates to false rather than erroring. This is what lets a
// gateway whose ordering condition references a not-yet-set key fall through to its
// declared default flow (spec C4: a missing key is the canonical "none match" case)
// instead of aborting the whole gateway. Genuinely malformed condition *expressions*
// are still caught earlier, at Validate time (compileCondition); this rule governs
// only runtime evaluation against a context that lacks or type-mismatches a key.
func compare(op string, l, r any) (bool, error) {
	switch op {
	case "==":
		return valuesEqual(l, r), nil
	case "!=":
		return !valuesEqual(l, r), nil
	}

	lf, lok := toFloat(l)
	rf, rok := toFloat(r)
	if !lok || !rok {
		// Ordering against an absent or non-numeric operand does not match. The flow
		// is skipped; gateway evaluation continues to the next flow and, ultimately,
		// to the declared default.
		return false, nil
	}
	switch op {
	case ">":
		return lf > rf, nil
	case ">=":
		return lf >= rf, nil
	case "<":
		return lf < rf, nil
	case "<=":
		return lf <= rf, nil
	}
	return false, fmt.Errorf("unknown comparison operator %q", op)
}

// valuesEqual compares two values for equality across the supported types. Numbers
// compare numerically (int and float forms unify); strings and bools compare
// directly; nil equals only nil.
func valuesEqual(l, r any) bool {
	if l == nil || r == nil {
		return l == nil && r == nil
	}
	if lf, lok := toFloat(l); lok {
		if rf, rok := toFloat(r); rok {
			return lf == rf
		}
		return false
	}
	if ls, lok := l.(string); lok {
		rs, rok := r.(string)
		return rok && ls == rs
	}
	if lb, lok := l.(bool); lok {
		rb, rok := r.(bool)
		return rok && lb == rb
	}
	return false
}

// toFloat converts a numeric value (any Go numeric type) to float64. Strings are NOT
// coerced here: a context key holding a numeric string is a string, not a number, for
// comparison purposes (the author writes string literals quoted; numbers unquoted).
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

func typeName(v any) string {
	if v == nil {
		return "absent"
	}
	return fmt.Sprintf("%T", v)
}
