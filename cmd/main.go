package main

import (
	"fmt"
	"log"
	"os"

	"github.com/viswesh-swaminathan/ashvattha/internal/account"
	"github.com/viswesh-swaminathan/ashvattha/internal/marketdata"

	ccxt "github.com/ccxt/ccxt/go/v4"
)

func main() {
	exchange := ccxt.NewHyperliquid(getUserConfig())

	// Define the symbol to analyze (use the perpetual/swap contract format)
	// For Hyperliquid perpetuals, use format: SYMBOL/USDC:USDC
	symbol := "BTC/USDC:USDC"

	// Get market data
	marketdataClient := marketdata.NewClient(exchange)
	data, err := marketdataClient.GetMarketData(symbol)
	if err != nil {
		log.Fatalf("Error fetching market data: %v", err)
	}
	fmt.Printf("%s", data.Format())

	// Get account information
	accountClient := account.NewClient(exchange)
	accountInfo, err := accountClient.GetAccountInfo()
	if err != nil {
		log.Fatalf("Error fetching account info: %v", err)
	}
	fmt.Printf("%s", accountInfo.Format())
}

func getUserConfig() map[string]interface{} {
	return map[string]interface{}{
		"apiKey":        getStringEnv("API_KEY"),
		"secret":        getStringEnv("SECRET"),
		"walletAddress": getStringEnv("WALLET_ADDRESS"), // Hyperliquid requires wallet address
		"privateKey":    getStringEnv("PRIVATE_KEY"),    // For signing transactions
	}
}

func getStringEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("Environment variable %s is not set", key)
	}
	return value
}
