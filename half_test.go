package half

import (
	"math"
	"math/big"
	"strconv"
	"strings"
	"testing"
)

// decodeHalf returns the exact rational value encoded by a finite or
// infinite half bit pattern (test-only decoder used for round trips).
func decodeHalf(bits uint16) *big.Rat {
	sign := bits >> 15
	exp := (bits >> 10) & 0x1f
	frac := bits & 0x3ff
	v := new(big.Rat)
	switch {
	case exp == 0 && frac == 0:
		// zero
	case exp == 0:
		v.SetInt64(int64(frac))
		v.Mul(v, ratPow2(-24))
	case exp == 0x1f:
		// infinity (NaN excluded by callers)
		v.SetInt64(1)
	default:
		m := int64(frac) + 1024
		v.SetInt64(m)
		v.Mul(v, ratPow2(int(exp)-25))
	}
	if sign == 1 && v.Sign() != 0 {
		v.Neg(v)
	}
	return v
}

func classOf(bits uint16) Class {
	exp := (bits >> 10) & 0x1f
	frac := bits & 0x3ff
	switch {
	case exp == 0 && frac == 0:
		return ClassZero
	case exp == 0:
		return ClassSubnormal
	case exp == 0x1f && frac == 0:
		return ClassInfinity
	default:
		return ClassNormal
	}
}

// TestAllFinitePatternsRoundTrip exercises every finite binary16 bit
// pattern (63488 of them, both signs): the exact decimal rendering of the
// encoded value must convert back to the same bits, the error must be
// zero, and re-parsing the rendered rounded value must equal the decoded
// rational.
func TestAllFinitePatternsRoundTrip(t *testing.T) {
	// 65536 encodings; exponent field 31 holds the two infinities and
	// 2046 NaNs (2048 encodings, none finite). The other 63488 encodings
	// are finite (including two signed zeros).
	const finiteCount = 65536 - 2048
	seen := 0
	for b := 0; b <= 0xffff; b++ {
		bits := uint16(b)
		if ((bits >> 10) & 0x1f) == 0x1f {
			continue // NaN and infinity are not finite
		}
		seen++
		for _, neg := range []bool{false, true} {
			pat := bits
			if neg {
				pat |= 0x8000
			}
			v := decodeHalf(pat)
			s := ratToTerminatingDecimal(v)
			if pat == 0x8000 {
				s = "-0"
			}
			r := Convert(s)
			if r.Reject() {
				t.Fatalf("pattern %04x input %q rejected", pat, s)
			}
			if r.Bits() != pat {
				t.Fatalf("pattern %04x: input %q -> %04x", pat, s, r.Bits())
			}
			if r.Class() != classOf(pat) {
				t.Fatalf("pattern %04x: class %s want %s", pat, r.Class(), classOf(pat))
			}
			if num, den, ok := r.ErrorFraction(); !ok || num != "0" || den != "1" {
				t.Fatalf("pattern %04x: delta %s/%s, want 0", pat, num, den)
			}
			// Rendered rounded value must parse back to the same rational.
			back := Convert(r.RoundedDecimal())
			if back.Bits() != pat {
				t.Fatalf("pattern %04x: rounded %q re-converted to %04x", pat, r.RoundedDecimal(), back.Bits())
			}
			want := new(big.Rat)
			if pat != 0x8000 {
				want.Set(v)
			}
			got, ok := ParseDecimal(r.RoundedDecimal())
			if !ok {
				t.Fatalf("rounded %q does not parse", r.RoundedDecimal())
			}
			if got.Rat().Cmp(want) != 0 {
				t.Fatalf("pattern %04x: rounded %q = %v want %v", pat, r.RoundedDecimal(), got.Rat(), want)
			}
		}
	}
	if seen != finiteCount {
		t.Fatalf("exercised %d patterns, want %d", seen, finiteCount)
	}
}

// dyadicDecimal renders n*2^e as an exact decimal string (test helper).
func dyadicDecimal(n int64, e int) string {
	r := new(big.Rat).SetInt64(n)
	r.Mul(r, ratPow2(e))
	return ratToTerminatingDecimal(r)
}

func wantBits(t *testing.T, s string, hexWant uint16, classWant Class) *Result {
	t.Helper()
	r := Convert(s)
	if r.Reject() {
		t.Fatalf("input %q unexpectedly rejected", s)
	}
	if r.Bits() != hexWant {
		t.Fatalf("input %q -> %04x (%s), want %04x", s, r.Bits(), r.Class(), hexWant)
	}
	if r.Class() != classWant {
		t.Fatalf("input %q -> class %s, want %s", s, r.Class(), classWant)
	}
	return r
}

func TestSubnormalBoundaries(t *testing.T) {
	// Smallest positive subnormal 2^-24 and its negative.
	wantBits(t, dyadicDecimal(1, -24), 0x0001, ClassSubnormal)
	wantBits(t, "-"+dyadicDecimal(1, -24), 0x8001, ClassSubnormal)

	// Largest subnormal 1023*2^-24.
	wantBits(t, dyadicDecimal(1023, -24), 0x03ff, ClassSubnormal)

	// Smallest normal 2^-14.
	wantBits(t, dyadicDecimal(1, -14), 0x0400, ClassNormal)

	// Tie at 3*2^-25: between 2^-24 and 2*2^-24; even candidate (2) wins.
	wantBits(t, dyadicDecimal(3, -25), 0x0002, ClassSubnormal)
	// Sides of that tie: 3*2^-25 ± 2^-26 => 5*2^-26 and 7*2^-26.
	wantBits(t, dyadicDecimal(5, -26), 0x0001, ClassSubnormal)
	wantBits(t, dyadicDecimal(7, -26), 0x0002, ClassSubnormal)

	// Tie 2^-15 = 512*2^-24: neighbors 511 (odd) and 513 (odd); 512 even.
	wantBits(t, dyadicDecimal(1, -15), 0x0200, ClassSubnormal)

	// Tie between largest subnormal and smallest normal: 2047*2^-25;
	// 1023 is odd, 1024 is even, so the value crosses into the normals.
	wantBits(t, dyadicDecimal(2047, -25), 0x0400, ClassNormal)
	// Sides: 2047*2^-25 ± 2^-26 = 4093*2^-26 and 4095*2^-26.
	wantBits(t, dyadicDecimal(4093, -26), 0x03ff, ClassSubnormal)
	wantBits(t, dyadicDecimal(4095, -26), 0x0400, ClassNormal)
}

func TestSignedZeros(t *testing.T) {
	for _, s := range []string{"0", "+0", "-0", "0.0", "-0.000", "0e7"} {
		want := uint16(0x0000)
		if strings.HasPrefix(s, "-") {
			want = 0x8000
		}
		r := wantBits(t, s, want, ClassZero)
		if want == 0x8000 && r.RoundedDecimal() != "-0" {
			t.Fatalf("%q rounded = %q, want -0", s, r.RoundedDecimal())
		}
		if want == 0x0000 && r.RoundedDecimal() != "0" {
			t.Fatalf("%q rounded = %q, want 0", s, r.RoundedDecimal())
		}
	}

	// Tiny magnitudes underflow but keep the input sign: a signed zero
	// rather than plain zero.
	r := wantBits(t, "-1e-50", 0x8000, ClassZero)
	num, den, ok := r.ErrorFraction()
	if !ok || num != "1" || den != "100000000000000000000000000000000000000000000000000" {
		t.Fatalf("-1e-50 delta = %s/%s", num, den)
	}
	wantBits(t, "1e-50", 0x0000, ClassZero)
}

func TestTiesToEvenNormal(t *testing.T) {
	// Tie between 1.0 (0x3c00) and 1+2^-10 (0x3c01) is 1+2^-11;
	// significand 1024 is even, so it stays at 1.0.
	tie := dyadicDecimal(2049, -11) // 2049*2^-11 = 1 + 2^-11
	wantBits(t, tie, 0x3c00, ClassNormal)
	// Quarter-grid sides (offset 2^-12): (4098 ∓ 1)*2^-12.
	wantBits(t, dyadicDecimal(4097, -12), 0x3c00, ClassNormal)
	wantBits(t, dyadicDecimal(4099, -12), 0x3c01, ClassNormal)

	// Tie between 0x3bff (2047*2^-11) and 0x3c00 (1.0): 4095*2^-12 =
	// 0.999755859375; even candidate 1024 (0x3c00) wins over 1023.
	wantBits(t, dyadicDecimal(4095, -12), 0x3c00, ClassNormal)
	// Quarter-grid sides (offset 2^-13): below the tie lands on 0x3bff,
	// above lands on 0x3c00; only the exact tie is decided by parity.
	wantBits(t, dyadicDecimal(8189, -13), 0x3bff, ClassNormal)
	wantBits(t, dyadicDecimal(8191, -13), 0x3c00, ClassNormal)

	// 0.1 lands between 1638*2^-14 = 0.0999755859375 and 1639*2^-14,
	// closer to the former; exact error is -1/40960.
	r := wantBits(t, "0.1", 0x2e66, ClassNormal)
	if r.RoundedDecimal() != "0.0999755859375" {
		t.Fatalf("0.1 rounded = %q", r.RoundedDecimal())
	}
	num, den, ok := r.ErrorFraction()
	if !ok || num != "-1" || den != "40960" {
		t.Fatalf("0.1 delta = %s/%s, want -1/40960", num, den)
	}
}

// TestDoubleRoundingTrap shows a value where the naive
// double-then-half route disagrees with correct decimal rounding. The
// value sits 10^-29 above the half tie 1+2^-11: that offset is far below
// one half of a double ulp (2^-53), so a host double first collapses onto
// the tie, which then rounds-to-even *down*. The exact path sees a value
// strictly above the tie and rounds up.
func TestDoubleRoundingTrap(t *testing.T) {
	const s = "1.00048828125000000000000000001" // 30 significant digits
	if n := len(strings.TrimPrefix(strings.Replace(s, ".", "", 1), "0")); n > 30 {
		t.Fatalf("trap fixture uses %d significant digits", n)
	}
	wantBits(t, s, 0x3c01, ClassNormal)

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatal(err)
	}
	if f != 1.0+math.Ldexp(1.0, -11) {
		t.Fatalf("test premise broken: double did not collapse to the tie")
	}
	// The tie itself rounds-to-even to 0x3c00, i.e. the host route errs
	// downward by one ulp.
	wantBits(t, dyadicDecimal(2049, -11), 0x3c00, ClassNormal)
}

func TestOverflowAndInfinity(t *testing.T) {
	// Largest finite.
	r := wantBits(t, "65504", 0x7bff, ClassNormal)
	if num, _, ok := r.ErrorFraction(); !ok || num != "0" {
		t.Fatalf("65504 delta should be zero")
	}

	// Tie 65520 between 65504 and +Inf rounds to infinity (infinity's
	// exponent field 31 is odd, significand treated as 2048 which is
	// even); its error is JSON null.
	r = wantBits(t, "65520", 0x7c00, ClassInfinity)
	if _, _, ok := r.ErrorFraction(); ok {
		t.Fatalf("infinity delta must be absent (null)")
	}
	if r.RoundedDecimal() != "Infinity" {
		t.Fatalf("rounded = %q", r.RoundedDecimal())
	}
	wantBits(t, "-65520", 0xfc00, ClassInfinity)

	// Just below the tie goes to the largest finite; above goes to inf.
	wantBits(t, "65519.9921875", 0x7bff, ClassNormal)
	wantBits(t, "65520.0078125", 0x7c00, ClassInfinity)

	// Decimal exponents at the accepted limits.
	wantBits(t, "1e50", 0x7c00, ClassInfinity)
	wantBits(t, "-1e50", 0xfc00, ClassInfinity)
}

func TestRejectedInputs(t *testing.T) {
	bad := []string{
		"", "  ", " 1", "1 ", "1\n",
		"NaN", "+NaN", "nan", "Infinity", "-Infinity", "inf", "Inf",
		"1e", "e3", "+-1", "1.2.3", "0x1", "1e3x", "1.5e", ".", "..",
		"1..2", ".1.2", "1_000", "1,0", "1e03x", "0x1p4", "true",
		"1e+99", "1e51", "1e-51", "1000000000000000000000000000000000000000000000000000000000000", // 10^60, decimal exponent 60
	}
	bad = append(bad, "0."+strings.Repeat("0", 50)+"1")  // leading digit at 10^-51
	bad = append(bad, "1234567890123456789012345678901") // 31 significant digits
	for _, s := range bad {
		if r := Convert(s); !r.Reject() {
			t.Fatalf("input %q should have been rejected, got %04x", s, r.Bits())
		}
	}

	good := []string{
		"1.", ".5", "+.5e2", "1e-50", "-1e-50", "1e50", "1E+0", "00007",
		"0." + strings.Repeat("0", 49) + "1", // 10^-50, exactly at the limit
		"123456789012345678901234567890",     // 30 significant digits
		"100000000000000000000000000000",     // one sig digit + trailing zeros (exp 29)
	}
	for _, s := range good {
		if r := Convert(s); r.Reject() {
			t.Fatalf("input %q should be accepted", s)
		}
	}
}

func TestBatch(t *testing.T) {
	if _, err := RunBatch(&Request{Values: nil}); err == nil {
		t.Fatal("empty batch must error")
	}
	big := make([]string, MaxBatch+1)
	for i := range big {
		big[i] = "1"
	}
	if _, err := RunBatch(&Request{Values: big}); err == nil {
		t.Fatal("batch of 1001 must error")
	}

	vals := make([]string, MaxBatch)
	for i := range vals {
		vals[i] = "1"
	}
	resp, err := RunBatch(&Request{Values: vals})
	if err != nil || resp.Count != MaxBatch {
		t.Fatalf("batch of 1000 failed: %v", err)
	}

	resp, err = RunBatch(&Request{Values: []string{"0.1", "NaN", "65520", "-0"}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count != 4 || len(resp.Results) != 4 {
		t.Fatalf("count = %d", resp.Count)
	}
	if resp.Results[0].Hex == nil || *resp.Results[0].Hex != "2e66" {
		t.Fatalf("first result = %+v", resp.Results[0])
	}
	if resp.Results[1].OK || resp.Results[1].Error == nil {
		t.Fatalf("NaN must be a rejected item: %+v", resp.Results[1])
	}
	if resp.Results[2].Class == nil || *resp.Results[2].Class != ClassInfinity || resp.Results[2].Delta != nil {
		t.Fatalf("infinity item wrong: %+v", resp.Results[2])
	}
	if resp.Results[3].Hex == nil || *resp.Results[3].Hex != "8000" {
		t.Fatalf("-0 item wrong: %+v", resp.Results[3])
	}
}
