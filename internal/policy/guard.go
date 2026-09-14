package policy

import "github.com/cris-wendler/zeroturn/internal/config"

// Guard is a configuration that has already been through the local
// Strict approval check. Evaluate takes one of these rather than a
// configuration, so the check cannot be skipped by forgetting to call
// it: the only way to obtain a Guard is to answer whether this machine
// approved Strict mode.
//
// The rule it carries is a security rule. A .zeroturn.json committed
// asking for Strict must not let a repository deny subagents for
// everyone who clones it, so until the person at this machine approves
// it the gate asks instead of denying.
//
// The zero value carries no mode, which evaluates to allow. It cannot
// carry Strict, which is the property that matters here.
type Guard struct {
	cfg config.Config
}

// NewGuard applies the Strict downgrade and reports the configuration
// the gate will act on. Pass whether this machine has approved Strict
// for the repository the configuration came from.
func NewGuard(c config.Config, strictApproved bool) Guard {
	if c.Guard.Mode == config.ModeStrict && !strictApproved {
		c.Guard.Mode = config.ModeConfirm
	}
	return Guard{cfg: c}
}

// Config reports the configuration with the downgrade applied. Callers
// that need a setting the gate does not read, such as the credential
// guard, read it from here rather than from the file again.
func (g Guard) Config() config.Config { return g.cfg }

// Mode reports the guard mode in force, which is not always the mode the
// repository asked for.
func (g Guard) Mode() string { return g.cfg.Guard.Mode }
