// Package half converts decimal strings into IEEE 754 binary16 (half
// precision) bit patterns using only arbitrary-precision integer and
// rational arithmetic. Host floating point is never consulted, so every
// rounding decision is exact.
package half

import (
	"math/big"
	"strings"
)

// Class of a binary16 encoding.
type Class string

const (
	ClassZero      Class = "zero"
	ClassSubnormal Class = "subnormal"
	ClassNormal    Class = "normal"
	ClassInfinity  Class = "infinity"
)

const (
	// MaxSigDigits is the maximum number of significant decimal digits
	// accepted in one value.
	MaxSigDigits = 30
	// MinDecExp / MaxDecExp bound the decimal exponent of an input value,
	// defined as the exponent of its leading significant digit in
	// scientific notation (so 0.001 is 10^-3).
	MinDecExp = -50
	MaxDecExp = 50
)

// parseError identifies a rejected decimal string. All other failure modes
// inside Convert are reported as parseError as well (they all mean the
// value did not meet the input contract).
func parseError() *Result { return &Result{reject: true} }

// decimal is a parsed decimal literal:
//
//	sign * coeff * 10^exp
//
// where coeff holds the decimal digits with leading zeros removed (so
// len(coeff) is the significant-digit count). exp is the decimal exponent
// of the least significant digit.
type decimal struct {
	negative bool
	coeff    *big.Int
	exp      int // 10 exponent attached to the last digit of coeff
}

// ParseDecimal parses a strict decimal literal.
//
// Accepted grammar (no whitespace, no NaN, no Infinity):
//
//	[+-]? ( [0-9]+ ('.' [0-9]*)? | '.' [0-9]+ ) ( [eE] [+-]? [0-9]+ )?
//
// The number of significant digits must not exceed 30 and the decimal
// exponent of the leading significant digit must be in [-50, 50].
func ParseDecimal(s string) (decimal, bool) {
	if s == "" {
		return decimal{}, false
	}
	negative := false
	switch s[0] {
	case '+', '-':
		negative = s[0] == '-'
		s = s[1:]
	}
	if s == "" {
		return decimal{}, false
	}

	// Split off optional exponent.
	mant := s
	rawExp := ""
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mant = s[:i]
		rawExp = s[i+1:]
		if rawExp == "" || strings.IndexAny(rawExp, "eE") >= 0 {
			return decimal{}, false // trailing e/E or more than one e/E
		}
	}

	if strings.Count(mant, ".") > 1 {
		return decimal{}, false
	}

	intPart, fracPart, hasDot := strings.Cut(mant, ".")
	if intPart == "" && (!hasDot || fracPart == "") {
		return decimal{}, false // ".", "" or ".e3" style
	}
	if !hasDot && intPart == "" {
		return decimal{}, false
	}
	for _, r := range intPart + fracPart {
		if r < '0' || r > '9' {
			return decimal{}, false
		}
	}

	exp := 0
	if rawExp != "" {
		e := rawExp
		eneg := false
		if len(e) > 0 && (e[0] == '+' || e[0] == '-') {
			eneg = e[0] == '-'
			e = e[1:]
		}
		if e == "" || len(e) > 2 {
			return decimal{}, false
		}
		for _, c := range e {
			if c < '0' || c > '9' {
				return decimal{}, false
			}
			exp = exp*10 + int(c-'0')
		}
		if eneg {
			exp = -exp
		}
	}

	digits := intPart + fracPart
	// fracDigits is the exponent attached to the last digit of digits
	// before stripping leading zeros.
	fracDigits := len(fracPart)
	t := strings.TrimLeft(digits, "0")
	if t == "" {
		// Signed zero of any shape is accepted with value zero.
		return decimal{negative: negative, coeff: new(big.Int), exp: 0}, true
	}
	if len(t) > MaxSigDigits {
		return decimal{}, false
	}
	coeff, ok := new(big.Int).SetString(t, 10)
	if !ok {
		return decimal{}, false
	}
	// The last character of digits (the last fractional digit when a
	// decimal point is present) has weight 10^(E-fracDigits). Stripping
	// leading zeros does not change that weight; stripping trailing zeros
	// is handled by normalizeCoeff.
	exp -= fracDigits
	coeff, exp = normalizeCoeff(coeff, exp)

	// Decimal exponent of the leading significant digit.
	leadExp := exp + len(coeff.String()) - 1
	if leadExp < MinDecExp || leadExp > MaxDecExp {
		return decimal{}, false
	}
	return decimal{negative: negative, coeff: coeff, exp: exp}, true
}

// normalizeCoeff removes trailing zeros of c, adjusting exp so that the
// represented value is unchanged. This keeps the significant-digit count
// exact ("1000" counts one significant digit).
func normalizeCoeff(c *big.Int, exp int) (*big.Int, int) {
	ten := big.NewInt(10)
	zero := new(big.Int)
	mod := new(big.Int)
	for c.Sign() != 0 {
		mod.Mod(c, ten)
		if mod.Cmp(zero) != 0 {
			break
		}
		c.Quo(c, ten)
		exp++
	}
	return c, exp
}

// Rat returns sign * coeff * 10^exp exactly.
func (d decimal) Rat() *big.Rat {
	r := new(big.Rat).SetInt(d.coeff)
	if d.exp >= 0 {
		r.Mul(r, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(d.exp)), nil)))
	} else {
		den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-int64(d.exp))), nil)
		r.Quo(r, new(big.Rat).SetInt(den))
	}
	if d.negative {
		r.Neg(r)
	}
	return r
}
