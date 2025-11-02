package main

import (
	"fmt"
	"github.com/viswesh-swaminathan/ashvattha/internal/marketdata"
	"log"
	"math"
	"os"

	ccxt "github.com/ccxt/ccxt/go/v4"
)

// ExitPlan contains the profit target, stop loss, and invalidation condition
type ExitPlan struct {
	ProfitTarget          float64 `json:"profit_target"`
	StopLoss              float64 `json:"stop_loss"`
	InvalidationCondition string  `json:"invalidation_condition"`
}

// Position represents a trading position with all relevant details
type Position struct {
	Symbol           string   `json:"symbol"`
	Quantity         float64  `json:"quantity"`
	EntryPrice       float64  `json:"entry_price"`
	CurrentPrice     float64  `json:"current_price"`
	LiquidationPrice float64  `json:"liquidation_price"`
	UnrealizedPnl    float64  `json:"unrealized_pnl"`
	Leverage         float64  `json:"leverage"`
	ExitPlan         ExitPlan `json:"exit_plan"`
	Confidence       float64  `json:"confidence"`
	RiskUsd          float64  `json:"risk_usd"`
	SlOid            int64    `json:"sl_oid"`
	TpOid            int64    `json:"tp_oid"`
	WaitForFill      bool     `json:"wait_for_fill"`
	EntryOid         int64    `json:"entry_oid"`
	NotionalUsd      float64  `json:"notional_usd"`
}

// AccountInfo holds all account-related information
type AccountInfo struct {
	TotalReturnPercent  float64
	AvailableCash       float64
	CurrentAccountValue float64
	Positions           []Position
	SharpeRatio         float64
}

// Helper function to safely get the last N values from a float64 slice
func getLastN(slice []float64, n int) []float64 {
	if len(slice) == 0 {
		return []float64{}
	}
	if n > len(slice) {
		n = len(slice)
	}
	return slice[len(slice)-n:]
}

// Helper function to format a float64 slice for printing
func formatFloatSlice(slice []float64) string {
	if len(slice) == 0 {
		return "[]"
	}
	result := "["
	for i, val := range slice {
		if i > 0 {
			result += ", "
		}
		result += fmt.Sprintf("%.3f", val)
	}
	result += "]"
	return result
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

// displayAccountInfo fetches and displays account information and performance
func displayAccountInfo(exchange *ccxt.Hyperliquid) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Error displaying account info: %v", r)
		}
	}()

	// Fetch account balance
	balance, err := exchange.FetchBalance()
	if err != nil {
		log.Printf("Error fetching balance: %v", err)
		return
	}

	// Get available cash (USDC balance)
	var availableCash float64
	if balance.Free != nil {
		if usdcFree, ok := balance.Free["USDC"]; ok && usdcFree != nil {
			availableCash = *usdcFree
		}
	}

	// Fetch open positions
	positions, err := exchange.FetchPositions()
	if err != nil {
		log.Printf("Error fetching positions: %v", err)
		return
	}

	// Convert CCXT positions to our Position struct and calculate totals
	var customPositions []Position
	var totalNotional float64

	for _, pos := range positions {
		// Extract position details (dereferencing pointers)
		var symbol string
		var quantity, entryPrice, currentPrice, liquidationPrice, unrealizedPnl, leverage, notional float64

		if pos.Symbol != nil {
			symbol = *pos.Symbol
		}
		if pos.Contracts != nil {
			quantity = *pos.Contracts
		}
		if pos.EntryPrice != nil {
			entryPrice = *pos.EntryPrice
		}

		// Try multiple sources for current price
		hasMarkPrice := false
		if pos.MarkPrice != nil {
			currentPrice = *pos.MarkPrice
			// Check if it's a valid number (not NaN and not zero)
			if !math.IsNaN(currentPrice) && currentPrice != 0 {
				hasMarkPrice = true
			} else {
				currentPrice = 0 // Reset if it was NaN
			}
		}

		// If MarkPrice is not available, extract from Hyperliquid's Info structure
		if !hasMarkPrice && pos.Info != nil {
			// Hyperliquid stores current price info in a nested structure
			// We can calculate it from entryPx, szi (size), and positionValue
			if positionData, ok := pos.Info["position"].(map[string]interface{}); ok {
				if positionValue, ok := positionData["positionValue"].(float64); ok {
					if szi, ok := positionData["szi"].(float64); ok && szi != 0 {
						// Current price = positionValue / abs(szi)
						absSzi := szi
						if szi < 0 {
							absSzi = -szi
						}
						currentPrice = positionValue / absSzi
					}
				}
			}
		}

		// If still no price, try fetching ticker as last resort
		if currentPrice == 0 {
			ticker, err := exchange.FetchTicker(symbol)
			if err == nil && ticker.Last != nil {
				currentPrice = *ticker.Last
			}
		}

		if pos.LiquidationPrice != nil {
			liquidationPrice = *pos.LiquidationPrice
		} else if pos.Info != nil {
			// Try to get liquidation price from Info
			if positionData, ok := pos.Info["position"].(map[string]interface{}); ok {
				if liqPx, ok := positionData["liquidationPx"].(float64); ok {
					liquidationPrice = liqPx
				}
			}
		}
		if pos.UnrealizedPnl != nil {
			unrealizedPnl = *pos.UnrealizedPnl
		}
		if pos.Leverage != nil {
			leverage = *pos.Leverage
		}
		if pos.Notional != nil {
			notional = *pos.Notional
		}

		// Try to extract additional fields from Info if available
		var slOid, tpOid, entryOid int64 = -1, -1, -1
		var profitTarget, stopLoss float64
		var invalidationCondition string
		var confidence, riskUsd float64

		// Check if Info contains additional data (Hyperliquid-specific)
		if pos.Info != nil {
			// Uncomment below to debug what's in the Info structure:
			// if len(customPositions) == 0 {
			// 	fmt.Printf("\nDEBUG - Position Info for %s:\n", symbol)
			// 	for k, v := range pos.Info {
			// 		fmt.Printf("  %s: %v (type: %T)\n", k, v, v)
			// 	}
			// 	fmt.Println()
			// }

			// Try to extract order IDs if present
			if slOidVal, ok := pos.Info["sl_oid"].(float64); ok {
				slOid = int64(slOidVal)
			}
			if tpOidVal, ok := pos.Info["tp_oid"].(float64); ok {
				tpOid = int64(tpOidVal)
			}
			if entryOidVal, ok := pos.Info["entry_oid"].(float64); ok {
				entryOid = int64(entryOidVal)
			}
			if confidenceVal, ok := pos.Info["confidence"].(float64); ok {
				confidence = confidenceVal
			}
			if riskVal, ok := pos.Info["risk_usd"].(float64); ok {
				riskUsd = riskVal
			}

			// Try to extract exit plan if present
			if exitPlan, ok := pos.Info["exit_plan"].(map[string]interface{}); ok {
				if pt, ok := exitPlan["profit_target"].(float64); ok {
					profitTarget = pt
				}
				if sl, ok := exitPlan["stop_loss"].(float64); ok {
					stopLoss = sl
				}
				if ic, ok := exitPlan["invalidation_condition"].(string); ok {
					invalidationCondition = ic
				}
			}
		}

		// Try to fetch open orders to get stop loss and take profit prices
		openOrders, err := exchange.FetchOpenOrders(ccxt.WithFetchOpenOrdersSymbol(symbol))
		if err == nil {
			for _, order := range openOrders {
				// Check if this is a trigger order (stop loss or take profit)
				isTriggerOrder := false
				isStopLoss := false
				isTakeProfit := false
				var triggerPrice float64
				var orderId int64

				// Check for trigger/stop orders using various fields
				if order.Info != nil {
					// Check if it's a trigger order from Info
					if isTrigger, ok := order.Info["isTrigger"].(bool); ok && isTrigger {
						isTriggerOrder = true
					}

					// Get the order type from Info.info.orderType
					if infoMap, ok := order.Info["info"].(map[string]interface{}); ok {
						if orderTypeStr, ok := infoMap["orderType"].(string); ok {
							if orderTypeStr == "Stop Market" || orderTypeStr == "Stop Limit" {
								isStopLoss = true
							} else if orderTypeStr == "Take Profit Market" || orderTypeStr == "Take Profit" {
								isTakeProfit = true
							}
						}
					}
				}

				// Get trigger price from Info
				if order.Info != nil {
					if stopPx, ok := order.Info["stopPrice"].(float64); ok && stopPx != 0 {
						triggerPrice = stopPx
					} else if trigPx, ok := order.Info["triggerPrice"].(float64); ok && trigPx != 0 {
						triggerPrice = trigPx
					}
				}

				// Get order ID
				if order.Id != nil {
					fmt.Sscanf(*order.Id, "%d", &orderId)
				}

				// Determine if it's SL or TP based on side and position
				if isTriggerOrder && triggerPrice != 0 && !isStopLoss && !isTakeProfit {
					// For a long position (quantity > 0), a sell order below entry is SL, above is TP
					// For a short position (quantity < 0), a buy order above entry is SL, below is TP
					if order.Side != nil {
						side := *order.Side
						if quantity > 0 { // Long position
							if side == "sell" {
								if triggerPrice < entryPrice {
									isStopLoss = true
								} else {
									isTakeProfit = true
								}
							}
						} else if quantity < 0 { // Short position
							if side == "buy" {
								if triggerPrice > entryPrice {
									isStopLoss = true
								} else {
									isTakeProfit = true
								}
							}
						}
					}
				}

				// Assign to stop loss or take profit
				if isStopLoss {
					stopLoss = triggerPrice
					slOid = orderId
				} else if isTakeProfit {
					profitTarget = triggerPrice
					tpOid = orderId
				}
			}
		}

		customPosition := Position{
			Symbol:           symbol,
			Quantity:         quantity,
			EntryPrice:       entryPrice,
			CurrentPrice:     currentPrice,
			LiquidationPrice: liquidationPrice,
			UnrealizedPnl:    unrealizedPnl,
			Leverage:         leverage,
			ExitPlan: ExitPlan{
				ProfitTarget:          profitTarget,
				StopLoss:              stopLoss,
				InvalidationCondition: invalidationCondition,
			},
			Confidence:  confidence,
			RiskUsd:     riskUsd,
			SlOid:       slOid,
			TpOid:       tpOid,
			WaitForFill: false,
			EntryOid:    entryOid,
			NotionalUsd: notional,
		}

		customPositions = append(customPositions, customPosition)
		totalNotional += notional
	}

	// Calculate total account value
	currentAccountValue := availableCash + totalNotional

	// Placeholder values for metrics that require historical data
	totalReturnPercent := 0.0 // Would need initial account value to calculate
	sharpeRatio := 0.0        // Would need historical returns to calculate

	// Print formatted output matching Python format
	fmt.Println("\nHERE IS YOUR ACCOUNT INFORMATION & PERFORMANCE")
	fmt.Printf("Current Total Return (percent): %.2f%%\n\n", totalReturnPercent)
	fmt.Printf("Available Cash: %.2f\n\n", availableCash)
	fmt.Printf("Current Account Value: %.2f\n\n", currentAccountValue)

	fmt.Print("Current live positions & performance: ")
	for _, pos := range customPositions {
		fmt.Printf("{'symbol': '%s', 'quantity': %.2f, 'entry_price': %.2f, 'current_price': %.5f, 'liquidation_price': %.2f, 'unrealized_pnl': %.2f, 'leverage': %.0f, 'exit_plan': {'profit_target': %.6g, 'stop_loss': %.6g, 'invalidation_condition': '%s'}, 'confidence': %.2f, 'risk_usd': %.3f, 'sl_oid': %d, 'tp_oid': %d, 'wait_for_fill': %t, 'entry_oid': %d, 'notional_usd': %.2f} ",
			pos.Symbol,
			pos.Quantity,
			pos.EntryPrice,
			pos.CurrentPrice,
			pos.LiquidationPrice,
			pos.UnrealizedPnl,
			pos.Leverage,
			pos.ExitPlan.ProfitTarget,
			pos.ExitPlan.StopLoss,
			pos.ExitPlan.InvalidationCondition,
			pos.Confidence,
			pos.RiskUsd,
			pos.SlOid,
			pos.TpOid,
			pos.WaitForFill,
			pos.EntryOid,
			pos.NotionalUsd,
		)
	}
	fmt.Println()

	fmt.Printf("\nSharpe Ratio: %.3f\n", sharpeRatio)
}

// MarketData holds all the technical indicators and market data for a symbol
type MarketData struct {
	Symbol string
	// 1m timeframe data
	Closes1m     []float64
	EMA20_1m     []float64
	MACDHist_1m  []float64
	RSI7_1m      []float64
	RSI14_1m     []float64
	CurrentPrice float64
	CurrentEMA20 float64
	CurrentMACD  float64
	CurrentRSI7  float64
	// 4h timeframe data
	Closes4h        []float64
	Highs4h         []float64
	Lows4h          []float64
	Volumes4h       []float64
	EMA20_4h        []float64
	EMA50_4h        []float64
	ATR3_4h         []float64
	ATR14_4h        []float64
	MACDHist_4h     []float64
	RSI14_4h        []float64
	CurrentEMA20_4h float64
	CurrentEMA50_4h float64
	CurrentATR3     float64
	CurrentATR14    float64
	CurrentVolume   float64
	AvgVolume       float64
	// Market metrics
	OpenInterest float64
	FundingRate  float64
}

func main() {
	exchange := ccxt.NewHyperliquid(getUserConfig())

	// Load markets
	/*_, err := exchange.LoadMarkets()
	if err != nil {
		log.Fatal(err)
	} */

	// Define the symbol to analyze (use the perpetual/swap contract format)
	// For Hyperliquid perpetuals, use format: SYMBOL/USDC:USDC
	symbol := "BTC/USDC:USDC"

	marketdataClient := marketdata.NewClient(exchange)

	data, err := marketdataClient.GetMarketData(symbol)
	if err != nil {
		log.Fatalf("Error fetching market data: %v", err)
	}
	data.Format()
	// Display account information and performance
	displayAccountInfo(exchange)
}
