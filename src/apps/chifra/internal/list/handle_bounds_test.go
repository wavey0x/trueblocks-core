package listPkg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TrueBlocks/trueblocks-core/src/apps/chifra/pkg/base"
	"github.com/TrueBlocks/trueblocks-core/src/apps/chifra/pkg/config"
	"github.com/TrueBlocks/trueblocks-core/src/apps/chifra/pkg/monitor"
	"github.com/TrueBlocks/trueblocks-core/src/apps/chifra/pkg/output"
	"github.com/TrueBlocks/trueblocks-core/src/apps/chifra/pkg/types"
)

type boundsTestResponse struct {
	Data []struct {
		Address string `json:"address"`
		Count   uint64 `json:"count"`
	} `json:"data"`
	Errors []string `json:"errors,omitempty"`
}

func TestHandleBoundsMixedBatchDoesNotEmitMissingErrors(t *testing.T) {
	setupBoundsTestConfig(t)
	withApps := boundsTestMonitor(t, "0x1111111111111111111111111111111111111111", []types.AppRecord{
		{BlockNumber: 10, TransactionIndex: 1},
		{BlockNumber: 20, TransactionIndex: 2},
	})
	empty := boundsTestMonitor(t, "0x2222222222222222222222222222222222222222", nil)

	resp, body := runHandleBoundsTest(t, []monitor.Monitor{withApps, empty})
	if len(resp.Errors) != 0 {
		t.Fatalf("expected no errors in mixed bounds response, got %v in %s", resp.Errors, body)
	}
	if strings.Contains(body, "no appearances found") {
		t.Fatalf("mixed bounds response should not contain missing-appearance errors: %s", body)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected one bounds result, got %d in %s", len(resp.Data), body)
	}
	if resp.Data[0].Address != withApps.Address.Hex() {
		t.Fatalf("expected bounds for %s, got %s", withApps.Address.Hex(), resp.Data[0].Address)
	}
	if resp.Data[0].Count != 2 {
		t.Fatalf("expected count 2, got %d", resp.Data[0].Count)
	}
}

func TestHandleBoundsAllEmptyReportsNoResults(t *testing.T) {
	setupBoundsTestConfig(t)
	monitors := []monitor.Monitor{
		boundsTestMonitor(t, "0x3333333333333333333333333333333333333333", nil),
		boundsTestMonitor(t, "0x4444444444444444444444444444444444444444", nil),
	}

	resp, body := runHandleBoundsTest(t, monitors)
	if len(resp.Data) != 0 {
		t.Fatalf("expected no bounds data, got %d items in %s", len(resp.Data), body)
	}
	if len(resp.Errors) != 1 {
		t.Fatalf("expected one no-results error, got %v in %s", resp.Errors, body)
	}
	if resp.Errors[0] != "no appearances found for any requested address" {
		t.Fatalf("unexpected no-results error: %q", resp.Errors[0])
	}
}

func setupBoundsTestConfig(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config", "mainnet"), 0700); err != nil {
		t.Fatal(err)
	}
	configText := fmt.Sprintf(`
[version]
current = "v999.0.0"

[settings]
cachePath = "%[1]s"
indexPath = "%[2]s"
defaultChain = "mainnet"

[pinning]
gatewayUrl = ""
localPinUrl = "http://localhost:5001"
remotePinUrl = "https://api.pinata.cloud/pinning/pinFileToIPFS"

[unchained]
preferredPublisher = "publisher.unchainedindex.eth"
smartContract = "0x0c316b7042b419d07d343f2f4f5bd54ff731183d"

[chains.mainnet]
chain = "mainnet"
chainId = "1"
rpcProviders = ["http://127.0.0.1:8545"]
symbol = "ETH"
blockTime = 12
`, filepath.Join(root, "cache"), filepath.Join(root, "unchained"))
	if err := os.WriteFile(filepath.Join(root, "trueBlocks.toml"), []byte(configText), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("TB_NO_PROVIDER_CHECK", "true")
	config.ReloadConfig()
	t.Cleanup(config.ReloadConfig)
}

func boundsTestMonitor(t *testing.T, addr string, apps []types.AppRecord) monitor.Monitor {
	t.Helper()
	mon, err := monitor.NewMonitorStaged("mainnet", addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) == 0 {
		return mon
	}
	if err := mon.WriteAppearancesAppend(apps[len(apps)-1].BlockNumber, &apps); err != nil {
		t.Fatal(err)
	}
	return mon
}

func runHandleBoundsTest(t *testing.T, monitors []monitor.Monitor) (boundsTestResponse, string) {
	t.Helper()
	var buf bytes.Buffer
	jw := output.NewDefaultJsonWriter(&buf, false)
	opts := &ListOptions{
		MaxRecords: 250,
		LastBlock:  base.NOPOSN,
	}
	opts.Globals.Chain = "mainnet"
	opts.Globals.Format = "json"
	opts.Globals.Writer = jw

	if err := opts.HandleBounds(output.NewRenderContext(), monitors); err != nil {
		t.Fatal(err)
	}
	if err := jw.Close(); err != nil {
		t.Fatal(err)
	}

	body := buf.String()
	var resp boundsTestResponse
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse bounds JSON %q: %v", body, err)
	}
	return resp, body
}
