// Command switches prints every guard switch as <name>=<enabled>, one per line.
package main

import (
	"fmt"

	"github.com/oritromax/lyceum/internal/guard"
)

func main() {
	fmt.Printf("selftest=%t\n", guard.SelftestEnabled)
	fmt.Printf("approvetty=%t\n", guard.ApprovettyEnabled)
	fmt.Printf("approveenv=%t\n", guard.ApproveenvEnabled)
	fmt.Printf("approvedirty=%t\n", guard.ApprovedirtyEnabled)
	fmt.Printf("approveowner=%t\n", guard.ApproveownerEnabled)
	fmt.Printf("approveconfirm=%t\n", guard.ApproveconfirmEnabled)
	fmt.Printf("checkintegrity=%t\n", guard.CheckintegrityEnabled)
	fmt.Printf("checkfields=%t\n", guard.CheckfieldsEnabled)
	fmt.Printf("checkgate=%t\n", guard.CheckgateEnabled)
	fmt.Printf("checknounit=%t\n", guard.ChecknounitEnabled)
	fmt.Printf("checkchore=%t\n", guard.CheckchoreEnabled)
	fmt.Printf("evidence=%t\n", guard.EvidenceEnabled)
	fmt.Printf("reprofreeze=%t\n", guard.ReprofreezeEnabled)
	fmt.Printf("baselinefreeze=%t\n", guard.BaselinefreezeEnabled)
	fmt.Printf("reprocode=%t\n", guard.ReprocodeEnabled)
	fmt.Printf("reopen=%t\n", guard.ReopenEnabled)
	fmt.Printf("hook=%t\n", guard.HookEnabled)
	fmt.Printf("harnessprompt=%t\n", guard.HarnesspromptEnabled)
}
