package output

import (
	"fmt"
	"strings"
)

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

// List writes names as a person would read them: "a", "a and b", or
// "a, b and c". Joining with commas alone produces "context, the five
// hour usage window, session duration", which reads as an unfinished
// sentence in the middle of a paragraph.
func List(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
