package main

import (
	"flag"
	"os"
)

type caseSelectionCLIFlags struct {
	flags       *flag.FlagSet
	profilePath *string
	profileHome *string
	storeRef    *string
	storeURL    *string
	filter      *string
	nodeID      *string
	status      *string
	owner       *string
	priority    *string
	tags        stringListFlag
}

func newCaseSelectionCLIFlags(commandName string, defaultStatus string) caseSelectionCLIFlags {
	flags := flag.NewFlagSet(commandName, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	out := caseSelectionCLIFlags{flags: flags}
	out.profilePath = flags.String("profile", "", "Profile bundle path or installed profile id")
	out.profileHome = flags.String("profile-home", "", "Installed profile bundle home")
	out.storeRef = flags.String("store", "", "Named Store config or Store DSN")
	out.storeURL = flags.String("store-url", "", legacyStoreURLFlagHelp)
	out.filter = flags.String("filter", "", "Filter by id, display name, scenario, description, tag, owner, or priority")
	out.nodeID = flags.String("node", "", "Only include cases attached to this interface node id")
	out.status = flags.String("status", defaultStatus, "Only include cases with this status")
	out.owner = flags.String("owner", "", "Only include cases owned by this value")
	out.priority = flags.String("priority", "", "Only include cases with this priority")
	flags.Var(&out.tags, "tag", "Only include cases with this tag; repeat for multiple tags")
	return out
}

func (f caseSelectionCLIFlags) parse(args []string) error {
	return f.flags.Parse(args)
}

func (f caseSelectionCLIFlags) caseListFilter() caseListFilter {
	return caseListFilter{
		Filter:   *f.filter,
		NodeID:   *f.nodeID,
		Tags:     f.tags.Values(),
		Status:   *f.status,
		Owner:    *f.owner,
		Priority: *f.priority,
	}
}
