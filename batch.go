package half

import (
	"encoding/json"
)

// MaxBatch is the maximum number of values accepted per request.
const MaxBatch = 1000

// Request is the CLI input document.
type Request struct {
	Values []string `json:"values"`
}

type fraction struct {
	Num string `json:"num"`
	Den string `json:"den"`
}

// item is one element of the response's results array. Finite fields are
// absent for rejected items; Delta is a fraction for finite values and an
// explicit null for infinity.
type item struct {
	Input   string    `json:"input"`
	OK      bool      `json:"ok"`
	Error   *string   `json:"error"`
	Hex     *string   `json:"hex"`
	Bits    *string   `json:"bits"`
	Class   *Class    `json:"class"`
	Rounded *string   `json:"rounded"`
	Delta   *fraction `json:"delta"`
}

func strptr(s string) *string { return &s }

// Response is the CLI output document.
type Response struct {
	Count   int    `json:"count"`
	Results []item `json:"results"`
}

// BatchError describes a malformed request envelope.
type BatchError struct{ Msg string }

func (e *BatchError) Error() string { return e.Msg }

// RunBatch converts one request document of decimal strings. The batch
// size must be between 1 and 1000. Individual bad values never abort the
// batch; they come back as rejected items.
func RunBatch(req *Request) (*Response, error) {
	n := len(req.Values)
	if n == 0 {
		return nil, &BatchError{Msg: "empty batch: provide 1 to 1000 decimal strings in \"values\""}
	}
	if n > MaxBatch {
		return nil, &BatchError{Msg: "batch too large: at most 1000 values per request"}
	}
	resp := &Response{Count: n, Results: make([]item, n)}
	for i, s := range req.Values {
		r := Convert(s)
		it := item{Input: s}
		if r.Reject() {
			it.OK = false
			it.Error = strptr("invalid decimal: expected an optional sign, digits with at most one decimal point, and an optional decimal exponent in [-50,50]; NaN and Infinity are not accepted; at most 30 significant digits")
			resp.Results[i] = it
			continue
		}
		it.OK = true
		it.Hex = strptr(r.Hex())
		it.Bits = strptr("0b" + padBits(r.Bits()))
		c := r.Class()
		it.Class = &c
		it.Rounded = strptr(r.RoundedDecimal())
		if num, den, ok := r.ErrorFraction(); ok {
			it.Delta = &fraction{Num: num, Den: den}
		} // else infinity: nil pointer marshals as explicit null
		resp.Results[i] = it
	}
	return resp, nil
}

func padBits(b uint16) string {
	const digits = "01"
	out := make([]byte, 16)
	for i := 15; i >= 0; i-- {
		out[i] = digits[b&1]
		b >>= 1
	}
	return string(out)
}

// Marshal is a convenience used by the CLI.
func Marshal(resp *Response) ([]byte, error) {
	return json.MarshalIndent(resp, "", "  ")
}

// UnmarshalRequest parses a request document.
func UnmarshalRequest(data []byte) (*Request, error) {
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}
