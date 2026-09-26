// Command halfconv reads a JSON batch of decimal strings and converts each
// one to its IEEE 754 binary16 bit pattern using exact integer/rational
// arithmetic.
//
// Usage:
//
//	halfconv [file.json]
//
// With no file argument the request is read from standard input.
// Request envelope:
//
//	{"values": ["0.1", "-1e-10", "65520"]}
//
// Every input value produces one result; malformed values are reported per
// item. A malformed envelope or a batch outside 1..1000 values exits with
// status 1.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	halfconv "halfconv"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "halfconv:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	var data []byte
	var err error
	switch len(args) {
	case 0:
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
	case 1:
		data, err = os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("reading %s: %w", args[0], err)
		}
	default:
		return errors.New("usage: halfconv [file.json]")
	}

	req, err := halfconv.UnmarshalRequest(data)
	if err != nil {
		return fmt.Errorf("invalid request JSON: %w", err)
	}
	resp, err := halfconv.RunBatch(req)
	if err != nil {
		return err
	}
	out, err := halfconv.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(out, '\n'))
	return err
}
