package main

import (
	"flag"
	"fmt"

	"github.com/Hack2p/chameleon/pkg/core"
	"github.com/Hack2p/chameleon/pkg/experiment"
)

func main() {
	profile := flag.String("profile", string(core.ProfileWebRTC), "traffic profile to benchmark")
	payload := flag.String("payload", "hello-chameleon", "payload to send")
	burst := flag.Int("burst", 2, "packets per round")
	rounds := flag.Int("rounds", 1, "number of rounds")
	psk := flag.String("psk", "research-secret", "shared secret for optional transport protection")
	compare := flag.Bool("compare", false, "run a multi-profile comparison matrix and print the report")
	flag.Parse()

	if *compare {
		report, err := experiment.CompareProfilesReport(
			experiment.Scenario{Profile: core.ProfileWebRTC, Payload: []byte(*payload), Burst: *burst, Rounds: *rounds, SharedSecret: *psk},
			experiment.Scenario{Profile: core.ProfileHTTP3, Payload: []byte(*payload), Burst: *burst, Rounds: *rounds, SharedSecret: *psk},
			experiment.Scenario{Profile: core.ProfileGaming, Payload: []byte(*payload), Burst: *burst, Rounds: *rounds, SharedSecret: *psk},
		)
		if err != nil {
			panic(err)
		}
		fmt.Println(report)
		return
	}

	metrics, err := experiment.Scenario{
		Profile:      core.BehaviorProfile(*profile),
		Payload:      []byte(*payload),
		Burst:        *burst,
		Rounds:       *rounds,
		SharedSecret: *psk,
	}.Run()
	if err != nil {
		panic(err)
	}

	fmt.Println(metrics.Report())
}
