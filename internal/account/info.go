package account

import (
	"fmt"
	"math"
	"strings"

	ccxt "github.com/ccxt/ccxt/go/v4"
)

// ExitPlan contains the profit target, stop loss, and invalidation condition
type (
	Client struct {
		exchange *ccxt.Hyperliquid
	}

	ExitPlan struct {
		ProfitTarget          float64 `json:"profit_target"`
		StopLoss              float64 `json:"stop_loss"`
		InvalidationCondition string  `json:"invalidation_condition"`
	}

	// Position represents a trading position with all relevant details
	Position struct {
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
	AccountInfo struct {
		TotalReturnPercent  float64
		AvailableCash       float64
		CurrentAccountValue float64
		Positions           []Position
		SharpeRatio         float64
	}
)

func NewClient(exchange *ccxt.Hyperliquid) *Client {
	return &Client{exchange: exchange}
}

// Format returns a formatted string representation of the account information
func (ai *AccountInfo) Format() string {
	var sb strings.Builder

	sb.WriteString("\nHERE IS YOUR ACCOUNT INFORMATION & PERFORMANCE\n")
	sb.WriteString(fmt.Sprintf("Current Total Return (percent): %.2f%%\n\n", ai.TotalReturnPercent))
	sb.WriteString(fmt.Sprintf("Available Cash: %.2f\n\n", ai.AvailableCash))
	sb.WriteString(fmt.Sprintf("Current Account Value: %.2f\n\n", ai.CurrentAccountValue))

	sb.WriteString("Current live positions & performance: ")
	for _, pos := range ai.Positions {
		sb.WriteString(fmt.Sprintf("{'symbol': '%s', 'quantity': %.2f, 'entry_price': %.2f, 'current_price': %.5f, 'liquidation_price': %.2f, 'unrealized_pnl': %.2f, 'leverage': %.0f, 'exit_plan': {'profit_target': %.6g, 'stop_loss': %.6g, 'invalidation_condition': '%s'}, 'confidence': %.2f, 'risk_usd': %.3f, 'sl_oid': %d, 'tp_oid': %d, 'wait_for_fill': %t, 'entry_oid': %d, 'notional_usd': %.2f} ",
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
		))
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("\nSharpe Ratio: %.3f\n", ai.SharpeRatio))

	return sb.String()
}

// GetAccountInfo fetches and returns account information and performance
func (c *Client) GetAccountInfo() (*AccountInfo, error) {
	// Fetch account balance
	balance, err := c.exchange.FetchBalance()
	if err != nil {
		return nil, fmt.Errorf("error fetching balance: %v", err)
	}

	// Get available cash (USDC balance)
	var availableCash float64
	if balance.Free != nil {
		if usdcFree, ok := balance.Free["USDC"]; ok && usdcFree != nil {
			availableCash = *usdcFree
		}
	}

	// Fetch open positions
	positions, err := c.exchange.FetchPositions()
	if err != nil {
		return nil, fmt.Errorf("error fetching positions: %v", err)
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
			ticker, err := c.exchange.FetchTicker(symbol)
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
		openOrders, err := c.exchange.FetchOpenOrders(ccxt.WithFetchOpenOrdersSymbol(symbol))
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

	return &AccountInfo{
		TotalReturnPercent:  totalReturnPercent,
		AvailableCash:       availableCash,
		CurrentAccountValue: currentAccountValue,
		Positions:           customPositions,
		SharpeRatio:         sharpeRatio,
	}, nil
}
