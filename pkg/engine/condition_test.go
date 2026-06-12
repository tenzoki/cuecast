package engine

import (
	"strings"
	"testing"

	"github.com/tenzoki/cuecast/pkg/model"
)

func TestEvalCondition_Truth(t *testing.T) {
	ctx := Context{Values: map[string]any{
		"amount":   1500.0,
		"region":   "EU",
		"approved": true,
		"count":    3,
	}}
	cases := []struct {
		expr string
		want bool
	}{
		{"amount > 1000", true},
		{"amount >= 1500", true},
		{"amount < 1000", false},
		{"amount == 1500", true},
		{"amount != 1500", false},
		{`region == "EU"`, true},
		{`region == "US"`, false},
		{`region != "US"`, true},
		{"approved", true},
		{"!approved", false},
		{`amount > 1000 && region == "EU"`, true},
		{`amount > 1000 && region == "US"`, false},
		{`amount < 1000 || region == "EU"`, true},
		{`amount < 1000 || region == "US"`, false},
		{`(amount > 1000 || count == 3) && approved`, true},
		{`!(amount < 1000)`, true},
		{"count == 3", true}, // int context value compared to number literal
		{"count >= 3", true},
		{"count > 3", false},
		{`missing == "x"`, false}, // absent key compares unequal
		{`missing != "x"`, true},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			got, err := evalCondition(model.Condition{Expr: tc.expr}, ctx)
			if err != nil {
				t.Fatalf("evalCondition(%q) error: %v", tc.expr, err)
			}
			if got != tc.want {
				t.Errorf("evalCondition(%q) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestEvalCondition_Deterministic(t *testing.T) {
	ctx := Context{Values: map[string]any{"amount": 1500.0, "region": "EU"}}
	c := model.Condition{Expr: `amount > 1000 && region == "EU"`}
	a, _ := evalCondition(c, ctx)
	b, _ := evalCondition(c, ctx)
	if a != b {
		t.Errorf("evalCondition not deterministic: %v vs %v", a, b)
	}
}

func TestCompileCondition_ParseErrors(t *testing.T) {
	cases := []struct {
		expr    string
		wantSub string
	}{
		{"amount >", "unexpected end"},
		{"amount = 1000", "use '=='"},
		{"amount & region", "did you mean"},
		{`region == "EU`, "unterminated string"},
		{"(amount > 1000", "expected ')'"},
		{"amount @ 1000", "unexpected character"},
		{"amount > 1000)", "unexpected token"},
		{"", "unexpected end"},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			_, err := compileCondition(model.Condition{Expr: tc.expr})
			if err == nil {
				t.Fatalf("compileCondition(%q) = nil error, want %q", tc.expr, tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestEvalCondition_OrderingNonNumericIsNonMatch(t *testing.T) {
	ctx := Context{Values: map[string]any{"region": "EU"}}
	// Ordering on a non-numeric operand is a canonical non-match (false), not an
	// error: it lets a gateway fall through to its default rather than aborting
	// (spec C4; issue 260612-1907[o]-gateway-default-bypassed-on-missing-key-eval-error).
	match, err := evalCondition(model.Condition{Expr: `region > 5`}, ctx)
	if err != nil {
		t.Fatalf("ordering on a string operand errored: %v, want non-match", err)
	}
	if match {
		t.Errorf("ordering on a string operand = true, want false (non-match)")
	}
}

func TestEvalCondition_OrderingAbsentKeyIsNonMatch(t *testing.T) {
	// Ordering against an absent context key is a non-match (false), not an error.
	match, err := evalCondition(model.Condition{Expr: `amount >= 1000`}, Context{})
	if err != nil {
		t.Fatalf("ordering on an absent key errored: %v, want non-match", err)
	}
	if match {
		t.Errorf("ordering on an absent key = true, want false (non-match)")
	}
}

func TestValidate_DetectsBadCondition(t *testing.T) {
	m := validModel()
	m.Flows[1].Condition = &model.Condition{Expr: "amount >"} // malformed
	errs := Validate(m)
	if !hasError(errs, "invalid condition", "f_auto") {
		t.Errorf("Validate did not flag the malformed condition on f_auto: %v", errs)
	}
}
