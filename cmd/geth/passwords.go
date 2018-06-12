package main

import (
	cli "gopkg.in/urfave/cli.v1"
	"github.com/ethereum/go-ethereum/cmd/utils"
)

func fetchPassword(ctx *cli.Context) (string, error){
	if (usingVaultPassword(ctx)){
		return fetchPasswordFromVault(ctx)
	} else {
		return fetchPasswordFromCLI(ctx)
	}
}

func fetchPasswordFromCLI(ctx *cli.Context) (string, error) {
	accountPass := ctx.GlobalString(utils.VoteAccountPasswordFlag.Name)
	blockPass := ctx.GlobalString(utils.VoteBlockMakerAccountPasswordFlag.Name)
	filePass := ctx.GlobalString(utils.PasswordFileFlag.Name)
	if (accountPass != ""){
		return accountPass, nil
	} else if (blockPass != "") {
		return blockPass, nil
	} else if (filePass !== "") {
		return filePass, nil
	} else {
		utils.Fatalf("Looked for password via fetchPasswordFromCLI, but no password arguments found.")
	}
}

func fetchPasswordFromVault(ctx *cli.Context) (string, error) {
	if (usingVaultPassword(ctx)){
		// Fetch our args, make our vars
		var password []string
		vaultAddr := ctx.GlobalString(utils.VaultAddrFlag.Name)
		vaultPassPath := ctx.GlobalString(utils.VaultPasswordPathFlag.Name)
		vaultPassKey := ctx.GlobalString(utils.VaultPasswordKeyFlag.Name)

		// Authenticate to Vault via the AWS method

		// Perform the query to retrieve the password value

		// Extract from response & return to caller
	} else {
		utils.Fatalf("fetchPasswordFromVault called even though CLI got a password argument.")
	}
}

func usingVaultPassword(ctx *cli.Context) bool {
	var passwordFlags map[cli.StringFlag]string{
		utils.VoteAccountPasswordFlag: ctx.GlobalString(utils.VoteAccountPasswordFlag.Name)
		utils.VoteBlockMakerAccountPasswordFlag: ctx.GlobalString(utils.VoteBlockMakerAccountPasswordFlag.Name)
		utils.PasswordFileFlag: ctx.GlobalString(utils.PasswordFileFlag.Name)
	}
	var setPassFlags []string = make([]string)
	for flag, val := range passwordFlags {
		if (val != "") {
			setPassFlags = append(setPassFlags, flag.Name)
		}
	}
	if (len(setPassFlags) > 0){
		if (len(setPassFlags) == 1){ return false } else {
			utils.Fatalf("Too many password flags have been set.  Only one of the following should be supplied: %v",setPassFlags)
		}
	} else {
		var vaultFlags map[cli.StringFlag]string{
			utils.VaultAddrFlag: ctx.GlobalString(utils.VaultAddrFlag.Name),
			utils.VaultPasswordKeyFlag: ctx.GlobalString(utils.VaultPasswordKeyFlag.Name),
			utils.VaultPasswordPathFlag: ctx.GlobalString(utils.VaultPasswordPathFlag.Name),
		}
		var missingFlags []string = make([]string)
		for flag, val := range vaultFlags {
			if (val == "") { missingFlags = append(missingFlags, flag.Name) }
		}
		if (len(missingFlags) > 0) {
			utils.Fatalf("No account password specified, but missing required for retrieving password from Vault.  Please supply: %v",missingFlags)
		}
		return true
	}
}