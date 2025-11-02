package main

import (
	"encoding/json"
	"fmt"
	"github.com/viswesh-swaminathan/ashvattha/internal/llm"
	"github.com/viswesh-swaminathan/ashvattha/internal/marketdata"
	"log"
	"os"

	ccxt "github.com/ccxt/ccxt/go/v4"
	"github.com/viswesh-swaminathan/ashvattha/internal/account"
)

func main() {
	exchange := ccxt.NewHyperliquid(getUserConfig())

	// Get account information
	accountClient := account.NewClient(exchange)
	accountInfo, err := accountClient.GetAccountInfo()
	if err != nil {
		log.Fatalf("Error fetching account info: %v", err)
	}
	formattedAccountInfo := accountInfo.Format()
	log.Printf("Fetched account info:\n%s", formattedAccountInfo)

	// Define the symbol to analyze (use the perpetual/swap contract format)
	// For Hyperliquid perpetuals, use format: SYMBOL/USDC:USDC
	symbols := []string{
		"BTC/USDC:USDC",
		"DOGE/USDC:USDC",
		"ETH/USDC:USDC",
		"XRP/USDC:USDC",
		"BNB/USDC:USDC",
		"SOL/USDC:USDC",
	}
	marketdataForAssets := make([]*marketdata.MarketData, len(symbols))
	for i, symbol := range symbols {
		marketdataClient := marketdata.NewClient(exchange)
		marketData, err := marketdataClient.GetMarketData(symbol)
		if err != nil {
			log.Fatalf("Error fetching market data for %s: %v", symbol, err)
		}
		log.Printf("Fetched market data for %s", symbol)
		log.Printf("market data summary:\n%s", marketData.Format())
		marketdataForAssets[i] = marketData
	}

	// Create LLM prompt with formatted account info and market data
	deepSeekApiKey := getStringEnv("DEEPSEEK_API_KEY")
	prompter := llm.NewPrompt(deepSeekApiKey, formattedAccountInfo, marketdataForAssets)

	// Get trading recommendations from DeepSeek
	fmt.Println("Querying DeepSeek for BTC trading analysis...")
	response, err := prompter.Prompt("What is your analysis and recommendations for my portfolio - provide recommendations for all 6 assets?")
	if err != nil {
		log.Fatalf("Error getting DeepSeek response: %v", err)
	}

	// Print the response in formatted JSON
	jsonOutput, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		log.Fatalf("Error formatting response: %v", err)
	}
	fmt.Println("\n=== DeepSeek Analysis ===")
	fmt.Println(string(jsonOutput))
	fmt.Printf("\nTokens used: %d\n", response.TokensUsed)
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
