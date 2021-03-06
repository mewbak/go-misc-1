// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

package main

import (
	"fmt"
	"log"
	"math"
	"os"
)

func sRGBToLinear(s float64) float64 {
	if s <= 0.0405 {
		return s / 12.92
	} else {
		return math.Pow(((s + 0.055) / 1.055), 2.4)
	}
}

func linearTosRGB(x float64) float64 {
	if x <= 0.0031308 {
		return x * 12.92
	} else {
		return 1.055*math.Pow(x, 1/2.4) - 0.055
	}
}

func main() {
	f, err := os.Create("srgbtab.go")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// Create the sRGB to linear table.
	fmt.Fprintf(f, `// Generated by makesrgbtab. DO NOT EDIT.

package palette

var sRGBToLinearTab = [256]uint16{
`)
	for i := 0; i < 256; i++ {
		s := float64(i) / 255
		l := sRGBToLinear(s)
		fmt.Fprintf(f, "\t%d,\n", uint16((1<<16-1)*l))
	}
	fmt.Fprintf(f, "}\n\n")

	// We could compute a complete table from uint16 linear RGB to
	// uint8 sRGB, but that table is fairly large and very
	// redundant. Instead, we try to find a smaller table where a
	// simple formula and lookup will get us within a reasonable
	// error bound.
	//
	// Specifically, given an error defined as
	//
	// err(l) = |table[(l + addend) >> shift] - linearTosRGB(l)|
	//
	// for l ∈ [0, 1<<16).
	//
	// we find addend, shift, and table that maximizes shift
	// (minimizing the table size) and minimizes the mean squared
	// error subject to err(l) < 1/256.
	//
	// There are fast ways to do this, but it doesn't matter. This
	// implementation strives for clarity.
	//
	// TODO: Consider using multiple tables with different shifts
	// to get better precision on the low range and better
	// compression on the high range.
	//
	// TODO: Maybe we also want to ensure round-tripping.
	type entry struct {
		srgbs []float64
		b     uint8
	}
	var best struct {
		shift, addend int
		table         []entry
		err           float64
	}
	best.err = 1
	const acceptError = 1 / 256.0
	for shift := 5; shift >= 0; shift-- {
		bits := 16 - shift
	nextTable:
		for addend := 0; addend < 1<<uint(shift); addend++ {
			fmt.Println("considering shift", shift, "addend", addend)

			table := make([]entry, 1<<uint(bits))

			// Compute all of the sRGB values that fall into each
			// table entry.
			for l := 0; l < 1<<16; l++ {
				index := (l + addend) >> uint(shift)
				for index >= len(table) {
					table = append(table, entry{})
				}
				srgb := linearTosRGB(float64(l) / 65535)
				table[index].srgbs = append(table[index].srgbs, srgb)
			}

			tableErr := 0.0
			for i := range table {
				ent := &table[i]

				// Find a uint8 value for this table
				// entry that minimizes the maximum
				// error to all sRGB values that fall
				// into this entry.
				mins := ent.srgbs[0]
				maxs := ent.srgbs[len(ent.srgbs)-1]
				minb := int(mins*255) - 1
				maxb := int(maxs*255) + 1
				entryErr := 1.0
				for b := minb; b <= maxb; b++ {
					if b < 0 || b > 255 {
						continue
					}
					s := float64(b) / 255
					err := math.Max(math.Abs(s-mins),
						math.Abs(s-maxs))
					if err < entryErr {
						entryErr = err
						ent.b = uint8(b)
					}
				}

				// If we couldn't find a good enough
				// table value, try another setting.
				if entryErr > acceptError {
					fmt.Println("entry error", entryErr, "> acceptable error", acceptError)
					continue nextTable
				}

				tableErr += entryErr * entryErr
			}
			tableErr = tableErr / float64(len(table))
			fmt.Println("MSE is", tableErr)

			// We found an acceptable table.
			if tableErr < best.err {
				best.err = tableErr
				best.shift, best.addend = shift, addend
				best.table = table
			}
		}

		// If we found a table, that's the best shift.
		if best.table != nil {
			break
		}
	}
	fmt.Println("best table is shift", best.shift, "addend", best.addend)

	// Create the linear to sRGB table.
	fmt.Fprintf(f, `const linearTosRGBShift = %d

const linearTosRGBAddend = %d

var linearTosRGBTab = [%d]uint8{
`, best.shift, best.addend, len(best.table))
	for _, ent := range best.table {
		fmt.Fprintf(f, "\t%d,\n", ent.b)
	}
	fmt.Fprintf(f, "}\n")
}
