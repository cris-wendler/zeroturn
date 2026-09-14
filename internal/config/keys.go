package config

import (
	"fmt"
	"strconv"
	"strings"
)

// What a key holds, which is what reading it, printing it, and refusing
// a bad value all have to differ on.
const (
	KindPercent = "percent"
	KindMinutes = "minutes"
	KindCount   = "count"
	KindChoice  = "choice"
)

// Key is one guard setting that can be read and changed by name.
//
// This registry is the only description of a setting. The table printed
// by policy show, the command that writes one, and the range checks in
// Validate all read it, so a setting cannot be offered in one place and
// missing from another. A test walks the Guard type and fails when a
// field has no key here, and when a key names a field that is gone.
type Key struct {
	// Name is what a person types, and is part of the public contract.
	Name string
	// Label is how the name is printed in the policy table.
	Label string
	Kind  string
	// Choices are the values a choice key accepts, in the order offered.
	Choices []string
	// Noun names what the value is, for the message that refuses one.
	Noun string

	// Exactly one of these is set, and it returns the field itself, so
	// reading and writing a setting cannot drift apart.
	num func(*Guard) *int
	str func(*Guard) *string
}

var keys = []Key{
	{Name: "guard.mode", Label: "mode", Kind: KindChoice, Noun: "a guard mode",
		Choices: []string{ModeObserve, ModeConfirm, ModeStrict},
		str:     func(g *Guard) *string { return &g.Mode }},

	{Name: "guard.context.warn", Label: "context warn", Kind: KindPercent,
		num: func(g *Guard) *int { return &g.Context.Warn }},
	{Name: "guard.context.confirm", Label: "context confirm", Kind: KindPercent,
		num: func(g *Guard) *int { return &g.Context.Confirm }},
	{Name: "guard.context.critical", Label: "context critical", Kind: KindPercent,
		num: func(g *Guard) *int { return &g.Context.Critical }},

	{Name: "guard.limits.fiveHourWarn", Label: "five hour warn", Kind: KindPercent,
		num: func(g *Guard) *int { return &g.Limits.FiveHourWarn }},
	{Name: "guard.limits.sevenDayWarn", Label: "seven day warn", Kind: KindPercent,
		num: func(g *Guard) *int { return &g.Limits.SevenDayWarn }},
	{Name: "guard.limits.projection", Label: "rate projection", Kind: KindChoice, Noun: "a projection setting",
		Choices: []string{ProjectionOn, ProjectionOff},
		str:     func(g *Guard) *string { return &g.Limits.Projection }},

	{Name: "guard.session.durationWarnMinutes", Label: "duration warn", Kind: KindMinutes,
		num: func(g *Guard) *int { return &g.Session.DurationWarnMinutes }},
	{Name: "guard.session.activeSubagentsWarn", Label: "active subagents warn", Kind: KindCount,
		num: func(g *Guard) *int { return &g.Session.ActiveSubagentsWarn }},
	{Name: "guard.session.subagentStartsWarn", Label: "subagent starts warn", Kind: KindCount,
		num: func(g *Guard) *int { return &g.Session.SubagentStartsWarn }},

	{Name: "guard.credentials.mode", Label: "credential guard", Kind: KindChoice, Noun: "a credential guard mode",
		Choices: []string{CredentialOff, CredentialAsk, CredentialDeny},
		str:     func(g *Guard) *string { return &g.Credentials.Mode }},
	{Name: "guard.credentials.prompts", Label: "prompt guard", Kind: KindChoice, Noun: "a prompt guard setting",
		Choices: []string{PromptsOff, PromptsBlock},
		str:     func(g *Guard) *string { return &g.Credentials.Prompts }},
}

// Keys reports every guard setting, in the order they are printed.
func Keys() []Key {
	out := make([]Key, len(keys))
	copy(out, keys)
	return out
}

// Lookup finds one setting by the name a person types.
func Lookup(name string) (Key, bool) {
	for _, k := range keys {
		if k.Name == name {
			return k, true
		}
	}
	return Key{}, false
}

// Names reports every setting name, for a message that has to list them.
func Names() []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k.Name)
	}
	return out
}

// Value reports the stored value as text, without the unit.
func (k Key) Value(g Guard) string {
	if k.num != nil {
		return strconv.Itoa(*k.num(&g))
	}
	return *k.str(&g)
}

// Display reports the value as the policy table prints it, with the unit
// that makes a bare number mean something.
func (k Key) Display(g Guard) string {
	switch k.Kind {
	case KindPercent:
		return k.Value(g) + "%"
	case KindMinutes:
		return k.Value(g) + " minutes"
	}
	return k.Value(g)
}

// Set writes one value, and refuses one it cannot read. It reports a
// ValidationError so that the range checks and the command both fail the
// same way, with the name, what is wrong, and what to do instead.
func (k Key) Set(g *Guard, value string) error {
	if k.num != nil {
		n, err := strconv.Atoi(value)
		if err != nil {
			return ValidationError{k.Name, "value " + quote(value) + " is not a whole number",
				"supply a whole number"}
		}
		*k.num(g) = n
		return nil
	}
	v := strings.ToLower(value)
	for _, c := range k.Choices {
		if v == c {
			*k.str(g) = v
			return nil
		}
	}
	return ValidationError{k.Name, "value " + quote(value) + " is not " + k.Noun,
		"use " + orList(k.Choices)}
}

// orList writes choices the way the messages already did: every value
// separated by a comma, and the last one introduced by or.
func orList(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	}
	return fmt.Sprintf("%s, or %s", strings.Join(values[:len(values)-1], ", "), values[len(values)-1])
}

// Check reports whether the stored value is one this key accepts. It is
// how a file that was edited by hand is refused, with the same wording a
// bad value typed at the command line gets.
func (k Key) Check(g Guard) error {
	if k.Kind != KindChoice {
		return nil
	}
	v := *k.str(&g)
	for _, c := range k.Choices {
		if v == c {
			return nil
		}
	}
	return ValidationError{k.Name, "value " + quote(v) + " is not " + k.Noun,
		"use " + orList(k.Choices)}
}
