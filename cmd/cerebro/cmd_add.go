package main

// cmd_add.go — `cerebro add` subcommand.
//
// --beads-id flag (HS-039): attaches a structured beadsId to the memory's metadata
// column via brain.WithMetadata(json.RawMessage). This replaces HS-030's prompt-side
// BD_ID embedding with a code-side, Wire-deterministic tag.
//
// N-S1 validation (HS-029 canonical regex, byte-identical to validate-hello-stack.sh:243
// and validate-meepo-beads-link.sh:79):
//   - Empty / whitespace-only post-trim → flag treated as absent (no metadata write)
//   - Non-empty + non-matching → CLI error with canonical pattern in message
//
// Stale-metadata merge contract (§2 forward-compatibility):
// If a future --metadata flag is added, beadsId MUST WIN on key collision.
// This contract is locked here so the next ticket adding --metadata doesn't
// need to re-derive the policy.
//
// JSON encoding uses json.Marshal (NOT fmt.Sprintf) — the beadsId value arrives
// from the session-context substrate and is bounded by the HS-029 regex above,
// but json.Marshal provides defence-in-depth JSON-escaping regardless (TL-N3).

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
	"github.com/spf13/cobra"
)

var addTypeFlag string
var addImportanceFlag float64
var addSubtypeFlag string
var addProvenanceRootFlag bool

// addBeadsIdFlag holds the --beads-id flag value (raw, pre-trim).
// N-S1: trimmed and validated against beadsIdRegexp before use.
var addBeadsIdFlag string

// addOriginFlags carries the --origin-* overrides for `cerebro add` (goc7).
var addOriginFlags originFlags

// addAnchorFlag / addAnchorRefFlag carry the cite-and-verify source anchor
// (agentic-k8an): the file that proves this memory, hashed at write time.
var addAnchorFlag string
var addAnchorRefFlag string

// beadsIdRegexp is the HS-029 canonical regex, byte-identical to:
//
//	validate-hello-stack.sh:243  ^[a-z][a-z0-9-]{0,31}-[0-9a-z]{3,32}$
//	validate-meepo-beads-link.sh:79 BD_ID_REGEX='^[a-z][a-z0-9-]{0,31}-[0-9a-z]{3,32}$'
//
// Propagation site 3: pi-cerebro / cerebro add CLI (HS-039; memory 9137a398).
var beadsIdRegexp = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}-[0-9a-z]{3,32}$`)

func init() {
	cmd := &cobra.Command{
		Use:   "add <content>",
		Short: "Store a new memory node",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runAdd,
	}
	cmd.Flags().StringVarP(&addTypeFlag, "type", "t", "episode", "Memory type: episode, concept, procedure, reflection")
	cmd.Flags().Float64VarP(&addImportanceFlag, "importance", "i", 0.5, "Importance score (0.0-1.0)")
	cmd.Flags().StringVar(&addSubtypeFlag, "subtype", "", "Memory subtype")
	// --provenance-root marks the node as a first-class provenance source
	// (nodes.provenance_root=1; agentic-lbjg). Default false (0).
	cmd.Flags().BoolVar(&addProvenanceRootFlag, "provenance-root", false,
		"Mark this node as a first-class provenance source (provenance_root=1)")
	// --beads-id: optional beads task id for forensic linkage (HS-039).
	// Value is trimmed and validated against the HS-029 canonical regex before persisting.
	cmd.Flags().StringVar(&addBeadsIdFlag, "beads-id", "", "Beads task id to tag this memory with (forensic linkage); must match ^[a-z][a-z0-9-]{0,31}-[0-9a-z]{3,32}$")
	cmd.Flags().StringVar(&addAnchorFlag, "anchor", "",
		"Source anchor: file path (relative to the project dir) that proves this memory; hashed at write, re-verified at recall (k8an)")
	cmd.Flags().StringVar(&addAnchorRefFlag, "anchor-ref", "",
		"Optional ref label stored with the anchor (e.g. a commit SHA)")
	addOriginFlags.register(cmd)
	rootCmd.AddCommand(cmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	nodeType, err := parseNodeType(addTypeFlag)
	if err != nil {
		return err
	}

	content := strings.Join(args, " ")

	// Anchor validation happens BEFORE the brain opens: a citation to a file
	// that does not exist is a dead pointer and must fail loudly, with no
	// store side effects (k8an).
	metaMap := map[string]any{}
	if addAnchorFlag != "" {
		projectDir := resolveProjectDir()
		if _, err := os.Stat(resolveAnchorPath(addAnchorFlag, projectDir)); err != nil {
			return fmt.Errorf("--anchor %q does not resolve: %w", addAnchorFlag, err)
		}
		am := anchorMetadata(projectDir, addAnchorFlag, addAnchorRefFlag)
		if am == nil {
			return fmt.Errorf("--anchor %q could not be hashed", addAnchorFlag)
		}
		for k, v := range am {
			metaMap[k] = v
		}
	}

	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	var opts []brain.AddOption
	opts = append(opts, brain.WithImportance(addImportanceFlag))
	if addSubtypeFlag != "" {
		opts = append(opts, brain.WithSubtype(addSubtypeFlag))
	}
	if addProvenanceRootFlag {
		opts = append(opts, brain.WithProvenanceRoot())
	}
	opts = append(opts, addOriginFlags.option(cmd))

	// N-S1 (HS-039): trim first, then validate.
	// Canonical trim location is the Go CLI layer (HS-031 / TL-N1 precedent).
	// TS-side runAdd also guards but the canonical enforcement is here.
	beadsId := strings.TrimSpace(addBeadsIdFlag)
	if beadsId != "" {
		// Non-empty post-trim: validate against HS-029 canonical regex.
		if !beadsIdRegexp.MatchString(beadsId) {
			return fmt.Errorf("invalid --beads-id: must match canonical pattern ^[a-z][a-z0-9-]{0,31}-[0-9a-z]{3,32}$")
		}
		// beadsId wins on key collision per the locked HS-039 merge contract.
		metaMap["beadsId"] = beadsId
	}
	if len(metaMap) > 0 {
		// JSON encoding via json.Marshal — NOT fmt.Sprintf — per TL-N3.
		meta, err := json.Marshal(metaMap)
		if err != nil {
			return fmt.Errorf("encoding metadata: %w", err)
		}
		opts = append(opts, brain.WithMetadata(json.RawMessage(meta)))
	}
	// No anchor and empty post-trim beads-id → no WithMetadata call. AC3 back-compat.

	id, err := b.Add(content, store.NodeType(nodeType), opts...)
	if err != nil {
		return err
	}

	if formatFlag == "json" {
		outputJSON(map[string]string{"id": id})
	} else if !quietFlag {
		fmt.Println(id)
	}

	return nil
}
