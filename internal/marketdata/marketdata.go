package marketdata

import (
	"fmt"
	ccxt "github.com/ccxt/ccxt/go/v4"
	"github.com/markcheno/go-talib"
	"log"
)

type (
	Client struct {
		exchange *ccxt.Hyperliquid
	}

	// MarketData holds all the technical indicators and market data for a symbol
	MarketData struct {
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
)

func NewClient(exchange *ccxt.Hyperliquid) *Client {
	return &Client{exchange: exchange}
}

func (marketData *MarketData) Format() string {
	// Print the results for the symbol
	fmt.Printf("\n========== %s Analysis ==========\n\n", marketData.Symbol)

	// Current values
	fmt.Printf("current_price = %.1f, current_ema20 = %.3f, current_macd = %.3f, current_rsi (7 period) = %.3f\n\n",
		marketData.CurrentPrice, marketData.CurrentEMA20, marketData.CurrentMACD, marketData.CurrentRSI7)

	// Open Interest and Funding Rate
	fmt.Println("In addition, here is the latest BTC open interest and funding rate for perps (the instrument you are trading):")
	fmt.Println()
	if marketData.OpenInterest != 0 {
		// For now, we'll use the current value as both Latest and Average
		// TODO: Fetch historical open interest to calculate true average
		fmt.Printf("Open Interest: Latest: %.2f Average: %.2f\n\n", marketData.OpenInterest, marketData.OpenInterest)
	}
	if marketData.FundingRate != 0 {
		fmt.Printf("Funding Rate: %.2e\n\n", marketData.FundingRate)
	}

	// Intraday series (1-minute, last 10 values)
	fmt.Println("Intraday series (by minute, oldest → latest):")
	fmt.Println()

	// Get last 10 values for 1m indicators
	last10Prices := getLastN(marketData.Closes1m, 10)
	last10EMA := getLastN(marketData.EMA20_1m, 10)
	last10MACD := getLastN(marketData.MACDHist_1m, 10)
	last10RSI7 := getLastN(marketData.RSI7_1m, 10)
	last10RSI14 := getLastN(marketData.RSI14_1m, 10)

	fmt.Printf("Mid prices: %s\n\n", formatFloatSlice(last10Prices))
	fmt.Printf("EMA indicators (20‑period): %s\n\n", formatFloatSlice(last10EMA))
	fmt.Printf("MACD indicators: %s\n\n", formatFloatSlice(last10MACD))
	fmt.Printf("RSI indicators (7‑Period): %s\n\n", formatFloatSlice(last10RSI7))
	fmt.Printf("RSI indicators (14‑Period): %s\n\n", formatFloatSlice(last10RSI14))

	// Longer-term context (4-hour timeframe)
	fmt.Println("Longer‑term context (4‑hour timeframe):")
	fmt.Println()
	fmt.Printf("20‑Period EMA: %.3f vs. 50‑Period EMA: %.3f\n\n", marketData.CurrentEMA20_4h, marketData.CurrentEMA50_4h)
	fmt.Printf("3‑Period ATR: %.3f vs. 14‑Period ATR: %.3f\n\n", marketData.CurrentATR3, marketData.CurrentATR14)
	fmt.Printf("Current Volume: %.3f vs. Average Volume: %.3f\n\n", marketData.CurrentVolume, marketData.AvgVolume)

	// Get last 10 values for 4h indicators
	last10MACD4h := getLastN(marketData.MACDHist_4h, 10)
	last10RSI14_4h := getLastN(marketData.RSI14_4h, 10)

	fmt.Printf("MACD indicators: %s\n\n", formatFloatSlice(last10MACD4h))
	fmt.Printf("RSI indicators (14‑Period): %s\n\n", formatFloatSlice(last10RSI14_4h))

	fmt.Println("========================================")
	return ""
}

// getMarketData fetches and calculates all technical indicators for a given symbol
func (c *Client) GetMarketData(symbol string) (*MarketData, error) {
	// Define timeframes
	timeframe1m := "1m"
	timeframe4h := "4h"

	// Fetch 1m data (1000 candles for sufficient indicator calculation)
	ohlcv1m, err := c.exchange.FetchOHLCV(
		symbol,
		ccxt.WithFetchOHLCVTimeframe(timeframe1m),
		ccxt.WithFetchOHLCVLimit(1000),
	)
	if err != nil {
		return nil, fmt.Errorf("error fetching 1m data for %s: %v", symbol, err)
	}

	// Extract closing prices from 1m data
	closes1m := make([]float64, len(ohlcv1m))
	for i, candle := range ohlcv1m {
		closes1m[i] = candle.Close
	}

	// Calculate 1m indicators
	ema20 := talib.Ema(closes1m, 20)
	_, _, macdHist := talib.Macd(closes1m, 12, 26, 9)
	rsi7 := talib.Rsi(closes1m, 7)
	rsi14 := talib.Rsi(closes1m, 14)

	// Current values are the last ones in the array
	currentEma20 := ema20[len(ema20)-1]
	currentMacd := macdHist[len(macdHist)-1]
	currentRsi7 := rsi7[len(rsi7)-1]
	currentPrice := closes1m[len(closes1m)-1]

	// Fetch 4h data (100 candles)
	ohlcv4h, err := c.exchange.FetchOHLCV(
		symbol,
		ccxt.WithFetchOHLCVTimeframe(timeframe4h),
		ccxt.WithFetchOHLCVLimit(100),
	)
	if err != nil {
		return nil, fmt.Errorf("error fetching 4h data for %s: %v", symbol, err)
	}

	// Extract closing prices, high, low, volume from 4h data
	closes4h := make([]float64, len(ohlcv4h))
	highs4h := make([]float64, len(ohlcv4h))
	lows4h := make([]float64, len(ohlcv4h))
	volumes4h := make([]float64, len(ohlcv4h))
	for i, candle := range ohlcv4h {
		closes4h[i] = candle.Close
		highs4h[i] = candle.High
		lows4h[i] = candle.Low
		volumes4h[i] = candle.Volume
	}

	// Calculate 4h indicators
	ema20_4h := talib.Ema(closes4h, 20)
	ema50_4h := talib.Ema(closes4h, 50)
	atr3 := talib.Atr(highs4h, lows4h, closes4h, 3)
	atr14 := talib.Atr(highs4h, lows4h, closes4h, 14)
	_, _, macdHist4h := talib.Macd(closes4h, 12, 26, 9)
	rsi14_4h := talib.Rsi(closes4h, 14)

	// Current 4h values
	currentEma20_4h := ema20_4h[len(ema20_4h)-1]
	currentEma50_4h := ema50_4h[len(ema50_4h)-1]
	currentAtr3 := atr3[len(atr3)-1]
	currentAtr14 := atr14[len(atr14)-1]

	// Volume: current and average (20-period average volume on 4h)
	currentVolume := volumes4h[len(volumes4h)-1]
	averageVolume := talib.Sma(volumes4h, 20)
	currentAverageVolume := averageVolume[len(averageVolume)-1]

	// Fetch open interest and funding rate for the symbol
	var openInterestValue float64
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Open interest not available for %s: %v", symbol, r)
			}
		}()
		openInterest, err := c.exchange.FetchOpenInterest(symbol)
		if err != nil {
			log.Printf("Error fetching open interest for %s: %v", symbol, err)
		} else if openInterest.OpenInterestAmount != nil {
			openInterestValue = *openInterest.OpenInterestAmount
		}
	}()

	var fundingRateValue float64
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Funding rate not available for %s: %v", symbol, r)
			}
		}()
		fundingRate, err := c.exchange.FetchFundingRate(symbol)
		if err != nil {
			log.Printf("Error fetching funding rate for %s: %v", symbol, err)
		} else if fundingRate.FundingRate != nil {
			fundingRateValue = *fundingRate.FundingRate
		}
	}()

	return &MarketData{
		Symbol:          symbol,
		Closes1m:        closes1m,
		EMA20_1m:        ema20,
		MACDHist_1m:     macdHist,
		RSI7_1m:         rsi7,
		RSI14_1m:        rsi14,
		CurrentPrice:    currentPrice,
		CurrentEMA20:    currentEma20,
		CurrentMACD:     currentMacd,
		CurrentRSI7:     currentRsi7,
		Closes4h:        closes4h,
		Highs4h:         highs4h,
		Lows4h:          lows4h,
		Volumes4h:       volumes4h,
		EMA20_4h:        ema20_4h,
		EMA50_4h:        ema50_4h,
		ATR3_4h:         atr3,
		ATR14_4h:        atr14,
		MACDHist_4h:     macdHist4h,
		RSI14_4h:        rsi14_4h,
		CurrentEMA20_4h: currentEma20_4h,
		CurrentEMA50_4h: currentEma50_4h,
		CurrentATR3:     currentAtr3,
		CurrentATR14:    currentAtr14,
		CurrentVolume:   currentVolume,
		AvgVolume:       currentAverageVolume,
		OpenInterest:    openInterestValue,
		FundingRate:     fundingRateValue,
	}, nil
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
