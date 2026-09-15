package output

import "fmt"

// Counted writes a counted noun in the right form.
//
// These strings are read by a person, and "Result: 1 checks passed" reads
// as a defect in the tool that just printed it. The singular is given in
// full rather than derived, because the verb changes with it: one step
// did not pass, two steps did not pass.
//
// This lived twice, once in internal/policy and once in the doctor
// command, and eighteen other places counted nouns with neither.
func Counted(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return fmt.Sprintf(many, n)
}

// CountedPair writes two counts in one phrase, for the "N of M" shape.
// The singular of the second is what usually goes wrong: "1 of 1 steps".
func CountedPair(n, of int, format, oneNoun, manyNoun string) string {
	noun := manyNoun
	if of == 1 {
		noun = oneNoun
	}
	return fmt.Sprintf(format, n, of, noun)
}
