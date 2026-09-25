package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/crakacr-alt/Chameleon-Protocol/pkg/carrier"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/dpi"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/networkctx"
	"github.com/crakacr-alt/Chameleon-Protocol/pkg/planner"
)

func main() {
	destination := flag.String("destination", "example.com:443", "destination host:port")
	protocol := flag.String("protocol", "tcp", "transport protocol: tcp or udp")
	purpose := flag.String("purpose", "web", "traffic purpose: web, streaming, gaming, bulk, realtime")
	networkID := flag.String("network-id", "", "optional network identity override")
	chameleonUDP := flag.String("chameleon-udp", "", "optional Chameleon UDP endpoint")
	chameleonTCP := flag.String("chameleon-tcp", "", "optional Chameleon TCP fallback endpoint")
	relay := flag.String("relay", "", "optional external relay endpoint, for example an existing sidecar")
	stateDir := flag.String("state-dir", defaultStateDir(), "directory for adaptive planner state")
	record := flag.String("record", "", "record selected plan result: success or failure")
	scope := flag.String("scope", "both", "failure scope: carrier, dpi or both")
	latencyMS := flag.Int("latency-ms", 0, "measured latency for --record")
	throughput := flag.Float64("throughput", 0, "measured throughput bytes/sec for --record")
	failure := flag.String("failure", "", "optional failure description for --record")
	flag.Parse()

	network, err := networkctx.Detect()
	if err != nil {
		exitErr(err)
	}
	if strings.TrimSpace(*networkID) != "" {
		network.ID = strings.TrimSpace(*networkID)
	}

	carrierPath := ""
	dpiPath := ""
	if strings.TrimSpace(*stateDir) != "" {
		carrierPath = filepath.Join(*stateDir, "carriers.json")
		dpiPath = filepath.Join(*stateDir, "dpi.json")
	}

	carrierEngine, err := carrier.NewEngine(carrierPath)
	if err != nil {
		exitErr(err)
	}
	dpiEngine, err := dpi.NewEngine(dpiPath)
	if err != nil {
		exitErr(err)
	}
	p, err := planner.New(carrierEngine, dpiEngine)
	if err != nil {
		exitErr(err)
	}

	req := planner.Request{
		Network:       network,
		Destination:   *destination,
		Protocol:      strings.ToLower(strings.TrimSpace(*protocol)),
		Purpose:       strings.ToLower(strings.TrimSpace(*purpose)),
		Carriers:      carrier.Defaults(*chameleonUDP, *chameleonTCP, *relay),
		DPIStrategies: dpi.DefaultStrategies(),
	}
	plan, err := p.Choose(req)
	if err != nil {
		exitErr(err)
	}

	printPlan(network, plan)

	if strings.TrimSpace(*record) == "" {
		return
	}

	success, err := parseResult(*record)
	if err != nil {
		exitErr(err)
	}
	resultScope, err := parseScope(*scope)
	if err != nil {
		exitErr(err)
	}

	if err := p.Observe(planner.Result{
		Plan:        plan,
		Destination: *destination,
		Protocol:    req.Protocol,
		Success:     success,
		Scope:       resultScope,
		Latency:     time.Duration(*latencyMS) * time.Millisecond,
		Throughput:  *throughput,
		Failure:     *failure,
		At:          time.Now(),
	}); err != nil {
		exitErr(err)
	}

	fmt.Printf("recorded: success=%t scope=%s latency=%dms throughput=%s B/s\n",
		success, resultScope, *latencyMS, strconv.FormatFloat(*throughput, 'f', 0, 64))
}

func printPlan(network networkctx.Context, plan planner.Plan) {
	fmt.Println("Chameleon adaptive plan")
	fmt.Printf("network: %s (%s, interface=%s, ipv4=%t, ipv6=%t)\n",
		network.ID, network.Link, network.Interface, network.IPv4, network.IPv6)
	fmt.Printf("traffic: %s\n", plan.TrafficClass)
	fmt.Printf("carrier: %s [%s]\n", plan.Carrier.Carrier.Name, plan.Carrier.Carrier.Kind)
	if plan.Carrier.Carrier.Endpoint != "" {
		fmt.Printf("carrier endpoint: %s\n", plan.Carrier.Carrier.Endpoint)
	}
	fmt.Printf("dpi: %s\n", plan.DPI.Strategy.Name)
	fmt.Printf("dpi target: %s\n", plan.DPITarget)
	fmt.Printf("reason: %s\n", plan.Reason)
	if len(plan.Carrier.Alternates) > 0 {
		fmt.Printf("carrier fallbacks: %s\n", strings.Join(plan.Carrier.Alternates, ", "))
	}
	if len(plan.DPI.Alternates) > 0 {
		fmt.Printf("dpi fallbacks: %s\n", strings.Join(plan.DPI.Alternates, ", "))
	}
}

func parseResult(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "success", "ok", "true":
		return true, nil
	case "failure", "fail", "false":
		return false, nil
	default:
		return false, fmt.Errorf("unknown --record value %q", v)
	}
}

func parseScope(v string) (planner.FailureScope, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "carrier":
		return planner.ScopeCarrier, nil
	case "dpi":
		return planner.ScopeDPI, nil
	case "both", "":
		return planner.ScopeBoth, nil
	default:
		return "", fmt.Errorf("unknown --scope value %q", v)
	}
}

func defaultStateDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		return ""
	}
	return filepath.Join(configDir, "chameleon")
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
