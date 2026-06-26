package store

// RelationDerivedFrom is the reserved built-in edge relation that links a
// derived node (concept/procedure/reflection) back to a source episode it was
// synthesized from. It is written automatically at consolidation time
// (ConsolidateInto) and is the relation provenance walks traverse outward
// (WalkRelation with outgoing=true). Reserving it as a single exported constant
// (rather than scattering the "derived_from" string literal) keeps the
// consolidation writer, the walk, and the provenance-status check in agreement.
//
// The typed-relation REGISTRY that seeds reserved relations on init is
// agentic-8l2g (out of scope here); lbjg only reserves the string in code.
const RelationDerivedFrom = "derived_from"

// MetaProvenanceConventionSince is the schema_meta key holding the instant the
// provenance convention took effect for a brain (the v4->v5 migration instant,
// or the brain's birth instant for a fresh v5 Init). provenanceStatus compares a
// node's created_at against this boundary to distinguish "legacy" (predates the
// convention — absence of provenance is expected) from "none" (created after the
// convention with no recorded source). The value is stored as a string in the
// repo's storageTimeLayout and parsed once into a time.Time before comparing —
// the comparison is a time.Time compare, never a lexicographic string compare.
const MetaProvenanceConventionSince = "provenance_convention_since"
