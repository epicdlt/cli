package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/epicdlt/go-client"
	"github.com/epicdlt/go-client/models"
	"github.com/treeder/gotils/v2"
	"github.com/urfave/cli"
	_ "golang.org/x/lint"
)

// Flags
var (
	verbose bool
	format  string
)

const (
	asciiLogo = `
	___________      .__        
	\_   _____/_____ |__| ____  
	 |    __)_\____ \|  |/ ___\ 
	 |        \  |_> >  \  \___ 
	/_______  /   __/|__|\___  >
			\/|__|           \/ 
	`

	pkVarName      = "EPIC_PK"
	addrVarName    = "EPIC_ADDRESS"
	networkVarName = "EPIC_NETWORK"
	rpcURLVarName  = "EPIC_RPC_URL"
	envVarName     = "EPIC_ENV"
)

func main() {
	ctx := context.Background()

	var err error

	// Flags
	var netName, rpcUrl, privateKey, txInputFormat string
	var testnet bool

	app := cli.NewApp()
	app.Name = "epic"
	app.Version = Version
	app.Usage = "Epic cli tool"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "network, n",
			Usage:       `The name of the network.`,
			Destination: &netName,
			EnvVar:      networkVarName,
			Hidden:      false},
		cli.BoolFlag{
			Name:        "testnet",
			Usage:       "Shorthand for '-network testnet'.",
			Destination: &testnet,
			Hidden:      false},
		cli.StringFlag{
			Name:        "rpc-url",
			Usage:       "The network RPC URL",
			Destination: &rpcUrl,
			EnvVar:      rpcURLVarName,
			Value:       "http://localhost:8080",
			Hidden:      false},
		cli.BoolFlag{
			Name:        "verbose",
			Usage:       "Enable verbose logging",
			Destination: &verbose,
			Hidden:      false},
		cli.StringFlag{
			Name:        "format, f",
			Usage:       "Output format. Options: json. Default: human readable output.",
			Destination: &format,
			Hidden:      false},
	}
	app.Before = func(*cli.Context) error {
		// use verbose flag here? either have to create new logger, or use this https://godoc.org/go.uber.org/zap#AtomicLevel
		return nil
	}
	app.Commands = []cli.Command{
	// 	{
	// 		Name:  "start",
	// 		Usage: "Start a local Epic server",
	// 		Flags: []cli.Flag{
	// 			cli.BoolTFlag{
	// 				Name:  "detach, d",
	// 				Usage: "Run container in background.",
	// 			},
	// 			cli.StringFlag{
	// 				Name:  "env-file",
	// 				Usage: "Path to custom configuration file.",
	// 			},
	// 			cli.StringFlag{
	// 				Name:   "private-key, pk",
	// 				Usage:  "Private key",
	// 				EnvVar: pkVarName,
	// 			},
	// 			cli.Int64Flag{
	// 				Name:  "genesis",
	// 				Usage: "For a brand new network, use genesis flag to mint initial coins. All transactions must branch from here.",
	// 			},
	// 		},
	// 		Action: func(c *cli.Context) error {
	// 			fmt.Println(asciiLogo)
	// 			genesisAmount := c.Int64("genesis")
	// 			serverPk := c.String("pk")
	// 			bs, err := epic.NewServer(ctx, &epic.ServerOptions{
	// 				GenesisAmount: genesisAmount,
	// 				SigningKey:    serverPk,
	// 			})
	// 			if err != nil {
	// 				return err
	// 			}
	// 			return bs.Start()
	// 		},
	// 	},
		{
			Name:    "transaction",
			Aliases: []string{"tx"},
			Usage:   "Transaction details for a tx ID",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "input",
					Usage:       "Transaction input data format: len/hex/utf8",
					Destination: &txInputFormat,
					Value:       "len",
				},
			},
			Action: func(c *cli.Context) {
				GetTransactionDetails(ctx, rpcUrl, c.Args().First(), txInputFormat)
			},
		},

		{
			Name:    "address",
			Aliases: []string{"addr"},
			Usage:   "Account details for a specific address, or the one corresponding to the private key.",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "private-key, pk",
					Usage:       "The private key",
					EnvVar:      pkVarName,
					Destination: &privateKey,
					Hidden:      false},
			},
			Action: func(c *cli.Context) {
				var err error
				address := c.Args().First()
				if address == "" && privateKey != "" {
					address, err = client.PublicKeyFromPrivate(ctx, privateKey)
					if err != nil {
						fatalExit(err)
					}
				}
				GetAddressDetails(ctx, rpcUrl, address)
			},
		},
		{
			Name:    "account",
			Aliases: []string{"a"},
			Usage:   "Account operations",
			Subcommands: []cli.Command{
				{
					Name:  "create",
					Usage: "Create a new account",
					Action: func(c *cli.Context) {
						pubKey, pk, err := client.GenerateKeyEncoded()
						if err != nil {
							fatalExit(err)
						}
						fmt.Printf("Private key: %v\n", pk)
						fmt.Printf("Public address: %v\n", pubKey)
					},
				},
			},
		},
		{
			Name:    "send",
			Usage:   fmt.Sprintf("Transfer token to an account (epic transfer X to ADDRESS)"),
			Aliases: []string{"transfer"},
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:   "private-key, pk",
					Usage:  "Private key",
					EnvVar: pkVarName,
					Hidden: false,
				},
				cli.StringFlag{
					Name:   "to",
					EnvVar: addrVarName,
					Usage:  "The recepient address",
					Hidden: false},
			},
			Action: func(c *cli.Context) {
				// 0 is amount
				// 2 is address
				if c.String("private-key") == "" {
					fatalExit(errors.New("private key required"))
				}
				amountS := c.Args().Get(0)
				amount, err := decimal.NewFromString(amountS)
				if err != nil {
					fatalExit(err)
				}
				to := c.Args().Get(2)
				tx, err := client.SubmitTx(ctx, rpcUrl, c.String("private-key"), amount, to, "", nil)
				if err != nil {
					fatalExit(err)
				}
				bs, err := json.MarshalIndent(tx, "", "    ")
				if err != nil {
					fatalExit(err)
				}
				fmt.Println(string(bs))
				// fmt.Printf("%v transferred to %v - ID: %v\n", amount, to, tx.ID)
			},
		},
		{
			Name:  "deploy",
			Usage: fmt.Sprintf("Deploy code/contract, eg: epic deploy treeder/example@sha256:123"), // todo: put in a real example here
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:   "private-key, pk",
					Usage:  "Private key",
					EnvVar: pkVarName,
					Hidden: false,
				},
				cli.StringFlag{
					Name:   "to",
					EnvVar: addrVarName,
					Usage:  "The recepient address",
					Hidden: false},
			},
			Action: func(c *cli.Context) {
				// 0 is amount
				// 2 is address
				if c.String("private-key") == "" {
					fatalExit(errors.New("private key required"))
				}
				codeRef := c.Args().Get(0) // this is a docker image name only for now
				_, err := client.SubmitTx(ctx, rpcUrl, c.String("private-key"), decimal.Zero, "", codeRef, nil)
				if err != nil {
					fatalExit(err)
				}
			},
		},
		{
			Name:  "run",
			Usage: fmt.Sprintf("Run a smart contract, eg: epic run CONTRACT_ADDRESS"), // todo: maybe a better name for this command?
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:   "private-key, pk",
					Usage:  "Private key",
					EnvVar: pkVarName,
					Hidden: false,
				},
				cli.StringFlag{
					Name:   "to",
					EnvVar: addrVarName,
					Usage:  "The recepient address",
					Hidden: false},
			},
			Action: func(c *cli.Context) {
				// 0 is amount
				// 2 is address
				if c.String("private-key") == "" {
					fatalExit(errors.New("private key required"))
				}
				_, err := client.SubmitTx(ctx, rpcUrl, c.String("private-key"), decimal.Zero, c.Args().Get(0), "", c.Args().Tail())
				if err != nil {
					fatalExit(err)
				}
			},
		},
		{
			Name:  "cstate",
			Usage: fmt.Sprintf("Get smart contract state data, eg: epic get CONTRACT_ADDRESS /"), // todo: maybe a better name for this command?
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:   "private-key, pk",
					Usage:  "Private key",
					EnvVar: pkVarName,
					Hidden: false,
				},
				cli.StringFlag{
					Name:   "to",
					EnvVar: addrVarName,
					Usage:  "The recepient address",
					Hidden: false},
				cli.BoolFlag{
					Name:   "pretty",
					Hidden: false},
			},
			Action: func(c *cli.Context) {
				GetCodeState(ctx, rpcUrl, c.Args().Get(0), c.Args().Get(1), c.Bool("pretty"))
			},
		},
		{
			Name:    "object",
			Aliases: []string{"state"},
			Usage:   fmt.Sprintf("Get object by hash"), // todo: maybe a better name for this command?
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:   "pretty",
					Hidden: false},
			},
			Action: func(c *cli.Context) {
				b, err := client.GetObjectBytesByHash(ctx, rpcUrl, c.Args().Get(0))
				if err != nil {
					fatalExit(err)
				}
				fmt.Println(string(b))
			},
		},
		{
			Name:  "env",
			Usage: "List environment variables",
			Action: func(c *cli.Context) {
				varNames := []string{addrVarName, pkVarName, networkVarName, rpcURLVarName}
				sort.Strings(varNames)
				for _, name := range varNames {
					fmt.Printf("%s=%s\n", name, os.Getenv(name))
				}
			},
		},
	}
	err = app.Run(os.Args)
	if err != nil {
		fmt.Println("ERROR on app.Run:", err)
	}
}

func GetTransactionDetails(ctx context.Context, rpcURL, txHash, inputFormat string) {
	if txHash == "" {
		fatalExit(errors.New("must provide tx ID"))
	}

	url := rpcURL + "/tx/" + txHash
	// fmt.Println("url:", url)
	txR := &models.TransactionResponse{}
	err := gotils.GetJSON(url, txR)
	if err != nil {
		fatalExit(err)
	}
	// fmt.Printf("%+v\n", txR)
	bs, err := json.MarshalIndent(txR, "", "    ")
	if err != nil {
		fatalExit(err)
	}
	fmt.Println(string(bs))

	// tx := txR.Tx
	// fmt.Println("ID:", tx.ID)
	// fmt.Println("From:", tx.From)
	// fmt.Println("To:", tx.To)
	// fmt.Println("Amount:", tx.Amount)
	// fmt.Println("Nonce:", tx.Nonce)
	// fmt.Println("State ID:", tx.StateID)
	// fmt.Println("Related Tx ID:", tx.RelatedTxID)
	// fmt.Println("Created:", tx.CreatedAt)
	// fmt.Println("Previous Tx ID:", tx.PreviousID)
}
func GetAddressDetails(ctx context.Context, rpcURL, addrHash string) {
	if addrHash == "" {
		fatalExit(errors.New("must provide address"))
	}

	stateM, err := client.GetState(ctx, rpcURL, addrHash)
	if err != nil {
		fatalExit(err)
	}
	bs, err := json.MarshalIndent(stateM, "", "    ")
	if err != nil {
		fatalExit(err)
	}
	fmt.Println(string(bs))

	// state := stateM.State
	// fmt.Println("Balance:", state.Balance)
	// fmt.Println("Nonce:", state.Nonce)
	// fmt.Println("Last Tx ID:", stateM.LastTxID)
	// if state.Code != "" {
	// 	fmt.Println("This is a smart contract, code:", state.Code)
	// 	fmt.Println("Code state ID:", state.CodeStateID)
	// }
}

func GetCodeState(ctx context.Context, rpcURL, key, path string, pretty bool) {
	if key == "" {
		fatalExit(errors.New("must provide address"))
	}
	if path != "" {
		if !strings.HasPrefix(path, "/") {
			fatalExit(errors.New("path must start with /"))
		}
	}

	url := rpcURL + "/addr/" + key + "/state" + path
	fmt.Println("url:", url)

	resp := &client.CodeStateResponse{}
	err := gotils.GetJSON(url, resp)
	if err != nil {
		fatalExit(err)
	}
	// if pretty {
	// 	b, err = json.MarshalIndent(group, "", "  ")
	// }
	jsonValue, err := json.Marshal(resp.State)
	if err != nil {
		fatalExit(err)
	}
	fmt.Println(string(jsonValue))
}

func writeStringToFile(s, fileName string) {
	err := ioutil.WriteFile(fileName+".sol", []byte(s), 0666)
	if err != nil {
		fatalExit(fmt.Errorf("Cannot create the file: %v", err))
	}
	fmt.Println("The sample contract has been successfully written to", fileName+".sol", "file")
}

func marshalJSON(data interface{}) string {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fatalExit(fmt.Errorf("Cannot marshal json: %v", err))
	}
	return string(b)
}

func fatalExit(err error) {
	fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
	os.Exit(1)
}
