// Copyright 2015 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

package utils

import (
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"math"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/ethash"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/logger"
	"github.com/ethereum/go-ethereum/logger/glog"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/pow"
	"github.com/ethereum/go-ethereum/raft"
	"github.com/ethereum/go-ethereum/rpc"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv2"
	"gopkg.in/urfave/cli.v1"
)

func init() {
	cli.AppHelpTemplate = `{{.Name}} {{if .Flags}}[global options] {{end}}command{{if .Flags}} [command options]{{end}} [arguments...]

VERSION:
   {{.Version}}

COMMANDS:
   {{range .Commands}}{{.Name}}{{with .ShortName}}, {{.}}{{end}}{{ "\t" }}{{.Usage}}
   {{end}}{{if .Flags}}
GLOBAL OPTIONS:
   {{range .Flags}}{{.}}
   {{end}}{{end}}
`

	cli.CommandHelpTemplate = `{{.Name}}{{if .Subcommands}} command{{end}}{{if .Flags}} [command options]{{end}} [arguments...]
{{if .Description}}{{.Description}}
{{end}}{{if .Subcommands}}
SUBCOMMANDS:
	{{range .Subcommands}}{{.Name}}{{with .ShortName}}, {{.}}{{end}}{{ "\t" }}{{.Usage}}
	{{end}}{{end}}{{if .Flags}}
OPTIONS:
	{{range .Flags}}{{.}}
	{{end}}{{end}}
`
}

// NewApp creates an app with sane defaults.
func NewApp(gitCommit, usage string) *cli.App {
	app := cli.NewApp()
	app.Name = filepath.Base(os.Args[0])
	app.Author = ""
	//app.Authors = nil
	app.Email = ""
	app.Version = Version
	if gitCommit != "" {
		app.Version += "-" + gitCommit[:8]
	}
	app.Usage = usage
	return app
}

// These are all the command line flags we support.
// If you add to this list, please remember to include the
// flag in the appropriate command definition.
//
// The flags are defined here so their names and help texts
// are the same for all commands.

var (
	// General settings
	DataDirFlag = DirectoryFlag{
		Name:  "datadir",
		Usage: "Data directory for the databases and keystore",
		Value: DirectoryString{node.DefaultDataDir()},
	}
	KeyStoreDirFlag = DirectoryFlag{
		Name:  "keystore",
		Usage: "Directory for the keystore (default = inside the datadir)",
	}
	NetworkIdFlag = cli.IntFlag{
		Name:  "networkid",
		Usage: "Network identifier (integer, 0=Olympic, 1=Frontier, 2=Morden)",
		Value: eth.NetworkId,
	}
	OlympicFlag = cli.BoolFlag{
		Name:  "olympic",
		Usage: "Olympic network: pre-configured pre-release test network",
	}
	TestNetFlag = cli.BoolFlag{
		Name:  "testnet",
		Usage: "Morden network: pre-configured test network with modified starting nonces (replay protection)",
	}
	DevModeFlag = cli.BoolFlag{
		Name:  "dev",
		Usage: "Developer mode: pre-configured private network with several debugging flags",
	}
	IdentityFlag = cli.StringFlag{
		Name:  "identity",
		Usage: "Custom node name",
	}
	NatspecEnabledFlag = cli.BoolFlag{
		Name:  "natspec",
		Usage: "Enable NatSpec confirmation notice",
	}
	DocRootFlag = DirectoryFlag{
		Name:  "docroot",
		Usage: "Document Root for HTTPClient file scheme",
		Value: DirectoryString{homeDir()},
	}
	LightKDFFlag = cli.BoolFlag{
		Name:  "lightkdf",
		Usage: "Reduce key-derivation RAM & CPU usage at some expense of KDF strength",
	}
	// Performance tuning settings
	CacheFlag = cli.IntFlag{
		Name:  "cache",
		Usage: "Megabytes of memory allocated to internal caching (min 16MB / database forced)",
		Value: 128,
	}
	TrieCacheGenFlag = cli.IntFlag{
		Name:  "trie-cache-gens",
		Usage: "Number of trie node generations to keep in memory",
		Value: int(state.MaxTrieCacheGen),
	}
	TargetGasLimitFlag = cli.StringFlag{
		Name:  "targetgaslimit",
		Usage: "Target gas limit sets the artificial target gas floor for the blocks to mine",
		Value: params.GenesisGasLimit.String(),
	}
	AutoDAGFlag = cli.BoolFlag{
		Name:  "autodag",
		Usage: "Enable automatic DAG pregeneration",
	}
	EtherbaseFlag = cli.StringFlag{
		Name:  "etherbase",
		Usage: "Public address for block mining rewards (default = first account created)",
		Value: "0",
	}
	ExtraDataFlag = cli.StringFlag{
		Name:  "extradata",
		Usage: "Block extra data set by the miner (default = client version)",
	}
	// Account settings
	UnlockedAccountFlag = cli.StringFlag{
		Name:  "unlock",
		Usage: "Comma separated list of accounts to unlock",
		Value: "",
	}
	PasswordFileFlag = cli.StringFlag{
		Name:  "password",
		Usage: "Password file to use for non-inteactive password input",
		Value: "",
	}

	VMForceJitFlag = cli.BoolFlag{
		Name:  "forcejit",
		Usage: "Force the JIT VM to take precedence",
	}
	VMJitCacheFlag = cli.IntFlag{
		Name:  "jitcache",
		Usage: "Amount of cached JIT VM programs",
		Value: 64,
	}
	VMEnableJitFlag = cli.BoolFlag{
		Name:  "jitvm",
		Usage: "Enable the JIT VM",
	}

	// logging and debug settings
	MetricsEnabledFlag = cli.BoolFlag{
		Name:  metrics.MetricsEnabledFlag,
		Usage: "Enable metrics collection and reporting",
	}
	FakePoWFlag = cli.BoolFlag{
		Name:  "fakepow",
		Usage: "Disables proof-of-work verification",
	}

	// RPC settings
	RPCEnabledFlag = cli.BoolFlag{
		Name:  "rpc",
		Usage: "Enable the HTTP-RPC server",
	}
	RPCListenAddrFlag = cli.StringFlag{
		Name:  "rpcaddr",
		Usage: "HTTP-RPC server listening interface",
		Value: node.DefaultHTTPHost,
	}
	RPCPortFlag = cli.IntFlag{
		Name:  "rpcport",
		Usage: "HTTP-RPC server listening port",
		Value: node.DefaultHTTPPort,
	}
	RPCCORSDomainFlag = cli.StringFlag{
		Name:  "rpccorsdomain",
		Usage: "Comma separated list of domains from which to accept cross origin requests (browser enforced)",
		Value: "",
	}
	RPCApiFlag = cli.StringFlag{
		Name:  "rpcapi",
		Usage: "API's offered over the HTTP-RPC interface",
		Value: rpc.DefaultHTTPApis,
	}
	IPCDisabledFlag = cli.BoolFlag{
		Name:  "ipcdisable",
		Usage: "Disable the IPC-RPC server",
	}
	IPCApiFlag = cli.StringFlag{
		Name:  "ipcapi",
		Usage: "APIs offered over the IPC-RPC interface",
		Value: rpc.DefaultIPCApis,
	}
	IPCPathFlag = DirectoryFlag{
		Name:  "ipcpath",
		Usage: "Filename for IPC socket/pipe within the datadir (explicit paths escape it)",
		Value: DirectoryString{"geth.ipc"},
	}
	WSEnabledFlag = cli.BoolFlag{
		Name:  "ws",
		Usage: "Enable the WS-RPC server",
	}
	WSListenAddrFlag = cli.StringFlag{
		Name:  "wsaddr",
		Usage: "WS-RPC server listening interface",
		Value: node.DefaultWSHost,
	}
	WSPortFlag = cli.IntFlag{
		Name:  "wsport",
		Usage: "WS-RPC server listening port",
		Value: node.DefaultWSPort,
	}
	WSApiFlag = cli.StringFlag{
		Name:  "wsapi",
		Usage: "API's offered over the WS-RPC interface",
		Value: rpc.DefaultHTTPApis,
	}
	WSAllowedOriginsFlag = cli.StringFlag{
		Name:  "wsorigins",
		Usage: "Origins from which to accept websockets requests",
		Value: "",
	}
	ExecFlag = cli.StringFlag{
		Name:  "exec",
		Usage: "Execute JavaScript statement (only in combination with console/attach)",
	}
	PreloadJSFlag = cli.StringFlag{
		Name:  "preload",
		Usage: "Comma separated list of JavaScript files to preload into the console",
	}

	// Network Settings
	MaxPeersFlag = cli.IntFlag{
		Name:  "maxpeers",
		Usage: "Maximum number of network peers (network disabled if set to 0)",
		Value: 25,
	}
	MaxPendingPeersFlag = cli.IntFlag{
		Name:  "maxpendpeers",
		Usage: "Maximum number of pending connection attempts (defaults used if set to 0)",
		Value: 0,
	}
	ListenPortFlag = cli.IntFlag{
		Name:  "port",
		Usage: "Network listening port",
		Value: 30303,
	}
	BootnodesFlag = cli.StringFlag{
		Name:  "bootnodes",
		Usage: "Comma separated enode URLs for P2P discovery bootstrap",
		Value: "",
	}
	NodeKeyFileFlag = cli.StringFlag{
		Name:  "nodekey",
		Usage: "P2P node key file",
	}
	NodeKeyHexFlag = cli.StringFlag{
		Name:  "nodekeyhex",
		Usage: "P2P node key as hex (for testing)",
	}
	NATFlag = cli.StringFlag{
		Name:  "nat",
		Usage: "NAT port mapping mechanism (any|none|upnp|pmp|extip:<IP>)",
		Value: "any",
	}
	NoDiscoverFlag = cli.BoolFlag{
		Name:  "nodiscover",
		Usage: "Disables the peer discovery mechanism (manual peer addition)",
	}
	WhisperEnabledFlag = cli.BoolFlag{
		Name:  "shh",
		Usage: "Enable Whisper",
	}
	// ATM the url is left to the user and deployment to
	JSpathFlag = cli.StringFlag{
		Name:  "jspath",
		Usage: "JavaScript root path for `loadScript` and document root for `admin.httpGet`",
		Value: ".",
	}
	SolcPathFlag = cli.StringFlag{
		Name:  "solc",
		Usage: "Solidity compiler command to be used",
		Value: "solc",
	}
	// Quorum flags
	VoteAccountFlag = cli.StringFlag{
		Name:  "voteaccount",
		Usage: "Address that is used to vote for blocks",
		Value: "",
	}
	VoteAccountPasswordFlag = cli.StringFlag{
		Name:  "votepassword",
		Usage: "Password to unlock the voting address",
		Value: "",
	}
	VoteBlockMakerAccountFlag = cli.StringFlag{
		Name:  "blockmakeraccount",
		Usage: "Address that is used to create blocks",
		Value: "",
	}
	VoteBlockMakerAccountPasswordFlag = cli.StringFlag{
		Name:  "blockmakerpassword",
		Usage: "Password to unlock the block maker address",
		Value: "",
	}
	MinBlockTimeFlag = cli.IntFlag{
		Name:  "minblocktime",
		Usage: "Set min block time",
		Value: 3,
	}
	MaxBlockTimeFlag = cli.IntFlag{
		Name:  "maxblocktime",
		Usage: "Set max block time",
		Value: 10,
	}
	MinVoteTimeFlag = cli.IntFlag{
		Name:  "minvotetime",
		Usage: "Set min vote time",
		Value: 3,
	}
	MaxVoteTimeFlag = cli.IntFlag{
		Name:  "maxvotetime",
		Usage: "Set max vote time",
		Value: 10,
	}
	SingleBlockMakerFlag = cli.BoolFlag{
		Name:  "singleblockmaker",
		Usage: "Indicate this node is the only node that can create blocks",
	}
	EnableNodePermissionFlag = cli.BoolFlag{
		Name:  "permissioned",
		Usage: "If enabled, the node will allow only a defined list of nodes to connect",
	}
	PrivateConfigPathFlag = cli.StringFlag{
		Name:  "privateconfigpath",
		Usage: "Path of thr constellation private config",
		Value: "",
	}
	// Vault flags
	VaultAddrFlag = cli.StringFlag{
		Name:  "vaultaddr",
		Usage: "Web address to a Hashicorp Vault installation holding passwords",
		Value: "",
	}
	VaultPrefixFlag = cli.StringFlag{
		Name:  "vaultprefix",
		Usage: "Prefix where the Vault KV engine is mounted, no outer slashes. Canonically set to `quorum` in Eximchain",
		Value: "quorum",
	}
	VaultPasswordPathFlag = cli.StringFlag{
		Name:  "vaultpasswordpath",
		Usage: "Vault path to where password is kept within KV engine.  No leading slash, does not include the engine's mount prefix",
		Value: "",
	}
	VaultPasswordNameFlag = cli.StringFlag{
		Name:  "vaultpasswordname",
		Usage: "Key name within KV store where password is kept. Canonically set to `geth-pw` in Eximchain",
		Value: "geth_pw",
	}
	// Raft flags
	RaftModeFlag = cli.BoolFlag{
		Name:  "raft",
		Usage: "If enabled, uses Raft instead of Quorum Chain for consensus",
	}
	RaftBlockTimeFlag = cli.IntFlag{
		Name:  "raftblocktime",
		Usage: "Amount of time between raft block creations in milliseconds",
		Value: 50,
	}
	RaftJoinExistingFlag = cli.IntFlag{
		Name:  "raftjoinexisting",
		Usage: "The raft ID to assume when joining an pre-existing cluster",
		Value: 0,
	}
	RaftPortFlag = cli.IntFlag{
		Name:  "raftport",
		Usage: "The port to bind for the raft transport",
		Value: 50400,
	}
)

// MakeDataDir retrieves the currently requested data directory, terminating
// if none (or the empty string) is specified. If the node is starting a testnet,
// the a subdirectory of the specified datadir will be used.
func MakeDataDir(ctx *cli.Context) string {
	if path := ctx.GlobalString(DataDirFlag.Name); path != "" {
		// TODO: choose a different location outside of the regular datadir.
		if ctx.GlobalBool(TestNetFlag.Name) {
			return filepath.Join(path, "testnet")
		}
		return path
	}
	Fatalf("Cannot determine default data directory, please set manually (--datadir)")
	return ""
}

// MakeIPCPath creates an IPC path configuration from the set command line flags,
// returning an empty string if IPC was explicitly disabled, or the set path.
func MakeIPCPath(ctx *cli.Context) string {
	if ctx.GlobalBool(IPCDisabledFlag.Name) {
		return ""
	}
	return ctx.GlobalString(IPCPathFlag.Name)
}

// MakeNodeKey creates a node key from set command line flags, either loading it
// from a file or as a specified hex value. If neither flags were provided, this
// method returns nil and an emphemeral key is to be generated.
func MakeNodeKey(ctx *cli.Context) *ecdsa.PrivateKey {
	var (
		hex  = ctx.GlobalString(NodeKeyHexFlag.Name)
		file = ctx.GlobalString(NodeKeyFileFlag.Name)

		key *ecdsa.PrivateKey
		err error
	)
	switch {
	case file != "" && hex != "":
		Fatalf("Options %q and %q are mutually exclusive", NodeKeyFileFlag.Name, NodeKeyHexFlag.Name)

	case file != "":
		if key, err = crypto.LoadECDSA(file); err != nil {
			Fatalf("Option %q: %v", NodeKeyFileFlag.Name, err)
		}

	case hex != "":
		if key, err = crypto.HexToECDSA(hex); err != nil {
			Fatalf("Option %q: %v", NodeKeyHexFlag.Name, err)
		}
	}
	return key
}

// makeNodeUserIdent creates the user identifier from CLI flags.
func makeNodeUserIdent(ctx *cli.Context) string {
	var comps []string
	if identity := ctx.GlobalString(IdentityFlag.Name); len(identity) > 0 {
		comps = append(comps, identity)
	}
	if ctx.GlobalBool(VMEnableJitFlag.Name) {
		comps = append(comps, "JIT")
	}
	return strings.Join(comps, "/")
}

// MakeBootstrapNodes creates a list of bootstrap nodes from the command line
// flags, reverting to pre-configured ones if none have been specified.
func MakeBootstrapNodes(ctx *cli.Context) []*discover.Node {
	// Return pre-configured nodes if none were manually requested
	if !ctx.GlobalIsSet(BootnodesFlag.Name) {
		if ctx.GlobalBool(TestNetFlag.Name) {
			return TestNetBootNodes
		}
		return FrontierBootNodes
	}
	// Otherwise parse and use the CLI bootstrap nodes
	bootnodes := []*discover.Node{}

	for _, url := range strings.Split(ctx.GlobalString(BootnodesFlag.Name), ",") {
		node, err := discover.ParseNode(url)
		if err != nil {
			glog.V(logger.Error).Infof("Bootstrap URL %s: %v\n", url, err)
			continue
		}
		bootnodes = append(bootnodes, node)
	}
	return bootnodes
}

// MakeListenAddress creates a TCP listening address string from set command
// line flags.
func MakeListenAddress(ctx *cli.Context) string {
	return fmt.Sprintf(":%d", ctx.GlobalInt(ListenPortFlag.Name))
}

// MakeNAT creates a port mapper from set command line flags.
func MakeNAT(ctx *cli.Context) nat.Interface {
	natif, err := nat.Parse(ctx.GlobalString(NATFlag.Name))
	if err != nil {
		Fatalf("Option %s: %v", NATFlag.Name, err)
	}
	return natif
}

// MakeRPCModules splits input separated by a comma and trims excessive white
// space from the substrings.
func MakeRPCModules(input string) []string {
	result := strings.Split(input, ",")
	for i, r := range result {
		result[i] = strings.TrimSpace(r)
	}
	return result
}

// MakeHTTPRpcHost creates the HTTP RPC listener interface string from the set
// command line flags, returning empty if the HTTP endpoint is disabled.
func MakeHTTPRpcHost(ctx *cli.Context) string {
	if !ctx.GlobalBool(RPCEnabledFlag.Name) {
		return ""
	}
	return ctx.GlobalString(RPCListenAddrFlag.Name)
}

// MakeWSRpcHost creates the WebSocket RPC listener interface string from the set
// command line flags, returning empty if the HTTP endpoint is disabled.
func MakeWSRpcHost(ctx *cli.Context) string {
	if !ctx.GlobalBool(WSEnabledFlag.Name) {
		return ""
	}
	return ctx.GlobalString(WSListenAddrFlag.Name)
}

// MakeDatabaseHandles raises out the number of allowed file handles per process
// for Geth and returns half of the allowance to assign to the database.
func MakeDatabaseHandles() int {
	if err := raiseFdLimit(2048); err != nil {
		Fatalf("Failed to raise file descriptor allowance: %v", err)
	}
	limit, err := getFdLimit()
	if err != nil {
		Fatalf("Failed to retrieve file descriptor allowance: %v", err)
	}
	if limit > 2048 { // cap database file descriptors even if more is available
		limit = 2048
	}
	return limit / 2 // Leave half for networking and other stuff
}

// MakeAddress converts an account specified directly as a hex encoded string or
// a key index in the key store to an internal account representation.
func MakeAddress(accman *accounts.Manager, account string) (accounts.Account, error) {
	// If the specified account is a valid address, return it
	if common.IsHexAddress(account) {
		return accounts.Account{Address: common.HexToAddress(account)}, nil
	}
	// Otherwise try to interpret the account as a keystore index
	index, err := strconv.Atoi(account)
	if err != nil {
		return accounts.Account{}, fmt.Errorf("invalid account address or index %q", account)
	}
	return accman.AccountByIndex(index)
}

// MakeEtherbase retrieves the etherbase either from the directly specified
// command line flags or from the keystore if CLI indexed.
func MakeEtherbase(accman *accounts.Manager, ctx *cli.Context) common.Address {
	accounts := accman.Accounts()
	if !ctx.GlobalIsSet(EtherbaseFlag.Name) && len(accounts) == 0 {
		glog.V(logger.Error).Infoln("WARNING: No etherbase set and no accounts found as default")
		return common.Address{}
	}
	etherbase := ctx.GlobalString(EtherbaseFlag.Name)
	if etherbase == "" {
		return common.Address{}
	}
	// If the specified etherbase is a valid address, return it
	account, err := MakeAddress(accman, etherbase)
	if err != nil {
		Fatalf("Option %q: %v", EtherbaseFlag.Name, err)
	}
	return account.Address
}

// MakeMinerExtra resolves extradata for the miner from the set command line flags
// or returns a default one composed on the client, runtime and OS metadata.
func MakeMinerExtra(extra []byte, ctx *cli.Context) []byte {
	if ctx.GlobalIsSet(ExtraDataFlag.Name) {
		return []byte(ctx.GlobalString(ExtraDataFlag.Name))
	}
	return extra
}

// MakePasswordList reads password lines from the file specified by --password.
func MakePasswordList(ctx *cli.Context) []string {
	path := ctx.GlobalString(PasswordFileFlag.Name)
	if path == "" {
		return nil
	}
	text, err := ioutil.ReadFile(path)
	if err != nil {
		Fatalf("Failed to read password file: %v", err)
	}
	lines := strings.Split(string(text), "\n")
	// Sanitise DOS line endings.
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	return lines
}

// MakeNode configures a node with no services from command line flags.
func MakeNode(ctx *cli.Context, name, gitCommit string) *node.Node {
	vsn := Version
	if gitCommit != "" {
		vsn += "-" + gitCommit[:8]
	}

	config := &node.Config{
		DataDir:              MakeDataDir(ctx),
		KeyStoreDir:          ctx.GlobalString(KeyStoreDirFlag.Name),
		UseLightweightKDF:    ctx.GlobalBool(LightKDFFlag.Name),
		PrivateKey:           MakeNodeKey(ctx),
		Name:                 name,
		Version:              vsn,
		UserIdent:            makeNodeUserIdent(ctx),
		NoDiscovery:          ctx.GlobalBool(NoDiscoverFlag.Name),
		BootstrapNodes:       MakeBootstrapNodes(ctx),
		ListenAddr:           MakeListenAddress(ctx),
		NAT:                  MakeNAT(ctx),
		MaxPeers:             ctx.GlobalInt(MaxPeersFlag.Name),
		MaxPendingPeers:      ctx.GlobalInt(MaxPendingPeersFlag.Name),
		IPCPath:              MakeIPCPath(ctx),
		HTTPHost:             MakeHTTPRpcHost(ctx),
		HTTPPort:             ctx.GlobalInt(RPCPortFlag.Name),
		HTTPCors:             ctx.GlobalString(RPCCORSDomainFlag.Name),
		HTTPModules:          MakeRPCModules(ctx.GlobalString(RPCApiFlag.Name)),
		WSHost:               MakeWSRpcHost(ctx),
		WSPort:               ctx.GlobalInt(WSPortFlag.Name),
		WSOrigins:            ctx.GlobalString(WSAllowedOriginsFlag.Name),
		WSModules:            MakeRPCModules(ctx.GlobalString(WSApiFlag.Name)),
		EnableNodePermission: ctx.GlobalBool(EnableNodePermissionFlag.Name),
	}
	if ctx.GlobalBool(DevModeFlag.Name) {
		if !ctx.GlobalIsSet(DataDirFlag.Name) {
			config.DataDir = filepath.Join(os.TempDir(), "/ethereum_dev_mode")
		}
		// --dev mode does not need p2p networking.
		config.MaxPeers = 0
		config.ListenAddr = ":0"
	}
	stack, err := node.New(config)
	if err != nil {
		Fatalf("Failed to create the protocol stack: %v", err)
	}
	return stack
}

// RegisterEthService configures eth.Ethereum from command line flags and adds it to the
// given node.
func RegisterEthService(ctx *cli.Context, stack *node.Node, extra []byte) {
	// Avoid conflicting network flags
	networks, netFlags := 0, []cli.BoolFlag{DevModeFlag, TestNetFlag, OlympicFlag}
	for _, flag := range netFlags {
		if ctx.GlobalBool(flag.Name) {
			networks++
		}
	}
	if networks > 1 {
		Fatalf("The %v flags are mutually exclusive", netFlags)
	}

	// initialise new random number generator
	rand := rand.New(rand.NewSource(time.Now().UnixNano()))
	// get enabled jit flag
	jitEnabled := ctx.GlobalBool(VMEnableJitFlag.Name)
	// if the jit is not enabled enable it for 10 pct of the people
	if !jitEnabled && rand.Float64() < 0.1 {
		jitEnabled = true
		glog.V(logger.Info).Infoln("You're one of the lucky few that will try out the JIT VM (random). If you get a consensus failure please be so kind to report this incident with the block hash that failed. You can switch to the regular VM by setting --jitvm=false")
	}

	chainConfig := MakeChainConfig(ctx, stack)

	ethConf := &eth.Config{
		Etherbase:       MakeEtherbase(stack.AccountManager(), ctx),
		ChainConfig:     MakeChainConfig(ctx, stack),
		AssumeSynced:    ctx.GlobalIsSet(VoteBlockMakerAccountFlag.Name), // assume block maker nodes are always synced until proven otherwise ctx.GlobalBool(SingleBlockMakerFlag.Name),
		DatabaseCache:   ctx.GlobalInt(CacheFlag.Name),
		DatabaseHandles: MakeDatabaseHandles(),
		NetworkId:       ctx.GlobalInt(NetworkIdFlag.Name),
		ExtraData:       MakeMinerExtra(extra, ctx),
		NatSpec:         ctx.GlobalBool(NatspecEnabledFlag.Name),
		DocRoot:         ctx.GlobalString(DocRootFlag.Name),
		EnableJit:       jitEnabled,
		ForceJit:        ctx.GlobalBool(VMForceJitFlag.Name),
		SolcPath:        ctx.GlobalString(SolcPathFlag.Name),
		MinBlockTime:    uint(ctx.GlobalInt(MinBlockTimeFlag.Name)),
		MaxBlockTime:    uint(ctx.GlobalInt(MaxBlockTimeFlag.Name)),
		MinVoteTime:     uint(ctx.GlobalInt(MinVoteTimeFlag.Name)),
		MaxVoteTime:     uint(ctx.GlobalInt(MaxVoteTimeFlag.Name)),
		RaftMode:        ctx.GlobalBool(RaftModeFlag.Name),
	}

	// Override any default configs in dev mode or the test net
	switch {
	case ctx.GlobalBool(OlympicFlag.Name):
		if !ctx.GlobalIsSet(NetworkIdFlag.Name) {
			ethConf.NetworkId = 1
		}
		ethConf.Genesis = core.OlympicGenesisBlock()

	case ctx.GlobalBool(TestNetFlag.Name):
		if !ctx.GlobalIsSet(NetworkIdFlag.Name) {
			ethConf.NetworkId = 2
		}
		ethConf.Genesis = core.TestNetGenesisBlock()
		state.StartingNonce = 1048576 // (2**20)

	case ctx.GlobalBool(DevModeFlag.Name):
		ethConf.Genesis = core.OlympicGenesisBlock()
		ethConf.PowTest = true
	}
	// Override any global options pertaining to the Ethereum protocol
	if gen := ctx.GlobalInt(TrieCacheGenFlag.Name); gen > 0 {
		state.MaxTrieCacheGen = uint16(gen)
	}

	// We need a pointer to the ethereum service so we can access it from the raft
	// service
	var ethereum *eth.Ethereum

	if err := stack.Register(func(ctx *node.ServiceContext) (node.Service, error) {
		var err error
		ethereum, err = eth.New(ctx, ethConf)
		return ethereum, err
	}); err != nil {
		Fatalf("Failed to register the Ethereum service: %v", err)
	}

	if ctx.GlobalBool(RaftModeFlag.Name) {
		blockTimeMillis := ctx.GlobalInt(RaftBlockTimeFlag.Name)
		datadir := ctx.GlobalString(DataDirFlag.Name)
		joinExistingId := ctx.GlobalInt(RaftJoinExistingFlag.Name)
		raftPort := uint16(ctx.GlobalInt(RaftPortFlag.Name))

		logger.DoLogRaft = true

		if err := stack.Register(func(ctx *node.ServiceContext) (node.Service, error) {
			strId := discover.PubkeyID(stack.PublicKey()).String()
			blockTimeNanos := time.Duration(blockTimeMillis) * time.Millisecond
			peers := stack.StaticNodes()

			var myId uint16
			var joinExisting bool

			if joinExistingId > 0 {
				myId = uint16(joinExistingId)
				joinExisting = true
			} else if len(peers) == 0 {
				Fatalf("Raft-based consensus requires either (1) an initial peers list (in static-nodes.json) including this enode hash (%v), or (2) the flag --raftjoinexisting RAFT_ID, where RAFT_ID has been issued by an existing cluster member calling `raft.addPeer(ENODE_ID)` with an enode ID containing this node's enode hash.", strId)
			} else {
				peerIds := make([]string, len(peers))

				for peerIdx, peer := range peers {
					if !peer.HasRaftPort() {
						Fatalf("raftport querystring parameter not specified in static-node enode ID: %v. please check your static-nodes.json file.", peer.String())
					}

					peerId := peer.ID.String()
					peerIds[peerIdx] = peerId
					if peerId == strId {
						myId = uint16(peerIdx) + 1
					}
				}

				if myId == 0 {
					Fatalf("failed to find local enode ID (%v) amongst peer IDs: %v", strId, peerIds)
				}
			}

			return raft.New(ctx, chainConfig, myId, raftPort, joinExisting, blockTimeNanos, ethereum, peers, datadir)
		}); err != nil {
			Fatalf("Failed to register the Raft service: %v", err)
		}
	}
}

// RegisterShhService configures whisper and adds it to the given node.
func RegisterShhService(stack *node.Node) {
	if err := stack.Register(func(*node.ServiceContext) (node.Service, error) { return whisper.New(), nil }); err != nil {
		Fatalf("Failed to register the Whisper service: %v", err)
	}
}

// SetupNetwork configures the system for either the main net or some test network.
func SetupNetwork(ctx *cli.Context) {
	switch {
	case ctx.GlobalBool(OlympicFlag.Name):
		params.DurationLimit = big.NewInt(8)
		params.GenesisGasLimit = big.NewInt(3141592)
		params.MinGasLimit = big.NewInt(125000)
		params.MaximumExtraDataSize = big.NewInt(1024)
		NetworkIdFlag.Value = 0
		core.BlockReward = big.NewInt(1.5e+18)
		core.ExpDiffPeriod = big.NewInt(math.MaxInt64)
	}
	params.TargetGasLimit = common.String2Big(ctx.GlobalString(TargetGasLimitFlag.Name))
}

// MakeChainConfig reads the chain configuration from the database in ctx.Datadir.
func MakeChainConfig(ctx *cli.Context, stack *node.Node) *core.ChainConfig {
	db := MakeChainDatabase(ctx, stack)
	defer db.Close()

	return MakeChainConfigFromDb(ctx, db)
}

// MakeChainConfigFromDb reads the chain configuration from the given database.
func MakeChainConfigFromDb(ctx *cli.Context, db ethdb.Database) *core.ChainConfig {
	// If the chain is already initialized, use any existing chain configs
	config := new(core.ChainConfig)

	genesis := core.GetBlock(db, core.GetCanonicalHash(db, 0), 0)
	if genesis != nil {
		storedConfig, err := core.GetChainConfig(db, genesis.Hash())
		switch err {
		case nil:
			config = storedConfig
		case core.ChainConfigNotFoundErr:
			// No configs found, use empty, will populate below
		default:
			Fatalf("Could not make chain configuration: %v", err)
		}
	}
	// Check whether we are allowed to set default config params or not:
	//  - If no genesis is set, we're running either mainnet or testnet (private nets use `geth init`)
	//  - If a genesis is already set, ensure we have a configuration for it (mainnet or testnet)
	defaults := genesis == nil ||
		(genesis.Hash() == params.MainNetGenesisHash && !ctx.GlobalBool(TestNetFlag.Name)) ||
		(genesis.Hash() == params.TestNetGenesisHash && ctx.GlobalBool(TestNetFlag.Name))

	// Set any missing chainConfig fields due to them being unset or system upgrade
	if defaults {
		if config.HomesteadBlock == nil {
			if ctx.GlobalBool(TestNetFlag.Name) {
				config.HomesteadBlock = params.TestNetHomesteadBlock
			} else {
				config.HomesteadBlock = params.MainNetHomesteadBlock
			}
		}
	}
	return config
}

// MakeChainDatabase open an LevelDB using the flags passed to the client and will hard crash if it fails.
func MakeChainDatabase(ctx *cli.Context, stack *node.Node) ethdb.Database {
	var (
		cache   = ctx.GlobalInt(CacheFlag.Name)
		handles = MakeDatabaseHandles()
	)

	chainDb, err := stack.OpenDatabase("chaindata", cache, handles)
	if err != nil {
		Fatalf("Could not open database: %v", err)
	}
	return chainDb
}

// MakeChain creates a chain manager from set command line flags.
func MakeChain(ctx *cli.Context, stack *node.Node) (chain *core.BlockChain, chainDb ethdb.Database) {
	var err error
	chainDb = MakeChainDatabase(ctx, stack)

	if ctx.GlobalBool(OlympicFlag.Name) {
		_, err := core.WriteTestNetGenesisBlock(chainDb)
		if err != nil {
			glog.Fatalln(err)
		}
	}
	chainConfig := MakeChainConfigFromDb(ctx, chainDb)

	pow := pow.PoW(core.FakePow{})
	if !ctx.GlobalBool(FakePoWFlag.Name) {
		pow = ethash.New()
	}
	chain, err = core.NewBlockChain(chainDb, chainConfig, pow, new(event.TypeMux), true)
	if err != nil {
		Fatalf("Could not start chainmanager: %v", err)
	}
	return chain, chainDb
}

// MakeConsolePreloads retrieves the absolute paths for the console JavaScript
// scripts to preload before starting.
func MakeConsolePreloads(ctx *cli.Context) []string {
	// Skip preloading if there's nothing to preload
	if ctx.GlobalString(PreloadJSFlag.Name) == "" {
		return nil
	}
	// Otherwise resolve absolute paths and return them
	preloads := []string{}

	assets := ctx.GlobalString(JSpathFlag.Name)
	for _, file := range strings.Split(ctx.GlobalString(PreloadJSFlag.Name), ",") {
		preloads = append(preloads, common.AbsolutePath(assets, strings.TrimSpace(file)))
	}
	return preloads
}
