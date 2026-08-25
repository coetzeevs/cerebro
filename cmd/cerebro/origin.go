package main

// origin.go — CLI origin-identity derivation (agentic-goc7).
//
// The CLI stamps only what it OBSERVES about the write event:
//
//	channel = "cli"            — the write demonstrably came through the CLI
//	host    = os.Hostname()    — the machine the write ran on
//	session = $CEREBRO_ORIGIN_SESSION, else $CLAUDE_SESSION_ID, else unset
//	actor   = $CEREBRO_ORIGIN_ACTOR, else unset — actor identity is NOT
//	          observable from inside the process, so it is never guessed;
//	          an actor-less write classifies origin_status "unknown", which
//	          is the honest surfacing of the gap.
//
// --origin-* flags override every derived value; an explicitly-set empty flag
// clears the derived default to NULL (cobra Changed() semantics).

import (
	"os"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/spf13/cobra"
)

// originFlags holds the flag targets for one command's --origin-* set. Each
// origin-stamping command owns its own instance so parallel flag registration
// never aliases (the addTypeFlag-style shared-global pattern is per-command
// here because add and supersede both stamp origin).
type originFlags struct {
	actor   string
	channel string
	session string
	host    string
}

// register wires the four --origin-* flags onto cmd.
func (f *originFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.actor, "origin-actor", "",
		"Origin identity: who wrote this memory (default: $CEREBRO_ORIGIN_ACTOR)")
	cmd.Flags().StringVar(&f.channel, "origin-channel", "",
		"Origin identity: the channel the write came through (default: \"cli\")")
	cmd.Flags().StringVar(&f.session, "origin-session", "",
		"Origin identity: the session the write belongs to (default: $CEREBRO_ORIGIN_SESSION or $CLAUDE_SESSION_ID)")
	cmd.Flags().StringVar(&f.host, "origin-host", "",
		"Origin identity: the host the write ran on (default: hostname)")
}

// option derives the effective origin for this invocation and returns the
// brain.WithOrigin option carrying it. Flag > env > observed default.
func (f *originFlags) option(cmd *cobra.Command) brain.AddOption {
	actor := os.Getenv("CEREBRO_ORIGIN_ACTOR")
	channel := "cli"
	session := os.Getenv("CEREBRO_ORIGIN_SESSION")
	if session == "" {
		session = os.Getenv("CLAUDE_SESSION_ID")
	}
	host, _ := os.Hostname()

	if cmd.Flags().Changed("origin-actor") {
		actor = f.actor
	}
	if cmd.Flags().Changed("origin-channel") {
		channel = f.channel
	}
	if cmd.Flags().Changed("origin-session") {
		session = f.session
	}
	if cmd.Flags().Changed("origin-host") {
		host = f.host
	}

	return brain.WithOrigin(actor, channel, session, host)
}
