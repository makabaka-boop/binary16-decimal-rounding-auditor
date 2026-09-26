package half

import (
	"fmt"
	"math/big"
	"strings"
)

// Result is the outcome of converting one decimal string. When reject is
// true, the input was malformed or outside the documented limits.
type Result struct {
	reject bool
	bits   uint16
	class  Class
	// rounded is the rounded value as an exact rational. For infinite
	// results it is nil.
	rounded *big.Rat
	// err is rounded - original in lowest terms; nil means the JSON error
	// field is null (used only for infinite results).
	err *big.Rat
}

// Reject reports whether the input was rejected.
func (r *Result) Reject() bool { return r.reject }

// Bits returns the binary16 bit pattern.
func (r *Result) Bits() uint16 { return r.bits }

// Class returns zero/subnormal/normal/infinity.
func (r *Result) Class() Class { return r.class }

// Hex returns the four-lowercase-hex-digit encoding, e.g. "3c00".
func (r *Result) Hex() string { return fmt.Sprintf("%04x", r.bits) }

// RoundedDecimal returns the rounded finite value as an exact decimal
// string; negative zero is "-0".
func (r *Result) RoundedDecimal() string {
	if r.class == ClassInfinity {
		if r.bits&0x8000 != 0 {
			return "-Infinity"
		}
		return "Infinity"
	}
	if r.class == ClassZero && r.bits&0x8000 != 0 {
		return "-0"
	}
	return ratToTerminatingDecimal(r.rounded)
}

// ErrorFraction returns (numerator, denominator) of rounded-original in
// lowest terms, and whether an error exists (false for infinity).
func (r *Result) ErrorFraction() (string, string, bool) {
	if r.err == nil {
		return "", "", false
	}
	return r.err.Num().String(), r.err.Denom().String(), true
}

// Convert converts one decimal literal to binary16 using integer and
// rational arithmetic only. The result is never nil; a rejected input has
// Reject() == true.
func Convert(s string) *Result {
	d, ok := ParseDecimal(s)
	if !ok {
		return parseError()
	}
	x := d.Rat()

	// Signed zero: every zero magnitude rounds to a signed zero; the sign
	// comes from the input sign so underflow can preserve sign.
	if d.coeff.Sign() == 0 {
		bits := uint16(0)
		if d.negative {
			bits = 0x8000
		}
		return finiteResult(bits, ClassZero, new(big.Rat), x)
	}

	mag := new(big.Rat).Abs(x)
	k := floorLog2Rat(mag) // mag in [2^k, 2^(k+1))

	// |x| >= 2^16: all such magnitudes (including the midpoint between
	// 65504 and infinity, 65520 = 2^16 - 2^4) round to infinity; 65520
	// itself is the tie point and infinity's significand is even.
	if k >= 16 {
		return infResult(d.negative)
	}

	var bits uint16
	var rounded *big.Rat
	var class Class

	if k <= -15 {
		// Subnormal grid: spacing 2^-24; significand m in [0, 2048].
		scaled := new(big.Rat).Mul(mag, ratPow2(24))
		m := roundEven(scaled) // 0..1024; m == 0 handled below
		if m.Cmp(big.NewInt(1024)) >= 0 {
			// Tie/carry exactly onto the smallest normal 2^-14 = 1024*2^-24.
			bits = 0x0400
			rounded = ratPow2(-14)
			class = ClassNormal
		} else {
			bits = uint16(m.Int64())
			rounded = new(big.Rat).SetInt(m)
			rounded.Mul(rounded, ratPow2(-24))
			if bits == 0 {
				class = ClassZero
				rounded = new(big.Rat)
			} else {
				class = ClassSubnormal
			}
		}
	} else {
		// Normal binade: spacing 2^(k-10); significand m in [1024,2048).
		scaled := new(big.Rat).Mul(mag, ratPow2(10-k))
		m := roundEven(scaled)
		if m.Cmp(big.NewInt(2048)) >= 0 {
			if k == 15 {
				// Midpoint above 65504 rounds to infinity; anything
				// larger also overflows.
				return infResult(d.negative)
			}
			// Carry into the next binade: 2048 * 2^(k-10) = 2^(k+1),
			// encoded as m=1024 with exponent k+1.
			k++
			m = big.NewInt(1024)
		}
		e := uint16((k + 15) << 10)
		bits = e | uint16(m.Int64()-1024)
		rounded = new(big.Rat).SetInt(m)
		rounded.Mul(rounded, ratPow2(k-10))
		class = ClassNormal
	}

	if d.negative {
		bits |= 0x8000
		rounded.Neg(rounded)
	}
	return finiteResult(bits, class, rounded, x)
}

func finiteResult(bits uint16, class Class, rounded, orig *big.Rat) *Result {
	err := new(big.Rat).Sub(rounded, orig)
	return &Result{bits: bits, class: class, rounded: new(big.Rat).Set(rounded), err: err}
}

func infResult(negative bool) *Result {
	bits := uint16(0x7c00)
	if negative {
		bits |= 0x8000
	}
	return &Result{bits: bits, class: ClassInfinity, rounded: nil, err: nil}
}

// roundEven rounds r to the nearest integer, breaking ties toward even.
func roundEven(r *big.Rat) *big.Int {
	q := new(big.Int)
	rem := new(big.Int)
	q.QuoRem(r.Num(), r.Denom(), rem) // truncated toward zero; r >= 0 here

	// Compare 2*rem with denominator to decide below-half / half /
	// above-half without any floating point.
	twice := new(big.Int).Lsh(rem, 1)
	cmp := twice.Cmp(r.Denom())
	if cmp < 0 {
		return q
	}
	if cmp > 0 {
		return q.Add(q, big.NewInt(1))
	}
	if q.Bit(0) == 1 { // exact tie: even wins
		q.Add(q, big.NewInt(1))
	}
	return q
}

// ratPow2 returns 2^n as a rational for any integer n.
func ratPow2(n int) *big.Rat {
	if n >= 0 {
		return new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), uint(n)))
	}
	u := uint(-n)
	return new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Lsh(big.NewInt(1), u))
}

// floorLog2Rat returns the unique k with 2^k <= r < 2^(k+1), r > 0.
func floorLog2Rat(r *big.Rat) int {
	// Start from the bit length of the integer part when one exists,
	// otherwise from the denominator's bit length, then adjust exactly.
	k := 0
	if num, den := r.Num(), r.Denom(); num.Cmp(den) >= 0 {
		k = num.BitLen() - den.BitLen()
	} else {
		k = num.BitLen() - den.BitLen() - 1
	}
	p := ratPow2(k)
	if r.Cmp(p) < 0 {
		for r.Cmp(p) < 0 {
			k--
			p = ratPow2(k)
		}
	} else {
		for r.Cmp(new(big.Rat).Mul(p, big.NewRat(2, 1))) >= 0 {
			k++
			p = ratPow2(k)
		}
	}
	return k
}

// ratToTerminatingDecimal renders r, known to have a terminating decimal
// expansion, exactly. The sign of zero is not represented (big.Rat has no
// signed zero); callers special-case -0.
func ratToTerminatingDecimal(r *big.Rat) string {
	if r.Sign() == 0 {
		return "0"
	}
	neg := r.Sign() < 0
	num := new(big.Int).Abs(r.Num())
	den := new(big.Int).Set(r.Denom())

	// den = 2^a * 5^b * c; terminating means c == 1. Multiply numerator
	// and denominator by the missing 2/5 factors to reach 10^max(a,b).
	a, b := 0, 0
	two, five := big.NewInt(2), big.NewInt(5)
	tmp := new(big.Int)
	factor := new(big.Int).Set(den)
	for tmp.Mod(factor, two); tmp.Sign() == 0; tmp.Mod(factor, two) {
		factor.Quo(factor, two)
		a++
	}
	for tmp.Mod(factor, five); tmp.Sign() == 0; tmp.Mod(factor, five) {
		factor.Quo(factor, five)
		b++
	}
	if factor.Cmp(big.NewInt(1)) != 0 {
		// All binary16 values have power-of-two denominators, so this
		// cannot happen for rounded outputs.
		return r.RatString()
	}
	m := a
	if b > m {
		m = b
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(m)), nil)
	// r * 10^m = num * 10^m / den, an integer because den | 10^m.
	num.Mul(num, scale)
	num.Quo(num, den)

	s := num.String()
	var whole, frac string
	if m == 0 {
		whole = s
	} else if len(s) > m {
		whole = s[:len(s)-m]
		frac = s[len(s)-m:]
	} else {
		whole = "0"
		frac = strings.Repeat("0", m-len(s)) + s
	}
	out := whole
	if frac != "" {
		frac = strings.TrimRight(frac, "0")
		if frac != "" {
			out += "." + frac
		}
	}
	if neg {
		out = "-" + out
	}
	return out
}
